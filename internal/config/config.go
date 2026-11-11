package config

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"kncc/internal/model"

	"gopkg.in/yaml.v3"
)

// Config holds all configuration settings for kncc.
type Config struct {
	Server   ServerConfig   `yaml:"server"`
	K8s      K8sConfig      `yaml:"k8s"`
	Security SecurityConfig `yaml:"security"`
	Storage  StorageConfig  `yaml:"storage"`
	Auth     AuthConfig     `yaml:"auth"`
	Theme    ThemeConfig    `yaml:"theme"`
	QuickAdd QuickAddConfig `yaml:"quick_add"`
	CACerts  CACertsConfig  `yaml:"ca_certs"`
}

type CACertsConfig struct {
	Path string `yaml:"path"`
	PEM  string `yaml:"pem"`
}

type ServerTLSConfig struct {
	Enabled      bool   `yaml:"enabled"`
	Port         int    `yaml:"port"`
	CertFile     string `yaml:"cert_file"`
	KeyFile      string `yaml:"key_file"`
	CertPEM      string `yaml:"cert_pem"`
	KeyPEM       string `yaml:"key_pem"`
	AutoGenerate bool   `yaml:"auto_generate"`
	MinVersion   string `yaml:"min_version"`
}

type OpenAPIConfig struct {
	Enabled bool `yaml:"enabled"`
	Public  bool `yaml:"public"`
}

type ServerConfig struct {
	Host      string          `yaml:"host"`
	Port      int             `yaml:"port"`
	JWTSecret string          `yaml:"jwt_secret"`
	TokenTTL  time.Duration   `yaml:"token_ttl"`
	TLS       ServerTLSConfig `yaml:"tls"`
	OpenAPI   OpenAPIConfig   `yaml:"openapi"`
}

type K8sConfig struct {
	Kubeconfig                 string        `yaml:"kubeconfig"`
	Namespace                  string        `yaml:"namespace"`
	ProbeImage                 string        `yaml:"probe_image"`
	MultiNamespaceAllowed      bool          `yaml:"multi_namespace_allowed"`
	NamespaceListEnabled       bool          `yaml:"namespace_list_enabled"`
	NamespaceExcludeList       []string      `yaml:"namespace_exclude_list"`
	NamespaceExcludeSelector   string        `yaml:"namespace_exclude_selector"`
	ProbeTimeoutSeconds           int           `yaml:"probe_timeout_seconds"`
	ProbeActiveDeadlineSeconds    int64         `yaml:"probe_active_deadline_seconds"`
	ProbeAllowPrivilegeEscalation *bool         `yaml:"probe_allow_privilege_escalation"`
	GCInterval                    time.Duration `yaml:"gc_interval"`
	GCStaleThreshold           time.Duration `yaml:"gc_stale_threshold"`
	LogoConfigMapName          string        `yaml:"logo_configmap_name"`
	FaviconConfigMapName       string        `yaml:"favicon_configmap_name"`
	CustomCABundle             string        `yaml:"custom_ca_bundle"`
}

type SecurityConfig struct {
	BlacklistCIDRs     []string `yaml:"blacklist_cidrs"`
	BlacklistDomains   []string `yaml:"blacklist_domains"`
	WhitelistCIDRs     []string `yaml:"whitelist_cidrs"`
	WhitelistDomains   []string `yaml:"whitelist_domains"`
	RateLimitPerMinute int      `yaml:"rate_limit_per_minute"`
}

type StorageConfig struct {
	DBPath    string        `yaml:"db_path"`
	ReportTTL time.Duration `yaml:"report_ttl"`
}

type LDAPConfig struct {
	URL                  string `yaml:"url"`
	BindDN               string `yaml:"bind_dn"`
	BindPassword         string `yaml:"bind_password"`
	BindDNFile           string `yaml:"bind_dn_file"`
	BindPasswordFile     string `yaml:"bind_password_file"`
	UserDNTemplate       string `yaml:"user_dn_template"`
	AnonymousSearch      bool   `yaml:"anonymous_search"`
	BaseDN               string `yaml:"base_dn"`
	UserFilter           string `yaml:"user_filter"`
	AdminGroup           string `yaml:"admin_group"`
	GroupFilter          string `yaml:"group_filter"`
	GroupBaseDN          string `yaml:"group_base_dn"`
	GroupMemberAttribute string `yaml:"group_member_attribute"`
	GroupNameAttribute   string `yaml:"group_name_attribute"`
	InsecureSkipVerify   bool   `yaml:"insecure_skip_verify"`
}

type OIDCConfig struct {
	IssuerURL        string `yaml:"issuer_url"`
	ClientID         string `yaml:"client_id"`
	ClientIDFile     string `yaml:"client_id_file"`
	ClientSecret     string `yaml:"client_secret"`
	ClientSecretFile string `yaml:"client_secret_file"`
	RedirectURL      string `yaml:"redirect_url"`
}

type AuthConfig struct {
	AdminUsername            string     `yaml:"admin_username"`
	AdminInitialPassword     string     `yaml:"admin_initial_password"`
	AdminInitialPasswordFile string     `yaml:"admin_initial_password_file"`
	LocalEnabled             bool       `yaml:"local_enabled"`
	LDAPEnabled              bool       `yaml:"ldap_enabled"`
	LDAP                     LDAPConfig `yaml:"ldap"`
	OIDCEnabled              bool       `yaml:"oidc_enabled"`
	OIDC                     OIDCConfig `yaml:"oidc"`
}

type ThemeConfig struct {
	PrimaryColor    string `yaml:"primary_color"`
	AccentColor     string `yaml:"accent_color"`
	AppName         string `yaml:"app_name"`
	DefaultTheme    string `yaml:"default_theme"`
	DefaultLanguage string `yaml:"default_language"`
}

type QuickAddPreset struct {
	Name           string         `yaml:"name" json:"name,omitempty"`
	Protocol       model.Protocol `yaml:"protocol" json:"protocol"`
	Host           string         `yaml:"host" json:"host"`
	Port           int            `yaml:"port,omitempty" json:"port,omitempty"`
	Path           string         `yaml:"path,omitempty" json:"path,omitempty"`
	InsecureTLS    bool           `yaml:"insecure_tls,omitempty" json:"insecure_tls,omitempty"`
	TimeoutSeconds int            `yaml:"timeout_seconds,omitempty" json:"timeout_seconds,omitempty"`
}

type QuickAddConfig struct {
	Presets []QuickAddPreset `yaml:"presets"`
}

// DefaultConfig returns safe defaults.
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Host:      "0.0.0.0",
			Port:      8080,
			JWTSecret: generateRandomSecret(32),
			TokenTTL:  24 * time.Hour,
			TLS: ServerTLSConfig{
				Enabled:      false,
				Port:         8443,
				CertFile:     "",
				KeyFile:      "",
				CertPEM:      "",
				KeyPEM:       "",
				AutoGenerate: true,
				MinVersion:   "1.2",
			},
			OpenAPI: OpenAPIConfig{
				Enabled: true,
				Public:  false,
			},
		},
		K8s: K8sConfig{
			Namespace:                  "kncc",
			ProbeImage:                 "wbitt/network-multitool:alpine-extra",
			MultiNamespaceAllowed:      false,
			NamespaceListEnabled:       false,
			NamespaceExcludeList: []string{
				"kube-system",
				"kube-public",
				"kube-node-lease",
			},
			NamespaceExcludeSelector:   "",
			ProbeTimeoutSeconds:           30,
			ProbeActiveDeadlineSeconds:    60,
			ProbeAllowPrivilegeEscalation: boolPtr(true),
			GCInterval:                    2 * time.Minute,
			GCStaleThreshold:           3 * time.Minute,
			LogoConfigMapName:          "kncc-logo",
			FaviconConfigMapName:       "kncc-favicon",
		},
		Security: SecurityConfig{
			BlacklistCIDRs: []string{
				"169.254.169.254/32", // Cloud metadata (AWS, GCP, Azure, OpenStack)
				"127.0.0.0/8",         // Loopback
				"::1/128",            // IPv6 Loopback
			},
			BlacklistDomains: []string{
				"metadata.google.internal",
				"instance-data",
			},
			RateLimitPerMinute: 30,
		},
		Storage: StorageConfig{
			DBPath:    "kncc.db",
			ReportTTL: 72 * time.Hour,
		},
		Auth: AuthConfig{
			AdminUsername:        "admin",
			AdminInitialPassword: "",
			LocalEnabled:         true,
			LDAPEnabled:          false,
			OIDCEnabled:          false,
		},
		Theme: ThemeConfig{
			PrimaryColor:    "#ffffff",
			AccentColor:     "#dc2626", // Red-600
			AppName:         "Kubernetes Network Connectivity Checker",
			DefaultTheme:    "light",
			DefaultLanguage: "en",
		},
		QuickAdd: QuickAddConfig{
			Presets: []QuickAddPreset{
				{
					Name:           "K8s API",
					Protocol:       model.ProtocolTCP,
					Host:           "10.96.0.1",
					Port:           443,
					TimeoutSeconds: 5,
				},
				{
					Name:           "Google DNS",
					Protocol:       model.ProtocolICMP,
					Host:           "8.8.8.8",
					TimeoutSeconds: 5,
				},
			},
		},
		CACerts: CACertsConfig{
			Path: "",
			PEM:  "",
		},
	}
}

// Load loads configuration from YAML file (if provided) and overrides from environment variables.
func Load(configPath string) (*Config, error) {
	cfg := DefaultConfig()

	if configPath != "" {
		data, err := os.ReadFile(configPath)
		if err != nil {
			if !os.IsNotExist(err) {
				return nil, fmt.Errorf("reading config file %s: %w", configPath, err)
			}
		} else {
			if err := yaml.Unmarshal(data, cfg); err != nil {
				return nil, fmt.Errorf("parsing config file %s: %w", configPath, err)
			}
		}
	}

	cfg.loadFromEnv()

	// If AdminInitialPasswordFile is specified (via YAML or env ADMIN_INITIAL_PASSWORD_FILE), read password from file
	if cfg.Auth.AdminInitialPasswordFile != "" {
		data, err := os.ReadFile(cfg.Auth.AdminInitialPasswordFile)
		if err != nil {
			return nil, fmt.Errorf("reading admin password file %s: %w", cfg.Auth.AdminInitialPasswordFile, err)
		}
		cfg.Auth.AdminInitialPassword = strings.TrimSpace(string(data))
	}

	// If BindDNFile is specified (via YAML or env LDAP_BIND_DN_FILE), read bindDN from file
	if cfg.Auth.LDAP.BindDNFile != "" {
		data, err := os.ReadFile(cfg.Auth.LDAP.BindDNFile)
		if err != nil {
			return nil, fmt.Errorf("reading ldap bind DN file %s: %w", cfg.Auth.LDAP.BindDNFile, err)
		}
		cfg.Auth.LDAP.BindDN = strings.TrimSpace(string(data))
	}

	// If BindPasswordFile is specified (via YAML or env LDAP_BIND_PASSWORD_FILE), read bindPassword from file
	if cfg.Auth.LDAP.BindPasswordFile != "" {
		data, err := os.ReadFile(cfg.Auth.LDAP.BindPasswordFile)
		if err != nil {
			return nil, fmt.Errorf("reading ldap bind password file %s: %w", cfg.Auth.LDAP.BindPasswordFile, err)
		}
		cfg.Auth.LDAP.BindPassword = strings.TrimSpace(string(data))
	}

	// If ClientIDFile is specified (via YAML or env OIDC_CLIENT_ID_FILE), read client_id from file
	if cfg.Auth.OIDC.ClientIDFile != "" {
		data, err := os.ReadFile(cfg.Auth.OIDC.ClientIDFile)
		if err != nil {
			return nil, fmt.Errorf("reading oidc client id file %s: %w", cfg.Auth.OIDC.ClientIDFile, err)
		}
		cfg.Auth.OIDC.ClientID = strings.TrimSpace(string(data))
	}

	// If ClientSecretFile is specified (via YAML or env OIDC_CLIENT_SECRET_FILE), read client_secret from file
	if cfg.Auth.OIDC.ClientSecretFile != "" {
		data, err := os.ReadFile(cfg.Auth.OIDC.ClientSecretFile)
		if err != nil {
			return nil, fmt.Errorf("reading oidc client secret file %s: %w", cfg.Auth.OIDC.ClientSecretFile, err)
		}
		cfg.Auth.OIDC.ClientSecret = strings.TrimSpace(string(data))
	}

	// Normalize default language (supported: "en", "ru")
	cfg.Theme.DefaultLanguage = strings.ToLower(strings.TrimSpace(cfg.Theme.DefaultLanguage))
	if cfg.Theme.DefaultLanguage != "ru" && cfg.Theme.DefaultLanguage != "en" {
		cfg.Theme.DefaultLanguage = "en"
	}

	// Normalize and limit quick add presets (from 1 to 3 items)
	if len(cfg.QuickAdd.Presets) > 3 {
		cfg.QuickAdd.Presets = cfg.QuickAdd.Presets[:3]
	}
	for i := range cfg.QuickAdd.Presets {
		cfg.QuickAdd.Presets[i].Protocol = model.Protocol(strings.ToLower(strings.TrimSpace(string(cfg.QuickAdd.Presets[i].Protocol))))
		cfg.QuickAdd.Presets[i].Host = strings.TrimSpace(cfg.QuickAdd.Presets[i].Host)
		if cfg.QuickAdd.Presets[i].TimeoutSeconds <= 0 {
			cfg.QuickAdd.Presets[i].TimeoutSeconds = 5
		}
	}

	return cfg, nil
}

func (c *Config) loadFromEnv() {
	// Server
	if v := os.Getenv("SERVER_HOST"); v != "" {
		c.Server.Host = v
	}
	if v := os.Getenv("SERVER_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil && p > 0 {
			c.Server.Port = p
		}
	}
	if v := os.Getenv("JWT_SECRET"); v != "" {
		c.Server.JWTSecret = v
	}
	if v := os.Getenv("TOKEN_TTL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			c.Server.TokenTTL = d
		}
	}
	if v := os.Getenv("SERVER_TLS_ENABLED"); v != "" {
		c.Server.TLS.Enabled = parseBool(v)
	}
	if v := os.Getenv("SERVER_TLS_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil && p > 0 {
			c.Server.TLS.Port = p
		}
	}
	if v := os.Getenv("SERVER_TLS_CERT_FILE"); v != "" {
		c.Server.TLS.CertFile = strings.TrimSpace(v)
	} else if v := os.Getenv("SERVER_TLS_CERT_PATH"); v != "" {
		c.Server.TLS.CertFile = strings.TrimSpace(v)
	}
	if v := os.Getenv("SERVER_TLS_KEY_FILE"); v != "" {
		c.Server.TLS.KeyFile = strings.TrimSpace(v)
	} else if v := os.Getenv("SERVER_TLS_KEY_PATH"); v != "" {
		c.Server.TLS.KeyFile = strings.TrimSpace(v)
	}
	if v := os.Getenv("SERVER_TLS_CERT_PEM"); v != "" {
		c.Server.TLS.CertPEM = v
	}
	if v := os.Getenv("SERVER_TLS_KEY_PEM"); v != "" {
		c.Server.TLS.KeyPEM = v
	}
	if v := os.Getenv("SERVER_TLS_AUTO_GENERATE"); v != "" {
		c.Server.TLS.AutoGenerate = parseBool(v)
	}
	if v := os.Getenv("SERVER_TLS_MIN_VERSION"); v != "" {
		c.Server.TLS.MinVersion = strings.TrimSpace(v)
	}
	if v := os.Getenv("SERVER_OPENAPI_ENABLED"); v != "" {
		c.Server.OpenAPI.Enabled = parseBool(v)
	}
	if v := os.Getenv("SERVER_OPENAPI_PUBLIC"); v != "" {
		c.Server.OpenAPI.Public = parseBool(v)
	}

	// Auto-detect standard mounted server TLS secret path (/etc/kncc-tls)
	if c.Server.TLS.Enabled && c.Server.TLS.CertFile == "" && c.Server.TLS.CertPEM == "" {
		defaultTLSCert := "/etc/kncc-tls/tls.crt"
		defaultTLSKey := "/etc/kncc-tls/tls.key"
		if infoC, errC := os.Stat(defaultTLSCert); errC == nil && !infoC.IsDir() {
			if infoK, errK := os.Stat(defaultTLSKey); errK == nil && !infoK.IsDir() {
				c.Server.TLS.CertFile = defaultTLSCert
				c.Server.TLS.KeyFile = defaultTLSKey
			}
		}
	}

	// K8s
	if v := os.Getenv("KUBECONFIG"); v != "" {
		c.K8s.Kubeconfig = v
	}
	if v := os.Getenv("K8S_NAMESPACE"); v != "" {
		c.K8s.Namespace = v
	}
	if v := os.Getenv("K8S_PROBE_IMAGE"); v != "" {
		c.K8s.ProbeImage = v
	} else if v := os.Getenv("KNCC_K8S_PROBE_IMAGE"); v != "" {
		c.K8s.ProbeImage = v
	}
	if v := os.Getenv("K8S_MULTI_NAMESPACE_ALLOWED"); v != "" {
		c.K8s.MultiNamespaceAllowed = parseBool(v)
	}
	if v := os.Getenv("K8S_NAMESPACE_LIST_ENABLED"); v != "" {
		c.K8s.NamespaceListEnabled = parseBool(v)
	}
	if v := os.Getenv("K8S_NAMESPACE_EXCLUDE_LIST"); v != "" {
		c.K8s.NamespaceExcludeList = splitAndTrim(v, ",")
	}
	if v := os.Getenv("K8S_NAMESPACE_EXCLUDE_SELECTOR"); v != "" {
		c.K8s.NamespaceExcludeSelector = strings.TrimSpace(v)
	}
	if v := os.Getenv("K8S_PROBE_TIMEOUT_SECONDS"); v != "" {
		if t, err := strconv.Atoi(v); err == nil && t > 0 {
			c.K8s.ProbeTimeoutSeconds = t
		}
	}
	if v := os.Getenv("K8S_PROBE_ALLOW_PRIVILEGE_ESCALATION"); v != "" {
		c.K8s.ProbeAllowPrivilegeEscalation = boolPtr(parseBool(v))
	} else if v := os.Getenv("KNCC_K8S_PROBE_ALLOW_PRIVILEGE_ESCALATION"); v != "" {
		c.K8s.ProbeAllowPrivilegeEscalation = boolPtr(parseBool(v))
	}
	if v := os.Getenv("K8S_GC_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			c.K8s.GCInterval = d
		}
	}
	if v := os.Getenv("K8S_GC_STALE_THRESHOLD"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			c.K8s.GCStaleThreshold = d
		}
	}
	if v := os.Getenv("K8S_LOGO_CONFIGMAP_NAME"); v != "" {
		c.K8s.LogoConfigMapName = strings.TrimSpace(v)
	}
	if v := os.Getenv("K8S_FAVICON_CONFIGMAP_NAME"); v != "" {
		c.K8s.FaviconConfigMapName = strings.TrimSpace(v)
	}

	// Security
	if v := os.Getenv("SECURITY_BLACKLIST_CIDRS"); v != "" {
		c.Security.BlacklistCIDRs = splitAndTrim(v, ",")
	}
	if v := os.Getenv("SECURITY_BLACKLIST_DOMAINS"); v != "" {
		c.Security.BlacklistDomains = splitAndTrim(v, ",")
	}
	if v := os.Getenv("SECURITY_WHITELIST_CIDRS"); v != "" {
		c.Security.WhitelistCIDRs = splitAndTrim(v, ",")
	}
	if v := os.Getenv("SECURITY_WHITELIST_DOMAINS"); v != "" {
		c.Security.WhitelistDomains = splitAndTrim(v, ",")
	}
	if v := os.Getenv("SECURITY_RATE_LIMIT_PER_MINUTE"); v != "" {
		if r, err := strconv.Atoi(v); err == nil && r > 0 {
			c.Security.RateLimitPerMinute = r
		}
	}

	// Storage
	if v := os.Getenv("STORAGE_DB_PATH"); v != "" {
		c.Storage.DBPath = v
	}
	if v := os.Getenv("STORAGE_REPORT_TTL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			c.Storage.ReportTTL = d
		}
	}

	// Auth
	if v := os.Getenv("ADMIN_USERNAME"); v != "" {
		c.Auth.AdminUsername = v
	}
	if v := os.Getenv("ADMIN_INITIAL_PASSWORD"); v != "" {
		c.Auth.AdminInitialPassword = v
	}
	if v := os.Getenv("ADMIN_INITIAL_PASSWORD_FILE"); v != "" {
		c.Auth.AdminInitialPasswordFile = strings.TrimSpace(v)
	}
	if v := os.Getenv("AUTH_LOCAL_ENABLED"); v != "" {
		c.Auth.LocalEnabled = parseBool(v)
	}
	if v := os.Getenv("AUTH_LDAP_ENABLED"); v != "" {
		c.Auth.LDAPEnabled = parseBool(v)
	}
	if v := os.Getenv("LDAP_URL"); v != "" {
		c.Auth.LDAP.URL = v
	}
	if v := os.Getenv("LDAP_BIND_DN"); v != "" {
		c.Auth.LDAP.BindDN = v
	}
	if v := os.Getenv("LDAP_BIND_PASSWORD"); v != "" {
		c.Auth.LDAP.BindPassword = v
	}
	if v := os.Getenv("LDAP_BIND_DN_FILE"); v != "" {
		c.Auth.LDAP.BindDNFile = strings.TrimSpace(v)
	}
	if v := os.Getenv("LDAP_BIND_PASSWORD_FILE"); v != "" {
		c.Auth.LDAP.BindPasswordFile = strings.TrimSpace(v)
	}
	if v := os.Getenv("LDAP_USER_DN_TEMPLATE"); v != "" {
		c.Auth.LDAP.UserDNTemplate = strings.TrimSpace(v)
	}
	if v := os.Getenv("LDAP_ANONYMOUS_SEARCH"); v != "" {
		c.Auth.LDAP.AnonymousSearch = parseBool(v)
	}
	if v := os.Getenv("LDAP_BASE_DN"); v != "" {
		c.Auth.LDAP.BaseDN = v
	}
	if v := os.Getenv("LDAP_USER_FILTER"); v != "" {
		c.Auth.LDAP.UserFilter = v
	}
	if v := os.Getenv("LDAP_ADMIN_GROUP"); v != "" {
		c.Auth.LDAP.AdminGroup = strings.TrimSpace(v)
	}
	if v := os.Getenv("LDAP_GROUP_FILTER"); v != "" {
		c.Auth.LDAP.GroupFilter = strings.TrimSpace(v)
	}
	if v := os.Getenv("LDAP_GROUP_BASE_DN"); v != "" {
		c.Auth.LDAP.GroupBaseDN = strings.TrimSpace(v)
	}
	if v := os.Getenv("LDAP_GROUP_MEMBER_ATTRIBUTE"); v != "" {
		c.Auth.LDAP.GroupMemberAttribute = strings.TrimSpace(v)
	}
	if v := os.Getenv("LDAP_GROUP_NAME_ATTRIBUTE"); v != "" {
		c.Auth.LDAP.GroupNameAttribute = strings.TrimSpace(v)
	}
	if v := os.Getenv("LDAP_INSECURE_SKIP_VERIFY"); v != "" {
		c.Auth.LDAP.InsecureSkipVerify = parseBool(v)
	}

	if v := os.Getenv("AUTH_OIDC_ENABLED"); v != "" {
		c.Auth.OIDCEnabled = parseBool(v)
	}
	if v := os.Getenv("OIDC_ISSUER_URL"); v != "" {
		c.Auth.OIDC.IssuerURL = v
	}
	if v := os.Getenv("OIDC_CLIENT_ID"); v != "" {
		c.Auth.OIDC.ClientID = v
	}
	if v := os.Getenv("OIDC_CLIENT_ID_FILE"); v != "" {
		c.Auth.OIDC.ClientIDFile = strings.TrimSpace(v)
	}
	if v := os.Getenv("OIDC_CLIENT_SECRET"); v != "" {
		c.Auth.OIDC.ClientSecret = v
	}
	if v := os.Getenv("OIDC_CLIENT_SECRET_FILE"); v != "" {
		c.Auth.OIDC.ClientSecretFile = strings.TrimSpace(v)
	}
	if v := os.Getenv("OIDC_REDIRECT_URL"); v != "" {
		c.Auth.OIDC.RedirectURL = v
	}

	// Theme
	if v := os.Getenv("THEME_PRIMARY_COLOR"); v != "" {
		c.Theme.PrimaryColor = v
	}
	if v := os.Getenv("THEME_ACCENT_COLOR"); v != "" {
		c.Theme.AccentColor = v
	}
	if v := os.Getenv("THEME_APP_NAME"); v != "" {
		c.Theme.AppName = v
	}
	if v := os.Getenv("THEME_DEFAULT_THEME"); v != "" {
		c.Theme.DefaultTheme = v
	}
	if v := os.Getenv("THEME_DEFAULT_LANGUAGE"); v != "" {
		c.Theme.DefaultLanguage = v
	} else if v := os.Getenv("KNCC_DEFAULT_LANGUAGE"); v != "" {
		c.Theme.DefaultLanguage = v
	} else if v := os.Getenv("KNCC_THEME_DEFAULT_LANGUAGE"); v != "" {
		c.Theme.DefaultLanguage = v
	}

	// Quick Add presets
	if v := os.Getenv("QUICK_ADD_PRESETS"); v != "" {
		var presets []QuickAddPreset
		if err := json.Unmarshal([]byte(v), &presets); err == nil && len(presets) > 0 {
			c.QuickAdd.Presets = presets
		}
	}

	// CA Certs
	if v := os.Getenv("CA_CERTS_PATH"); v != "" {
		c.CACerts.Path = strings.TrimSpace(v)
	} else if v := os.Getenv("KNCC_CA_CERTS_PATH"); v != "" {
		c.CACerts.Path = strings.TrimSpace(v)
	}
	if v := os.Getenv("CA_CERTS_PEM"); v != "" {
		c.CACerts.PEM = v
	} else if v := os.Getenv("KNCC_CA_CERTS_PEM"); v != "" {
		c.CACerts.PEM = v
	}
	if v := os.Getenv("K8S_CUSTOM_CA_BUNDLE"); v != "" {
		c.K8s.CustomCABundle = v
	}

	// Auto-detect standard mount directory if not explicitly set
	if c.CACerts.Path == "" {
		defaultMountPath := "/etc/ssl/kncc-ca"
		if info, err := os.Stat(defaultMountPath); err == nil && (info.IsDir() || info.Mode().IsRegular()) {
			c.CACerts.Path = defaultMountPath
		}
	}
}

func parseBool(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	return s == "true" || s == "1" || s == "yes" || s == "on"
}

func boolPtr(b bool) *bool {
	return &b
}

func splitAndTrim(s, sep string) []string {
	parts := strings.Split(s, sep)
	var res []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			res = append(res, p)
		}
	}
	return res
}

func generateRandomSecret(length int) string {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "fallback-secret-kncc-change-in-production"
	}
	return hex.EncodeToString(bytes)
}
