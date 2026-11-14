package k8s

import (
	"testing"

	"kncc/internal/config"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestIsNodeExcluded(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.K8s.NodeExcludeList = []string{"k3s-master", "node-audit-01"}
	cfg.K8s.NodeExcludeSelector = "security.zone=audit,kncc.io/exclude=true"

	// 1. Match by name (exact and case-insensitive)
	excluded, err := IsNodeExcluded(cfg, "k3s-master", nil)
	if err != nil || !excluded {
		t.Errorf("expected node k3s-master to be excluded by name, err: %v", err)
	}
	excluded, err = IsNodeExcluded(cfg, "K3S-MASTER", nil)
	if err != nil || !excluded {
		t.Errorf("expected node K3S-MASTER to be excluded (case-insensitive), err: %v", err)
	}
	excluded, err = IsNodeExcluded(cfg, "node-audit-01", nil)
	if err != nil || !excluded {
		t.Errorf("expected node node-audit-01 to be excluded by name, err: %v", err)
	}

	// 2. Allowed node by name
	excluded, err = IsNodeExcluded(cfg, "k3s-worker", map[string]string{"node-role.kubernetes.io/worker": "true"})
	if err != nil || excluded {
		t.Errorf("expected node k3s-worker to be allowed, got excluded: %v, err: %v", excluded, err)
	}

	// 3. Match by label selector
	excluded, err = IsNodeExcluded(cfg, "worker-special", map[string]string{
		"security.zone":   "audit",
		"kncc.io/exclude": "true",
	})
	if err != nil || !excluded {
		t.Errorf("expected worker-special to be excluded by selector, err: %v", err)
	}

	// Partial match does not satisfy full selector with comma (AND)
	excluded, err = IsNodeExcluded(cfg, "worker-partial", map[string]string{
		"security.zone": "audit",
	})
	if err != nil || excluded {
		t.Errorf("expected worker-partial to be allowed (not all labels matched), got excluded: %v", excluded)
	}
}

func TestIsNodeExcluded_NodeSelectorWhitelist(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.K8s.NodeSelector = "node-role.kubernetes.io/worker=true"

	// Matching worker node
	excluded, err := IsNodeExcluded(cfg, "k3s-worker", map[string]string{
		"node-role.kubernetes.io/worker": "true",
	})
	if err != nil || excluded {
		t.Errorf("expected worker node to be allowed, got excluded: %v", excluded)
	}

	// Non-matching master node
	excluded, err = IsNodeExcluded(cfg, "k3s-master", map[string]string{
		"node-role.kubernetes.io/control-plane": "true",
	})
	if err != nil || !excluded {
		t.Errorf("expected master node to be excluded by whitelist nodeSelector, err: %v", err)
	}
}

func TestIsNodeExcluded_InvalidSelector(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.K8s.NodeExcludeSelector = "invalid===selector"

	_, err := IsNodeExcluded(cfg, "worker-01", map[string]string{"key": "val"})
	if err == nil {
		t.Errorf("expected error for invalid label selector, got nil")
	}
}

func TestFilterEligibleNodes(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.K8s.NodeExcludeList = []string{"k3s-master"}
	cfg.K8s.NodeExcludeSelector = "security=restricted"

	nodes := []corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "k3s-master",
				Labels: map[string]string{"node-role.kubernetes.io/control-plane": "true"},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "worker-audit",
				Labels: map[string]string{"security": "restricted"},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "k3s-worker-1",
				Labels: map[string]string{"node-role.kubernetes.io/worker": "true"},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "k3s-worker-2",
				Labels: map[string]string{"node-role.kubernetes.io/worker": "true"},
			},
		},
	}

	eligible := FilterEligibleNodes(cfg, nodes)
	if len(eligible) != 2 {
		t.Fatalf("expected 2 eligible nodes, got %d", len(eligible))
	}
	if eligible[0].Name != "k3s-worker-1" || eligible[1].Name != "k3s-worker-2" {
		t.Errorf("unexpected eligible nodes: %v, %v", eligible[0].Name, eligible[1].Name)
	}
}

func TestFilterMockNodes(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.K8s.NodeExcludeSelector = "node-role.kubernetes.io/control-plane=true"
	cfg.K8s.NodeExcludeList = []string{"k8s-worker-03"}

	mockNodes := []MockNode{
		{Name: "k8s-control-plane-01", Labels: map[string]string{"node-role.kubernetes.io/control-plane": "true"}},
		{Name: "k8s-worker-01", Labels: map[string]string{"node-role.kubernetes.io/worker": "true"}},
		{Name: "k8s-worker-02", Labels: map[string]string{"node-role.kubernetes.io/worker": "true"}},
		{Name: "k8s-worker-03", Labels: map[string]string{"node-role.kubernetes.io/worker": "true"}},
	}

	filtered := FilterMockNodes(cfg, mockNodes)
	if len(filtered) != 2 {
		t.Fatalf("expected 2 mock nodes, got %d", len(filtered))
	}
	if filtered[0].Name != "k8s-worker-01" || filtered[1].Name != "k8s-worker-02" {
		t.Errorf("unexpected filtered mock nodes: %v, %v", filtered[0].Name, filtered[1].Name)
	}
}
