package auth

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
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
	}

	// Initialize OIDC if enabled
	if cfg.OIDCEnabled && cfg.OIDC.IssuerURL != "" {
		oidcCtx := ctx
		if httpClient != nil {
			oidcCtx = oidc.ClientContext(ctx, httpClient)
		}
		provider, err := oidc.NewProvider(oidcCtx, cfg.OIDC.IssuerURL)
		if err != nil {
			// Do not fail hard on startup in case OIDC provider is temporarily down; log warning
			fmt.Printf("Warning: failed to initialize OIDC provider (%s): %v\n", cfg.OIDC.IssuerURL, err)
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
		}
	}

	return svc, nil
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

	u, err := s.store.GetUserByUsername(ctx, username)
	if err != nil {
		return nil, "", ErrUnauthorized
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return nil, "", ErrUnauthorized
	}

	token, err := s.jwtMgr.GenerateToken(u)
	if err != nil {
		return nil, "", fmt.Errorf("generating token: %w", err)
	}

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
		return nil, "", fmt.Errorf("connecting to LDAP server: %w", err)
	}
	defer l.Close()

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

		if err := l.Bind(userDN, password); err != nil {
			return nil, "", ErrUnauthorized
		}

		// Retrieve user attributes using baseObject search
		searchRequest := ldap.NewSearchRequest(
			userDN,
			ldap.ScopeBaseObject, ldap.NeverDerefAliases, 0, 0, false,
			"(objectClass=*)",
			[]string{"dn", "mail", "cn", "memberOf"},
			nil,
		)
		sr, err := l.Search(searchRequest)
		if err == nil && len(sr.Entries) > 0 {
			userEntry = sr.Entries[0]
			userMail = userEntry.GetAttributeValue("mail")
		}

		// If service account is configured, re-bind as service account for group search
		if ldapCfg.BindDN != "" && !ldapCfg.AnonymousSearch {
			_ = l.Bind(ldapCfg.BindDN, ldapCfg.BindPassword)
		} else if ldapCfg.AnonymousSearch {
			_ = l.UnauthenticatedBind("")
		}
	} else {
		// 2. Search-and-Bind (Service Account or Anonymous Search)
		if ldapCfg.AnonymousSearch || ldapCfg.BindDN == "" {
			if err := l.UnauthenticatedBind(""); err != nil {
				return nil, "", fmt.Errorf("LDAP unauthenticated bind failed: %w", err)
			}
		} else {
			if err := l.Bind(ldapCfg.BindDN, ldapCfg.BindPassword); err != nil {
				return nil, "", fmt.Errorf("LDAP admin bind failed: %w", err)
			}
		}

		// Search for user DN and groups
		filter := fmt.Sprintf(ldapCfg.UserFilter, ldap.EscapeFilter(username))
		searchRequest := ldap.NewSearchRequest(
			ldapCfg.BaseDN,
			ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
			filter,
			[]string{"dn", "mail", "cn", "memberOf"},
			nil,
		)

		sr, err := l.Search(searchRequest)
		if err != nil || len(sr.Entries) == 0 {
			return nil, "", ErrUnauthorized
		}

		userEntry = sr.Entries[0]
		userDN = userEntry.DN
		userMail = userEntry.GetAttributeValue("mail")

		// Bind with user credentials
		if err := l.Bind(userDN, password); err != nil {
			return nil, "", ErrUnauthorized
		}

		// If service account is configured, re-bind as admin to ensure full search permissions across directory
		if ldapCfg.BindDN != "" && !ldapCfg.AnonymousSearch {
			_ = l.Bind(ldapCfg.BindDN, ldapCfg.BindPassword)
		} else if ldapCfg.AnonymousSearch {
			_ = l.UnauthenticatedBind("")
		}
	}

	// Fetch user's LDAP groups
	userGroups, err := s.fetchUserLDAPGroups(l, userDN, username, userEntry)
	if err != nil {
		fmt.Printf("Warning: error fetching LDAP groups for %s: %v\n", username, err)
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
			return nil, "", fmt.Errorf("creating LDAP user: %w", err)
		}
	} else if err != nil {
		return nil, "", fmt.Errorf("retrieving LDAP user: %w", err)
	} else {
		// Existing user: recalculate role on every login (promote if admin group present, demote if removed)
		if u.Role != role {
			if err := s.store.UpdateUserRole(ctx, u.ID, role); err != nil {
				return nil, "", fmt.Errorf("updating user role: %w", err)
			}
			u.Role = role
		}
	}

	token, err := s.jwtMgr.GenerateToken(u)
	if err != nil {
		return nil, "", fmt.Errorf("generating token: %w", err)
	}

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

	exchangeCtx := ctx
	if s.httpClient != nil {
		exchangeCtx = context.WithValue(ctx, oauth2.HTTPClient, s.httpClient)
	}

	oauth2Token, err := s.oauth2Cfg.Exchange(exchangeCtx, code)
	if err != nil {
		return nil, "", fmt.Errorf("exchanging code for token: %w", err)
	}

	rawIDToken, ok := oauth2Token.Extra("id_token").(string)
	if !ok {
		return nil, "", errors.New("no id_token in OAuth2 response")
	}

	verifyCtx := ctx
	if s.httpClient != nil {
		verifyCtx = oidc.ClientContext(ctx, s.httpClient)
	}

	idToken, err := s.oidcVerifier.Verify(verifyCtx, rawIDToken)
	if err != nil {
		return nil, "", fmt.Errorf("verifying ID token: %w", err)
	}

	var claims struct {
		PreferredUsername string   `json:"preferred_username"`
		Email             string   `json:"email"`
		Groups            []string `json:"groups"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return nil, "", fmt.Errorf("parsing ID token claims: %w", err)
	}

	username := claims.PreferredUsername
	if username == "" {
		username = claims.Email
	}
	if username == "" {
		return nil, "", errors.New("OIDC identity missing username and email")
	}

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
	} else if err == nil && u.Role != role {
		_ = s.store.UpdateUserRole(ctx, u.ID, role)
		u.Role = role
	}

	token, err := s.jwtMgr.GenerateToken(u)
	if err != nil {
		return nil, "", fmt.Errorf("generating token: %w", err)
	}

	return u, token, nil
}

// ValidateJWT verifies token string.
func (s *AuthService) ValidateJWT(token string) (*UserClaims, error) {
	return s.jwtMgr.ValidateToken(token)
}
