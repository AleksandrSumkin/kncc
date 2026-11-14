package k8s

import (
	"fmt"
	"log"
	"strings"

	"kncc/internal/config"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// IsNodeExcluded checks whether a specific node is excluded from probe pod execution
// based on the administrator-configured node blacklist (by name or label selector)
// or required positive nodeSelector.
func IsNodeExcluded(cfg *config.Config, nodeName string, nodeLabels map[string]string) (bool, error) {
	if cfg == nil {
		return false, nil
	}

	trimmedName := strings.TrimSpace(nodeName)
	for _, excluded := range cfg.K8s.NodeExcludeList {
		if strings.EqualFold(strings.TrimSpace(excluded), trimmedName) {
			return true, nil
		}
	}

	lbls := labels.Set(nodeLabels)

	if selStr := strings.TrimSpace(cfg.K8s.NodeExcludeSelector); selStr != "" {
		selector, err := labels.Parse(selStr)
		if err != nil {
			return false, fmt.Errorf("invalid node exclude selector %q: %w", selStr, err)
		}
		if selector.Matches(lbls) {
			return true, nil
		}
	}

	if selStr := strings.TrimSpace(cfg.K8s.NodeSelector); selStr != "" {
		selector, err := labels.Parse(selStr)
		if err != nil {
			return false, fmt.Errorf("invalid node selector %q: %w", selStr, err)
		}
		if !selector.Matches(lbls) {
			return true, nil
		}
	}

	return false, nil
}

// FilterEligibleNodes filters Kubernetes cluster nodes based on administrator node exclusion policies.
// Excluded nodes are silently eliminated from probe scheduling to protect audit/security nodes.
func FilterEligibleNodes(cfg *config.Config, nodes []corev1.Node) []corev1.Node {
	if cfg == nil {
		return nodes
	}

	var excludeSelector labels.Selector
	if selStr := strings.TrimSpace(cfg.K8s.NodeExcludeSelector); selStr != "" {
		var err error
		excludeSelector, err = labels.Parse(selStr)
		if err != nil {
			log.Printf("Warning: invalid node exclude selector %q: %v", selStr, err)
		}
	}

	var nodeSelector labels.Selector
	if selStr := strings.TrimSpace(cfg.K8s.NodeSelector); selStr != "" {
		var err error
		nodeSelector, err = labels.Parse(selStr)
		if err != nil {
			log.Printf("Warning: invalid node selector %q: %v", selStr, err)
		}
	}

	excludeNames := make(map[string]bool)
	for _, name := range cfg.K8s.NodeExcludeList {
		if trimmed := strings.ToLower(strings.TrimSpace(name)); trimmed != "" {
			excludeNames[trimmed] = true
		}
	}

	var eligible []corev1.Node
	for _, node := range nodes {
		if excludeNames[strings.ToLower(node.Name)] {
			log.Printf("Node %s excluded by node exclude list", node.Name)
			continue
		}

		lbls := labels.Set(node.Labels)
		if excludeSelector != nil && excludeSelector.Matches(lbls) {
			log.Printf("Node %s excluded by node exclude selector %q", node.Name, cfg.K8s.NodeExcludeSelector)
			continue
		}

		if nodeSelector != nil && !nodeSelector.Matches(lbls) {
			log.Printf("Node %s excluded by node selector %q", node.Name, cfg.K8s.NodeSelector)
			continue
		}

		eligible = append(eligible, node)
	}

	return eligible
}

// FilterMockNodes filters mock nodes based on administrator node exclusion policies for simulation mode.
func FilterMockNodes(cfg *config.Config, nodes []MockNode) []MockNode {
	if cfg == nil {
		return nodes
	}

	var excludeSelector labels.Selector
	if selStr := strings.TrimSpace(cfg.K8s.NodeExcludeSelector); selStr != "" {
		var err error
		excludeSelector, err = labels.Parse(selStr)
		if err != nil {
			log.Printf("Warning: invalid node exclude selector %q: %v", selStr, err)
		}
	}

	var nodeSelector labels.Selector
	if selStr := strings.TrimSpace(cfg.K8s.NodeSelector); selStr != "" {
		var err error
		nodeSelector, err = labels.Parse(selStr)
		if err != nil {
			log.Printf("Warning: invalid node selector %q: %v", selStr, err)
		}
	}

	excludeNames := make(map[string]bool)
	for _, name := range cfg.K8s.NodeExcludeList {
		if trimmed := strings.ToLower(strings.TrimSpace(name)); trimmed != "" {
			excludeNames[trimmed] = true
		}
	}

	var eligible []MockNode
	for _, node := range nodes {
		if excludeNames[strings.ToLower(node.Name)] {
			continue
		}

		lbls := labels.Set(node.Labels)
		if excludeSelector != nil && excludeSelector.Matches(lbls) {
			continue
		}

		if nodeSelector != nil && !nodeSelector.Matches(lbls) {
			continue
		}

		eligible = append(eligible, node)
	}

	return eligible
}
