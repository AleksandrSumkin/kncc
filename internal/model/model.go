package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
)

// Protocol defines the network protocol for checks.
type Protocol string

const (
	ProtocolTCP   Protocol = "tcp"
	ProtocolUDP   Protocol = "udp"
	ProtocolHTTP  Protocol = "http"
	ProtocolHTTPS Protocol = "https"
	ProtocolICMP  Protocol = "icmp"
)

// CheckMode defines whether check runs on host network or pod network.
type CheckMode string

const (
	ModeHostNetwork CheckMode = "host"
	ModePodNetwork  CheckMode = "pod"
)

// ReportStatus represents the overall status of a check run.
type ReportStatus string

const (
	StatusPending   ReportStatus = "pending"
	StatusRunning   ReportStatus = "running"
	StatusCompleted ReportStatus = "completed"
	StatusFailed    ReportStatus = "failed"
)

// Role defines user access level.
type Role string

const (
	RoleAdmin Role = "admin"
	RoleUser  Role = "user"
)

// TargetService defines a single endpoint to check.
type TargetService struct {
	Name           string   `json:"name,omitempty" yaml:"name,omitempty"`
	Protocol       Protocol `json:"protocol" yaml:"protocol"`
	Host           string   `json:"host" yaml:"host"`
	Port           int      `json:"port,omitempty" yaml:"port,omitempty"`
	Path           string   `json:"path,omitempty" yaml:"path,omitempty"`
	InsecureTLS    bool     `json:"insecure_tls,omitempty" yaml:"insecure_tls,omitempty"`
	TimeoutSeconds int      `json:"timeout_seconds,omitempty" yaml:"timeout_seconds,omitempty"`
}

// Validate checks if the target service parameters are sound.
func (t *TargetService) Validate() error {
	t.Protocol = Protocol(strings.ToLower(strings.TrimSpace(string(t.Protocol))))
	t.Host = strings.TrimSpace(t.Host)
	if t.Host == "" {
		return errors.New("host cannot be empty")
	}

	switch t.Protocol {
	case ProtocolTCP, ProtocolUDP:
		if t.Port < 1 || t.Port > 65535 {
			return fmt.Errorf("invalid port %d for protocol %s (must be 1-65535)", t.Port, t.Protocol)
		}
	case ProtocolHTTP, ProtocolHTTPS:
		if t.Port == 0 {
			if t.Protocol == ProtocolHTTP {
				t.Port = 80
			} else {
				t.Port = 443
			}
		} else if t.Port < 1 || t.Port > 65535 {
			return fmt.Errorf("invalid port %d for %s", t.Port, t.Protocol)
		}
		if t.Path == "" {
			t.Path = "/"
		} else if !strings.HasPrefix(t.Path, "/") {
			t.Path = "/" + t.Path
		}
	case ProtocolICMP:
		// Port is not used for ICMP ping
		t.Port = 0
	default:
		return fmt.Errorf("unsupported protocol %q, must be one of: tcp, udp, http, https, icmp", t.Protocol)
	}

	if t.TimeoutSeconds <= 0 {
		t.TimeoutSeconds = 5
	} else if t.TimeoutSeconds > 30 {
		t.TimeoutSeconds = 30
	}

	return nil
}

// TargetIdentifier returns a human-readable identifier for display.
func (t TargetService) TargetIdentifier() string {
	if t.Protocol == ProtocolICMP {
		return fmt.Sprintf("icmp://%s", t.Host)
	}
	if t.Protocol == ProtocolHTTP || t.Protocol == ProtocolHTTPS {
		portPart := ""
		if (t.Protocol == ProtocolHTTP && t.Port != 80) || (t.Protocol == ProtocolHTTPS && t.Port != 443) {
			portPart = fmt.Sprintf(":%d", t.Port)
		}
		return fmt.Sprintf("%s://%s%s%s", t.Protocol, t.Host, portPart, t.Path)
	}
	return fmt.Sprintf("%s://%s:%d", t.Protocol, t.Host, t.Port)
}

// CheckRequest is the payload sent by a client to initiate a check.
type CheckRequest struct {
	Targets   []TargetService `json:"targets"`
	Mode      CheckMode       `json:"mode"`
	Namespace string          `json:"namespace,omitempty"`
}

// Validate checks request validity.
func (r *CheckRequest) Validate(multiNamespaceAllowed bool, defaultNamespace string) error {
	r.Mode = CheckMode(strings.ToLower(strings.TrimSpace(string(r.Mode))))
	if r.Mode != ModeHostNetwork && r.Mode != ModePodNetwork {
		return fmt.Errorf("invalid check mode %q: must be 'host' or 'pod'", r.Mode)
	}

	r.Namespace = strings.TrimSpace(r.Namespace)
	if r.Mode == ModePodNetwork {
		if r.Namespace == "" {
			r.Namespace = defaultNamespace
		} else if !multiNamespaceAllowed && r.Namespace != defaultNamespace {
			return fmt.Errorf("running checks in namespace %q is forbidden by cluster policy", r.Namespace)
		}
	} else {
		// In host network mode, pod runs in default kncc namespace
		if r.Namespace == "" {
			r.Namespace = defaultNamespace
		}
	}

	if len(r.Targets) == 0 {
		return errors.New("at least one target service must be specified")
	}
	if len(r.Targets) > 10 {
		return errors.New("maximum 10 target services allowed per check request")
	}

	for i := range r.Targets {
		if err := r.Targets[i].Validate(); err != nil {
			return fmt.Errorf("target [%d] invalid: %w", i, err)
		}
	}

	return nil
}

// CertStatus represents the validity state of a TLS/SSL certificate.
type CertStatus string

const (
	CertStatusValid        CertStatus = "valid"
	CertStatusExpiringSoon CertStatus = "expiring_soon"
	CertStatusExpired      CertStatus = "expired"
)

// ProbeOutput matches the structured JSON produced by probe pods in /dev/termination-log.
type ProbeOutput struct {
	Target         TargetService `json:"target"`
	Success        bool          `json:"success"`
	LatencyMs      int64         `json:"latency_ms"`
	ResolvedIP     string        `json:"resolved_ip,omitempty"`
	HTTPStatusCode int           `json:"http_status_code,omitempty"`
	Error          string        `json:"error,omitempty"`
	Details        string        `json:"details,omitempty"`
	CertExpiry     string        `json:"cert_expiry,omitempty"`
	CertDaysLeft   *int          `json:"cert_days_left,omitempty"`
	CertStatus     CertStatus    `json:"cert_status,omitempty"`
	CertSubject    string        `json:"cert_subject,omitempty"`
	CertIssuer     string        `json:"cert_issuer,omitempty"`
}

// NodeResult contains check results for a single Kubernetes node.
type NodeResult struct {
	NodeName       string        `json:"node_name"`
	NodeIP         string        `json:"node_ip,omitempty"`
	NodeRoles      []string      `json:"node_roles,omitempty"`
	Status         string        `json:"status"` // "completed", "failed", "timeout", "running"
	ServiceResults []ProbeOutput `json:"service_results"`
	Error          string        `json:"error,omitempty"`
	DurationMs     int64         `json:"duration_ms"`
	FinishedAt     time.Time     `json:"finished_at"`
}

// Report holds the complete state and results of a check.
type Report struct {
	ID             string                `json:"id"`
	UserID         string                `json:"user_id"`
	Username       string                `json:"username"`
	Mode           CheckMode             `json:"mode"`
	Namespace      string                `json:"namespace"`
	Targets        []TargetService       `json:"targets"`
	Status         ReportStatus          `json:"status"`
	TotalNodes     int                   `json:"total_nodes"`
	CompletedNodes int                   `json:"completed_nodes"`
	NodeResults    map[string]NodeResult `json:"node_results"`
	CreatedAt      time.Time             `json:"created_at"`
	CompletedAt    *time.Time            `json:"completed_at,omitempty"`
	ExpiresAt      time.Time             `json:"expires_at"`
}

// TargetsJSON returns serialized targets for SQLite storage.
func (r *Report) TargetsJSON() string {
	b, _ := json.Marshal(r.Targets)
	return string(b)
}

// NodeResultsJSON returns serialized node results for SQLite storage.
func (r *Report) NodeResultsJSON() string {
	b, _ := json.Marshal(r.NodeResults)
	return string(b)
}

// User represents an authenticated user.
type User struct {
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	Role         Role      `json:"role"`
	Provider     string    `json:"provider"` // "local", "ldap", "oidc"
	Email        string    `json:"email,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// SafeUser returns a user object safe for JSON responses (no hashes).
func (u *User) SafeUser() User {
	return User{
		ID:        u.ID,
		Username:  u.Username,
		Role:      u.Role,
		Provider:  u.Provider,
		Email:     u.Email,
		CreatedAt: u.CreatedAt,
	}
}

// UserStats represents network check request statistics for a user.
type UserStats struct {
	UserID     string     `json:"user_id"`
	Username   string     `json:"username"`
	Role       Role       `json:"role"`
	Provider   string     `json:"provider"`
	DayCount   int        `json:"day_count"`
	WeekCount  int        `json:"week_count"`
	MonthCount int        `json:"month_count"`
	YearCount  int        `json:"year_count"`
	TotalCount int        `json:"total_count"`
	LastActive *time.Time `json:"last_active,omitempty"`
}

// UserStatsSummary aggregates totals across all users.
type UserStatsSummary struct {
	DayCount   int `json:"day_count"`
	WeekCount  int `json:"week_count"`
	MonthCount int `json:"month_count"`
	YearCount  int `json:"year_count"`
	TotalCount int `json:"total_count"`
}

// UserStatsResponse is the API response payload for user statistics.
type UserStatsResponse struct {
	Users []UserStats      `json:"users"`
	Total UserStatsSummary `json:"total"`
}

// IsIPOrHostname returns true if string is valid IP or valid hostname.
func IsIPOrHostname(host string) bool {
	if net.ParseIP(host) != nil {
		return true
	}
	if len(host) > 253 {
		return false
	}
	// Check RFC 1123 hostname
	for _, part := range strings.Split(host, ".") {
		if len(part) == 0 || len(part) > 63 {
			return false
		}
	}
	return true
}
