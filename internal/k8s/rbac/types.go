package rbac

import (
	"time"
)

// SubjectInfo identifies the subject whose RBAC permissions are evaluated.
type SubjectInfo struct {
	Kind        string `json:"kind"`                   // "ServiceAccount" or "User"
	Name        string `json:"name"`                   // e.g. "kncc"
	Namespace   string `json:"namespace,omitempty"`    // e.g. "kncc" (empty for cluster users)
	RawUsername string `json:"raw_username,omitempty"` // e.g. "system:serviceaccount:kncc:kncc"
}

// PolicyRule represents an effective Kubernetes policy rule flattened for display (matching rbac-tool policy-rules format).
type PolicyRule struct {
	Type            string   `json:"type"`                        // "ServiceAccount" or "User"
	Subject         string   `json:"subject"`                     // "namespace/name" or "name"
	Verbs           []string `json:"verbs"`                       // ["get", "list", "watch"]
	Namespace       string   `json:"namespace"`                   // "*" or namespace name
	APIGroup        string   `json:"api_group"`                   // "core" or named group
	Resource        string   `json:"resource"`                    // "pods", "nodes", "namespaces", etc.
	ResourceNames   []string `json:"resource_names,omitempty"`    // ["*"] or specific names
	NonResourceURLs []string `json:"non_resource_urls,omitempty"` // ["/healthz", ...]
	OriginatedFrom  string   `json:"originated_from,omitempty"`   // "ClusterRole/kncc-node-viewer", etc.
}

// RBACReport represents the consolidated RBAC permissions report for the application's ServiceAccount.
type RBACReport struct {
	Subject         SubjectInfo  `json:"subject"`
	Rules           []PolicyRule `json:"rules"`
	Source          string       `json:"source"` // "cluster-rbac", "self-subject-rules-review", "mock"
	EvaluationError string       `json:"evaluation_error,omitempty"`
	GeneratedAt     time.Time    `json:"generated_at"`
}
