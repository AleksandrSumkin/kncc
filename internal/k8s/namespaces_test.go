package k8s

import (
	"context"
	"testing"

	"kncc/internal/config"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestListAllowedNamespaces_Disabled(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.K8s.NamespaceListEnabled = false

	_, err := ListAllowedNamespaces(context.Background(), nil, cfg)
	if err == nil {
		t.Fatalf("expected error when namespace listing is disabled, got nil")
	}
}

func TestListAllowedNamespaces_Mock(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.K8s.NamespaceListEnabled = true
	cfg.K8s.NamespaceExcludeList = []string{"kube-system", "kube-public", "kube-node-lease"}
	cfg.K8s.NamespaceExcludeSelector = "kncc.io/exclude=true"

	namespaces, err := ListAllowedNamespaces(context.Background(), nil, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, ns := range namespaces {
		if ns == "kube-system" || ns == "kube-public" || ns == "kube-node-lease" {
			t.Errorf("system namespace %s should have been excluded", ns)
		}
		if ns == "system-monitoring" {
			t.Errorf("labeled namespace %s should have been excluded by selector", ns)
		}
	}

	expectedPresent := []string{"app-prod", "app-staging", "default", "kncc"}
	for _, exp := range expectedPresent {
		found := false
		for _, ns := range namespaces {
			if ns == exp {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected namespace %s to be in allowed list", exp)
		}
	}
}

func TestListAllowedNamespaces_FakeClient(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "production"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "development"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}},
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "security-isolated",
				Labels: map[string]string{
					"tier": "restricted",
				},
			},
		},
	)

	cfg := config.DefaultConfig()
	cfg.K8s.NamespaceListEnabled = true
	cfg.K8s.NamespaceExcludeList = []string{"kube-system"}
	cfg.K8s.NamespaceExcludeSelector = "tier=restricted"

	namespaces, err := ListAllowedNamespaces(context.Background(), client, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(namespaces) != 2 {
		t.Fatalf("expected 2 namespaces, got %d: %v", len(namespaces), namespaces)
	}

	if namespaces[0] != "development" || namespaces[1] != "production" {
		t.Errorf("expected [development, production], got %v", namespaces)
	}
}

func TestIsNamespaceAllowed(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app-prod"}},
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "restricted-ns",
				Labels: map[string]string{
					"security.kncc.io/ignore": "true",
				},
			},
		},
	)

	cfg := config.DefaultConfig()
	cfg.K8s.NamespaceListEnabled = true
	cfg.K8s.NamespaceExcludeList = []string{"kube-system", "kube-public"}
	cfg.K8s.NamespaceExcludeSelector = "security.kncc.io/ignore=true"

	ctx := context.Background()

	// 1. Allowed namespace
	ok, err := IsNamespaceAllowed(ctx, client, cfg, "app-prod")
	if !ok || err != nil {
		t.Errorf("expected app-prod to be allowed, got ok=%v, err=%v", ok, err)
	}

	// 2. Blacklisted by name
	ok, err = IsNamespaceAllowed(ctx, client, cfg, "kube-system")
	if ok || err == nil {
		t.Errorf("expected kube-system to be rejected by exclude list")
	}

	// 3. Blacklisted by label selector
	ok, err = IsNamespaceAllowed(ctx, client, cfg, "restricted-ns")
	if ok || err == nil {
		t.Errorf("expected restricted-ns to be rejected by label selector")
	}

	// 4. Case insensitivity check
	ok, err = IsNamespaceAllowed(ctx, client, cfg, "KUBE-SYSTEM")
	if ok || err == nil {
		t.Errorf("expected KUBE-SYSTEM case-insensitive to be rejected")
	}
}
