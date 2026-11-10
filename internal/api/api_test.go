package api

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kncc/internal/auth"
	"kncc/internal/certutil"
	"kncc/internal/config"
	"kncc/internal/model"
	"kncc/internal/ratelimit"
	"kncc/internal/security"
	"kncc/internal/store"
	"kncc/web"
)

type mockRunner struct {
	executed []*model.Report
}

func (m *mockRunner) ExecuteCheck(ctx context.Context, report *model.Report) {
	m.executed = append(m.executed, report)
}

func setupTestServer(t *testing.T) (http.Handler, store.Store, *mockRunner, func()) {
	t.Helper()

	dir, err := os.MkdirTemp("", "kncc-api-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	dbPath := filepath.Join(dir, "api_test.db")
	s, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.Security.RateLimitPerMinute = 5
	_ = s.EnsureAdminUser(context.Background(), "admin", "admin123")

	jwtMgr := auth.NewJWTManager("test-jwt-secret-1234567890", 1*time.Hour)
	authSvc, err := auth.NewAuthService(context.Background(), &cfg.Auth, s, jwtMgr)
	if err != nil {
		t.Fatalf("failed to create auth service: %v", err)
	}

	validator, err := security.NewTargetValidator(&cfg.Security)
	if err != nil {
		t.Fatalf("failed to create target validator: %v", err)
	}

	limiter := ratelimit.NewLimiter(cfg.Security.RateLimitPerMinute)
	runner := &mockRunner{}

	webFS, _ := web.GetFS()

	h := NewHandler(cfg, s, authSvc, validator, limiter, runner, nil, "1.5.0")
	router := SetupRouter(h, authSvc, limiter, webFS)

	cleanup := func() {
		limiter.Stop()
		_ = s.Close()
		_ = os.RemoveAll(dir)
	}

	return router, s, runner, cleanup
}

func loginHelper(t *testing.T, router http.Handler, username, password string) string {
	t.Helper()
	body, _ := json.Marshal(LoginRequest{
		Username: username,
		Password: password,
		Provider: "local",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("login failed: %d %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	return resp.Token
}

func TestAPI_SystemConfig(t *testing.T) {
	router, _, _, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/system/config", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var cfg map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &cfg); err != nil {
		t.Fatalf("failed to parse json: %v", err)
	}

	if cfg["primary_color"] != "#ffffff" || cfg["accent_color"] != "#dc2626" {
		t.Errorf("unexpected theme config: %+v", cfg)
	}

	if cfg["version"] != "1.5.0" {
		t.Errorf("expected version 1.5.0, got %v", cfg["version"])
	}
}

func TestAPI_EmbeddedFrontendSPA(t *testing.T) {
	router, _, _, cleanup := setupTestServer(t)
	defer cleanup()

	// 1. Root / returns index.html
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for root, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "kncc") || !strings.Contains(body, "id=\"root\"") {
		t.Errorf("expected HTML containing root div, got: %s", body)
	}

	// 2. SPA Route /report/12345 falls back to index.html
	req = httptest.NewRequest(http.MethodGet, "/report/12345", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for SPA route, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "id=\"root\"") {
		t.Errorf("expected SPA fallback to serve index.html")
	}

	// 3. API route /api/unknown returns 404 JSON, NOT index.html
	req = httptest.NewRequest(http.MethodGet, "/api/unknown", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 for unknown api endpoint, got %d", rec.Code)
	}
}

func TestAPI_Auth_LoginAndMe(t *testing.T) {
	router, _, _, cleanup := setupTestServer(t)
	defer cleanup()

	// 1. Wrong credentials -> 401
	body, _ := json.Marshal(LoginRequest{Username: "admin", Password: "wrong"})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for wrong credentials, got %d", rec.Code)
	}

	// 2. Correct credentials -> 200 & JWT
	token := loginHelper(t, router, "admin", "admin123")
	if token == "" {
		t.Fatalf("expected non-empty token")
	}

	// 3. /api/auth/me without token -> 401
	req = httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for unauthenticated /api/auth/me, got %d", rec.Code)
	}

	// 4. /api/auth/me with Bearer token -> 200
	req = httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for authenticated /api/auth/me, got %d", rec.Code)
	}
}

func TestAPI_ChecksAndAntiSSRF(t *testing.T) {
	router, _, runner, cleanup := setupTestServer(t)
	defer cleanup()

	token := loginHelper(t, router, "admin", "admin123")

	// 1. Check with blacklisted IP (Cloud metadata 169.254.169.254) -> must be blocked 400
	blockedPayload, _ := json.Marshal(model.CheckRequest{
		Mode: model.ModeHostNetwork,
		Targets: []model.TargetService{
			{Protocol: model.ProtocolTCP, Host: "169.254.169.254", Port: 80},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/checks", bytes.NewReader(blockedPayload))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for blacklisted host, got %d", rec.Code)
	}

	// 2. Legitimate check -> 201 Created
	validPayload, _ := json.Marshal(model.CheckRequest{
		Mode: model.ModeHostNetwork,
		Targets: []model.TargetService{
			{Protocol: model.ProtocolTCP, Host: "10.0.0.1", Port: 443},
		},
	})
	req = httptest.NewRequest(http.MethodPost, "/api/checks", bytes.NewReader(validPayload))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created for valid check, got %d (%s)", rec.Code, rec.Body.String())
	}

	var createdReport model.Report
	_ = json.Unmarshal(rec.Body.Bytes(), &createdReport)
	if createdReport.ID == "" {
		t.Errorf("expected non-empty report ID")
	}

	if len(runner.executed) != 1 || runner.executed[0].ID != createdReport.ID {
		t.Errorf("expected runner to be triggered for report %s", createdReport.ID)
	}

	// 3. Get Report by ID
	req = httptest.NewRequest(http.MethodGet, "/api/reports/"+createdReport.ID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for GetReport, got %d", rec.Code)
	}

	// 4. Rerun Report
	req = httptest.NewRequest(http.MethodPost, "/api/reports/"+createdReport.ID+"/rerun", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201 Created for Rerun, got %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestAPI_AdminUsers(t *testing.T) {
	router, s, _, cleanup := setupTestServer(t)
	defer cleanup()

	// Create regular user
	_ = s.EnsureAdminUser(context.Background(), "user1", "user123")
	userObj, _ := s.GetUserByUsername(context.Background(), "user1")
	userObj.Role = model.RoleUser
	_ = s.CreateUser(context.Background(), &model.User{
		ID:           "u-reg",
		Username:     "regular",
		PasswordHash: userObj.PasswordHash,
		Role:         model.RoleUser,
		Provider:     "local",
		CreatedAt:    time.Now().UTC(),
	})

	adminToken := loginHelper(t, router, "admin", "admin123")
	userToken := loginHelper(t, router, "regular", "user123")

	// 1. Regular user accessing /api/admin/users -> 403 Forbidden
	req := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	req.Header.Set("Authorization", "Bearer "+userToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for regular user, got %d", rec.Code)
	}

	// 2. Admin accessing /api/admin/users -> 200 OK
	req = httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 OK for admin, got %d", rec.Code)
	}

	// 3. Admin creating a user
	newUserBody, _ := json.Marshal(CreateUserRequest{
		Username: "newdev",
		Password: "password123",
		Role:     model.RoleUser,
		Email:    "dev@corp.local",
	})
	req = httptest.NewRequest(http.MethodPost, "/api/admin/users", bytes.NewReader(newUserBody))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201 Created for user creation, got %d", rec.Code)
	}

	// 4. Regular user creating a user -> 403 Forbidden
	req = httptest.NewRequest(http.MethodPost, "/api/admin/users", bytes.NewReader(newUserBody))
	req.Header.Set("Authorization", "Bearer "+userToken)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for regular user creating user, got %d", rec.Code)
	}

	// 5. Unauthenticated user accessing admin routes -> 401 Unauthorized
	req = httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized for unauthenticated access, got %d", rec.Code)
	}
}

func TestAPI_GetNamespaces(t *testing.T) {
	router, _, _, cleanup := setupTestServer(t)
	defer cleanup()

	token := loginHelper(t, router, "admin", "admin123")

	// 1. By default, namespace listing is disabled -> 403 Forbidden
	req := httptest.NewRequest(http.MethodGet, "/api/k8s/namespaces", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden when listing disabled, got %d", rec.Code)
	}
}

func TestAPI_GetNamespaces_Enabled(t *testing.T) {
	dir, err := os.MkdirTemp("", "kncc-api-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(dir)
	dbPath := filepath.Join(dir, "api_test.db")
	s, _ := store.NewSQLiteStore(dbPath)
	defer s.Close()

	cfg := config.DefaultConfig()
	cfg.K8s.NamespaceListEnabled = true
	cfg.K8s.NamespaceExcludeList = []string{"kube-system"}
	cfg.K8s.NamespaceExcludeSelector = "kncc.io/exclude=true"
	_ = s.EnsureAdminUser(context.Background(), "admin", "admin123")

	jwtMgr := auth.NewJWTManager("test-jwt-secret-1234567890", 1*time.Hour)
	authSvc, _ := auth.NewAuthService(context.Background(), &cfg.Auth, s, jwtMgr)
	validator, _ := security.NewTargetValidator(&cfg.Security)
	limiter := ratelimit.NewLimiter(cfg.Security.RateLimitPerMinute)
	defer limiter.Stop()
	runner := &mockRunner{}
	webFS, _ := web.GetFS()

	h := NewHandler(cfg, s, authSvc, validator, limiter, runner, nil, "1.5.0")
	router := SetupRouter(h, authSvc, limiter, webFS)

	token := loginHelper(t, router, "admin", "admin123")

	req := httptest.NewRequest(http.MethodGet, "/api/k8s/namespaces", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Namespaces []string `json:"namespaces"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if len(resp.Namespaces) == 0 {
		t.Fatalf("expected non-empty namespaces list")
	}

	for _, ns := range resp.Namespaces {
		if ns == "kube-system" || ns == "system-monitoring" {
			t.Errorf("excluded namespace %s returned in response", ns)
		}
	}
}

func TestAPI_CreateCheck_ForbiddenNamespace(t *testing.T) {
	dir, err := os.MkdirTemp("", "kncc-api-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(dir)
	dbPath := filepath.Join(dir, "api_test.db")
	s, _ := store.NewSQLiteStore(dbPath)
	defer s.Close()

	cfg := config.DefaultConfig()
	cfg.K8s.MultiNamespaceAllowed = true
	cfg.K8s.NamespaceExcludeList = []string{"kube-system"}
	_ = s.EnsureAdminUser(context.Background(), "admin", "admin123")

	jwtMgr := auth.NewJWTManager("test-jwt-secret-1234567890", 1*time.Hour)
	authSvc, _ := auth.NewAuthService(context.Background(), &cfg.Auth, s, jwtMgr)
	validator, _ := security.NewTargetValidator(&cfg.Security)
	limiter := ratelimit.NewLimiter(cfg.Security.RateLimitPerMinute)
	defer limiter.Stop()
	runner := &mockRunner{}
	webFS, _ := web.GetFS()

	h := NewHandler(cfg, s, authSvc, validator, limiter, runner, nil, "1.5.0")
	router := SetupRouter(h, authSvc, limiter, webFS)

	token := loginHelper(t, router, "admin", "admin123")

	// Attempt check in kube-system
	checkPayload := model.CheckRequest{
		Mode:      model.ModePodNetwork,
		Namespace: "kube-system",
		Targets: []model.TargetService{
			{Protocol: model.ProtocolTCP, Host: "10.0.0.1", Port: 80},
		},
	}
	b, _ := json.Marshal(checkPayload)
	req := httptest.NewRequest(http.MethodPost, "/api/checks", bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for forbidden namespace, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAPI_GetRBACPermissions(t *testing.T) {
	router, s, _, cleanup := setupTestServer(t)
	defer cleanup()

	// Create regular user
	user := &model.User{
		ID:           "test-user-id",
		Username:     "regular",
		PasswordHash: "$2a$10$w857aQ43lB5kG70O356gqenpP5Z8qUf5Z0P.U5r32i9K922pQoU0y", // not used directly
		Role:         model.RoleUser,
		Provider:     "local",
		CreatedAt:    time.Now().UTC(),
	}
	_ = s.CreateUser(context.Background(), user)

	adminToken := loginHelper(t, router, "admin", "admin123")

	// 1. Unauthenticated request -> 401 Unauthorized
	req := httptest.NewRequest(http.MethodGet, "/api/admin/rbac", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized, got %d", rec.Code)
	}

	// 2. Regular user request -> 403 Forbidden
	jwtMgr := auth.NewJWTManager("test-jwt-secret-1234567890", 1*time.Hour)
	userToken, _ := jwtMgr.GenerateToken(user)
	req = httptest.NewRequest(http.MethodGet, "/api/admin/rbac", nil)
	req.Header.Set("Authorization", "Bearer "+userToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for regular user, got %d", rec.Code)
	}

	// 3. Admin user request -> 200 OK
	req = httptest.NewRequest(http.MethodGet, "/api/admin/rbac", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for admin, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Subject struct {
			Kind string `json:"kind"`
			Name string `json:"name"`
		} `json:"subject"`
		Rules  []any  `json:"rules"`
		Source string `json:"source"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response JSON: %v", err)
	}

	if resp.Subject.Kind == "" {
		t.Error("expected non-empty subject.kind")
	}
	if len(resp.Rules) == 0 {
		t.Error("expected at least one policy rule in response")
	}
}

func TestAPI_ThemeSettings(t *testing.T) {
	router, s, _, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	// 1. Initial GetSystemConfig has default_theme="light"
	req := httptest.NewRequest(http.MethodGet, "/api/system/config", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var sysCfg map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &sysCfg); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if sysCfg["default_theme"] != "light" {
		t.Errorf("expected default_theme 'light', got %v", sysCfg["default_theme"])
	}
	if sysCfg["default_language"] != "en" {
		t.Errorf("expected default_language 'en', got %v", sysCfg["default_language"])
	}

	// 2. Anonymous request to PUT /api/admin/settings/theme -> 401
	payload := []byte(`{"default_theme":"dark"}`)
	req = httptest.NewRequest(http.MethodPut, "/api/admin/settings/theme", bytes.NewReader(payload))
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized for anonymous, got %d", rec.Code)
	}

	// Create user & admin tokens
	user := &model.User{
		ID:        "user-theme-1",
		Username:  "regular-theme-user",
		Role:      model.RoleUser,
		Provider:  "local",
		CreatedAt: time.Now().UTC(),
	}
	_ = s.CreateUser(ctx, user)
	jwtMgr := auth.NewJWTManager("test-jwt-secret-1234567890", 1*time.Hour)
	userToken, _ := jwtMgr.GenerateToken(user)

	admin, _ := s.GetUserByUsername(ctx, "admin")
	adminToken, _ := jwtMgr.GenerateToken(admin)

	// 3. Regular user -> 403 Forbidden
	req = httptest.NewRequest(http.MethodPut, "/api/admin/settings/theme", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+userToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for regular user, got %d", rec.Code)
	}

	// 4. Admin with invalid theme value -> 400 Bad Request
	badPayload := []byte(`{"default_theme":"invalid-mode"}`)
	req = httptest.NewRequest(http.MethodPut, "/api/admin/settings/theme", bytes.NewReader(badPayload))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request, got %d", rec.Code)
	}

	// 5. Admin with valid value 'dark' -> 200 OK
	req = httptest.NewRequest(http.MethodPut, "/api/admin/settings/theme", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for admin, got %d: %s", rec.Code, rec.Body.String())
	}

	// 6. Verify GetSystemConfig now returns 'dark'
	req = httptest.NewRequest(http.MethodGet, "/api/system/config", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	sysCfg = nil
	_ = json.Unmarshal(rec.Body.Bytes(), &sysCfg)
	if sysCfg["default_theme"] != "dark" {
		t.Errorf("expected default_theme 'dark' after update, got %v", sysCfg["default_theme"])
	}

	// 7. Admin updates default_language to 'ru'
	langPayload := []byte(`{"default_language":"ru"}`)
	req = httptest.NewRequest(http.MethodPut, "/api/admin/settings/theme", bytes.NewReader(langPayload))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for admin updating language, got %d: %s", rec.Code, rec.Body.String())
	}

	// 8. Verify GetSystemConfig now returns 'ru'
	req = httptest.NewRequest(http.MethodGet, "/api/system/config", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	sysCfg = nil
	_ = json.Unmarshal(rec.Body.Bytes(), &sysCfg)
	if sysCfg["default_language"] != "ru" {
		t.Errorf("expected default_language 'ru' after update, got %v", sysCfg["default_language"])
	}
}

func TestAPI_SecurityHeaders(t *testing.T) {
	router, _, _, cleanup := setupTestServer(t)
	defer cleanup()

	endpoints := []struct {
		name   string
		method string
		path   string
	}{
		{name: "HealthCheck", method: http.MethodGet, path: "/api/healthz"},
		{name: "SystemConfig", method: http.MethodGet, path: "/api/system/config"},
		{name: "RootFrontendSPA", method: http.MethodGet, path: "/"},
	}

	for _, ep := range endpoints {
		t.Run(ep.name, func(t *testing.T) {
			req := httptest.NewRequest(ep.method, ep.path, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			headers := rec.Header()

			if val := headers.Get("X-Content-Type-Options"); val != "nosniff" {
				t.Errorf("expected X-Content-Type-Options: nosniff, got %q", val)
			}
			if val := headers.Get("X-Frame-Options"); val != "DENY" {
				t.Errorf("expected X-Frame-Options: DENY, got %q", val)
			}
			if val := headers.Get("X-XSS-Protection"); val != "1; mode=block" {
				t.Errorf("expected X-XSS-Protection: 1; mode=block, got %q", val)
			}
			if val := headers.Get("Referrer-Policy"); val != "strict-origin-when-cross-origin" {
				t.Errorf("expected Referrer-Policy: strict-origin-when-cross-origin, got %q", val)
			}
			if val := headers.Get("Permissions-Policy"); !strings.Contains(val, "camera=()") || !strings.Contains(val, "microphone=()") {
				t.Errorf("expected Permissions-Policy with restricted APIs, got %q", val)
			}

			csp := headers.Get("Content-Security-Policy")
			if !strings.Contains(csp, "default-src 'self'") {
				t.Errorf("expected CSP to contain default-src 'self', got %q", csp)
			}
			if !strings.Contains(csp, "frame-ancestors 'none'") {
				t.Errorf("expected CSP to contain frame-ancestors 'none', got %q", csp)
			}
			if !strings.Contains(csp, "img-src 'self' data:") {
				t.Errorf("expected CSP to contain img-src 'self' data:, got %q", csp)
			}
		})
	}
}

func TestAPI_CORS_Policy(t *testing.T) {
	router, _, _, cleanup := setupTestServer(t)
	defer cleanup()

	// 1. Same-origin request without Origin header -> No Access-Control-Allow-Origin header
	req := httptest.NewRequest(http.MethodGet, "/api/healthz", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if acao := rec.Header().Get("Access-Control-Allow-Origin"); acao != "" {
		t.Errorf("expected no Access-Control-Allow-Origin for non-CORS request, got %q", acao)
	}

	// 2. Cross-origin request with Origin header -> Origin reflected and Vary: Origin set
	req = httptest.NewRequest(http.MethodGet, "/api/healthz", nil)
	req.Header.Set("Origin", "http://example.com")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if acao := rec.Header().Get("Access-Control-Allow-Origin"); acao != "http://example.com" {
		t.Errorf("expected Access-Control-Allow-Origin: http://example.com, got %q", acao)
	}
	if vary := rec.Header().Get("Vary"); !strings.Contains(vary, "Origin") {
		t.Errorf("expected Vary: Origin, got %q", vary)
	}

	// 3. Preflight OPTIONS request
	req = httptest.NewRequest(http.MethodOptions, "/api/checks", nil)
	req.Header.Set("Origin", "http://example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 OK for OPTIONS preflight, got %d", rec.Code)
	}
	if acao := rec.Header().Get("Access-Control-Allow-Origin"); acao != "http://example.com" {
		t.Errorf("expected Access-Control-Allow-Origin: http://example.com, got %q", acao)
	}
	if methods := rec.Header().Get("Access-Control-Allow-Methods"); !strings.Contains(methods, "POST") {
		t.Errorf("expected Access-Control-Allow-Methods to include POST, got %q", methods)
	}
}

func TestAPI_GetUserStats(t *testing.T) {
	router, _, _, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	// 1. Create a normal user (role: user)
	regReq := httptest.NewRequest(http.MethodPost, "/api/admin/users", bytes.NewReader([]byte(`{
		"username": "regularuser",
		"password": "userpass123",
		"role": "user"
	}`)))
	adminToken := loginHelper(t, router, "admin", "admin123")
	regReq.Header.Set("Authorization", "Bearer "+adminToken)
	regReq.Header.Set("Content-Type", "application/json")
	regRec := httptest.NewRecorder()
	router.ServeHTTP(regRec, regReq)
	if regRec.Code != http.StatusCreated {
		t.Fatalf("failed to create normal user: %d %s", regRec.Code, regRec.Body.String())
	}

	userToken := loginHelper(t, router, "regularuser", "userpass123")

	// 2. Unauthenticated request to /api/admin/stats/users -> 401
	reqUnauth := httptest.NewRequest(http.MethodGet, "/api/admin/stats/users", nil)
	recUnauth := httptest.NewRecorder()
	router.ServeHTTP(recUnauth, reqUnauth)
	if recUnauth.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized, got %d", recUnauth.Code)
	}

	// 3. User with 'user' role -> 403 Forbidden
	reqForbidden := httptest.NewRequest(http.MethodGet, "/api/admin/stats/users", nil)
	reqForbidden.Header.Set("Authorization", "Bearer "+userToken)
	recForbidden := httptest.NewRecorder()
	router.ServeHTTP(recForbidden, reqForbidden)
	if recForbidden.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden, got %d", recForbidden.Code)
	}

	// 4. Admin request -> 200 OK
	reqAdmin := httptest.NewRequest(http.MethodGet, "/api/admin/stats/users", nil)
	reqAdmin.Header.Set("Authorization", "Bearer "+adminToken)
	recAdmin := httptest.NewRecorder()
	router.ServeHTTP(recAdmin, reqAdmin)
	if recAdmin.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", recAdmin.Code, recAdmin.Body.String())
	}

	var statsResp model.UserStatsResponse
	if err := json.Unmarshal(recAdmin.Body.Bytes(), &statsResp); err != nil {
		t.Fatalf("failed to unmarshal UserStatsResponse: %v", err)
	}
	if len(statsResp.Users) < 2 {
		t.Errorf("expected at least 2 users, got %d", len(statsResp.Users))
	}

	// 5. Normal user creates a check -> should record in stats
	checkBody := `{"mode":"host","targets":[{"protocol":"tcp","host":"10.0.0.1","port":80}]}`
	checkReq := httptest.NewRequest(http.MethodPost, "/api/checks", strings.NewReader(checkBody))
	checkReq.Header.Set("Authorization", "Bearer "+userToken)
	checkReq.Header.Set("Content-Type", "application/json")
	checkRec := httptest.NewRecorder()
	router.ServeHTTP(checkRec, checkReq)
	if checkRec.Code != http.StatusCreated {
		t.Fatalf("CreateCheck failed: %d %s", checkRec.Code, checkRec.Body.String())
	}

	// Fetch stats again as admin
	reqAdmin2 := httptest.NewRequest(http.MethodGet, "/api/admin/stats/users", nil)
	reqAdmin2.Header.Set("Authorization", "Bearer "+adminToken)
	recAdmin2 := httptest.NewRecorder()
	router.ServeHTTP(recAdmin2, reqAdmin2)
	if recAdmin2.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", recAdmin2.Code)
	}

	var statsAfter model.UserStatsResponse
	_ = json.Unmarshal(recAdmin2.Body.Bytes(), &statsAfter)

	var foundRegularUser bool
	for _, u := range statsAfter.Users {
		if u.Username == "regularuser" {
			foundRegularUser = true
			if u.DayCount != 1 || u.TotalCount != 1 {
				t.Errorf("expected regularuser counts to be 1, got day=%d total=%d", u.DayCount, u.TotalCount)
			}
			if u.LastActive == nil {
				t.Errorf("expected regularuser last_active to be set")
			}
		}
	}
	if !foundRegularUser {
		t.Errorf("regularuser not found in stats response")
	}
	if statsAfter.Total.TotalCount < 1 {
		t.Errorf("expected total count >= 1, got %d", statsAfter.Total.TotalCount)
	}
	_ = ctx
}

func TestAPI_QuickAdd(t *testing.T) {
	router, _, _, cleanup := setupTestServer(t)
	defer cleanup()

	// 1. Create a user via admin
	adminToken := loginHelper(t, router, "admin", "admin123")
	regReq := httptest.NewRequest(http.MethodPost, "/api/admin/users", bytes.NewReader([]byte(`{
		"username": "quser",
		"password": "userpass123",
		"role": "user"
	}`)))
	regReq.Header.Set("Authorization", "Bearer "+adminToken)
	regReq.Header.Set("Content-Type", "application/json")
	regRec := httptest.NewRecorder()
	router.ServeHTTP(regRec, regReq)
	if regRec.Code != http.StatusCreated {
		t.Fatalf("failed to create user: %d %s", regRec.Code, regRec.Body.String())
	}
	token := loginHelper(t, router, "quser", "userpass123")

	// 2. Fresh user calls GET /api/checks/quick-add -> should receive default presets
	req := httptest.NewRequest(http.MethodGet, "/api/checks/quick-add", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		QuickAdd []model.TargetService `json:"quick_add"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if len(resp.QuickAdd) != 2 {
		t.Fatalf("expected 2 default presets, got %d", len(resp.QuickAdd))
	}
	if resp.QuickAdd[0].Name != "K8s API" || resp.QuickAdd[1].Name != "Google DNS" {
		t.Errorf("unexpected presets: %+v", resp.QuickAdd)
	}

	// 3. User creates a check with a custom target
	checkPayload := `{"mode":"host","targets":[{"protocol":"tcp","host":"10.10.10.10","port":8080}]}`
	checkReq := httptest.NewRequest(http.MethodPost, "/api/checks", strings.NewReader(checkPayload))
	checkReq.Header.Set("Authorization", "Bearer "+token)
	checkReq.Header.Set("Content-Type", "application/json")
	checkRec := httptest.NewRecorder()
	router.ServeHTTP(checkRec, checkReq)
	if checkRec.Code != http.StatusCreated {
		t.Fatalf("CreateCheck failed: %d: %s", checkRec.Code, checkRec.Body.String())
	}

	// 4. Quick add should now have the user's first request as the first item + default presets (total 3 items!)
	req2 := httptest.NewRequest(http.MethodGet, "/api/checks/quick-add", nil)
	req2.Header.Set("Authorization", "Bearer "+token)
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec2.Code, rec2.Body.String())
	}

	var resp2 struct {
		QuickAdd []model.TargetService `json:"quick_add"`
	}
	if err := json.Unmarshal(rec2.Body.Bytes(), &resp2); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if len(resp2.QuickAdd) != 3 {
		t.Fatalf("expected 3 items (user target + 2 defaults), got %d: %+v", len(resp2.QuickAdd), resp2.QuickAdd)
	}
	if resp2.QuickAdd[0].Host != "10.10.10.10" {
		t.Errorf("expected first item to be user's request 10.10.10.10, got %s", resp2.QuickAdd[0].Host)
	}

	// 5. User performs checks to establish frequent services (within rate limit)
	// Check with 10.20.20.20 and 10.30.30.30
	check2Payload := `{"mode":"host","targets":[
		{"protocol":"tcp","host":"10.20.20.20","port":9000},
		{"protocol":"tcp","host":"10.30.30.30","port":3000}
	]}`
	check2Req := httptest.NewRequest(http.MethodPost, "/api/checks", strings.NewReader(check2Payload))
	check2Req.Header.Set("Authorization", "Bearer "+token)
	check2Req.Header.Set("Content-Type", "application/json")
	check2Rec := httptest.NewRecorder()
	router.ServeHTTP(check2Rec, check2Req)
	if check2Rec.Code != http.StatusCreated {
		t.Fatalf("Check 2 failed: %d: %s", check2Rec.Code, check2Rec.Body.String())
	}

	// Another check with 10.20.20.20 to make it the most frequent (count = 2)
	check3Payload := `{"mode":"host","targets":[{"protocol":"tcp","host":"10.20.20.20","port":9000}]}`
	check3Req := httptest.NewRequest(http.MethodPost, "/api/checks", strings.NewReader(check3Payload))
	check3Req.Header.Set("Authorization", "Bearer "+token)
	check3Req.Header.Set("Content-Type", "application/json")
	check3Rec := httptest.NewRecorder()
	router.ServeHTTP(check3Rec, check3Req)
	if check3Rec.Code != http.StatusCreated {
		t.Fatalf("Check 3 failed: %d: %s", check3Rec.Code, check3Rec.Body.String())
	}

	// 6. User now has 3 distinct targets: 10.20.20.20 (2 reqs), 10.30.30.30 (1 req, newer), 10.10.10.10 (1 req, older)
	req3 := httptest.NewRequest(http.MethodGet, "/api/checks/quick-add", nil)
	req3.Header.Set("Authorization", "Bearer "+token)
	rec3 := httptest.NewRecorder()
	router.ServeHTTP(rec3, req3)

	var resp3 struct {
		QuickAdd []model.TargetService `json:"quick_add"`
	}
	_ = json.Unmarshal(rec3.Body.Bytes(), &resp3)
	if len(resp3.QuickAdd) != 3 {
		t.Fatalf("expected 3 top user targets, got %d", len(resp3.QuickAdd))
	}
	if resp3.QuickAdd[0].Host != "10.20.20.20" {
		t.Errorf("expected top service to be 10.20.20.20, got %s", resp3.QuickAdd[0].Host)
	}
	if resp3.QuickAdd[1].Host != "10.30.30.30" {
		t.Errorf("expected 2nd service to be 10.30.30.30, got %s", resp3.QuickAdd[1].Host)
	}
	if resp3.QuickAdd[2].Host != "10.10.10.10" {
		t.Errorf("expected 3rd service to be 10.10.10.10, got %s", resp3.QuickAdd[2].Host)
	}

	// 7. Verify GetSystemConfig includes quick_add_presets
	cfgReq := httptest.NewRequest(http.MethodGet, "/api/system/config", nil)
	cfgRec := httptest.NewRecorder()
	router.ServeHTTP(cfgRec, cfgReq)
	var sysCfg map[string]any
	_ = json.Unmarshal(cfgRec.Body.Bytes(), &sysCfg)
	if sysCfg["quick_add_presets"] == nil {
		t.Errorf("expected quick_add_presets in system config")
	}
}

func TestAPI_KubernetesProbes(t *testing.T) {
	router, storeInstance, _, cleanup := setupTestServer(t)
	defer cleanup()

	// 1. Test GET /api/livez -> 200 OK
	reqLive := httptest.NewRequest(http.MethodGet, "/api/livez", nil)
	recLive := httptest.NewRecorder()
	router.ServeHTTP(recLive, reqLive)
	if recLive.Code != http.StatusOK {
		t.Fatalf("expected 200 for /api/livez, got %d: %s", recLive.Code, recLive.Body.String())
	}
	var liveResp map[string]string
	if err := json.Unmarshal(recLive.Body.Bytes(), &liveResp); err != nil || liveResp["status"] != "ok" {
		t.Errorf("unexpected livez response: %+v", liveResp)
	}

	// 2. Test GET /api/startupz -> 200 OK
	reqStartup := httptest.NewRequest(http.MethodGet, "/api/startupz", nil)
	recStartup := httptest.NewRecorder()
	router.ServeHTTP(recStartup, reqStartup)
	if recStartup.Code != http.StatusOK {
		t.Fatalf("expected 200 for /api/startupz, got %d: %s", recStartup.Code, recStartup.Body.String())
	}
	var startupResp map[string]string
	if err := json.Unmarshal(recStartup.Body.Bytes(), &startupResp); err != nil || startupResp["status"] != "ok" {
		t.Errorf("unexpected startupz response: %+v", startupResp)
	}

	// 3. Test GET /api/readyz -> 200 OK when DB is healthy
	reqReady := httptest.NewRequest(http.MethodGet, "/api/readyz", nil)
	recReady := httptest.NewRecorder()
	router.ServeHTTP(recReady, reqReady)
	if recReady.Code != http.StatusOK {
		t.Fatalf("expected 200 for /api/readyz, got %d: %s", recReady.Code, recReady.Body.String())
	}
	var readyResp map[string]string
	if err := json.Unmarshal(recReady.Body.Bytes(), &readyResp); err != nil || readyResp["status"] != "ok" {
		t.Errorf("unexpected readyz response: %+v", readyResp)
	}

	// 4. Test GET /api/healthz (legacy backward compatibility) -> 200 OK
	reqHealth := httptest.NewRequest(http.MethodGet, "/api/healthz", nil)
	recHealth := httptest.NewRecorder()
	router.ServeHTTP(recHealth, reqHealth)
	if recHealth.Code != http.StatusOK {
		t.Fatalf("expected 200 for /api/healthz, got %d: %s", recHealth.Code, recHealth.Body.String())
	}
	var healthResp map[string]string
	if err := json.Unmarshal(recHealth.Body.Bytes(), &healthResp); err != nil || healthResp["status"] != "ok" {
		t.Errorf("unexpected healthz response: %+v", healthResp)
	}

	// 5. Simulate Database failure: close the DB connection
	if err := storeInstance.Close(); err != nil {
		t.Fatalf("failed to close store: %v", err)
	}

	// 6. Test GET /api/readyz with failed DB -> 503 Service Unavailable
	recReadyDegraded := httptest.NewRecorder()
	router.ServeHTTP(recReadyDegraded, reqReady)
	if recReadyDegraded.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for degraded /api/readyz, got %d: %s", recReadyDegraded.Code, recReadyDegraded.Body.String())
	}
	var degradedResp map[string]string
	if err := json.Unmarshal(recReadyDegraded.Body.Bytes(), &degradedResp); err != nil || degradedResp["status"] != "degraded" {
		t.Errorf("unexpected degraded readyz response: %+v", degradedResp)
	}
	if !strings.Contains(degradedResp["reason"], "database ping failed") {
		t.Errorf("expected reason to contain 'database ping failed', got %q", degradedResp["reason"])
	}

	// 7. Verify /api/healthz also reflects degraded status (503)
	recHealthDegraded := httptest.NewRecorder()
	router.ServeHTTP(recHealthDegraded, reqHealth)
	if recHealthDegraded.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for degraded /api/healthz, got %d: %s", recHealthDegraded.Code, recHealthDegraded.Body.String())
	}

	// 8. Verify /api/livez STILL returns 200 OK (Kubernetes must NOT restart container during temporary DB outage)
	recLiveStillOk := httptest.NewRecorder()
	router.ServeHTTP(recLiveStillOk, reqLive)
	if recLiveStillOk.Code != http.StatusOK {
		t.Fatalf("expected 200 for /api/livez even when DB is degraded, got %d: %s", recLiveStillOk.Code, recLiveStillOk.Body.String())
	}
}

func TestServer_TLSEndpoint(t *testing.T) {
	router, _, _, cleanup := setupTestServer(t)
	defer cleanup()

	// Generate a self-signed cert for the server
	cert, err := certutil.GenerateSelfSignedCertificate([]string{"127.0.0.1", "localhost"}, time.Hour)
	if err != nil {
		t.Fatalf("failed to generate self-signed cert: %v", err)
	}

	ts := httptest.NewUnstartedServer(router)
	ts.TLS = &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}
	ts.StartTLS()
	defer ts.Close()

	// Make request via HTTPS client
	client := ts.Client()
	resp, err := client.Get(ts.URL + "/api/healthz")
	if err != nil {
		t.Fatalf("failed to make HTTPS request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK over HTTPS, got: %d", resp.StatusCode)
	}

	var data map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		t.Fatalf("failed to decode json response: %v", err)
	}
	if data["status"] != "ok" {
		t.Errorf("expected status 'ok', got: %s", data["status"])
	}
}

func TestOIDC_DisabledEndpoints(t *testing.T) {
	router, _, _, cleanup := setupTestServer(t)
	defer cleanup()

	// 1. OIDC Login should return 400 when disabled
	req := httptest.NewRequest(http.MethodGet, "/api/auth/oidc/login", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for disabled OIDC login, got: %d", rec.Code)
	}

	// 2. OIDC Callback should return 400 when disabled
	reqCallback := httptest.NewRequest(http.MethodGet, "/api/auth/oidc/callback?code=testcode", nil)
	recCallback := httptest.NewRecorder()
	router.ServeHTTP(recCallback, reqCallback)

	if recCallback.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for disabled OIDC callback, got: %d", recCallback.Code)
	}
}
