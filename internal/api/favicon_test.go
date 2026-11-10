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

func setupFaviconTestServer(t *testing.T, withK8s bool) (http.Handler, store.Store, *fake.Clientset, *auth.JWTManager, func()) {
	t.Helper()

	dir, err := os.MkdirTemp("", "kncc-favicon-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	dbPath := filepath.Join(dir, "favicon_test.db")
	s, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}

	cfg := config.DefaultConfig()
	_ = s.EnsureAdminUser(context.Background(), "admin", "admin123")

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

	h := NewHandler(cfg, s, authSvc, validator, limiter, runner, k8sClient, "1.7.0")
	router := SetupRouter(h, authSvc, limiter, webFS)

	cleanup := func() {
		limiter.Stop()
		_ = s.Close()
		_ = os.RemoveAll(dir)
	}

	return router, s, fakeK8s, jwtMgr, cleanup
}

func getFaviconAdminToken(t *testing.T, s store.Store, jwtMgr *auth.JWTManager) string {
	t.Helper()
	adminUser, err := s.GetUserByUsername(context.Background(), "admin")
	if err != nil {
		t.Fatalf("failed to get admin user: %v", err)
	}
	token, err := jwtMgr.GenerateToken(adminUser)
	if err != nil {
		t.Fatalf("failed to generate admin token: %v", err)
	}
	return token
}

func getFaviconUserToken(t *testing.T, s store.Store, jwtMgr *auth.JWTManager) string {
	t.Helper()
	user, err := s.GetUserByUsername(context.Background(), "developer")
	if err != nil {
		t.Fatalf("failed to get regular user: %v", err)
	}
	token, err := jwtMgr.GenerateToken(user)
	if err != nil {
		t.Fatalf("failed to generate user token: %v", err)
	}
	return token
}

func TestGetSystemFavicon_DefaultFallback(t *testing.T) {
	router, _, _, _, cleanup := setupFaviconTestServer(t, false)
	defer cleanup()

	// 1. Test GET /api/system/favicon
	req := httptest.NewRequest("GET", "/api/system/favicon", nil)
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
		t.Errorf("expected ETag header to be set")
	}

	// 2. Test ETag caching (304 Not Modified)
	reqCache := httptest.NewRequest("GET", "/api/system/favicon", nil)
	reqCache.Header.Set("If-None-Match", etag)
	wCache := httptest.NewRecorder()
	router.ServeHTTP(wCache, reqCache)

	if wCache.Code != http.StatusNotModified {
		t.Errorf("expected 304 Not Modified, got %d", wCache.Code)
	}

	// 3. Test GET /favicon.ico direct route
	reqIco := httptest.NewRequest("GET", "/favicon.ico", nil)
	wIco := httptest.NewRecorder()
	router.ServeHTTP(wIco, reqIco)

	if wIco.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for /favicon.ico, got %d", wIco.Code)
	}
	if wIco.Header().Get("Content-Type") != "image/svg+xml" {
		t.Errorf("expected SVG content type for /favicon.ico fallback, got %s", wIco.Header().Get("Content-Type"))
	}

	// 4. Verify GetSystemConfig metadata
	cfgReq := httptest.NewRequest("GET", "/api/system/config", nil)
	cfgW := httptest.NewRecorder()
	router.ServeHTTP(cfgW, cfgReq)

	var res map[string]any
	_ = json.NewDecoder(cfgW.Body).Decode(&res)
	if res["has_custom_favicon"] != false {
		t.Errorf("expected has_custom_favicon to be false by default, got %v", res["has_custom_favicon"])
	}
	if !strings.HasPrefix(res["favicon_url"].(string), "/api/system/favicon?v=") {
		t.Errorf("expected favicon_url with version hash, got %s", res["favicon_url"])
	}
}

func TestGetSystemFavicon_InheritCustomLogo(t *testing.T) {
	router, s, _, jwtMgr, cleanup := setupFaviconTestServer(t, false)
	defer cleanup()

	adminToken := getFaviconAdminToken(t, s, jwtMgr)

	// Upload custom logo
	customLogoSVG := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100"><circle cx="50" cy="50" r="40" fill="#10b981"/></svg>`
	logoReqBody, _ := json.Marshal(map[string]string{
		"data": customLogoSVG,
	})
	logoReq := httptest.NewRequest("POST", "/api/admin/settings/logo", bytes.NewReader(logoReqBody))
	logoReq.Header.Set("Content-Type", "application/json")
	logoReq.Header.Set("Authorization", "Bearer "+adminToken)
	logoW := httptest.NewRecorder()
	router.ServeHTTP(logoW, logoReq)

	if logoW.Code != http.StatusOK {
		t.Fatalf("expected logo upload 200 OK, got %d", logoW.Code)
	}

	// GET /api/system/favicon must now inherit this custom logo!
	favReq := httptest.NewRequest("GET", "/api/system/favicon", nil)
	favW := httptest.NewRecorder()
	router.ServeHTTP(favW, favReq)

	if favW.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", favW.Code)
	}
	if favW.Body.String() != customLogoSVG {
		t.Errorf("expected favicon to inherit custom logo, got %s", favW.Body.String())
	}
}

func TestUploadFavicon_Multipart(t *testing.T) {
	router, s, _, jwtMgr, cleanup := setupFaviconTestServer(t, false)
	defer cleanup()

	adminToken := getFaviconAdminToken(t, s, jwtMgr)

	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("favicon", "favicon.svg")
	if err != nil {
		t.Fatalf("failed to create form file: %v", err)
	}
	customFavicon := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32"><rect width="32" height="32" rx="8" fill="#3b82f6"/></svg>`
	_, _ = part.Write([]byte(customFavicon))
	_ = writer.Close()

	req := httptest.NewRequest("POST", "/api/admin/settings/favicon", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	var res map[string]any
	_ = json.NewDecoder(w.Body).Decode(&res)
	if res["success"] != true {
		t.Errorf("expected success=true, got %v", res["success"])
	}

	// Verify favicon is now custom
	favReq := httptest.NewRequest("GET", "/api/system/favicon", nil)
	favW := httptest.NewRecorder()
	router.ServeHTTP(favW, favReq)

	if favW.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", favW.Code)
	}
	if favW.Body.String() != customFavicon {
		t.Errorf("expected favicon to match custom SVG, got %s", favW.Body.String())
	}

	// Verify SystemConfig has has_custom_favicon: true
	cfgReq := httptest.NewRequest("GET", "/api/system/config", nil)
	cfgW := httptest.NewRecorder()
	router.ServeHTTP(cfgW, cfgReq)

	var cfgRes map[string]any
	_ = json.NewDecoder(cfgW.Body).Decode(&cfgRes)
	if cfgRes["has_custom_favicon"] != true {
		t.Errorf("expected has_custom_favicon to be true, got %v", cfgRes["has_custom_favicon"])
	}
}

func TestUploadFavicon_Base64JSON_And_ICO(t *testing.T) {
	router, s, _, jwtMgr, cleanup := setupFaviconTestServer(t, false)
	defer cleanup()

	adminToken := getFaviconAdminToken(t, s, jwtMgr)

	// Upload Windows ICO magic number (0x00000100)
	icoData := []byte{0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x10, 0x10}
	encodedIco := base64.StdEncoding.EncodeToString(icoData)

	reqPayload, _ := json.Marshal(map[string]string{
		"data":      encodedIco,
		"mime_type": "image/x-icon",
	})
	req := httptest.NewRequest("POST", "/api/admin/settings/favicon", bytes.NewReader(reqPayload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for ICO upload, got %d: %s", w.Code, w.Body.String())
	}

	favReq := httptest.NewRequest("GET", "/api/system/favicon", nil)
	favW := httptest.NewRecorder()
	router.ServeHTTP(favW, favReq)

	if favW.Header().Get("Content-Type") != "image/x-icon" {
		t.Errorf("expected Content-Type image/x-icon, got %s", favW.Header().Get("Content-Type"))
	}
	if !bytes.Equal(favW.Body.Bytes(), icoData) {
		t.Errorf("expected binary ICO bytes to match")
	}
}

func TestUploadFavicon_SecurityValidation(t *testing.T) {
	router, s, _, jwtMgr, cleanup := setupFaviconTestServer(t, false)
	defer cleanup()

	adminToken := getFaviconAdminToken(t, s, jwtMgr)

	// 1. Script tag injection in SVG
	maliciousSVG := `<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`
	body, _ := json.Marshal(map[string]string{
		"data": maliciousSVG,
	})
	req := httptest.NewRequest("POST", "/api/admin/settings/favicon", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for malicious SVG, got %d", w.Code)
	}

	// 2. Event handler injection
	onloadSVG := `<svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"></svg>`
	body2, _ := json.Marshal(map[string]string{
		"data": onloadSVG,
	})
	req2 := httptest.NewRequest("POST", "/api/admin/settings/favicon", bytes.NewReader(body2))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", "Bearer "+adminToken)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)

	if w2.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for onload SVG, got %d", w2.Code)
	}

	// 3. Size limit (> 512 KB)
	hugeData := make([]byte, 600*1024)
	hugeEncoded := base64.StdEncoding.EncodeToString(hugeData)
	body3, _ := json.Marshal(map[string]string{
		"data": hugeEncoded,
	})
	req3 := httptest.NewRequest("POST", "/api/admin/settings/favicon", bytes.NewReader(body3))
	req3.Header.Set("Content-Type", "application/json")
	req3.Header.Set("Authorization", "Bearer "+adminToken)
	w3 := httptest.NewRecorder()
	router.ServeHTTP(w3, req3)

	if w3.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for oversized favicon, got %d", w3.Code)
	}
}

func TestFavicon_Permissions(t *testing.T) {
	router, s, _, jwtMgr, cleanup := setupFaviconTestServer(t, false)
	defer cleanup()

	// 1. Anonymous request -> 401
	reqAnon := httptest.NewRequest("POST", "/api/admin/settings/favicon", strings.NewReader(`{"data":"<svg></svg>"}`))
	reqAnon.Header.Set("Content-Type", "application/json")
	wAnon := httptest.NewRecorder()
	router.ServeHTTP(wAnon, reqAnon)

	if wAnon.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized for anonymous upload, got %d", wAnon.Code)
	}

	// 2. Regular user (non-admin) -> 403
	userToken := getFaviconUserToken(t, s, jwtMgr)
	reqUser := httptest.NewRequest("POST", "/api/admin/settings/favicon", strings.NewReader(`{"data":"<svg></svg>"}`))
	reqUser.Header.Set("Content-Type", "application/json")
	reqUser.Header.Set("Authorization", "Bearer "+userToken)
	wUser := httptest.NewRecorder()
	router.ServeHTTP(wUser, reqUser)

	if wUser.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for non-admin, got %d", wUser.Code)
	}
}

func TestResetFavicon(t *testing.T) {
	router, s, _, jwtMgr, cleanup := setupFaviconTestServer(t, false)
	defer cleanup()

	adminToken := getFaviconAdminToken(t, s, jwtMgr)

	// 1. Upload custom favicon
	customFavicon := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32"><circle cx="16" cy="16" r="16" fill="#f59e0b"/></svg>`
	body, _ := json.Marshal(map[string]string{
		"data": customFavicon,
	})
	req := httptest.NewRequest("POST", "/api/admin/settings/favicon", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}

	// 2. Reset favicon
	delReq := httptest.NewRequest("DELETE", "/api/admin/settings/favicon", nil)
	delReq.Header.Set("Authorization", "Bearer "+adminToken)
	delW := httptest.NewRecorder()
	router.ServeHTTP(delW, delReq)

	if delW.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for reset, got %d", delW.Code)
	}

	// 3. Verify it falls back to corporate logo
	favReq := httptest.NewRequest("GET", "/api/system/favicon", nil)
	favW := httptest.NewRecorder()
	router.ServeHTTP(favW, favReq)

	if favW.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", favW.Code)
	}
	if favW.Body.String() == customFavicon {
		t.Errorf("expected favicon to NOT match custom SVG after reset")
	}

	// 4. Verify has_custom_favicon is false
	cfgReq := httptest.NewRequest("GET", "/api/system/config", nil)
	cfgW := httptest.NewRecorder()
	router.ServeHTTP(cfgW, cfgReq)

	var cfgRes map[string]any
	_ = json.NewDecoder(cfgW.Body).Decode(&cfgRes)
	if cfgRes["has_custom_favicon"] != false {
		t.Errorf("expected has_custom_favicon to be false after reset, got %v", cfgRes["has_custom_favicon"])
	}
}

func TestFavicon_ConfigMapSync(t *testing.T) {
	router, s, fakeK8s, jwtMgr, cleanup := setupFaviconTestServer(t, true)
	defer cleanup()

	adminToken := getFaviconAdminToken(t, s, jwtMgr)

	// 1. Upload custom favicon and verify ConfigMap is created
	customFavicon := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32"><rect width="32" height="32" fill="#ec4899"/></svg>`
	body, _ := json.Marshal(map[string]string{
		"data": customFavicon,
	})
	req := httptest.NewRequest("POST", "/api/admin/settings/favicon", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}

	cm, err := fakeK8s.CoreV1().ConfigMaps("kncc").Get(context.Background(), "kncc-favicon", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("expected ConfigMap kncc-favicon to exist: %v", err)
	}
	if cm.Data["favicon.data"] != customFavicon {
		t.Errorf("expected ConfigMap favicon.data to match uploaded SVG, got %s", cm.Data["favicon.data"])
	}
	if cm.Data["favicon.mime_type"] != "image/svg+xml" {
		t.Errorf("expected ConfigMap favicon.mime_type to be image/svg+xml, got %s", cm.Data["favicon.mime_type"])
	}

	// 2. Pre-set ConfigMap directly (e.g. from Helm chart) and check priority
	helmFavicon := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32"><circle cx="16" cy="16" r="16" fill="#14b8a6"/></svg>`
	cm.Data["favicon.data"] = helmFavicon
	cm.Data["favicon.mime_type"] = "image/svg+xml"
	_, err = fakeK8s.CoreV1().ConfigMaps("kncc").Update(context.Background(), cm, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("failed updating ConfigMap: %v", err)
	}

	favReq := httptest.NewRequest("GET", "/api/system/favicon", nil)
	favW := httptest.NewRecorder()
	router.ServeHTTP(favW, favReq)

	if favW.Body.String() != helmFavicon {
		t.Errorf("expected ConfigMap value to take priority, got %s", favW.Body.String())
	}

	// 3. Reset favicon and check ConfigMap is cleared
	delReq := httptest.NewRequest("DELETE", "/api/admin/settings/favicon", nil)
	delReq.Header.Set("Authorization", "Bearer "+adminToken)
	delW := httptest.NewRecorder()
	router.ServeHTTP(delW, delReq)

	cmAfter, _ := fakeK8s.CoreV1().ConfigMaps("kncc").Get(context.Background(), "kncc-favicon", metav1.GetOptions{})
	if cmAfter.Data["favicon.data"] != "" {
		t.Errorf("expected ConfigMap favicon.data to be empty after reset, got %s", cmAfter.Data["favicon.data"])
	}
}
