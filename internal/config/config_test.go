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
	if cfg.Server.LogLevel != "info" || cfg.LogLevel != "info" {
		t.Errorf("expected default log level 'info', got server=%q root=%q", cfg.Server.LogLevel, cfg.LogLevel)
	}
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
	if cfg.K8s.PrintAccessList {
		t.Errorf("expected PrintAccessList to be false by default")
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
	if cfg.Theme.DefaultLanguage != "en" {
		t.Errorf("expected default language 'en', got %s", cfg.Theme.DefaultLanguage)
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
	os.Setenv("LOG_LEVEL", "debug")
	os.Setenv("SERVER_PORT", "9090")
	os.Setenv("K8S_MULTI_NAMESPACE_ALLOWED", "true")
	os.Setenv("K8S_NAMESPACE_LIST_ENABLED", "true")
	os.Setenv("K8S_NAMESPACE_EXCLUDE_LIST", "kube-system,custom-ns")
	os.Setenv("K8S_NAMESPACE_EXCLUDE_SELECTOR", "kncc.io/exclude=true")
	os.Setenv("K8S_NODE_EXCLUDE_LIST", "k3s-master,node-audit-01")
	os.Setenv("K8S_NODE_EXCLUDE_SELECTOR", "security.zone=audit")
	os.Setenv("K8S_NODE_SELECTOR", "node-role.kubernetes.io/worker=true")
	os.Setenv("K8S_GC_INTERVAL", "5m")
	os.Setenv("K8S_GC_STALE_THRESHOLD", "7m")
	os.Setenv("K8S_LOGO_CONFIGMAP_NAME", "custom-logo-cm")
	os.Setenv("K8S_FAVICON_CONFIGMAP_NAME", "custom-favicon-cm")
	os.Setenv("THEME_PRIMARY_COLOR", "#000000")
	os.Setenv("THEME_DEFAULT_THEME", "dark")
	os.Setenv("SERVER_OPENAPI_ENABLED", "false")
	os.Setenv("SERVER_OPENAPI_PUBLIC", "true")
	os.Setenv("STORAGE_REPORT_TTL", "48h")
	os.Setenv("K8S_PRINT_ACCESS_LIST", "true")
	defer func() {
		os.Unsetenv("K8S_PRINT_ACCESS_LIST")
		os.Unsetenv("LOG_LEVEL")
		os.Unsetenv("SERVER_OPENAPI_ENABLED")
		os.Unsetenv("SERVER_OPENAPI_PUBLIC")
		os.Unsetenv("SERVER_PORT")
		os.Unsetenv("K8S_MULTI_NAMESPACE_ALLOWED")
		os.Unsetenv("K8S_NAMESPACE_LIST_ENABLED")
		os.Unsetenv("K8S_NAMESPACE_EXCLUDE_LIST")
		os.Unsetenv("K8S_NAMESPACE_EXCLUDE_SELECTOR")
		os.Unsetenv("K8S_NODE_EXCLUDE_LIST")
		os.Unsetenv("K8S_NODE_EXCLUDE_SELECTOR")
		os.Unsetenv("K8S_NODE_SELECTOR")
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

	if cfg.Server.LogLevel != "debug" || cfg.LogLevel != "debug" {
		t.Errorf("expected log level debug from env, got server=%q root=%q", cfg.Server.LogLevel, cfg.LogLevel)
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("expected port 9090 from env, got %d", cfg.Server.Port)
	}
	if !cfg.K8s.MultiNamespaceAllowed {
		t.Errorf("expected multi-namespace allowed to be true")
	}
	if !cfg.K8s.PrintAccessList {
		t.Errorf("expected PrintAccessList to be true from env")
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
	if len(cfg.K8s.NodeExcludeList) != 2 || cfg.K8s.NodeExcludeList[0] != "k3s-master" || cfg.K8s.NodeExcludeList[1] != "node-audit-01" {
		t.Errorf("expected node exclude list [k3s-master node-audit-01], got %v", cfg.K8s.NodeExcludeList)
	}
	if cfg.K8s.NodeExcludeSelector != "security.zone=audit" {
		t.Errorf("expected node exclude selector 'security.zone=audit', got %s", cfg.K8s.NodeExcludeSelector)
	}
	if cfg.K8s.NodeSelector != "node-role.kubernetes.io/worker=true" {
		t.Errorf("expected node selector 'node-role.kubernetes.io/worker=true', got %s", cfg.K8s.NodeSelector)
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
	os.Setenv("KNCC_K8S_PROBE_IMAGE", "ghcr.io/AleksandrSumkin/kncc-agent:latest")
	defer os.Unsetenv("KNCC_K8S_PROBE_IMAGE")

	loaded2, err := Load("")
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if loaded2.K8s.ProbeImage != "ghcr.io/AleksandrSumkin/kncc-agent:latest" {
		t.Errorf("expected ProbeImage ghcr.io/AleksandrSumkin/kncc-agent:latest, got %s", loaded2.K8s.ProbeImage)
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

func TestConfig_DefaultLanguage(t *testing.T) {
	// 1. Test from YAML
	tmpDir := t.TempDir()
	cfgFile := filepath.Join(tmpDir, "config.yaml")
	content := `
theme:
  default_language: ru
`
	if err := os.WriteFile(cfgFile, []byte(content), 0644); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	cfg, err := Load(cfgFile)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if cfg.Theme.DefaultLanguage != "ru" {
		t.Errorf("expected default language 'ru', got %s", cfg.Theme.DefaultLanguage)
	}

	// 2. Test invalid language fallback to "en"
	contentInvalid := `
theme:
  default_language: fr
`
	if err := os.WriteFile(cfgFile, []byte(contentInvalid), 0644); err != nil {
		t.Fatalf("writing config: %v", err)
	}
	cfgInv, err := Load(cfgFile)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if cfgInv.Theme.DefaultLanguage != "en" {
		t.Errorf("expected fallback default language 'en', got %s", cfgInv.Theme.DefaultLanguage)
	}

	// 3. Test ENV variable override
	os.Setenv("THEME_DEFAULT_LANGUAGE", "RU")
	defer os.Unsetenv("THEME_DEFAULT_LANGUAGE")
	cfgEnv, err := Load("")
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if cfgEnv.Theme.DefaultLanguage != "ru" {
		t.Errorf("expected env default language 'ru', got %s", cfgEnv.Theme.DefaultLanguage)
	}
}

func TestLoad_ProbeAllowPrivilegeEscalation(t *testing.T) {
	// 1. Default value should be true
	defCfg := DefaultConfig()
	if defCfg.K8s.ProbeAllowPrivilegeEscalation == nil || !*defCfg.K8s.ProbeAllowPrivilegeEscalation {
		t.Errorf("expected default ProbeAllowPrivilegeEscalation to be true")
	}

	// 2. YAML override to false
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "config.yaml")
	content := `
k8s:
  probe_allow_privilege_escalation: false
`
	if err := os.WriteFile(cfgFile, []byte(content), 0644); err != nil {
		t.Fatalf("writing config: %v", err)
	}
	loaded, err := Load(cfgFile)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if loaded.K8s.ProbeAllowPrivilegeEscalation == nil || *loaded.K8s.ProbeAllowPrivilegeEscalation != false {
		t.Errorf("expected ProbeAllowPrivilegeEscalation to be false from YAML")
	}

	// 3. ENV override
	os.Setenv("K8S_PROBE_ALLOW_PRIVILEGE_ESCALATION", "false")
	defer os.Unsetenv("K8S_PROBE_ALLOW_PRIVILEGE_ESCALATION")

	loadedEnv, err := Load("")
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if loadedEnv.K8s.ProbeAllowPrivilegeEscalation == nil || *loadedEnv.K8s.ProbeAllowPrivilegeEscalation != false {
		t.Errorf("expected ProbeAllowPrivilegeEscalation to be false from env")
	}

	os.Unsetenv("K8S_PROBE_ALLOW_PRIVILEGE_ESCALATION")
	os.Setenv("KNCC_K8S_PROBE_ALLOW_PRIVILEGE_ESCALATION", "true")
	defer os.Unsetenv("KNCC_K8S_PROBE_ALLOW_PRIVILEGE_ESCALATION")

	loadedEnv2, err := Load(cfgFile)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if loadedEnv2.K8s.ProbeAllowPrivilegeEscalation == nil || *loadedEnv2.K8s.ProbeAllowPrivilegeEscalation != true {
		t.Errorf("expected ProbeAllowPrivilegeEscalation to be true from KNCC_ env overriding YAML")
	}
}

func TestLoad_ProbeCustomAgentConfig(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "custom_probe.yaml")
	content := `
k8s:
  probe_image_pull_policy: "Always"
  probe_image_pull_secrets:
    - "my-regcred"
    - "corp-secret"
  probe_extra_labels:
    tier: "diagnostics"
    team: "netops"
  probe_extra_annotations:
    sidecar.istio.io/inject: "false"
  probe_priority_class_name: "system-cluster-critical"
  probe_resources:
    requests:
      cpu: "25m"
      memory: "64Mi"
    limits:
      cpu: "200m"
      memory: "256Mi"
  probe_pod_security_context:
    run_as_user: 10001
    run_as_non_root: true
    fs_group: 10001
  probe_security_context:
    read_only_root_filesystem: true
    allow_privilege_escalation: false
    capabilities:
      add:
        - "NET_RAW"
        - "NET_ADMIN"
      drop:
        - "ALL"
`
	if err := os.WriteFile(cfgFile, []byte(content), 0644); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	loaded, err := Load(cfgFile)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}

	if loaded.K8s.ProbeImagePullPolicy != "Always" {
		t.Errorf("expected ProbeImagePullPolicy Always, got %s", loaded.K8s.ProbeImagePullPolicy)
	}
	if len(loaded.K8s.ProbeImagePullSecrets) != 2 || loaded.K8s.ProbeImagePullSecrets[0] != "my-regcred" {
		t.Errorf("unexpected ProbeImagePullSecrets: %v", loaded.K8s.ProbeImagePullSecrets)
	}
	if loaded.K8s.ProbeExtraLabels["tier"] != "diagnostics" {
		t.Errorf("expected extra label tier=diagnostics, got %v", loaded.K8s.ProbeExtraLabels)
	}
	if loaded.K8s.ProbeExtraAnnotations["sidecar.istio.io/inject"] != "false" {
		t.Errorf("expected extra annotation, got %v", loaded.K8s.ProbeExtraAnnotations)
	}
	if loaded.K8s.ProbePriorityClassName != "system-cluster-critical" {
		t.Errorf("expected priority class system-cluster-critical, got %s", loaded.K8s.ProbePriorityClassName)
	}
	if loaded.K8s.ProbeResources == nil || loaded.K8s.ProbeResources.Requests["cpu"] != "25m" {
		t.Errorf("expected custom requests.cpu 25m, got %v", loaded.K8s.ProbeResources)
	}
	if loaded.K8s.ProbePodSecurityContext == nil || *loaded.K8s.ProbePodSecurityContext.RunAsUser != 10001 {
		t.Errorf("expected custom run_as_user 10001, got %v", loaded.K8s.ProbePodSecurityContext)
	}
	if loaded.K8s.ProbeSecurityContext == nil || *loaded.K8s.ProbeSecurityContext.ReadOnlyRootFilesystem != true {
		t.Errorf("expected custom read_only_root_filesystem true, got %v", loaded.K8s.ProbeSecurityContext)
	}

	// Test Env Overrides
	os.Setenv("K8S_PROBE_IMAGE_PULL_POLICY", "Never")
	os.Setenv("K8S_PROBE_IMAGE_PULL_SECRETS", "env-secret-1, env-secret-2")
	os.Setenv("K8S_PROBE_PRIORITY_CLASS_NAME", "low-priority")
	defer func() {
		os.Unsetenv("K8S_PROBE_IMAGE_PULL_POLICY")
		os.Unsetenv("K8S_PROBE_IMAGE_PULL_SECRETS")
		os.Unsetenv("K8S_PROBE_PRIORITY_CLASS_NAME")
	}()

	loadedEnv, err := Load(cfgFile)
	if err != nil {
		t.Fatalf("Load with env error: %v", err)
	}
	if loadedEnv.K8s.ProbeImagePullPolicy != "Never" {
		t.Errorf("expected env override ProbeImagePullPolicy Never, got %s", loadedEnv.K8s.ProbeImagePullPolicy)
	}
	if len(loadedEnv.K8s.ProbeImagePullSecrets) != 2 || loadedEnv.K8s.ProbeImagePullSecrets[0] != "env-secret-1" {
		t.Errorf("expected env override ProbeImagePullSecrets, got %v", loadedEnv.K8s.ProbeImagePullSecrets)
	}
	if loadedEnv.K8s.ProbePriorityClassName != "low-priority" {
		t.Errorf("expected env override ProbePriorityClassName low-priority, got %s", loadedEnv.K8s.ProbePriorityClassName)
	}
}

func TestConfig_LogLevelYAML(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. Test server.log_level in YAML
	cfgPath1 := filepath.Join(tmpDir, "config1.yaml")
	if err := os.WriteFile(cfgPath1, []byte("server:\n  log_level: \"warn\"\n"), 0600); err != nil {
		t.Fatalf("failed to write config1: %v", err)
	}

	cfg1, err := Load(cfgPath1)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if cfg1.Server.LogLevel != "warn" || cfg1.LogLevel != "warn" {
		t.Errorf("expected warn, got server=%q root=%q", cfg1.Server.LogLevel, cfg1.LogLevel)
	}

	// 2. Test root log_level in YAML
	cfgPath2 := filepath.Join(tmpDir, "config2.yaml")
	if err := os.WriteFile(cfgPath2, []byte("log_level: \"debug\"\n"), 0600); err != nil {
		t.Fatalf("failed to write config2: %v", err)
	}

	cfg2, err := Load(cfgPath2)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if cfg2.Server.LogLevel != "debug" || cfg2.LogLevel != "debug" {
		t.Errorf("expected debug, got server=%q root=%q", cfg2.Server.LogLevel, cfg2.LogLevel)
	}
}
