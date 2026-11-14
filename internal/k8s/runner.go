package k8s

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"kncc/internal/config"
	"kncc/internal/model"
	"kncc/internal/store"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Runner manages orchestrating network probe checks across Kubernetes nodes.
type Runner struct {
	client kubernetes.Interface
	store  store.Store
	cfg    *config.Config
}

// NewRunner creates a new Kubernetes check runner.
func NewRunner(client kubernetes.Interface, s store.Store, cfg *config.Config) *Runner {
	return &Runner{
		client: client,
		store:  s,
		cfg:    cfg,
	}
}

// ExecuteCheck starts parallel checks on all nodes for the given report.
// It runs asynchronously in a goroutine and updates report status and node results in store.
func (r *Runner) ExecuteCheck(ctx context.Context, report *model.Report) {
	go func() {
		checkTimeout := time.Duration(r.cfg.K8s.ProbeTimeoutSeconds+15) * time.Second
		if checkTimeout < 45*time.Second {
			checkTimeout = 45 * time.Second
		}
		execCtx, cancel := context.WithTimeout(context.Background(), checkTimeout)
		defer cancel()

		r.run(execCtx, report)
	}()
}

func (r *Runner) run(ctx context.Context, report *model.Report) {
	// 1. List all nodes in cluster
	nodeList, err := r.client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		slog.Error("Failed to list nodes for report", "report_id", report.ID, "error", err)
		now := time.Now().UTC()
		_ = r.store.UpdateReportProgress(ctx, report.ID, 0, 0, model.StatusFailed, &now)
		return
	}

	nodes := nodeList.Items
	if len(nodes) == 0 {
		slog.Warn("No nodes found in cluster for report", "report_id", report.ID)
		now := time.Now().UTC()
		_ = r.store.UpdateReportProgress(ctx, report.ID, 0, 0, model.StatusFailed, &now)
		return
	}

	// Filter nodes based on administrator exclude list, exclude selector, and nodeSelector
	eligibleNodes := FilterEligibleNodes(r.cfg, nodes)
	if len(eligibleNodes) == 0 {
		slog.Warn("No eligible nodes found in cluster for report (all nodes excluded by policy)", "report_id", report.ID)
		now := time.Now().UTC()
		_ = r.store.UpdateReportProgress(ctx, report.ID, 0, 0, model.StatusFailed, &now)
		return
	}

	totalNodes := len(eligibleNodes)
	report.TotalNodes = totalNodes
	report.Status = model.StatusRunning
	_ = r.store.UpdateReportProgress(ctx, report.ID, totalNodes, 0, model.StatusRunning, nil)
	slog.Debug("Starting probe execution across nodes", "report_id", report.ID, "total_nodes", totalNodes)

	// 2. Concurrency control: max 25 simultaneous pod creations to protect API server
	concurrency := 25
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	var completedCount int32

	for _, node := range eligibleNodes {
		wg.Add(1)
		go func(n corev1.Node) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			nodeResult := r.probeNode(ctx, report, n)
			if err := r.store.SaveNodeResult(ctx, report.ID, nodeResult); err != nil {
				slog.Warn("Failed to save node result for report", "report_id", report.ID, "node", n.Name, "error", err)
			}

			done := atomic.AddInt32(&completedCount, 1)
			_ = r.store.UpdateReportProgress(ctx, report.ID, totalNodes, int(done), model.StatusRunning, nil)
		}(node)
	}

	wg.Wait()

	now := time.Now().UTC()
	finalStatus := model.StatusCompleted
	if completedCount == 0 {
		finalStatus = model.StatusFailed
	}
	_ = r.store.UpdateReportProgress(context.Background(), report.ID, totalNodes, int(completedCount), finalStatus, &now)
	slog.Info("Report finished", "report_id", report.ID, "completed", completedCount, "total", totalNodes, "status", finalStatus)
}

func (r *Runner) probeNode(ctx context.Context, report *model.Report, node corev1.Node) model.NodeResult {
	start := time.Now().UTC()
	nodeResult := model.NodeResult{
		NodeName:   node.Name,
		NodeIP:     getNodeInternalIP(node),
		NodeRoles:  ExtractNodeRoles(node.Labels),
		Status:     "running",
		FinishedAt: start,
	}

	podNamespace := report.Namespace
	if podNamespace == "" {
		podNamespace = r.cfg.K8s.Namespace
	}

	podManifest, err := BuildProbePod(report, node.Name, podNamespace, r.cfg)
	if err != nil {
		nodeResult.Status = "failed"
		nodeResult.Error = fmt.Sprintf("Building pod manifest: %v", err)
		nodeResult.DurationMs = time.Since(start).Milliseconds()
		nodeResult.FinishedAt = time.Now().UTC()
		return nodeResult
	}

	// Create Pod
	createdPod, err := r.client.CoreV1().Pods(podNamespace).Create(ctx, podManifest, metav1.CreateOptions{})
	if err != nil {
		nodeResult.Status = "failed"
		nodeResult.Error = fmt.Sprintf("Creating probe pod on node %s: %v", node.Name, err)
		nodeResult.DurationMs = time.Since(start).Milliseconds()
		nodeResult.FinishedAt = time.Now().UTC()
		return nodeResult
	}

	podName := createdPod.Name
	// Always ensure cleanup of the probe pod
	defer func() {
		delCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		graceZero := int64(0)
		_ = r.client.CoreV1().Pods(podNamespace).Delete(delCtx, podName, metav1.DeleteOptions{
			GracePeriodSeconds: &graceZero,
		})
	}()

	// Wait for pod completion
	pollInterval := 1 * time.Second
	pollTimeout := time.Duration(r.cfg.K8s.ProbeTimeoutSeconds+10) * time.Second
	deadline := time.Now().Add(pollTimeout)

	for {
		if time.Now().After(deadline) {
			nodeResult.Status = "timeout"
			nodeResult.Error = fmt.Sprintf("Timed out after %v waiting for probe pod %s", pollTimeout, podName)
			break
		}

		select {
		case <-ctx.Done():
			nodeResult.Status = "cancelled"
			nodeResult.Error = ctx.Err().Error()
			return nodeResult
		default:
		}

		currentPod, err := r.client.CoreV1().Pods(podNamespace).Get(ctx, podName, metav1.GetOptions{})
		if err != nil {
			nodeResult.Status = "failed"
			nodeResult.Error = fmt.Sprintf("Getting pod status: %v", err)
			break
		}

		// Check pod phase
		if currentPod.Status.Phase == corev1.PodSucceeded || currentPod.Status.Phase == corev1.PodFailed {
			r.extractPodResults(ctx, currentPod, &nodeResult)
			break
		}

		// Check for container errors (ImagePullBackOff, CrashLoopBackOff, etc.)
		if len(currentPod.Status.ContainerStatuses) > 0 {
			cs := currentPod.Status.ContainerStatuses[0]
			if cs.State.Waiting != nil && cs.State.Waiting.Reason != "" {
				reason := cs.State.Waiting.Reason
				if reason == "ImagePullBackOff" || reason == "ErrImagePull" || reason == "CrashLoopBackOff" {
					nodeResult.Status = "failed"
					nodeResult.Error = fmt.Sprintf("Container failed to start: %s (%s)", reason, cs.State.Waiting.Message)
					break
				}
			}
		}

		time.Sleep(pollInterval)
	}

	nodeResult.DurationMs = time.Since(start).Milliseconds()
	nodeResult.FinishedAt = time.Now().UTC()
	return nodeResult
}

// extractPodResults parses termination-log JSON from pod status or falls back to logs.
func (r *Runner) extractPodResults(ctx context.Context, pod *corev1.Pod, result *model.NodeResult) {
	var rawJSON string

	// 1. Try reading termination log from container status
	if len(pod.Status.ContainerStatuses) > 0 {
		cs := pod.Status.ContainerStatuses[0]
		if cs.State.Terminated != nil && cs.State.Terminated.Message != "" {
			rawJSON = cs.State.Terminated.Message
		}
	}

	// 2. Fallback to pod logs if termination message is empty
	if rawJSON == "" {
		logs, err := r.getPodLogs(ctx, pod.Namespace, pod.Name)
		if err == nil && len(logs) > 0 {
			rawJSON = logs
		}
	}

	if rawJSON == "" {
		result.Status = "failed"
		result.Error = "Probe pod finished without termination output"
		return
	}

	var probeOutputs []model.ProbeOutput
	if err := json.Unmarshal([]byte(rawJSON), &probeOutputs); err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("Parsing probe results JSON: %v (raw: %s)", err, rawJSON)
		return
	}

	for i := range probeOutputs {
		ProcessCertInfo(&probeOutputs[i])
	}

	result.Status = "completed"
	result.ServiceResults = probeOutputs
}

// ProcessCertInfo parses certificate expiration date and calculates days left and status.
func ProcessCertInfo(probe *model.ProbeOutput) {
	if probe.CertExpiry == "" {
		return
	}

	t, err := ParseCertExpiry(probe.CertExpiry)
	if err != nil {
		return
	}

	now := time.Now().UTC()
	diff := t.Sub(now)
	daysLeft := int(diff.Hours() / 24)
	probe.CertDaysLeft = &daysLeft

	if diff < 0 {
		probe.CertStatus = model.CertStatusExpired
		if probe.Error == "" && !probe.Success {
			probe.Error = fmt.Sprintf("TLS certificate expired on %s", t.Format("2006-01-02"))
		}
	} else if daysLeft <= 30 {
		probe.CertStatus = model.CertStatusExpiringSoon
	} else {
		probe.CertStatus = model.CertStatusValid
	}
}

// ParseCertExpiry tries several formats for SSL certificate expiration dates.
func ParseCertExpiry(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	formats := []string{
		"2006-01-02 15:04:05Z",
		"2006-01-02 15:04:05 MST",
		"2006-01-02 15:04:05",
		time.RFC3339,
		"Jan _2 15:04:05 2006 MST",
		"Jan _2 15:04:05 2006",
		"2006-01-02",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, s); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("unable to parse certificate expiry date: %q", s)
}

func (r *Runner) getPodLogs(ctx context.Context, namespace, podName string) (string, error) {
	req := r.client.CoreV1().Pods(namespace).GetLogs(podName, &corev1.PodLogOptions{})
	stream, err := req.Stream(ctx)
	if err != nil {
		return "", err
	}
	defer stream.Close()

	buf := new(bytesBuffer)
	_, err = io.Copy(buf, stream)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

type bytesBuffer struct {
	b []byte
}

func (b *bytesBuffer) Write(p []byte) (n int, err error) {
	b.b = append(b.b, p...)
	return len(p), nil
}

func (b *bytesBuffer) String() string {
	return string(b.b)
}

func getNodeInternalIP(node corev1.Node) string {
	for _, addr := range node.Status.Addresses {
		if addr.Type == corev1.NodeInternalIP {
			return addr.Address
		}
	}
	if len(node.Status.Addresses) > 0 {
		return node.Status.Addresses[0].Address
	}
	return ""
}

const (
	labelNodeRolePrefix = "node-role.kubernetes.io/"
	nodeLabelRole       = "kubernetes.io/role"
)

// ExtractNodeRoles extracts node roles from node labels following the kubectl convention.
// Roles are discovered from labels starting with "node-role.kubernetes.io/" and the legacy
// "kubernetes.io/role" label. The returned list contains unique role names sorted alphabetically.
func ExtractNodeRoles(labels map[string]string) []string {
	if len(labels) == 0 {
		return nil
	}

	roleSet := make(map[string]struct{})
	for k, v := range labels {
		switch {
		case strings.HasPrefix(k, labelNodeRolePrefix):
			if role := strings.TrimPrefix(k, labelNodeRolePrefix); len(role) > 0 {
				roleSet[role] = struct{}{}
			}
		case k == nodeLabelRole && len(v) > 0:
			roleSet[v] = struct{}{}
		}
	}

	if len(roleSet) == 0 {
		return nil
	}

	roles := make([]string, 0, len(roleSet))
	for role := range roleSet {
		roles = append(roles, role)
	}
	sort.Strings(roles)
	return roles
}
