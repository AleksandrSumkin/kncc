package api

import (
	"kncc/internal/config"
	"kncc/internal/model"
)

// ErrorResponse represents an HTTP error response.
type ErrorResponse struct {
	Error string `json:"error" example:"invalid request parameter"`
}

// StatusOKResponse represents a generic success response.
type StatusOKResponse struct {
	Status string `json:"status" example:"ok"`
}

// MessageResponse represents a generic message response.
type MessageResponse struct {
	Message string `json:"message" example:"operation completed successfully"`
}

// SystemConfigResponse represents public system configuration.
type SystemConfigResponse struct {
	Version               string                  `json:"version" example:"1.11.0"`
	AppName               string                  `json:"app_name" example:"Kubernetes Network Connectivity Checker"`
	PrimaryColor          string                  `json:"primary_color" example:"#ffffff"`
	AccentColor           string                  `json:"accent_color" example:"#dc2626"`
	DefaultTheme          string                  `json:"default_theme" example:"light"`
	DefaultLanguage       string                  `json:"default_language" example:"en"`
	AuthProviders         []string                `json:"auth_providers"`
	MultiNamespaceAllowed bool                    `json:"multi_namespace_allowed" example:"false"`
	NamespaceListEnabled  bool                    `json:"namespace_list_enabled" example:"true"`
	DefaultNamespace      string                  `json:"default_namespace" example:"kncc"`
	RateLimitPerMinute    int                     `json:"rate_limit_per_minute" example:"30"`
	QuickAddPresets       []config.QuickAddPreset `json:"quick_add_presets"`
	HasCustomLogo         bool                    `json:"has_custom_logo" example:"false"`
	LogoURL               string                  `json:"logo_url" example:"/api/system/logo?v=123"`
	HasCustomFavicon      bool                    `json:"has_custom_favicon" example:"false"`
	FaviconURL            string                  `json:"favicon_url" example:"/api/system/favicon?v=123"`
}

// AuthProvidersResponse represents available authentication providers.
type AuthProvidersResponse struct {
	Providers   []string `json:"providers"`
	OIDCAuthURL string   `json:"oidc_auth_url,omitempty" example:"http://dex:5556/auth"`
}

// LoginResponse represents successful login response.
type LoginResponse struct {
	User  *model.User `json:"user"`
	Token string      `json:"token" example:"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."`
}

// NamespacesResponse represents Kubernetes namespaces list.
type NamespacesResponse struct {
	Namespaces []string `json:"namespaces"`
	Source     string   `json:"source" example:"cluster"`
}

// QuickAddResponse represents quick diagnostic presets.
type QuickAddResponse struct {
	QuickAdd []model.TargetService `json:"quick_add"`
}

// ReportsResponse represents reports list.
type ReportsResponse struct {
	Reports []model.Report `json:"reports"`
}

// UsersResponse represents users list.
type UsersResponse struct {
	Users []model.User `json:"users"`
}

// CreateUserResponse represents user creation response.
type CreateUserResponse struct {
	User *model.User `json:"user"`
}
