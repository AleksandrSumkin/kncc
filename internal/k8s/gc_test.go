package k8s

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// TestGC_FreshPodNotDeleted verifies that freshly created probe pods (even if Succeeded)
// are NOT deleted by GC, preventing race conditions with Runner.probeNode.
func TestGC_FreshPodNotDeleted(t *testing.T) {
	fakeClient := fake.NewSimpleClientset()
	ctx := context.Background()

	// 1. Fresh Succeeded pod (created 10s ago)
	freshSucceededPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "kncc-probe-fresh-succeeded",
			Namespace:         "kncc",
			Labels:            map[string]string{"app.kubernetes.io/managed-by": "kncc"},
			CreationTimestamp: metav1.NewTime(time.Now().Add(-10 * time.Second)),
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodSucceeded,
		},
	}
	_, _ = fakeClient.CoreV1().Pods("kncc").Create(ctx, freshSucceededPod, metav1.CreateOptions{})

	// 2. Fresh Running pod (created 5s ago)
	freshRunningPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "kncc-probe-fresh-running",
			Namespace:         "kncc",
			Labels:            map[string]string{"app.kubernetes.io/managed-by": "kncc"},
			CreationTimestamp: metav1.NewTime(time.Now().Add(-5 * time.Second)),
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}
	_, _ = fakeClient.CoreV1().Pods("kncc").Create(ctx, freshRunningPod, metav1.CreateOptions{})

	// Run GC cleanup with 3m threshold
	cleanStalePods(ctx, fakeClient, "kncc", false, 3*time.Minute)

	// Verify both fresh pods remain intact
	pods, err := fakeClient.CoreV1().Pods("kncc").List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("failed to list pods: %v", err)
	}
	if len(pods.Items) != 2 {
		t.Fatalf("expected 2 pods to remain, got %d", len(pods.Items))
	}
}

// TestGC_StalePodDeleted verifies that orphaned pods older than staleThreshold are removed.
func TestGC_StalePodDeleted(t *testing.T) {
	fakeClient := fake.NewSimpleClientset()
	ctx := context.Background()

	// 1. Stale Succeeded pod (created 5m ago)
	stalePod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "kncc-probe-stale-1",
			Namespace:         "kncc",
			Labels:            map[string]string{"app.kubernetes.io/managed-by": "kncc"},
			CreationTimestamp: metav1.NewTime(time.Now().Add(-5 * time.Minute)),
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodSucceeded,
		},
	}
	_, _ = fakeClient.CoreV1().Pods("kncc").Create(ctx, stalePod, metav1.CreateOptions{})

	// 2. Stale zombie pod stuck in Pending/Running (created 10m ago)
	staleZombie := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "kncc-probe-zombie-1",
			Namespace:         "kncc",
			Labels:            map[string]string{"app.kubernetes.io/managed-by": "kncc"},
			CreationTimestamp: metav1.NewTime(time.Now().Add(-10 * time.Minute)),
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}
	_, _ = fakeClient.CoreV1().Pods("kncc").Create(ctx, staleZombie, metav1.CreateOptions{})

	// 3. Fresh pod (created 10s ago)
	freshPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "kncc-probe-fresh",
			Namespace:         "kncc",
			Labels:            map[string]string{"app.kubernetes.io/managed-by": "kncc"},
			CreationTimestamp: metav1.NewTime(time.Now().Add(-10 * time.Second)),
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodSucceeded,
		},
	}
	_, _ = fakeClient.CoreV1().Pods("kncc").Create(ctx, freshPod, metav1.CreateOptions{})

	// Run GC cleanup
	cleanStalePods(ctx, fakeClient, "kncc", false, 3*time.Minute)

	// Verify only freshPod remains
	pods, err := fakeClient.CoreV1().Pods("kncc").List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("failed to list pods: %v", err)
	}
	if len(pods.Items) != 1 {
		t.Fatalf("expected 1 pod to remain, got %d", len(pods.Items))
	}
	if pods.Items[0].Name != "kncc-probe-fresh" {
		t.Errorf("expected kncc-probe-fresh to remain, got %s", pods.Items[0].Name)
	}
}

// TestGC_MultiNamespaceDisabled verifies that when multiNamespace is false,
// GC only cleans pods inside the default namespace and does not touch other namespaces.
func TestGC_MultiNamespaceDisabled(t *testing.T) {
	fakeClient := fake.NewSimpleClientset()
	ctx := context.Background()

	// 1. Stale pod in default kncc namespace
	staleInKncc := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "probe-in-kncc",
			Namespace:         "kncc",
			Labels:            map[string]string{"app.kubernetes.io/managed-by": "kncc"},
			CreationTimestamp: metav1.NewTime(time.Now().Add(-5 * time.Minute)),
		},
	}
	_, _ = fakeClient.CoreV1().Pods("kncc").Create(ctx, staleInKncc, metav1.CreateOptions{})

	// 2. Stale pod in custom namespace
	staleInCustom := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "probe-in-custom",
			Namespace:         "custom-ns",
			Labels:            map[string]string{"app.kubernetes.io/managed-by": "kncc"},
			CreationTimestamp: metav1.NewTime(time.Now().Add(-5 * time.Minute)),
		},
	}
	_, _ = fakeClient.CoreV1().Pods("custom-ns").Create(ctx, staleInCustom, metav1.CreateOptions{})

	// Run GC cleanup with multiNamespace = false
	cleanStalePods(ctx, fakeClient, "kncc", false, 3*time.Minute)

	// Verify probe in kncc was deleted
	knccPods, _ := fakeClient.CoreV1().Pods("kncc").List(ctx, metav1.ListOptions{})
	if len(knccPods.Items) != 0 {
		t.Errorf("expected probe-in-kncc to be deleted, got %d pods", len(knccPods.Items))
	}

	// Verify probe in custom-ns was NOT deleted because multiNamespace is false
	customPods, _ := fakeClient.CoreV1().Pods("custom-ns").List(ctx, metav1.ListOptions{})
	if len(customPods.Items) != 1 || customPods.Items[0].Name != "probe-in-custom" {
		t.Errorf("expected probe-in-custom to remain untouched when multiNamespace=false")
	}
}

// TestGC_MultiNamespaceEnabled verifies that when multiNamespace is true,
// GC scans all namespaces in the cluster and cleans stale probe pods anywhere.
func TestGC_MultiNamespaceEnabled(t *testing.T) {
	fakeClient := fake.NewSimpleClientset()
	ctx := context.Background()

	// 1. Stale pod in default kncc namespace
	staleInKncc := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "probe-in-kncc",
			Namespace:         "kncc",
			Labels:            map[string]string{"app.kubernetes.io/managed-by": "kncc"},
			CreationTimestamp: metav1.NewTime(time.Now().Add(-5 * time.Minute)),
		},
	}
	_, _ = fakeClient.CoreV1().Pods("kncc").Create(ctx, staleInKncc, metav1.CreateOptions{})

	// 2. Stale pod in namespace "production"
	staleInProd := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "probe-in-prod",
			Namespace:         "production",
			Labels:            map[string]string{"app.kubernetes.io/managed-by": "kncc"},
			CreationTimestamp: metav1.NewTime(time.Now().Add(-5 * time.Minute)),
		},
	}
	_, _ = fakeClient.CoreV1().Pods("production").Create(ctx, staleInProd, metav1.CreateOptions{})

	// 3. Fresh pod in namespace "production" (should NOT be deleted)
	freshInProd := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "probe-fresh-in-prod",
			Namespace:         "production",
			Labels:            map[string]string{"app.kubernetes.io/managed-by": "kncc"},
			CreationTimestamp: metav1.NewTime(time.Now().Add(-10 * time.Second)),
		},
	}
	_, _ = fakeClient.CoreV1().Pods("production").Create(ctx, freshInProd, metav1.CreateOptions{})

	// Run GC cleanup with multiNamespace = true
	cleanStalePods(ctx, fakeClient, "kncc", true, 3*time.Minute)

	// Verify probe in kncc was deleted
	knccPods, _ := fakeClient.CoreV1().Pods("kncc").List(ctx, metav1.ListOptions{})
	if len(knccPods.Items) != 0 {
		t.Errorf("expected probe-in-kncc to be deleted, got %d pods", len(knccPods.Items))
	}

	// Verify stale probe in production was deleted, and fresh probe was preserved
	prodPods, _ := fakeClient.CoreV1().Pods("production").List(ctx, metav1.ListOptions{})
	if len(prodPods.Items) != 1 || prodPods.Items[0].Name != "probe-fresh-in-prod" {
		t.Errorf("expected only probe-fresh-in-prod to remain, got %d pods", len(prodPods.Items))
	}
}

// TestGC_UnmanagedPodsUntouched verifies that pods without the kncc label are ignored.
func TestGC_UnmanagedPodsUntouched(t *testing.T) {
	fakeClient := fake.NewSimpleClientset()
	ctx := context.Background()

	unmanagedPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "unmanaged-system-pod",
			Namespace:         "kncc",
			Labels:            map[string]string{"app": "system"},
			CreationTimestamp: metav1.NewTime(time.Now().Add(-20 * time.Minute)),
		},
	}
	_, _ = fakeClient.CoreV1().Pods("kncc").Create(ctx, unmanagedPod, metav1.CreateOptions{})

	cleanStalePods(ctx, fakeClient, "kncc", true, 3*time.Minute)

	pods, _ := fakeClient.CoreV1().Pods("kncc").List(ctx, metav1.ListOptions{})
	if len(pods.Items) != 1 || pods.Items[0].Name != "unmanaged-system-pod" {
		t.Errorf("unmanaged pod was deleted or altered")
	}
}
