package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kncc/internal/auth"
	"kncc/internal/config"
	"kncc/internal/model"
	"kncc/internal/ratelimit"
	"kncc/internal/security"
	"kncc/internal/store"
	"kncc/web"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

func setupLogoTestServer(t *testing.T, withK8s bool) (http.Handler, store.Store, *fake.Clientset, func()) {
	t.Helper()

	dir, err := os.MkdirTemp("", "kncc-logo-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	dbPath := filepath.Join(dir, "logo_test.db")
	s, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}

	cfg := config.DefaultConfig()
	_ = s.EnsureAdminUser(context.Background(), "admin", "admin123")

	// Create a standard user for permission tests
	regularUser := &model.User{
		ID:           "test-regular-user",
		Username:     "developer",
		PasswordHash: "$2a$10$dummyhashregularuser1234567890",
		Role:         model.RoleUser,
		Provider:     "local",
		CreatedAt:    time.Now().UTC(),
	}
	_ = s.CreateUser(context.Background(), regularUser)

	jwtMgr := auth.NewJWTManager("test-jwt-secret-1234567890", 1*time.Hour)
	authSvc, err := auth.NewAuthService(context.Background(), &cfg.Auth, s, jwtMgr)
	if err != nil {
		t.Fatalf("failed to create auth service: %v", err)
	}

	validator, err := security.NewTargetValidator(&cfg.Security)
	if err != nil {
		t.Fatalf("failed to create target validator: %v", err)
	}

	limiter := ratelimit.NewLimiter(100)
	runner := &mockRunner{}
	webFS, _ := web.GetFS()

	var fakeK8s *fake.Clientset
	var k8sClient kubernetes.Interface
	if withK8s {
		fakeK8s = fake.NewSimpleClientset()
		k8sClient = fakeK8s
	}

	h := NewHandler(cfg, s, authSvc, validator, limiter, runner, k8sClient, "1.6.0")
	router := SetupRouter(h, authSvc, limiter, webFS)

	cleanup := func() {
		limiter.Stop()
		_ = s.Close()
		_ = os.RemoveAll(dir)
	}

	return router, s, fakeK8s, cleanup
}

func TestGetSystemLogo_Default(t *testing.T) {
	router, _, _, cleanup := setupLogoTestServer(t, false)
	defer cleanup()

	req := httptest.NewRequest("GET", "/api/system/logo", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "image/svg+xml" {
		t.Errorf("expected Content-Type image/svg+xml, got %s", contentType)
	}

	etag := w.Header().Get("ETag")
	if etag == "" {
		t.Errorf("expected non-empty ETag header")
	}

	body := w.Body.String()
	if !strings.Contains(body, "knccBg") || !strings.Contains(body, "hullGrad") {
		t.Errorf("expected body to contain default shipbuilding logo SVG elements")
	}

	// Test 304 Not Modified
	req304 := httptest.NewRequest("GET", "/api/system/logo", nil)
	req304.Header.Set("If-None-Match", etag)
	w304 := httptest.NewRecorder()
	router.ServeHTTP(w304, req304)

	if w304.Code != http.StatusNotModified {
		t.Errorf("expected 304 Not Modified, got %d", w304.Code)
	}
}

func TestGetSystemConfig_LogoMetadata(t *testing.T) {
	router, _, _, cleanup := setupLogoTestServer(t, false)
	defer cleanup()

	req := httptest.NewRequest("GET", "/api/system/config", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}

	var res map[string]any
	if err := json.NewDecoder(w.Body).Decode(&res); err != nil {
		t.Fatalf("failed to decode json: %v", err)
	}

	if res["has_custom_logo"] != false {
		t.Errorf("expected has_custom_logo to be false initially, got %v", res["has_custom_logo"])
	}

	logoURL, ok := res["logo_url"].(string)
	if !ok || !strings.HasPrefix(logoURL, "/api/system/logo?v=") {
		t.Errorf("expected logo_url starting with /api/system/logo?v=, got %v", res["logo_url"])
	}
}

func TestUploadLogo_Permissions(t *testing.T) {
	router, s, _, cleanup := setupLogoTestServer(t, false)
	defer cleanup()

	// 1. Unauthenticated -> 401
	svgPayload := `{"data": "<svg><rect width=\"10\" height=\"10\"/></svg>"}`
	req := httptest.NewRequest("POST", "/api/admin/settings/logo", strings.NewReader(svgPayload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized, got %d", w.Code)
	}

	// 2. Regular user -> 403
	jwtMgr := auth.NewJWTManager("test-jwt-secret-1234567890", 1*time.Hour)
	regUser, _ := s.GetUserByUsername(context.Background(), "developer")
	userToken, _ := jwtMgr.GenerateToken(regUser)

	reqUser := httptest.NewRequest("POST", "/api/admin/settings/logo", strings.NewReader(svgPayload))
	reqUser.Header.Set("Content-Type", "application/json")
	reqUser.Header.Set("Authorization", "Bearer "+userToken)
	wUser := httptest.NewRecorder()
	router.ServeHTTP(wUser, reqUser)

	if wUser.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for non-admin, got %d", wUser.Code)
	}
}

func TestUploadLogo_ValidSVG_And_Reset(t *testing.T) {
	router, s, _, cleanup := setupLogoTestServer(t, false)
	defer cleanup()

	jwtMgr := auth.NewJWTManager("test-jwt-secret-1234567890", 1*time.Hour)
	adminUser, _ := s.GetUserByUsername(context.Background(), "admin")
	adminToken, _ := jwtMgr.GenerateToken(adminUser)

	customSVG := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100"><circle cx="50" cy="50" r="40" fill="#00ff00"/></svg>`
	payload, _ := json.Marshal(UploadLogoRequestJSON{
		Data:     customSVG,
		MimeType: "image/svg+xml",
	})

	req := httptest.NewRequest("POST", "/api/admin/settings/logo", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	// Verify config reports custom logo
	cfgReq := httptest.NewRequest("GET", "/api/system/config", nil)
	cfgW := httptest.NewRecorder()
	router.ServeHTTP(cfgW, cfgReq)

	var cfgRes map[string]any
	_ = json.NewDecoder(cfgW.Body).Decode(&cfgRes)
	if cfgRes["has_custom_logo"] != true {
		t.Errorf("expected has_custom_logo to be true, got %v", cfgRes["has_custom_logo"])
	}

	// Verify GET logo returns new custom SVG
	logoReq := httptest.NewRequest("GET", "/api/system/logo", nil)
	logoW := httptest.NewRecorder()
	router.ServeHTTP(logoW, logoReq)

	if logoW.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", logoW.Code)
	}
	if !strings.Contains(logoW.Body.String(), `fill="#00ff00"`) {
		t.Errorf("expected custom SVG content in response")
	}

	// Test Reset Logo
	delReq := httptest.NewRequest("DELETE", "/api/admin/settings/logo", nil)
	delReq.Header.Set("Authorization", "Bearer "+adminToken)
	delW := httptest.NewRecorder()
	router.ServeHTTP(delW, delReq)

	if delW.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for reset, got %d: %s", delW.Code, delW.Body.String())
	}

	// Verify config reports false for custom logo
	cfgReq2 := httptest.NewRequest("GET", "/api/system/config", nil)
	cfgW2 := httptest.NewRecorder()
	router.ServeHTTP(cfgW2, cfgReq2)

	var cfgRes2 map[string]any
	_ = json.NewDecoder(cfgW2.Body).Decode(&cfgRes2)
	if cfgRes2["has_custom_logo"] != false {
		t.Errorf("expected has_custom_logo to be false after reset, got %v", cfgRes2["has_custom_logo"])
	}

	// Verify default shipbuilding logo returned
	logoReq2 := httptest.NewRequest("GET", "/api/system/logo", nil)
	logoW2 := httptest.NewRecorder()
	router.ServeHTTP(logoW2, logoReq2)

	if !strings.Contains(logoW2.Body.String(), "hullGrad") {
		t.Errorf("expected default shipbuilding logo after reset")
	}
}

func TestUploadLogo_UnsafeSVG_Rejected(t *testing.T) {
	router, s, _, cleanup := setupLogoTestServer(t, false)
	defer cleanup()

	jwtMgr := auth.NewJWTManager("test-jwt-secret-1234567890", 1*time.Hour)
	adminUser, _ := s.GetUserByUsername(context.Background(), "admin")
	adminToken, _ := jwtMgr.GenerateToken(adminUser)

	maliciousSVGs := []string{
		`<svg><script>alert("XSS")</script></svg>`,
		`<svg onload="fetch('http://evil.com')"></svg>`,
		`<svg><circle cx="10" cy="10" r="5" onclick="alert(1)"/></svg>`,
		`<svg><a href="javascript:alert(1)"><text>Click</text></a></svg>`,
		`<svg><foreignObject><body xmlns="http://www.w3.org/1999/xhtml"><script>alert(1)</script></body></foreignObject></svg>`,
	}

	for _, svg := range maliciousSVGs {
		payload, _ := json.Marshal(UploadLogoRequestJSON{
			Data:     svg,
			MimeType: "image/svg+xml",
		})

		req := httptest.NewRequest("POST", "/api/admin/settings/logo", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+adminToken)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400 Bad Request for unsafe SVG '%s', got %d", svg, w.Code)
		}
	}
}

func TestUploadLogo_PNG_Multipart(t *testing.T) {
	router, s, _, cleanup := setupLogoTestServer(t, false)
	defer cleanup()

	jwtMgr := auth.NewJWTManager("test-jwt-secret-1234567890", 1*time.Hour)
	adminUser, _ := s.GetUserByUsername(context.Background(), "admin")
	adminToken, _ := jwtMgr.GenerateToken(adminUser)

	// Minimal 1x1 PNG bytes
	pngBytes, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkWPjfDwAEeAF+gL4yYgAAAABJRU5ErkJggg==")

	// Create multipart request
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("logo", "logo.png")
	_, _ = part.Write(pngBytes)
	_ = writer.Close()

	req := httptest.NewRequest("POST", "/api/admin/settings/logo", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for PNG multipart upload, got %d: %s", w.Code, w.Body.String())
	}

	// Verify GET logo returns PNG
	logoReq := httptest.NewRequest("GET", "/api/system/logo", nil)
	logoW := httptest.NewRecorder()
	router.ServeHTTP(logoW, logoReq)

	if logoW.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", logoW.Code)
	}
	if logoW.Header().Get("Content-Type") != "image/png" {
		t.Errorf("expected Content-Type image/png, got %s", logoW.Header().Get("Content-Type"))
	}
	if !bytes.Equal(logoW.Body.Bytes(), pngBytes) {
		t.Errorf("expected returned PNG bytes to match uploaded PNG")
	}
}

func TestUploadLogo_WithKubernetesConfigMap(t *testing.T) {
	router, s, fakeK8s, cleanup := setupLogoTestServer(t, true)
	defer cleanup()

	jwtMgr := auth.NewJWTManager("test-jwt-secret-1234567890", 1*time.Hour)
	adminUser, _ := s.GetUserByUsername(context.Background(), "admin")
	adminToken, _ := jwtMgr.GenerateToken(adminUser)

	customSVG := `<svg xmlns="http://www.w3.org/2000/svg"><rect width="50" height="50" fill="#3b82f6"/></svg>`
	payload, _ := json.Marshal(UploadLogoRequestJSON{
		Data: customSVG,
	})

	req := httptest.NewRequest("POST", "/api/admin/settings/logo", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	// Verify ConfigMap 'kncc-logo' was created in 'kncc' namespace
	cm, err := fakeK8s.CoreV1().ConfigMaps("kncc").Get(context.Background(), "kncc-logo", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("expected ConfigMap kncc-logo to exist: %v", err)
	}

	if cm.Data["logo.data"] != customSVG {
		t.Errorf("expected ConfigMap logo.data to match custom SVG, got %s", cm.Data["logo.data"])
	}
	if cm.Data["logo.mime_type"] != "image/svg+xml" {
		t.Errorf("expected ConfigMap logo.mime_type to be image/svg+xml, got %s", cm.Data["logo.mime_type"])
	}

	// Test Reset with ConfigMap
	delReq := httptest.NewRequest("DELETE", "/api/admin/settings/logo", nil)
	delReq.Header.Set("Authorization", "Bearer "+adminToken)
	delW := httptest.NewRecorder()
	router.ServeHTTP(delW, delReq)

	if delW.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for reset, got %d", delW.Code)
	}

	cmAfter, _ := fakeK8s.CoreV1().ConfigMaps("kncc").Get(context.Background(), "kncc-logo", metav1.GetOptions{})
	if cmAfter.Data["logo.data"] != "" {
		t.Errorf("expected ConfigMap logo.data to be cleared after reset, got %s", cmAfter.Data["logo.data"])
	}
}
