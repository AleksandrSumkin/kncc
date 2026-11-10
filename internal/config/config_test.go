package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestConfig_Defaults(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Server.Port != 8080 {
		t.Errorf("expected port 8080, got %d", cfg.Server.Port)
	}
	if cfg.K8s.Namespace != "kncc" {
		t.Errorf("expected namespace kncc, got %s", cfg.K8s.Namespace)
	}
	if cfg.K8s.GCInterval != 2*time.Minute {
		t.Errorf("expected gc interval 2m, got %v", cfg.K8s.GCInterval)
	}
	if cfg.K8s.GCStaleThreshold != 3*time.Minute {
		t.Errorf("expected gc stale threshold 3m, got %v", cfg.K8s.GCStaleThreshold)
	}
	if cfg.K8s.LogoConfigMapName != "kncc-logo" {
		t.Errorf("expected logo configmap kncc-logo, got %s", cfg.K8s.LogoConfigMapName)
	}
	if cfg.K8s.FaviconConfigMapName != "kncc-favicon" {
		t.Errorf("expected favicon configmap kncc-favicon, got %s", cfg.K8s.FaviconConfigMapName)
	}
	if cfg.Security.RateLimitPerMinute != 30 {
		t.Errorf("expected rate limit 30, got %d", cfg.Security.RateLimitPerMinute)
	}
	if len(cfg.Security.BlacklistCIDRs) == 0 {
		t.Errorf("expected default blacklist CIDRs")
	}
	if cfg.Theme.AccentColor != "#dc2626" {
		t.Errorf("expected accent color #dc2626, got %s", cfg.Theme.AccentColor)
	}
	if cfg.Theme.DefaultTheme != "light" {
		t.Errorf("expected default theme 'light', got %s", cfg.Theme.DefaultTheme)
	}
	if len(cfg.QuickAdd.Presets) != 2 {
		t.Fatalf("expected 2 default quick add presets, got %d", len(cfg.QuickAdd.Presets))
	}
	if cfg.QuickAdd.Presets[0].Name != "K8s API" || cfg.QuickAdd.Presets[1].Name != "Google DNS" {
		t.Errorf("unexpected default quick add presets: %+v", cfg.QuickAdd.Presets)
	}
	if !cfg.Server.OpenAPI.Enabled {
		t.Errorf("expected openapi enabled by default")
	}
	if cfg.Server.OpenAPI.Public {
		t.Errorf("expected openapi not public by default")
	}
}

func TestConfig_EnvOverride(t *testing.T) {
	os.Setenv("SERVER_PORT", "9090")
	os.Setenv("K8S_MULTI_NAMESPACE_ALLOWED", "true")
	os.Setenv("K8S_NAMESPACE_LIST_ENABLED", "true")
	os.Setenv("K8S_NAMESPACE_EXCLUDE_LIST", "kube-system,custom-ns")
	os.Setenv("K8S_NAMESPACE_EXCLUDE_SELECTOR", "kncc.io/exclude=true")
	os.Setenv("K8S_GC_INTERVAL", "5m")
	os.Setenv("K8S_GC_STALE_THRESHOLD", "7m")
	os.Setenv("K8S_LOGO_CONFIGMAP_NAME", "custom-logo-cm")
	os.Setenv("K8S_FAVICON_CONFIGMAP_NAME", "custom-favicon-cm")
	os.Setenv("THEME_PRIMARY_COLOR", "#000000")
	os.Setenv("THEME_DEFAULT_THEME", "dark")
	os.Setenv("SERVER_OPENAPI_ENABLED", "false")
	os.Setenv("SERVER_OPENAPI_PUBLIC", "true")
	os.Setenv("STORAGE_REPORT_TTL", "48h")
	defer func() {
		os.Unsetenv("SERVER_OPENAPI_ENABLED")
		os.Unsetenv("SERVER_OPENAPI_PUBLIC")
		os.Unsetenv("SERVER_PORT")
		os.Unsetenv("K8S_MULTI_NAMESPACE_ALLOWED")
		os.Unsetenv("K8S_NAMESPACE_LIST_ENABLED")
		os.Unsetenv("K8S_NAMESPACE_EXCLUDE_LIST")
		os.Unsetenv("K8S_NAMESPACE_EXCLUDE_SELECTOR")
		os.Unsetenv("K8S_GC_INTERVAL")
		os.Unsetenv("K8S_GC_STALE_THRESHOLD")
		os.Unsetenv("K8S_LOGO_CONFIGMAP_NAME")
		os.Unsetenv("K8S_FAVICON_CONFIGMAP_NAME")
		os.Unsetenv("THEME_PRIMARY_COLOR")
		os.Unsetenv("THEME_DEFAULT_THEME")
		os.Unsetenv("STORAGE_REPORT_TTL")
	}()

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}

	if cfg.Server.Port != 9090 {
		t.Errorf("expected port 9090 from env, got %d", cfg.Server.Port)
	}
	if !cfg.K8s.MultiNamespaceAllowed {
		t.Errorf("expected multi-namespace allowed to be true")
	}
	if !cfg.K8s.NamespaceListEnabled {
		t.Errorf("expected namespace-list-enabled to be true")
	}
	if len(cfg.K8s.NamespaceExcludeList) != 2 || cfg.K8s.NamespaceExcludeList[1] != "custom-ns" {
		t.Errorf("expected exclude list [kube-system custom-ns], got %v", cfg.K8s.NamespaceExcludeList)
	}
	if cfg.K8s.NamespaceExcludeSelector != "kncc.io/exclude=true" {
		t.Errorf("expected selector 'kncc.io/exclude=true', got %s", cfg.K8s.NamespaceExcludeSelector)
	}
	if cfg.K8s.GCInterval != 5*time.Minute {
		t.Errorf("expected gc interval 5m, got %v", cfg.K8s.GCInterval)
	}
	if cfg.K8s.GCStaleThreshold != 7*time.Minute {
		t.Errorf("expected gc stale threshold 7m, got %v", cfg.K8s.GCStaleThreshold)
	}
	if cfg.K8s.LogoConfigMapName != "custom-logo-cm" {
		t.Errorf("expected logo configmap 'custom-logo-cm', got %s", cfg.K8s.LogoConfigMapName)
	}
	if cfg.K8s.FaviconConfigMapName != "custom-favicon-cm" {
		t.Errorf("expected favicon configmap 'custom-favicon-cm', got %s", cfg.K8s.FaviconConfigMapName)
	}
	if cfg.Theme.PrimaryColor != "#000000" {
		t.Errorf("expected primary color #000000, got %s", cfg.Theme.PrimaryColor)
	}
	if cfg.Theme.DefaultTheme != "dark" {
		t.Errorf("expected default theme 'dark', got %s", cfg.Theme.DefaultTheme)
	}
	if cfg.Server.OpenAPI.Enabled {
		t.Errorf("expected openapi enabled to be false from env")
	}
	if !cfg.Server.OpenAPI.Public {
		t.Errorf("expected openapi public to be true from env")
	}
	if cfg.Storage.ReportTTL != 48*time.Hour {
		t.Errorf("expected 48h TTL, got %v", cfg.Storage.ReportTTL)
	}
}

func TestConfig_QuickAddEnvAndCap(t *testing.T) {
	// Test passing 4 presets in env, should be capped to 3
	envJSON := `[
		{"name":"S1","protocol":"tcp","host":"1.1.1.1","port":80},
		{"name":"S2","protocol":"tcp","host":"2.2.2.2","port":443},
		{"name":"S3","protocol":"icmp","host":"3.3.3.3"},
		{"name":"S4","protocol":"http","host":"4.4.4.4","port":8080}
	]`
	os.Setenv("QUICK_ADD_PRESETS", envJSON)
	defer os.Unsetenv("QUICK_ADD_PRESETS")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}

	if len(cfg.QuickAdd.Presets) != 3 {
		t.Fatalf("expected quick add presets to be capped at 3, got %d", len(cfg.QuickAdd.Presets))
	}
	if cfg.QuickAdd.Presets[0].Name != "S1" || cfg.QuickAdd.Presets[1].Name != "S2" || cfg.QuickAdd.Presets[2].Name != "S3" {
		t.Errorf("unexpected presets order: %+v", cfg.QuickAdd.Presets)
	}
}

func TestConfig_CACertsEnv(t *testing.T) {
	os.Setenv("CA_CERTS_PATH", "/custom/certs/ca.crt")
	os.Setenv("CA_CERTS_PEM", "-----BEGIN CERTIFICATE-----\ntest\n-----END CERTIFICATE-----")
	os.Setenv("K8S_CUSTOM_CA_BUNDLE", "test-ca-bundle")
	defer func() {
		os.Unsetenv("CA_CERTS_PATH")
		os.Unsetenv("CA_CERTS_PEM")
		os.Unsetenv("K8S_CUSTOM_CA_BUNDLE")
	}()

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}

	if cfg.CACerts.Path != "/custom/certs/ca.crt" {
		t.Errorf("expected path /custom/certs/ca.crt, got %s", cfg.CACerts.Path)
	}
	if !strings.Contains(cfg.CACerts.PEM, "BEGIN CERTIFICATE") {
		t.Errorf("expected PEM content, got %s", cfg.CACerts.PEM)
	}
	if cfg.K8s.CustomCABundle != "test-ca-bundle" {
		t.Errorf("expected bundle test-ca-bundle, got %s", cfg.K8s.CustomCABundle)
	}
}

func TestConfig_AdminPasswordFile(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "kncc-pass-*")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	rawPassword := "  SuperSecretPasswordFromTmpfs\r\n\n  "
	if _, err := tmpFile.WriteString(rawPassword); err != nil {
		t.Fatalf("failed to write to temp file: %v", err)
	}
	tmpFile.Close()

	// 1. Env override with ADMIN_INITIAL_PASSWORD_FILE
	os.Setenv("ADMIN_INITIAL_PASSWORD", "fallbackEnvPassword")
	os.Setenv("ADMIN_INITIAL_PASSWORD_FILE", tmpFile.Name())
	defer func() {
		os.Unsetenv("ADMIN_INITIAL_PASSWORD")
		os.Unsetenv("ADMIN_INITIAL_PASSWORD_FILE")
	}()

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("unexpected error loading config: %v", err)
	}
	if cfg.Auth.AdminInitialPassword != "SuperSecretPasswordFromTmpfs" {
		t.Errorf("expected trimmed password 'SuperSecretPasswordFromTmpfs', got '%s'", cfg.Auth.AdminInitialPassword)
	}

	// 2. Non-existent file should return error
	os.Setenv("ADMIN_INITIAL_PASSWORD_FILE", tmpFile.Name()+"-non-existent")
	_, err = Load("")
	if err == nil {
		t.Errorf("expected error for non-existent admin password file, got nil")
	}
}

func TestConfig_LDAPGroupMapping(t *testing.T) {
	// Test environment variables for LDAP group mapping
	os.Setenv("AUTH_LDAP_ENABLED", "true")
	os.Setenv("LDAP_ADMIN_GROUP", "RBAC_KNCC_ADMIN")
	os.Setenv("LDAP_GROUP_FILTER", "(cn=RBAC_KNCC_*)")
	os.Setenv("LDAP_GROUP_BASE_DN", "ou=groups,dc=example,dc=com")
	os.Setenv("LDAP_GROUP_MEMBER_ATTRIBUTE", "member")
	os.Setenv("LDAP_GROUP_NAME_ATTRIBUTE", "cn")
	defer func() {
		os.Unsetenv("AUTH_LDAP_ENABLED")
		os.Unsetenv("LDAP_ADMIN_GROUP")
		os.Unsetenv("LDAP_GROUP_FILTER")
		os.Unsetenv("LDAP_GROUP_BASE_DN")
		os.Unsetenv("LDAP_GROUP_MEMBER_ATTRIBUTE")
		os.Unsetenv("LDAP_GROUP_NAME_ATTRIBUTE")
	}()

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("unexpected error loading config: %v", err)
	}

	if !cfg.Auth.LDAPEnabled {
		t.Errorf("expected LDAPEnabled to be true")
	}
	if cfg.Auth.LDAP.AdminGroup != "RBAC_KNCC_ADMIN" {
		t.Errorf("expected AdminGroup 'RBAC_KNCC_ADMIN', got '%s'", cfg.Auth.LDAP.AdminGroup)
	}
	if cfg.Auth.LDAP.GroupFilter != "(cn=RBAC_KNCC_*)" {
		t.Errorf("expected GroupFilter '(cn=RBAC_KNCC_*)', got '%s'", cfg.Auth.LDAP.GroupFilter)
	}
	if cfg.Auth.LDAP.GroupBaseDN != "ou=groups,dc=example,dc=com" {
		t.Errorf("expected GroupBaseDN 'ou=groups,dc=example,dc=com', got '%s'", cfg.Auth.LDAP.GroupBaseDN)
	}
	if cfg.Auth.LDAP.GroupMemberAttribute != "member" {
		t.Errorf("expected GroupMemberAttribute 'member', got '%s'", cfg.Auth.LDAP.GroupMemberAttribute)
	}
	if cfg.Auth.LDAP.GroupNameAttribute != "cn" {
		t.Errorf("expected GroupNameAttribute 'cn', got '%s'", cfg.Auth.LDAP.GroupNameAttribute)
	}
}

func TestConfig_ServerTLS(t *testing.T) {
	// 1. Defaults
	cfg := DefaultConfig()
	if cfg.Server.TLS.Enabled {
		t.Errorf("expected TLS to be disabled by default")
	}
	if cfg.Server.TLS.Port != 8443 {
		t.Errorf("expected default TLS port 8443, got %d", cfg.Server.TLS.Port)
	}
	if !cfg.Server.TLS.AutoGenerate {
		t.Errorf("expected default AutoGenerate to be true")
	}
	if cfg.Server.TLS.MinVersion != "1.2" {
		t.Errorf("expected default MinVersion 1.2, got %s", cfg.Server.TLS.MinVersion)
	}

	// 2. Env Overrides
	os.Setenv("SERVER_TLS_ENABLED", "true")
	os.Setenv("SERVER_TLS_PORT", "9443")
	os.Setenv("SERVER_TLS_CERT_FILE", "/custom/tls.crt")
	os.Setenv("SERVER_TLS_KEY_FILE", "/custom/tls.key")
	os.Setenv("SERVER_TLS_CERT_PEM", "-----BEGIN CERTIFICATE-----\nxyz\n-----END CERTIFICATE-----")
	os.Setenv("SERVER_TLS_KEY_PEM", "-----BEGIN EC PRIVATE KEY-----\nabc\n-----END EC PRIVATE KEY-----")
	os.Setenv("SERVER_TLS_AUTO_GENERATE", "false")
	os.Setenv("SERVER_TLS_MIN_VERSION", "1.3")
	defer func() {
		os.Unsetenv("SERVER_TLS_ENABLED")
		os.Unsetenv("SERVER_TLS_PORT")
		os.Unsetenv("SERVER_TLS_CERT_FILE")
		os.Unsetenv("SERVER_TLS_KEY_FILE")
		os.Unsetenv("SERVER_TLS_CERT_PEM")
		os.Unsetenv("SERVER_TLS_KEY_PEM")
		os.Unsetenv("SERVER_TLS_AUTO_GENERATE")
		os.Unsetenv("SERVER_TLS_MIN_VERSION")
	}()

	loaded, err := Load("")
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}

	if !loaded.Server.TLS.Enabled {
		t.Errorf("expected TLS to be enabled")
	}
	if loaded.Server.TLS.Port != 9443 {
		t.Errorf("expected TLS port 9443, got %d", loaded.Server.TLS.Port)
	}
	if loaded.Server.TLS.CertFile != "/custom/tls.crt" {
		t.Errorf("expected CertFile /custom/tls.crt, got %s", loaded.Server.TLS.CertFile)
	}
	if loaded.Server.TLS.KeyFile != "/custom/tls.key" {
		t.Errorf("expected KeyFile /custom/tls.key, got %s", loaded.Server.TLS.KeyFile)
	}
	if !strings.Contains(loaded.Server.TLS.CertPEM, "BEGIN CERTIFICATE") {
		t.Errorf("expected CertPEM to be loaded")
	}
	if !strings.Contains(loaded.Server.TLS.KeyPEM, "BEGIN EC PRIVATE KEY") {
		t.Errorf("expected KeyPEM to be loaded")
	}
	if loaded.Server.TLS.AutoGenerate {
		t.Errorf("expected AutoGenerate to be false")
	}
	if loaded.Server.TLS.MinVersion != "1.3" {
		t.Errorf("expected MinVersion 1.3, got %s", loaded.Server.TLS.MinVersion)
	}
}

func TestLoad_ProbeImage_EnvVars(t *testing.T) {
	os.Setenv("K8S_PROBE_IMAGE", "homelab/kncc-agent:v1.0.0")
	defer os.Unsetenv("K8S_PROBE_IMAGE")

	loaded, err := Load("")
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if loaded.K8s.ProbeImage != "homelab/kncc-agent:v1.0.0" {
		t.Errorf("expected ProbeImage homelab/kncc-agent:v1.0.0, got %s", loaded.K8s.ProbeImage)
	}

	os.Unsetenv("K8S_PROBE_IMAGE")
	os.Setenv("KNCC_K8S_PROBE_IMAGE", "ghcr.io/homelab-public/kncc-agent:latest")
	defer os.Unsetenv("KNCC_K8S_PROBE_IMAGE")

	loaded2, err := Load("")
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if loaded2.K8s.ProbeImage != "ghcr.io/homelab-public/kncc-agent:latest" {
		t.Errorf("expected ProbeImage ghcr.io/homelab-public/kncc-agent:latest, got %s", loaded2.K8s.ProbeImage)
	}
}

func TestConfig_OIDC_EnvAndFiles(t *testing.T) {
	// 1. Env variables override
	os.Setenv("AUTH_OIDC_ENABLED", "true")
	os.Setenv("OIDC_ISSUER_URL", "http://dex.example.com/dex")
	os.Setenv("OIDC_CLIENT_ID", "env-client-id")
	os.Setenv("OIDC_CLIENT_SECRET", "env-client-secret")
	os.Setenv("OIDC_REDIRECT_URL", "http://kncc.example.com/callback")
	defer func() {
		os.Unsetenv("AUTH_OIDC_ENABLED")
		os.Unsetenv("OIDC_ISSUER_URL")
		os.Unsetenv("OIDC_CLIENT_ID")
		os.Unsetenv("OIDC_CLIENT_SECRET")
		os.Unsetenv("OIDC_REDIRECT_URL")
	}()

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if !cfg.Auth.OIDCEnabled {
		t.Errorf("expected OIDCEnabled=true")
	}
	if cfg.Auth.OIDC.IssuerURL != "http://dex.example.com/dex" {
		t.Errorf("expected IssuerURL http://dex.example.com/dex, got %s", cfg.Auth.OIDC.IssuerURL)
	}
	if cfg.Auth.OIDC.ClientID != "env-client-id" {
		t.Errorf("expected ClientID env-client-id, got %s", cfg.Auth.OIDC.ClientID)
	}
	if cfg.Auth.OIDC.ClientSecret != "env-client-secret" {
		t.Errorf("expected ClientSecret env-client-secret, got %s", cfg.Auth.OIDC.ClientSecret)
	}
	if cfg.Auth.OIDC.RedirectURL != "http://kncc.example.com/callback" {
		t.Errorf("expected RedirectURL http://kncc.example.com/callback, got %s", cfg.Auth.OIDC.RedirectURL)
	}

	// 2. File paths override
	tmpIDFile, err := os.CreateTemp("", "kncc-oidc-id-*")
	if err != nil {
		t.Fatalf("temp file error: %v", err)
	}
	defer os.Remove(tmpIDFile.Name())
	tmpIDFile.WriteString("file-client-id\n")
	tmpIDFile.Close()

	tmpSecFile, err := os.CreateTemp("", "kncc-oidc-sec-*")
	if err != nil {
		t.Fatalf("temp file error: %v", err)
	}
	defer os.Remove(tmpSecFile.Name())
	tmpSecFile.WriteString("file-client-secret\n")
	tmpSecFile.Close()

	os.Setenv("OIDC_CLIENT_ID_FILE", tmpIDFile.Name())
	os.Setenv("OIDC_CLIENT_SECRET_FILE", tmpSecFile.Name())
	defer func() {
		os.Unsetenv("OIDC_CLIENT_ID_FILE")
		os.Unsetenv("OIDC_CLIENT_SECRET_FILE")
	}()

	cfgFile, err := Load("")
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if cfgFile.Auth.OIDC.ClientID != "file-client-id" {
		t.Errorf("expected ClientID file-client-id, got %s", cfgFile.Auth.OIDC.ClientID)
	}
	if cfgFile.Auth.OIDC.ClientSecret != "file-client-secret" {
		t.Errorf("expected ClientSecret file-client-secret, got %s", cfgFile.Auth.OIDC.ClientSecret)
	}
}

func TestConfig_LDAP_AdditionalMethods(t *testing.T) {
	tmpDir := t.TempDir()
	dnFile := filepath.Join(tmpDir, "bind-dn.txt")
	passFile := filepath.Join(tmpDir, "bind-pass.txt")

	if err := os.WriteFile(dnFile, []byte("cn=file-svc,dc=corp,dc=local\n"), 0600); err != nil {
		t.Fatalf("WriteFile dnFile failed: %v", err)
	}
	if err := os.WriteFile(passFile, []byte("FileSecretPassword123!\n"), 0600); err != nil {
		t.Fatalf("WriteFile passFile failed: %v", err)
	}

	os.Setenv("AUTH_LDAP_ENABLED", "true")
	os.Setenv("LDAP_BIND_DN_FILE", dnFile)
	os.Setenv("LDAP_BIND_PASSWORD_FILE", passFile)
	os.Setenv("LDAP_USER_DN_TEMPLATE", "uid=%s,ou=people,dc=corp,dc=local")
	os.Setenv("LDAP_ANONYMOUS_SEARCH", "true")
	defer func() {
		os.Unsetenv("AUTH_LDAP_ENABLED")
		os.Unsetenv("LDAP_BIND_DN_FILE")
		os.Unsetenv("LDAP_BIND_PASSWORD_FILE")
		os.Unsetenv("LDAP_USER_DN_TEMPLATE")
		os.Unsetenv("LDAP_ANONYMOUS_SEARCH")
	}()

	loaded, err := Load("")
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}

	if !loaded.Auth.LDAPEnabled {
		t.Errorf("expected LDAPEnabled to be true")
	}
	if loaded.Auth.LDAP.BindDN != "cn=file-svc,dc=corp,dc=local" {
		t.Errorf("expected BindDN from file, got %q", loaded.Auth.LDAP.BindDN)
	}
	if loaded.Auth.LDAP.BindPassword != "FileSecretPassword123!" {
		t.Errorf("expected BindPassword from file, got %q", loaded.Auth.LDAP.BindPassword)
	}
	if loaded.Auth.LDAP.UserDNTemplate != "uid=%s,ou=people,dc=corp,dc=local" {
		t.Errorf("expected UserDNTemplate, got %q", loaded.Auth.LDAP.UserDNTemplate)
	}
	if !loaded.Auth.LDAP.AnonymousSearch {
		t.Errorf("expected AnonymousSearch to be true")
	}
}

func TestDefaultConfig_AdminInitialPasswordEmpty(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Auth.AdminInitialPassword != "" {
		t.Errorf("expected default AdminInitialPassword to be empty for auto-generation, got '%s'", cfg.Auth.AdminInitialPassword)
	}
}
