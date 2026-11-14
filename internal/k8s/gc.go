package k8s

import (
	"context"
	"log/slog"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// StartGarbageCollector launches a background loop that deletes orphaned kncc probe pods.
func StartGarbageCollector(ctx context.Context, client kubernetes.Interface, namespace string, multiNamespace bool, interval time.Duration, staleThreshold time.Duration, stopCh <-chan struct{}) {
	if interval <= 0 {
		interval = 2 * time.Minute
	}
	if staleThreshold <= 0 {
		staleThreshold = 3 * time.Minute
	}

	scopeMsg := namespace
	if multiNamespace {
		scopeMsg = "all namespaces (multi-namespace enabled)"
	}
	slog.Info("Starting Kubernetes probe garbage collector",
		"interval", interval.String(),
		"stale_threshold", staleThreshold.String(),
		"scope", scopeMsg,
	)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			cleanStalePods(ctx, client, namespace, multiNamespace, staleThreshold)
		case <-stopCh:
			slog.Info("Stopping Kubernetes probe garbage collector")
			return
		case <-ctx.Done():
			return
		}
	}
}

func cleanStalePods(ctx context.Context, client kubernetes.Interface, namespace string, multiNamespace bool, staleThreshold time.Duration) {
	if staleThreshold <= 0 {
		staleThreshold = 3 * time.Minute
	}

	cCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	listOpts := metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/managed-by=kncc",
	}

	listNamespace := namespace
	if multiNamespace {
		listNamespace = metav1.NamespaceAll // query across all namespaces in cluster
	}

	slog.Debug("GC: scanning for stale kncc probe pods", "namespace", listNamespace)
	pods, err := client.CoreV1().Pods(listNamespace).List(cCtx, listOpts)
	if err != nil {
		slog.Warn("GC: failed to list kncc pods", "error", err)
		return
	}

	now := time.Now()
	graceZero := int64(0)
	deleted := 0

	for _, pod := range pods.Items {
		// Verify pod age to prevent race conditions with actively running or just finished checks.
		// Pods younger than staleThreshold are still being processed or cleaned up by runner defer.
		if pod.CreationTimestamp.IsZero() {
			continue
		}

		age := now.Sub(pod.CreationTimestamp.Time)
		if age < staleThreshold {
			continue
		}

		// Pod has exceeded the stale threshold: safe to delete orphaned or hung zombie pod.
		slog.Debug("GC: deleting stale pod", "namespace", pod.Namespace, "pod", pod.Name, "age", age.String())
		err := client.CoreV1().Pods(pod.Namespace).Delete(cCtx, pod.Name, metav1.DeleteOptions{
			GracePeriodSeconds: &graceZero,
		})
		if err != nil {
			slog.Warn("GC: failed to delete stale pod", "namespace", pod.Namespace, "pod", pod.Name, "error", err)
		} else {
			deleted++
		}
	}

	if deleted > 0 {
		slog.Info("GC: removed stale kncc probe pods", "deleted", deleted)
	} else {
		slog.Debug("GC: scan complete, no stale pods found")
	}
}
