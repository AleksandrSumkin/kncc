package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kncc/internal/auth"
	"kncc/internal/config"
	"kncc/internal/ratelimit"
	"kncc/internal/security"
	"kncc/internal/store"
	"kncc/web"
)

func setupOpenAPITestServer(t *testing.T, enabled, public bool) (http.Handler, string, func()) {
	t.Helper()

	dir, err := os.MkdirTemp("", "kncc-openapi-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	dbPath := filepath.Join(dir, "openapi_test.db")
	s, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.Server.OpenAPI.Enabled = enabled
	cfg.Server.OpenAPI.Public = public
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

	h := NewHandler(cfg, s, authSvc, validator, limiter, runner, nil, "1.11.0")
	router := SetupRouter(h, authSvc, limiter, webFS)

	// Get a valid admin token
	token := loginHelper(t, router, "admin", "admin123")

	cleanup := func() {
		limiter.Stop()
		_ = s.Close()
		_ = os.RemoveAll(dir)
	}

	return router, token, cleanup
}

func TestOpenAPI_PublicMode(t *testing.T) {
	router, _, cleanup := setupOpenAPITestServer(t, true, true)
	defer cleanup()

	// 1. Raw JSON specification
	req := httptest.NewRequest(http.MethodGet, "/api/openapi.json", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for /api/openapi.json in public mode, got %d: %s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("expected application/json Content-Type, got %s", ct)
	}

	var spec map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &spec); err != nil {
		t.Fatalf("failed to parse /api/openapi.json as JSON: %v", err)
	}
	if spec["swagger"] != "2.0" {
		t.Errorf("expected swagger: '2.0', got %v", spec["swagger"])
	}
	paths, ok := spec["paths"].(map[string]any)
	if !ok || len(paths) == 0 {
		t.Fatalf("expected non-empty paths map in OpenAPI spec")
	}
	if _, ok := paths["/api/checks"]; !ok {
		t.Errorf("expected /api/checks path in OpenAPI spec")
	}

	// 2. Raw YAML specification
	reqYAML := httptest.NewRequest(http.MethodGet, "/api/openapi.yaml", nil)
	rrYAML := httptest.NewRecorder()
	router.ServeHTTP(rrYAML, reqYAML)

	if rrYAML.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for /api/openapi.yaml in public mode, got %d: %s", rrYAML.Code, rrYAML.Body.String())
	}
	if ct := rrYAML.Header().Get("Content-Type"); !strings.Contains(ct, "yaml") {
		t.Errorf("expected yaml Content-Type, got %s", ct)
	}
	if !strings.Contains(rrYAML.Body.String(), "swagger: \"2.0\"") {
		t.Errorf("expected swagger: \"2.0\" in YAML output")
	}

	// 3. Swagger UI redirect (/api/docs -> /api/docs/index.html)
	reqDocs := httptest.NewRequest(http.MethodGet, "/api/docs", nil)
	rrDocs := httptest.NewRecorder()
	router.ServeHTTP(rrDocs, reqDocs)

	if rrDocs.Code != http.StatusMovedPermanently {
		t.Fatalf("expected 301 Moved Permanently for /api/docs, got %d", rrDocs.Code)
	}
	if loc := rrDocs.Header().Get("Location"); loc != "/api/docs/index.html" {
		t.Errorf("expected Location '/api/docs/index.html', got %s", loc)
	}

	// 4. Swagger UI index.html
	reqUI := httptest.NewRequest(http.MethodGet, "/api/docs/index.html", nil)
	rrUI := httptest.NewRecorder()
	router.ServeHTTP(rrUI, reqUI)

	if rrUI.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for /api/docs/index.html, got %d: %s", rrUI.Code, rrUI.Body.String())
	}
	if !strings.Contains(rrUI.Body.String(), "swagger-ui") {
		t.Errorf("expected 'swagger-ui' in Swagger UI HTML body")
	}

	// 5. Content Security Policy headers
	cspUI := rrUI.Header().Get("Content-Security-Policy")
	if !strings.Contains(cspUI, "script-src 'self' 'unsafe-inline'") {
		t.Errorf("expected Swagger UI CSP to permit 'unsafe-inline' for scripts, got: %s", cspUI)
	}

	cspJSON := rr.Header().Get("Content-Security-Policy")
	if strings.Contains(cspJSON, "script-src 'self' 'unsafe-inline'") {
		t.Errorf("expected standard API routes not to have 'unsafe-inline' for scripts, got: %s", cspJSON)
	}
}

func TestOpenAPI_ProtectedMode(t *testing.T) {
	router, token, cleanup := setupOpenAPITestServer(t, true, false)
	defer cleanup()

	// 1. Unauthenticated requests must receive 401 Unauthorized
	endpoints := []string{"/api/openapi.json", "/api/openapi.yaml", "/api/docs", "/api/docs/index.html"}
	for _, ep := range endpoints {
		req := httptest.NewRequest(http.MethodGet, ep, nil)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected 401 Unauthorized for unauthenticated %s, got %d", ep, rr.Code)
		}
	}

	// 2. Authenticated requests with Bearer token must succeed
	reqJSON := httptest.NewRequest(http.MethodGet, "/api/openapi.json", nil)
	reqJSON.Header.Set("Authorization", "Bearer "+token)
	rrJSON := httptest.NewRecorder()
	router.ServeHTTP(rrJSON, reqJSON)

	if rrJSON.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for authenticated /api/openapi.json, got %d: %s", rrJSON.Code, rrJSON.Body.String())
	}

	reqYAML := httptest.NewRequest(http.MethodGet, "/api/openapi.yaml", nil)
	reqYAML.Header.Set("Authorization", "Bearer "+token)
	rrYAML := httptest.NewRecorder()
	router.ServeHTTP(rrYAML, reqYAML)

	if rrYAML.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for authenticated /api/openapi.yaml, got %d: %s", rrYAML.Code, rrYAML.Body.String())
	}

	reqDocs := httptest.NewRequest(http.MethodGet, "/api/docs/index.html", nil)
	reqDocs.Header.Set("Authorization", "Bearer "+token)
	rrDocs := httptest.NewRecorder()
	router.ServeHTTP(rrDocs, reqDocs)

	if rrDocs.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for authenticated /api/docs/index.html, got %d: %s", rrDocs.Code, rrDocs.Body.String())
	}
}

func TestOpenAPI_DisabledMode(t *testing.T) {
	router, _, cleanup := setupOpenAPITestServer(t, false, false)
	defer cleanup()

	endpoints := []string{"/api/openapi.json", "/api/openapi.yaml", "/api/docs", "/api/docs/index.html"}
	for _, ep := range endpoints {
		req := httptest.NewRequest(http.MethodGet, ep, nil)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Errorf("expected 404 Not Found for disabled %s, got %d", ep, rr.Code)
		}
	}
}
