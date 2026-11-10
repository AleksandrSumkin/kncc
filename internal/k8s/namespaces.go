package k8s

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"kncc/internal/config"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
)

// DefaultMockNamespaces provides sample namespaces for mock/standalone mode.
var DefaultMockNamespaces = []corev1.Namespace{
	{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
	{ObjectMeta: metav1.ObjectMeta{Name: "app-prod"}},
	{ObjectMeta: metav1.ObjectMeta{Name: "app-staging"}},
	{ObjectMeta: metav1.ObjectMeta{Name: "kncc"}},
	{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}},
	{ObjectMeta: metav1.ObjectMeta{Name: "kube-public"}},
	{ObjectMeta: metav1.ObjectMeta{Name: "kube-node-lease"}},
	{
		ObjectMeta: metav1.ObjectMeta{
			Name: "system-monitoring",
			Labels: map[string]string{
				"kncc.io/exclude": "true",
			},
		},
	},
}

// ListAllowedNamespaces lists cluster namespaces that are allowed based on the exclude list and label selector.
// If client is nil (e.g., running in mock mode), simulated namespaces are returned.
func ListAllowedNamespaces(ctx context.Context, client kubernetes.Interface, cfg *config.Config) ([]string, error) {
	if !cfg.K8s.NamespaceListEnabled {
		return nil, fmt.Errorf("namespace listing is disabled by administrator")
	}

	var rawNamespaces []corev1.Namespace

	if client == nil {
		rawNamespaces = DefaultMockNamespaces
	} else {
		nsList, err := client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
		if err != nil {
			return nil, fmt.Errorf("listing cluster namespaces: %w", err)
		}
		rawNamespaces = nsList.Items
	}

	excludeMap := make(map[string]bool)
	for _, ns := range cfg.K8s.NamespaceExcludeList {
		trimmed := strings.ToLower(strings.TrimSpace(ns))
		if trimmed != "" {
			excludeMap[trimmed] = true
		}
	}

	var excludeSelector labels.Selector
	if cfg.K8s.NamespaceExcludeSelector != "" {
		sel, err := labels.Parse(cfg.K8s.NamespaceExcludeSelector)
		if err != nil {
			return nil, fmt.Errorf("invalid namespace exclude selector %q: %w", cfg.K8s.NamespaceExcludeSelector, err)
		}
		excludeSelector = sel
	}

	var allowed []string
	for _, ns := range rawNamespaces {
		nsName := ns.Name
		if excludeMap[strings.ToLower(nsName)] {
			continue
		}

		if excludeSelector != nil && excludeSelector.Matches(labels.Set(ns.Labels)) {
			continue
		}

		allowed = append(allowed, nsName)
	}

	sort.Strings(allowed)
	return allowed, nil
}

// IsNamespaceAllowed checks whether a specific namespace is permitted for running probe pods.
func IsNamespaceAllowed(ctx context.Context, client kubernetes.Interface, cfg *config.Config, targetNs string) (bool, error) {
	targetNs = strings.TrimSpace(targetNs)
	if targetNs == "" {
		return false, fmt.Errorf("namespace cannot be empty")
	}

	// 1. Check explicit exclude list (in-memory, case-insensitive)
	for _, excluded := range cfg.K8s.NamespaceExcludeList {
		if strings.EqualFold(strings.TrimSpace(excluded), targetNs) {
			return false, fmt.Errorf("namespace %q is forbidden by cluster security policy (exclude list)", targetNs)
		}
	}

	// 2. If namespace listing is enabled and exclude selector is configured, verify label selector
	if cfg.K8s.NamespaceListEnabled && cfg.K8s.NamespaceExcludeSelector != "" {
		sel, err := labels.Parse(cfg.K8s.NamespaceExcludeSelector)
		if err != nil {
			return false, fmt.Errorf("invalid namespace exclude selector %q: %w", cfg.K8s.NamespaceExcludeSelector, err)
		}

		if client != nil {
			ns, err := client.CoreV1().Namespaces().Get(ctx, targetNs, metav1.GetOptions{})
			if err != nil {
				return false, fmt.Errorf("verifying namespace %q: %w", targetNs, err)
			}
			if sel.Matches(labels.Set(ns.Labels)) {
				return false, fmt.Errorf("namespace %q is forbidden by cluster security policy (matches exclude selector %s)", targetNs, cfg.K8s.NamespaceExcludeSelector)
			}
		} else {
			// Mock verification
			for _, mns := range DefaultMockNamespaces {
				if strings.EqualFold(mns.Name, targetNs) {
					if sel.Matches(labels.Set(mns.Labels)) {
						return false, fmt.Errorf("namespace %q is forbidden by cluster security policy (matches exclude selector %s)", targetNs, cfg.K8s.NamespaceExcludeSelector)
					}
					break
				}
			}
		}
	}

	return true, nil
}
