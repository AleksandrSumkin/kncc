package auth

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"kncc/internal/config"
	"kncc/internal/model"
	"kncc/internal/store"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-ldap/ldap/v3"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/oauth2"
)

var (
	ErrUnauthorized       = errors.New("invalid credentials")
	ErrProviderDisabled   = errors.New("authentication provider is not enabled")
	ErrUserNotFound       = errors.New("user not found")
)

// AuthService handles user authentication across Local, LDAP, and OIDC providers.
type AuthService struct {
	cfg          *config.AuthConfig
	store        store.Store
	jwtMgr       *JWTManager
	rootCAs      *x509.CertPool
	httpClient   *http.Client
	oidcProvider *oidc.Provider
	oauth2Cfg    *oauth2.Config
	oidcVerifier *oidc.IDTokenVerifier
	logger       *slog.Logger
}

// NewAuthService creates a new AuthService with optional custom RootCAs.
func NewAuthService(ctx context.Context, cfg *config.AuthConfig, s store.Store, jwtMgr *JWTManager, customRootCAs ...*x509.CertPool) (*AuthService, error) {
	var rootCAs *x509.CertPool
	if len(customRootCAs) > 0 && customRootCAs[0] != nil {
		rootCAs = customRootCAs[0]
	}

	var httpClient *http.Client
	if rootCAs != nil {
		transport := &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			TLSClientConfig: &tls.Config{
				RootCAs: rootCAs,
			},
		}
		httpClient = &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
		}
	} else {
		httpClient = http.DefaultClient
	}

	svc := &AuthService{
		cfg:        cfg,
		store:      s,
		jwtMgr:     jwtMgr,
		rootCAs:    rootCAs,
		httpClient: httpClient,
		logger:     slog.Default().With("module", "auth"),
	}

	if cfg.LDAPEnabled {
		svc.logger.Info("LDAP authentication provider enabled",
			"url", cfg.LDAP.URL,
			"base_dn", cfg.LDAP.BaseDN,
			"user_dn_template", cfg.LDAP.UserDNTemplate != "",
			"anonymous_search", cfg.LDAP.AnonymousSearch,
			"insecure_skip_verify", cfg.LDAP.InsecureSkipVerify,
		)
	}

	// Initialize OIDC if enabled
	if cfg.OIDCEnabled && cfg.OIDC.IssuerURL != "" {
		svc.logger.Info("Initializing OIDC provider", "issuer_url", cfg.OIDC.IssuerURL, "client_id", cfg.OIDC.ClientID)
		oidcCtx := ctx
		if httpClient != nil {
			oidcCtx = oidc.ClientContext(ctx, httpClient)
		}
		provider, err := oidc.NewProvider(oidcCtx, cfg.OIDC.IssuerURL)
		if err != nil {
			// Do not fail hard on startup in case OIDC provider is temporarily down; log warning
			svc.logger.Warn("Failed to initialize OIDC provider", "issuer_url", cfg.OIDC.IssuerURL, "error", err)
		} else {
			svc.oidcProvider = provider
			svc.oidcVerifier = provider.Verifier(&oidc.Config{ClientID: cfg.OIDC.ClientID})
			svc.oauth2Cfg = &oauth2.Config{
				ClientID:     cfg.OIDC.ClientID,
				ClientSecret: cfg.OIDC.ClientSecret,
				Endpoint:     provider.Endpoint(),
				RedirectURL:  cfg.OIDC.RedirectURL,
				Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
			}
			svc.logger.Info("OIDC provider initialized successfully", "issuer_url", cfg.OIDC.IssuerURL, "client_id", cfg.OIDC.ClientID)
		}
	}

	return svc, nil
}

// SetLogger overrides the logger used by AuthService.
func (s *AuthService) SetLogger(l *slog.Logger) {
	if l != nil {
		s.logger = l
	}
}

// GetEnabledProviders returns list of active authentication providers.
func (s *AuthService) GetEnabledProviders() []string {
	var providers []string
	if s.cfg.LocalEnabled {
		providers = append(providers, "local")
	}
	if s.cfg.LDAPEnabled {
		providers = append(providers, "ldap")
	}
	if s.cfg.OIDCEnabled {
		providers = append(providers, "oidc")
	}
	return providers
}

// GetOIDCAuthURL returns the authorization redirect URL for SSO.
func (s *AuthService) GetOIDCAuthURL(state string) (string, error) {
	if !s.cfg.OIDCEnabled || s.oauth2Cfg == nil {
		return "", ErrProviderDisabled
	}
	return s.oauth2Cfg.AuthCodeURL(state), nil
}

// AuthenticateLocal checks credentials against SQLite store.
func (s *AuthService) AuthenticateLocal(ctx context.Context, username, password string) (*model.User, string, error) {
	if !s.cfg.LocalEnabled {
		return nil, "", ErrProviderDisabled
	}

	s.logger.Debug("Local authentication attempt", "username", username)
	u, err := s.store.GetUserByUsername(ctx, username)
	if err != nil {
		s.logger.Warn("Local authentication failed: user not found", "username", username)
		return nil, "", ErrUnauthorized
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		s.logger.Warn("Local authentication failed: invalid password", "username", username)
		return nil, "", ErrUnauthorized
	}

	token, err := s.jwtMgr.GenerateToken(u)
	if err != nil {
		s.logger.Error("Failed to generate JWT token for local user", "username", username, "error", err)
		return nil, "", fmt.Errorf("generating token: %w", err)
	}

	s.logger.Info("Local user authenticated successfully", "username", username, "role", u.Role)
	return u, token, nil
}

// AuthenticateLDAP binds with LDAP server and retrieves user profile.
func (s *AuthService) AuthenticateLDAP(ctx context.Context, username, password string) (*model.User, string, error) {
	if !s.cfg.LDAPEnabled {
		return nil, "", ErrProviderDisabled
	}
	if username == "" || password == "" {
		return nil, "", ErrUnauthorized
	}

	ldapCfg := s.cfg.LDAP
	s.logger.Debug("LDAP authentication attempt",
		"username", username,
		"url", ldapCfg.URL,
		"insecure_skip_verify", ldapCfg.InsecureSkipVerify,
	)

	var l *ldap.Conn
	var err error

	tlsConfig := &tls.Config{
		InsecureSkipVerify: ldapCfg.InsecureSkipVerify,
		RootCAs:            s.rootCAs,
	}
	if strings.HasPrefix(ldapCfg.URL, "ldaps://") {
		l, err = ldap.DialURL(ldapCfg.URL, ldap.DialWithTLSConfig(tlsConfig))
	} else {
		l, err = ldap.DialURL(ldapCfg.URL)
	}
	if err != nil {
		s.logger.Error("Connecting to LDAP server failed", "url", ldapCfg.URL, "error", err)
		return nil, "", fmt.Errorf("connecting to LDAP server: %w", err)
	}
	defer l.Close()
	s.logger.Debug("Connected to LDAP server successfully", "url", ldapCfg.URL)

	var userEntry *ldap.Entry
	var userDN string
	var userMail string

	if ldapCfg.UserDNTemplate != "" {
		// 1. Direct Bind (User Bind Pattern) using DN template
		if strings.Contains(ldapCfg.UserDNTemplate, "%s") {
			userDN = fmt.Sprintf(ldapCfg.UserDNTemplate, ldap.EscapeDN(username))
		} else {
			userDN = ldapCfg.UserDNTemplate
		}

		s.logger.Debug("Attempting direct user bind", "user_dn", userDN)
		if err := l.Bind(userDN, password); err != nil {
			s.logger.Warn("Direct user bind failed", "user_dn", userDN, "error", err)
			return nil, "", ErrUnauthorized
		}
		s.logger.Debug("Direct user bind successful", "user_dn", userDN)

		// Retrieve user attributes using baseObject search
		searchRequest := ldap.NewSearchRequest(
			userDN,
			ldap.ScopeBaseObject, ldap.NeverDerefAliases, 0, 0, false,
			"(objectClass=*)",
			[]string{"dn", "mail", "cn", "memberOf"},
			nil,
		)
		s.logger.Debug("Retrieving user attributes via baseObject search", "user_dn", userDN)
		sr, err := l.Search(searchRequest)
		if err == nil && len(sr.Entries) > 0 {
			userEntry = sr.Entries[0]
			userMail = userEntry.GetAttributeValue("mail")
			s.logger.Debug("User attributes retrieved", "user_dn", userDN, "mail", userMail)
		} else if err != nil {
			s.logger.Debug("Could not retrieve user attributes", "user_dn", userDN, "error", err)
		}

		// If service account is configured, re-bind as service account for group search
		if ldapCfg.BindDN != "" && !ldapCfg.AnonymousSearch {
			s.logger.Debug("Re-binding as service account for group search", "bind_dn", ldapCfg.BindDN)
			_ = l.Bind(ldapCfg.BindDN, ldapCfg.BindPassword)
		} else if ldapCfg.AnonymousSearch {
			s.logger.Debug("Re-binding anonymously for group search")
			_ = l.UnauthenticatedBind("")
		}
	} else {
		// 2. Search-and-Bind (Service Account or Anonymous Search)
		if ldapCfg.AnonymousSearch || ldapCfg.BindDN == "" {
			s.logger.Debug("Performing anonymous bind for user search")
			if err := l.UnauthenticatedBind(""); err != nil {
				s.logger.Error("LDAP unauthenticated bind failed", "error", err)
				return nil, "", fmt.Errorf("LDAP unauthenticated bind failed: %w", err)
			}
		} else {
			s.logger.Debug("Binding with LDAP service account", "bind_dn", ldapCfg.BindDN)
			if err := l.Bind(ldapCfg.BindDN, ldapCfg.BindPassword); err != nil {
				s.logger.Error("LDAP admin bind failed", "bind_dn", ldapCfg.BindDN, "error", err)
				return nil, "", fmt.Errorf("LDAP admin bind failed: %w", err)
			}
		}

		// Search for user DN and groups
		filter := fmt.Sprintf(ldapCfg.UserFilter, ldap.EscapeFilter(username))
		s.logger.Debug("Searching for user DN", "base_dn", ldapCfg.BaseDN, "filter", filter)
		searchRequest := ldap.NewSearchRequest(
			ldapCfg.BaseDN,
			ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
			filter,
			[]string{"dn", "mail", "cn", "memberOf"},
			nil,
		)

		sr, err := l.Search(searchRequest)
		if err != nil || len(sr.Entries) == 0 {
			s.logger.Warn("LDAP user not found or search failed", "username", username, "filter", filter, "error", err)
			return nil, "", ErrUnauthorized
		}

		userEntry = sr.Entries[0]
		userDN = userEntry.DN
		userMail = userEntry.GetAttributeValue("mail")
		s.logger.Debug("Found LDAP user entry", "user_dn", userDN, "mail", userMail)

		// Bind with user credentials
		s.logger.Debug("Binding with user credentials", "user_dn", userDN)
		if err := l.Bind(userDN, password); err != nil {
			s.logger.Warn("User credentials bind failed", "user_dn", userDN, "error", err)
			return nil, "", ErrUnauthorized
		}
		s.logger.Debug("User credentials bind successful", "user_dn", userDN)

		// If service account is configured, re-bind as admin to ensure full search permissions across directory
		if ldapCfg.BindDN != "" && !ldapCfg.AnonymousSearch {
			s.logger.Debug("Re-binding as service account for group search", "bind_dn", ldapCfg.BindDN)
			_ = l.Bind(ldapCfg.BindDN, ldapCfg.BindPassword)
		} else if ldapCfg.AnonymousSearch {
			s.logger.Debug("Re-binding anonymously for group search")
			_ = l.UnauthenticatedBind("")
		}
	}

	// Fetch user's LDAP groups
	s.logger.Debug("Fetching LDAP groups for user", "user_dn", userDN, "username", username)
	userGroups, err := s.fetchUserLDAPGroups(l, userDN, username, userEntry)
	if err != nil {
		s.logger.Warn("Error fetching LDAP groups", "username", username, "error", err)
	} else {
		s.logger.Debug("LDAP groups fetched successfully", "username", username, "count", len(userGroups), "groups", userGroups)
	}

	// Dynamic role calculation on every login
	isAdmin := false
	if ldapCfg.AdminGroup != "" {
		for _, g := range userGroups {
			if strings.EqualFold(g, ldapCfg.AdminGroup) || strings.EqualFold(parseCN(g), ldapCfg.AdminGroup) {
				isAdmin = true
				break
			}
		}
	}
	if !isAdmin && strings.EqualFold(username, s.cfg.AdminUsername) {
		isAdmin = true
	}

	role := model.RoleUser
	if isAdmin {
		role = model.RoleAdmin
	}
	s.logger.Debug("Evaluated role for LDAP user", "username", username, "role", role, "is_admin", isAdmin, "admin_group", ldapCfg.AdminGroup)

	// Sync or create user in SQLite with dynamic role recalculation
	u, err := s.store.GetUserByUsername(ctx, username)
	if errors.Is(err, store.ErrNotFound) {
		u = &model.User{
			ID:        uuid.NewString(),
			Username:  username,
			Role:      role,
			Provider:  "ldap",
			Email:     userMail,
			CreatedAt: time.Now().UTC(),
		}
		if err := s.store.CreateUser(ctx, u); err != nil {
			s.logger.Error("Failed to create LDAP user in database", "username", username, "error", err)
			return nil, "", fmt.Errorf("creating LDAP user: %w", err)
		}
		s.logger.Debug("Created new LDAP user in database", "username", username, "id", u.ID)
	} else if err != nil {
		s.logger.Error("Failed to query LDAP user in database", "username", username, "error", err)
		return nil, "", fmt.Errorf("retrieving LDAP user: %w", err)
	} else {
		// Existing user: recalculate role on every login (promote if admin group present, demote if removed)
		if u.Role != role {
			s.logger.Info("Updating LDAP user role in database", "username", username, "old_role", u.Role, "new_role", role)
			if err := s.store.UpdateUserRole(ctx, u.ID, role); err != nil {
				return nil, "", fmt.Errorf("updating user role: %w", err)
			}
			u.Role = role
		}
	}

	token, err := s.jwtMgr.GenerateToken(u)
	if err != nil {
		s.logger.Error("Failed to generate JWT token for LDAP user", "username", username, "error", err)
		return nil, "", fmt.Errorf("generating token: %w", err)
	}

	s.logger.Info("LDAP user authenticated successfully", "username", username, "role", role)
	return u, token, nil
}

// fetchUserLDAPGroups retrieves all matching LDAP groups for the authenticated user.
func (s *AuthService) fetchUserLDAPGroups(l *ldap.Conn, userDN, username string, userEntry *ldap.Entry) ([]string, error) {
	ldapCfg := s.cfg.LDAP
	groupSet := make(map[string]struct{})

	// 1. Inspect memberOf attribute on user entry (Active Directory & OpenLDAP slapo-memberof)
	if userEntry != nil {
		for _, rawGroup := range userEntry.GetAttributeValues("memberOf") {
			cn := parseCN(rawGroup)
			if cn != "" {
				groupSet[cn] = struct{}{}
			}
			groupSet[rawGroup] = struct{}{}
		}
	}

	// 2. Perform LDAP directory search if connection and search criteria are available
	groupBaseDN := ldapCfg.GroupBaseDN
	if groupBaseDN == "" {
		groupBaseDN = ldapCfg.BaseDN
	}

	nameAttr := ldapCfg.GroupNameAttribute
	if nameAttr == "" {
		nameAttr = "cn"
	}

	var groupFilter string
	if ldapCfg.GroupFilter != "" {
		if strings.Contains(ldapCfg.GroupFilter, "%s") {
			groupFilter = fmt.Sprintf(ldapCfg.GroupFilter, ldap.EscapeFilter(userDN))
		} else {
			groupFilter = fmt.Sprintf("(&%s(|(member=%s)(uniqueMember=%s)(memberUid=%s)))",
				ldapCfg.GroupFilter,
				ldap.EscapeFilter(userDN),
				ldap.EscapeFilter(userDN),
				ldap.EscapeFilter(username),
			)
		}
	} else if ldapCfg.AdminGroup != "" {
		groupFilter = fmt.Sprintf("(&(cn=%s)(|(member=%s)(uniqueMember=%s)(memberUid=%s)))",
			ldap.EscapeFilter(ldapCfg.AdminGroup),
			ldap.EscapeFilter(userDN),
			ldap.EscapeFilter(userDN),
			ldap.EscapeFilter(username),
		)
	}

	if groupFilter != "" && l != nil {
		req := ldap.NewSearchRequest(
			groupBaseDN,
			ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
			groupFilter,
			[]string{"dn", nameAttr, "cn"},
			nil,
		)
		sr, err := l.Search(req)
		if err == nil && len(sr.Entries) > 0 {
			for _, entry := range sr.Entries {
				cn := entry.GetAttributeValue(nameAttr)
				if cn == "" {
					cn = entry.GetAttributeValue("cn")
				}
				if cn == "" {
					cn = parseCN(entry.DN)
				}
				if cn != "" {
					groupSet[cn] = struct{}{}
				}
				groupSet[entry.DN] = struct{}{}
			}
		} else if ldapCfg.GroupFilter != "" && !strings.Contains(ldapCfg.GroupFilter, "%s") {
			// Fallback: search by GroupFilter and verify membership attributes in Go
			fallbackReq := ldap.NewSearchRequest(
				groupBaseDN,
				ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
				ldapCfg.GroupFilter,
				[]string{"dn", nameAttr, "cn", "member", "uniqueMember", "memberUid"},
				nil,
			)
			if fallbackSr, fallbackErr := l.Search(fallbackReq); fallbackErr == nil {
				for _, entry := range fallbackSr.Entries {
					isMember := false
					for _, m := range entry.GetAttributeValues("member") {
						if strings.EqualFold(m, userDN) || strings.EqualFold(m, username) {
							isMember = true
							break
						}
					}
					if !isMember {
						for _, m := range entry.GetAttributeValues("uniqueMember") {
							if strings.EqualFold(m, userDN) || strings.EqualFold(m, username) {
								isMember = true
								break
							}
						}
					}
					if !isMember {
						for _, m := range entry.GetAttributeValues("memberUid") {
							if strings.EqualFold(m, username) {
								isMember = true
								break
							}
						}
					}
					if isMember {
						cn := entry.GetAttributeValue(nameAttr)
						if cn == "" {
							cn = entry.GetAttributeValue("cn")
						}
						if cn == "" {
							cn = parseCN(entry.DN)
						}
						if cn != "" {
							groupSet[cn] = struct{}{}
						}
						groupSet[entry.DN] = struct{}{}
					}
				}
			}
		}
	}

	// 3. Filter groups by groupFilter if specified
	var result []string
	for g := range groupSet {
		cn := parseCN(g)
		if matchesGroupFilter(cn, ldapCfg.GroupFilter) || matchesGroupFilter(g, ldapCfg.GroupFilter) {
			result = append(result, g)
		}
	}

	return result, nil
}

// parseCN extracts the CN attribute value from an LDAP DN or returns the original string.
func parseCN(dnOrName string) string {
	parts := strings.Split(dnOrName, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(strings.ToLower(part), "cn=") {
			return strings.TrimSpace(part[3:])
		}
	}
	return strings.TrimSpace(dnOrName)
}

// extractPatternFromFilter extracts the wildcard or attribute pattern from an LDAP filter.
// e.g. "(cn=RBAC_KNCC_*)" -> "RBAC_KNCC_*", "(&(objectClass=group)(cn=RBAC_KNCC_*))" -> "RBAC_KNCC_*"
func extractPatternFromFilter(filter string) string {
	lower := strings.ToLower(filter)
	for _, attr := range []string{"cn=", "name="} {
		if idx := strings.Index(lower, attr); idx != -1 {
			rem := filter[idx+len(attr):]
			end := strings.IndexAny(rem, ")(&|")
			if end != -1 {
				return strings.TrimSpace(rem[:end])
			}
			return strings.TrimSpace(rem)
		}
	}
	trimmed := strings.Trim(filter, "()")
	if idx := strings.Index(trimmed, "="); idx != -1 {
		return strings.TrimSpace(trimmed[idx+1:])
	}
	return strings.TrimSpace(filter)
}

// matchesGroupFilter checks if a group name matches the configured group filter.
func matchesGroupFilter(groupName, filter string) bool {
	if filter == "" {
		return true
	}
	pattern := extractPatternFromFilter(filter)
	if pattern == "" {
		return true
	}
	if strings.ContainsAny(pattern, "*?[") {
		matched, err := filepath.Match(strings.ToLower(pattern), strings.ToLower(groupName))
		if err == nil {
			return matched
		}
	}
	return strings.EqualFold(groupName, pattern)
}

// AuthenticateOIDC processes OAuth2 callback and verifies ID token.
func (s *AuthService) AuthenticateOIDC(ctx context.Context, code string) (*model.User, string, error) {
	if !s.cfg.OIDCEnabled || s.oauth2Cfg == nil || s.oidcVerifier == nil {
		return nil, "", ErrProviderDisabled
	}

	s.logger.Debug("OIDC callback initiated, exchanging authorization code for token")
	exchangeCtx := ctx
	if s.httpClient != nil {
		exchangeCtx = context.WithValue(ctx, oauth2.HTTPClient, s.httpClient)
	}

	oauth2Token, err := s.oauth2Cfg.Exchange(exchangeCtx, code)
	if err != nil {
		s.logger.Warn("OIDC authorization code exchange failed", "error", err)
		return nil, "", fmt.Errorf("exchanging code for token: %w", err)
	}

	rawIDToken, ok := oauth2Token.Extra("id_token").(string)
	if !ok {
		s.logger.Warn("OIDC response missing id_token")
		return nil, "", errors.New("no id_token in OAuth2 response")
	}

	s.logger.Debug("Verifying OIDC ID token")
	verifyCtx := ctx
	if s.httpClient != nil {
		verifyCtx = oidc.ClientContext(ctx, s.httpClient)
	}

	idToken, err := s.oidcVerifier.Verify(verifyCtx, rawIDToken)
	if err != nil {
		s.logger.Warn("OIDC ID token verification failed", "error", err)
		return nil, "", fmt.Errorf("verifying ID token: %w", err)
	}

	var claims struct {
		PreferredUsername string   `json:"preferred_username"`
		Email             string   `json:"email"`
		Groups            []string `json:"groups"`
	}
	if err := idToken.Claims(&claims); err != nil {
		s.logger.Warn("Failed to parse OIDC ID token claims", "error", err)
		return nil, "", fmt.Errorf("parsing ID token claims: %w", err)
	}

	username := claims.PreferredUsername
	if username == "" {
		username = claims.Email
	}
	if username == "" {
		s.logger.Warn("OIDC identity missing username and email claims")
		return nil, "", errors.New("OIDC identity missing username and email")
	}

	s.logger.Debug("OIDC claims verified", "username", username, "email", claims.Email, "groups", claims.Groups)

	role := model.RoleUser
	if strings.EqualFold(username, s.cfg.AdminUsername) {
		role = model.RoleAdmin
	}
	for _, g := range claims.Groups {
		if strings.EqualFold(g, "admin") || strings.EqualFold(g, "kncc-admin") || strings.EqualFold(g, "kncc-admins") {
			role = model.RoleAdmin
			break
		}
	}
	s.logger.Debug("Evaluated role for OIDC user", "username", username, "role", role, "groups", claims.Groups)

	u, err := s.store.GetUserByUsername(ctx, username)
	if errors.Is(err, store.ErrNotFound) {
		u = &model.User{
			ID:        uuid.NewString(),
			Username:  username,
			Role:      role,
			Provider:  "oidc",
			Email:     claims.Email,
			CreatedAt: time.Now().UTC(),
		}
		_ = s.store.CreateUser(ctx, u)
		s.logger.Debug("Created new OIDC user in database", "username", username, "id", u.ID)
	} else if err == nil && u.Role != role {
		s.logger.Info("Updating OIDC user role in database", "username", username, "old_role", u.Role, "new_role", role)
		_ = s.store.UpdateUserRole(ctx, u.ID, role)
		u.Role = role
	}

	token, err := s.jwtMgr.GenerateToken(u)
	if err != nil {
		s.logger.Error("Failed to generate JWT token for OIDC user", "username", username, "error", err)
		return nil, "", fmt.Errorf("generating token: %w", err)
	}

	s.logger.Info("OIDC user authenticated successfully", "username", username, "role", role)
	return u, token, nil
}

// ValidateJWT verifies token string.
func (s *AuthService) ValidateJWT(token string) (*UserClaims, error) {
	return s.jwtMgr.ValidateToken(token)
}
