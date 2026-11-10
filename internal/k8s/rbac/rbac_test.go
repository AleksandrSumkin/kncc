package rbac

import (
	"context"
	"strings"
	"testing"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestGetMockRBACReport(t *testing.T) {
	report := GetMockRBACReport("test-ns")
	if report == nil {
		t.Fatal("expected report, got nil")
	}

	if report.Subject.Namespace != "test-ns" {
		t.Errorf("expected namespace 'test-ns', got '%s'", report.Subject.Namespace)
	}

	if len(report.Rules) != 5 {
		t.Errorf("expected 5 rules, got %d", len(report.Rules))
	}

	table := FormatPolicyRulesTable(report)
	if !strings.Contains(table, "ClusterRole/kncc-node-viewer") {
		t.Error("expected table to contain ClusterRole/kncc-node-viewer")
	}
	if !strings.Contains(table, "Role/test-ns/kncc-probe-executor") {
		t.Error("expected table to contain Role/test-ns/kncc-probe-executor")
	}
	if !strings.Contains(table, "TYPE") || !strings.Contains(table, "ORIGINATED FROM") {
		t.Error("expected table headers in formatted output")
	}
}

func TestInspectPermissions_FakeClient_RBAC(t *testing.T) {
	client := fake.NewSimpleClientset(
		&rbacv1.ClusterRole{
			ObjectMeta: metav1.ObjectMeta{Name: "test-node-viewer"},
			Rules: []rbacv1.PolicyRule{
				{
					Verbs:     []string{"get", "list"},
					APIGroups: []string{""},
					Resources: []string{"nodes"},
				},
			},
		},
		&rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "test-node-viewer-binding"},
			RoleRef: rbacv1.RoleRef{
				Kind: "ClusterRole",
				Name: "test-node-viewer",
			},
			Subjects: []rbacv1.Subject{
				{
					Kind:      "ServiceAccount",
					Name:      "kncc",
					Namespace: "kncc",
				},
			},
		},
		&rbacv1.Role{
			ObjectMeta: metav1.ObjectMeta{Name: "test-pod-executor", Namespace: "kncc"},
			Rules: []rbacv1.PolicyRule{
				{
					Verbs:     []string{"create", "delete"},
					APIGroups: []string{""},
					Resources: []string{"pods"},
				},
			},
		},
		&rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "test-pod-executor-binding", Namespace: "kncc"},
			RoleRef: rbacv1.RoleRef{
				Kind: "Role",
				Name: "test-pod-executor",
			},
			Subjects: []rbacv1.Subject{
				{
					Kind:      "ServiceAccount",
					Name:      "kncc",
					Namespace: "kncc",
				},
			},
		},
	)

	report := InspectCurrentPermissions(context.Background(), client, "kncc", false)
	if report == nil {
		t.Fatal("expected report, got nil")
	}

	if report.Source != "cluster-rbac" {
		t.Errorf("expected source 'cluster-rbac', got '%s'", report.Source)
	}

	if len(report.Rules) < 2 {
		t.Fatalf("expected at least 2 rules, got %d", len(report.Rules))
	}

	hasNodes := false
	hasPods := false
	for _, r := range report.Rules {
		if r.Resource == "nodes" && r.OriginatedFrom == "ClusterRole/test-node-viewer" {
			hasNodes = true
		}
		if r.Resource == "pods" && r.OriginatedFrom == "Role/kncc/test-pod-executor" {
			hasPods = true
		}
	}

	if !hasNodes {
		t.Error("expected rule for nodes from ClusterRole/test-node-viewer")
	}
	if !hasPods {
		t.Error("expected rule for pods from Role/kncc/test-pod-executor")
	}

	table := FormatPolicyRulesTable(report)
	if !strings.Contains(table, "test-node-viewer") {
		t.Error("expected table to contain test-node-viewer")
	}
}

func TestInspectPermissions_MockMode(t *testing.T) {
	report := InspectCurrentPermissions(context.Background(), nil, "custom-ns", true)
	if report.Source != "mock" {
		t.Errorf("expected source 'mock', got '%s'", report.Source)
	}
	if report.Subject.Namespace != "custom-ns" {
		t.Errorf("expected namespace 'custom-ns', got '%s'", report.Subject.Namespace)
	}
}
