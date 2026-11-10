package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"kncc/internal/auth"
	"kncc/internal/config"
	"kncc/internal/k8s"
	"kncc/internal/k8s/rbac"
	"kncc/internal/model"
	"kncc/internal/ratelimit"
	"kncc/internal/security"
	"kncc/internal/store"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"k8s.io/client-go/kubernetes"
)

// CheckRunner defines the interface for running connectivity checks.
type CheckRunner interface {
	ExecuteCheck(ctx context.Context, report *model.Report)
}

// Handler holds dependencies for API requests.
type Handler struct {
	version   string
	cfg       *config.Config
	store     store.Store
	authSvc   *auth.AuthService
	validator *security.TargetValidator
	limiter   *ratelimit.Limiter
	runner    CheckRunner
	k8sClient kubernetes.Interface
}

// NewHandler creates a new Handler.
func NewHandler(
	cfg *config.Config,
	s store.Store,
	authSvc *auth.AuthService,
	validator *security.TargetValidator,
	limiter *ratelimit.Limiter,
	runner CheckRunner,
	k8sClient kubernetes.Interface,
	version string,
) *Handler {
	return &Handler{
		version:   version,
		cfg:       cfg,
		store:     s,
		authSvc:   authSvc,
		validator: validator,
		limiter:   limiter,
		runner:    runner,
		k8sClient: k8sClient,
	}
}

func (h *Handler) getEffectiveDefaultTheme(ctx context.Context) string {
	if h.store != nil {
		if val, err := h.store.GetSetting(ctx, "default_theme"); err == nil && val != "" {
			return val
		}
	}
	if h.cfg.Theme.DefaultTheme != "" {
		return h.cfg.Theme.DefaultTheme
	}
	return "light"
}

func (h *Handler) getEffectiveDefaultLanguage(ctx context.Context) string {
	if h.store != nil {
		if val, err := h.store.GetSetting(ctx, "default_language"); err == nil && val != "" {
			val = strings.ToLower(strings.TrimSpace(val))
			if val == "en" || val == "ru" {
				return val
			}
		}
	}
	if h.cfg.Theme.DefaultLanguage != "" {
		val := strings.ToLower(strings.TrimSpace(h.cfg.Theme.DefaultLanguage))
		if val == "en" || val == "ru" {
			return val
		}
	}
	return "en"
}

// GetSystemConfig godoc
// @Summary Get system configuration
// @Description Returns public theme, features, branding, and auth provider configuration
// @Tags system
// @Produce json
// @Success 200 {object} SystemConfigResponse
// @Router /api/system/config [get]
func (h *Handler) GetSystemConfig(w http.ResponseWriter, r *http.Request) {
	_, _, etag, isCustom, _ := h.getEffectiveLogo(r.Context())
	trimmedEtag := strings.Trim(etag, "\"")

	_, _, faviconEtag, isCustomFavicon, _ := h.getEffectiveFavicon(r.Context())
	trimmedFaviconEtag := strings.Trim(faviconEtag, "\"")

	writeJSON(w, http.StatusOK, map[string]any{
		"version":                 h.version,
		"app_name":                h.cfg.Theme.AppName,
		"primary_color":           h.cfg.Theme.PrimaryColor,
		"accent_color":            h.cfg.Theme.AccentColor,
		"default_theme":           h.getEffectiveDefaultTheme(r.Context()),
		"default_language":        h.getEffectiveDefaultLanguage(r.Context()),
		"auth_providers":          h.authSvc.GetEnabledProviders(),
		"multi_namespace_allowed": h.cfg.K8s.MultiNamespaceAllowed,
		"namespace_list_enabled":  h.cfg.K8s.NamespaceListEnabled,
		"default_namespace":       h.cfg.K8s.Namespace,
		"rate_limit_per_minute":   h.cfg.Security.RateLimitPerMinute,
		"quick_add_presets":       h.cfg.QuickAdd.Presets,
		"has_custom_logo":         isCustom,
		"logo_url":                "/api/system/logo?v=" + trimmedEtag,
		"has_custom_favicon":      isCustomFavicon,
		"favicon_url":             "/api/system/favicon?v=" + trimmedFaviconEtag,
	})
}

// GetAuthProviders godoc
// @Summary Get active authentication providers
// @Description Returns list of active login methods (local, ldap, oidc)
// @Tags auth
// @Produce json
// @Success 200 {object} AuthProvidersResponse
// @Router /api/auth/providers [get]
func (h *Handler) GetAuthProviders(w http.ResponseWriter, r *http.Request) {
	resp := map[string]any{
		"providers": h.authSvc.GetEnabledProviders(),
	}
	if h.cfg.Auth.OIDCEnabled {
		authURL, err := h.authSvc.GetOIDCAuthURL(uuid.NewString())
		if err == nil {
			resp["oidc_auth_url"] = authURL
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// GetNamespaces godoc
// @Summary List allowed Kubernetes namespaces
// @Description Returns list of accessible namespaces if namespace listing is enabled
// @Tags k8s
// @Produce json
// @Security BearerAuth
// @Success 200 {object} NamespacesResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/k8s/namespaces [get]
func (h *Handler) GetNamespaces(w http.ResponseWriter, r *http.Request) {
	if !h.cfg.K8s.NamespaceListEnabled {
		writeJSONError(w, http.StatusForbidden, "Namespace listing is disabled by administrator")
		return
	}

	namespaces, err := k8s.ListAllowedNamespaces(r.Context(), h.k8sClient, h.cfg)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to list namespaces: %v", err))
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"namespaces": namespaces,
	})
}

// GetQuickAddTargets godoc
// @Summary Get quick-add diagnostic targets
// @Description Returns personalized or default quick-add diagnostic targets
// @Tags checks
// @Produce json
// @Security BearerAuth
// @Success 200 {object} QuickAddResponse
// @Failure 401 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/checks/quick-add [get]
func (h *Handler) GetQuickAddTargets(w http.ResponseWriter, r *http.Request) {
	claims := UserFromContext(r.Context())
	if claims == nil {
		writeJSONError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	userTargets, err := h.store.GetUserTopTargets(r.Context(), claims.UserID, 3)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to fetch user targets: %v", err))
		return
	}

	res := make([]model.TargetService, 0, 3)
	seen := make(map[string]bool)

	isDuplicate := func(t model.TargetService) bool {
		key := fmt.Sprintf("%s|%s|%d|%s",
			strings.ToLower(string(t.Protocol)),
			strings.ToLower(t.Host),
			t.Port,
			t.Path,
		)
		return seen[key]
	}

	markSeen := func(t model.TargetService) {
		key := fmt.Sprintf("%s|%s|%d|%s",
			strings.ToLower(string(t.Protocol)),
			strings.ToLower(t.Host),
			t.Port,
			t.Path,
		)
		seen[key] = true
	}

	for _, t := range userTargets {
		if len(res) >= 3 {
			break
		}
		if !isDuplicate(t) {
			markSeen(t)
			res = append(res, t)
		}
	}

	// Fill remaining slots from configured presets
	if len(res) < 3 && h.cfg != nil {
		for _, preset := range h.cfg.QuickAdd.Presets {
			if len(res) >= 3 {
				break
			}
			t := model.TargetService{
				Name:           preset.Name,
				Protocol:       preset.Protocol,
				Host:           preset.Host,
				Port:           preset.Port,
				Path:           preset.Path,
				InsecureTLS:    preset.InsecureTLS,
				TimeoutSeconds: preset.TimeoutSeconds,
			}
			if !isDuplicate(t) {
				markSeen(t)
				res = append(res, t)
			}
		}
	}

	// Fallback to defaults if still empty
	if len(res) == 0 {
		res = []model.TargetService{
			{Name: "K8s API", Protocol: model.ProtocolTCP, Host: "10.96.0.1", Port: 443, TimeoutSeconds: 5},
			{Name: "Google DNS", Protocol: model.ProtocolICMP, Host: "8.8.8.8", TimeoutSeconds: 5},
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"quick_add": res,
	})
}

// LoginRequest is the login payload.
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Provider string `json:"provider"` // "local", "ldap"
}

// Login godoc
// @Summary User login
// @Description Authenticates a user via local credentials or LDAP, returning a JWT token
// @Tags auth
// @Accept json
// @Produce json
// @Param request body LoginRequest true "Login credentials"
// @Success 200 {object} LoginResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/auth/login [post]
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	provider := req.Provider
	if provider == "" {
		provider = "local"
	}

	var (
		user  *model.User
		token string
		err   error
	)

	switch provider {
	case "local":
		user, token, err = h.authSvc.AuthenticateLocal(r.Context(), req.Username, req.Password)
	case "ldap":
		user, token, err = h.authSvc.AuthenticateLDAP(r.Context(), req.Username, req.Password)
	default:
		writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("Unsupported provider %q", provider))
		return
	}

	if err != nil {
		if errors.Is(err, auth.ErrUnauthorized) {
			writeJSONError(w, http.StatusUnauthorized, "Invalid username or password")
			return
		}
		if errors.Is(err, auth.ErrProviderDisabled) {
			writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("Provider %s is disabled", provider))
			return
		}
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Set HTTP-only Cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "kncc_token",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(h.cfg.Server.TokenTTL),
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"user":  user.SafeUser(),
		"token": token,
	})
}

// Logout godoc
// @Summary User logout
// @Description Clears the authentication session and cookie
// @Tags auth
// @Produce json
// @Success 200 {object} MessageResponse
// @Router /api/auth/logout [post]
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "kncc_token",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	})
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

// OIDCLogin godoc
// @Summary Initiate OIDC Single Sign-On
// @Description Redirects client to configured OIDC / Dex identity provider
// @Tags auth
// @Param state query string false "Optional state parameter"
// @Success 302 {string} string "Redirect to IdP authorization endpoint"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/auth/oidc/login [get]
func (h *Handler) OIDCLogin(w http.ResponseWriter, r *http.Request) {
	if !h.cfg.Auth.OIDCEnabled {
		writeJSONError(w, http.StatusBadRequest, "OIDC authentication is not enabled")
		return
	}

	state := r.URL.Query().Get("state")
	if state == "" {
		state = uuid.NewString()
	}

	authURL, err := h.authSvc.GetOIDCAuthURL(state)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to generate OIDC authorization URL: %v", err))
		return
	}

	http.Redirect(w, r, authURL, http.StatusFound)
}

// OIDCCallback godoc
// @Summary OIDC callback handler
// @Description Exchanges authorization code with OIDC provider and issues JWT token
// @Tags auth
// @Produce json
// @Param code query string false "Authorization code"
// @Param error query string false "Error code from IdP"
// @Success 200 {object} LoginResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Router /api/auth/oidc/callback [get]
// @Router /api/auth/oidc/callback [post]
func (h *Handler) OIDCCallback(w http.ResponseWriter, r *http.Request) {
	if !h.cfg.Auth.OIDCEnabled {
		writeJSONError(w, http.StatusBadRequest, "OIDC authentication is not enabled")
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" && r.Method == http.MethodPost {
		var req struct {
			Code string `json:"code"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
			code = req.Code
		}
	}

	if code == "" {
		errMsg := r.URL.Query().Get("error_description")
		if errMsg == "" {
			errMsg = r.URL.Query().Get("error")
		}
		if errMsg == "" {
			errMsg = "Missing authorization code"
		}
		if r.Method == http.MethodGet {
			http.Redirect(w, r, "/?error="+url.QueryEscape(errMsg), http.StatusFound)
			return
		}
		writeJSONError(w, http.StatusBadRequest, errMsg)
		return
	}

	user, token, err := h.authSvc.AuthenticateOIDC(r.Context(), code)
	if err != nil {
		if r.Method == http.MethodGet {
			http.Redirect(w, r, "/?error="+url.QueryEscape(err.Error()), http.StatusFound)
			return
		}
		writeJSONError(w, http.StatusUnauthorized, fmt.Sprintf("OIDC authentication failed: %v", err))
		return
	}

	// Set kncc_token cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "kncc_token",
		Value:    token,
		Path:     "/",
		HttpOnly: false,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(h.cfg.Server.TokenTTL),
	})

	if r.Method == http.MethodGet {
		http.Redirect(w, r, "/?token="+url.QueryEscape(token), http.StatusFound)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"user":  user.SafeUser(),
		"token": token,
	})
}

// GetMe godoc
// @Summary Get current user profile
// @Description Returns the profile of the currently authenticated user
// @Tags auth
// @Produce json
// @Security BearerAuth
// @Success 200 {object} CreateUserResponse
// @Failure 401 {object} ErrorResponse
// @Router /api/auth/me [get]
func (h *Handler) GetMe(w http.ResponseWriter, r *http.Request) {
	claims := UserFromContext(r.Context())
	user, err := h.store.GetUserByID(r.Context(), claims.UserID)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"user": map[string]any{
				"id":       claims.UserID,
				"username": claims.Username,
				"role":     claims.Role,
				"provider": claims.Provider,
			},
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user": user.SafeUser(),
	})
}

// CreateCheck godoc
// @Summary Submit network connectivity check
// @Description Creates a new network probe check job between Kubernetes pods/targets
// @Tags checks
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body model.CheckRequest true "Connectivity check specification"
// @Success 201 {object} model.Report
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/checks [post]
func (h *Handler) CreateCheck(w http.ResponseWriter, r *http.Request) {
	claims := UserFromContext(r.Context())

	var req model.CheckRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	// 1. Validate request structure
	if err := req.Validate(h.cfg.K8s.MultiNamespaceAllowed, h.cfg.K8s.Namespace); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	// 1b. Check if target namespace is allowed by security policy
	if req.Mode == model.ModePodNetwork && req.Namespace != "" {
		if ok, err := k8s.IsNamespaceAllowed(r.Context(), h.k8sClient, h.cfg, req.Namespace); !ok || err != nil {
			writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("Namespace rejected: %v", err))
			return
		}
	}

	// 2. Validate targets against security policy (anti-SSRF, blacklist, whitelist)
	for i, target := range req.Targets {
		if err := h.validator.ValidateTarget(r.Context(), target); err != nil {
			writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("Target #%d (%s) blocked: %v", i+1, target.TargetIdentifier(), err))
			return
		}
	}

	// 3. Create Report in store
	now := time.Now().UTC()
	report := &model.Report{
		ID:             uuid.NewString(),
		UserID:         claims.UserID,
		Username:       claims.Username,
		Mode:           req.Mode,
		Namespace:      req.Namespace,
		Targets:        req.Targets,
		Status:         model.StatusPending,
		TotalNodes:     0,
		CompletedNodes: 0,
		NodeResults:    make(map[string]model.NodeResult),
		CreatedAt:      now,
		ExpiresAt:      now.Add(h.cfg.Storage.ReportTTL),
	}

	if err := h.store.CreateReport(r.Context(), report); err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to store report: %v", err))
		return
	}

	_ = h.store.RecordUserRequest(r.Context(), report.ID, claims.UserID, claims.Username, now)
	_ = h.store.RecordUserTargets(r.Context(), claims.UserID, req.Targets, now)

	// 4. Asynchronously launch runner
	if h.runner != nil {
		h.runner.ExecuteCheck(r.Context(), report)
	}

	writeJSON(w, http.StatusCreated, report)
}

// ListReports godoc
// @Summary List connectivity reports
// @Description Returns paginated connectivity check reports for current user (or all if admin)
// @Tags checks
// @Produce json
// @Security BearerAuth
// @Param limit query int false "Pagination limit (default: 20)"
// @Param offset query int false "Pagination offset (default: 0)"
// @Success 200 {object} ReportsResponse
// @Failure 401 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/reports [get]
func (h *Handler) ListReports(w http.ResponseWriter, r *http.Request) {
	claims := UserFromContext(r.Context())

	limit := 20
	offset := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		if val, err := strconv.Atoi(l); err == nil && val > 0 {
			limit = val
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if val, err := strconv.Atoi(o); err == nil && val >= 0 {
			offset = val
		}
	}

	reports, total, err := h.store.ListReports(r.Context(), claims.UserID, claims.Role, limit, offset)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Listing reports failed: %v", err))
		return
	}

	if reports == nil {
		reports = []*model.Report{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"reports": reports,
		"total":   total,
		"limit":   limit,
		"offset":  offset,
	})
}

// GetReport godoc
// @Summary Get connectivity report by ID
// @Description Returns full diagnostic results for a specific check report
// @Tags checks
// @Produce json
// @Security BearerAuth
// @Param id path string true "Report UUID"
// @Success 200 {object} model.Report
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/reports/{id} [get]
func (h *Handler) GetReport(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeJSONError(w, http.StatusBadRequest, "Missing report ID")
		return
	}

	report, err := h.store.GetReport(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeJSONError(w, http.StatusNotFound, "Report not found or has expired")
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, report)
}

// RerunCheck godoc
// @Summary Rerun existing connectivity check
// @Description Re-executes targets from an existing report under current user
// @Tags checks
// @Produce json
// @Security BearerAuth
// @Param id path string true "Report UUID to rerun"
// @Success 201 {object} model.Report
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/reports/{id}/rerun [post]
func (h *Handler) RerunCheck(w http.ResponseWriter, r *http.Request) {
	claims := UserFromContext(r.Context())

	id := r.PathValue("id")
	if id == "" {
		writeJSONError(w, http.StatusBadRequest, "Missing report ID")
		return
	}

	existing, err := h.store.GetReport(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeJSONError(w, http.StatusNotFound, "Original report not found or has expired")
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Validate targets again (in case security policies changed)
	for i, target := range existing.Targets {
		if err := h.validator.ValidateTarget(r.Context(), target); err != nil {
			writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("Target #%d (%s) blocked: %v", i+1, target.TargetIdentifier(), err))
			return
		}
	}

	now := time.Now().UTC()
	newReport := &model.Report{
		ID:             uuid.NewString(),
		UserID:         claims.UserID,
		Username:       claims.Username,
		Mode:           existing.Mode,
		Namespace:      existing.Namespace,
		Targets:        existing.Targets,
		Status:         model.StatusPending,
		TotalNodes:     0,
		CompletedNodes: 0,
		NodeResults:    make(map[string]model.NodeResult),
		CreatedAt:      now,
		ExpiresAt:      now.Add(h.cfg.Storage.ReportTTL),
	}

	if err := h.store.CreateReport(r.Context(), newReport); err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to store new report: %v", err))
		return
	}

	_ = h.store.RecordUserRequest(r.Context(), newReport.ID, claims.UserID, claims.Username, now)
	_ = h.store.RecordUserTargets(r.Context(), claims.UserID, newReport.Targets, now)

	if h.runner != nil {
		h.runner.ExecuteCheck(r.Context(), newReport)
	}

	writeJSON(w, http.StatusCreated, newReport)
}

// ListUsers godoc
// @Summary List all users
// @Description Admin endpoint to list all registered users
// @Tags admin
// @Produce json
// @Security BearerAuth
// @Success 200 {object} UsersResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/admin/users [get]
func (h *Handler) ListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.store.ListUsers(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var safeUsers []model.User
	for _, u := range users {
		safeUsers = append(safeUsers, u.SafeUser())
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": safeUsers})
}

// CreateUserRequest is the admin user creation payload.
type CreateUserRequest struct {
	Username string     `json:"username"`
	Password string     `json:"password"`
	Role     model.Role `json:"role"`
	Email    string     `json:"email"`
}

// CreateUser godoc
// @Summary Create a new user
// @Description Admin endpoint to create a new local user
// @Tags admin
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body CreateUserRequest true "User creation payload"
// @Success 201 {object} model.User
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Router /api/admin/users [post]
func (h *Handler) CreateUser(w http.ResponseWriter, r *http.Request) {
	var req CreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Username == "" || req.Password == "" {
		writeJSONError(w, http.StatusBadRequest, "Username and password cannot be empty")
		return
	}

	if req.Role != model.RoleAdmin && req.Role != model.RoleUser {
		req.Role = model.RoleUser
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "Failed to hash password")
		return
	}

	user := &model.User{
		ID:           uuid.NewString(),
		Username:     req.Username,
		PasswordHash: string(hash),
		Role:         req.Role,
		Provider:     "local",
		Email:        req.Email,
		CreatedAt:    time.Now().UTC(),
	}

	if err := h.store.CreateUser(r.Context(), user); err != nil {
		writeJSONError(w, http.StatusConflict, fmt.Sprintf("User creation failed: %v", err))
		return
	}

	writeJSON(w, http.StatusCreated, user.SafeUser())
}

// GetRBACPermissions godoc
// @Summary Inspect cluster RBAC permissions
// @Description Admin endpoint to inspect current ServiceAccount RBAC rules and node access
// @Tags admin
// @Produce json
// @Security BearerAuth
// @Success 200 {object} rbac.RBACReport
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Router /api/admin/rbac [get]
func (h *Handler) GetRBACPermissions(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	isMock := h.k8sClient == nil
	report := rbac.InspectCurrentPermissions(ctx, h.k8sClient, h.cfg.K8s.Namespace, isMock)
	writeJSON(w, http.StatusOK, report)
}

// UpdateThemeSettingsRequest payload for updating default theme and language.
type UpdateThemeSettingsRequest struct {
	DefaultTheme    string `json:"default_theme,omitempty"`
	DefaultLanguage string `json:"default_language,omitempty"`
}

// UpdateThemeSettings godoc
// @Summary Update system theme setting
// @Description Admin endpoint to update default theme (light/dark) and language (en/ru)
// @Tags admin
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body UpdateThemeSettingsRequest true "Theme setting payload"
// @Success 200 {object} StatusOKResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/admin/settings/theme [put]
func (h *Handler) UpdateThemeSettings(w http.ResponseWriter, r *http.Request) {
	var req UpdateThemeSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	resp := map[string]any{"success": true}

	if req.DefaultTheme != "" {
		themeVal := strings.ToLower(strings.TrimSpace(req.DefaultTheme))
		if themeVal != "light" && themeVal != "dark" {
			writeJSONError(w, http.StatusBadRequest, "Invalid default_theme: must be 'light' or 'dark'")
			return
		}

		if err := h.store.SetSetting(r.Context(), "default_theme", themeVal); err != nil {
			writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to save theme setting: %v", err))
			return
		}

		h.cfg.Theme.DefaultTheme = themeVal
		resp["default_theme"] = themeVal
	}

	if req.DefaultLanguage != "" {
		langVal := strings.ToLower(strings.TrimSpace(req.DefaultLanguage))
		if langVal != "en" && langVal != "ru" {
			writeJSONError(w, http.StatusBadRequest, "Invalid default_language: must be 'en' or 'ru'")
			return
		}

		if err := h.store.SetSetting(r.Context(), "default_language", langVal); err != nil {
			writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to save language setting: %v", err))
			return
		}

		h.cfg.Theme.DefaultLanguage = langVal
		resp["default_language"] = langVal
	}

	if req.DefaultTheme == "" && req.DefaultLanguage == "" {
		writeJSONError(w, http.StatusBadRequest, "At least one of default_theme or default_language must be provided")
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// GetUserStats godoc
// @Summary Get user diagnostic request statistics
// @Description Admin endpoint returning aggregate check counts per user
// @Tags admin
// @Produce json
// @Security BearerAuth
// @Success 200 {object} model.UserStatsResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/admin/stats/users [get]
func (h *Handler) GetUserStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.store.GetUserRequestStats(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to load user statistics: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

// Livez godoc
// @Summary Kubernetes liveness probe
// @Description Returns 200 OK if the HTTP server process is running and alive
// @Tags system
// @Produce json
// @Success 200 {object} StatusOKResponse
// @Router /api/livez [get]
func (h *Handler) Livez(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
	})
}

// Readyz godoc
// @Summary Kubernetes readiness probe
// @Description Verifies critical service dependencies such as the SQLite storage
// @Tags system
// @Produce json
// @Success 200 {object} StatusOKResponse
// @Failure 503 {object} map[string]string
// @Router /api/readyz [get]
func (h *Handler) Readyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if h.store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "degraded",
			"reason": "storage is not initialized",
		})
		return
	}

	if err := h.store.Ping(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "degraded",
			"reason": fmt.Sprintf("database ping failed: %v", err),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
	})
}

// Startupz godoc
// @Summary Kubernetes startup probe
// @Description Validates that bootstrapping (database migrations, admin setup, auth initialization) is complete
// @Tags system
// @Produce json
// @Success 200 {object} StatusOKResponse
// @Failure 503 {object} map[string]string
// @Router /api/startupz [get]
func (h *Handler) Startupz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if h.store == nil || h.authSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "degraded",
			"reason": "startup bootstrap not completed",
		})
		return
	}

	if err := h.store.Ping(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "degraded",
			"reason": fmt.Sprintf("database ping failed: %v", err),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
	})
}

// Healthz godoc
// @Summary General health check probe
// @Description Legacy endpoint delegating to readiness check
// @Tags system
// @Produce json
// @Success 200 {object} StatusOKResponse
// @Failure 503 {object} map[string]string
// @Router /api/healthz [get]
func (h *Handler) Healthz(w http.ResponseWriter, r *http.Request) {
	h.Readyz(w, r)
}
