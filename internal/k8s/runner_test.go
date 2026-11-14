package k8s

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"kncc/internal/config"
	"kncc/internal/model"
	"kncc/internal/store"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func setupTestRunner(t *testing.T) (*Runner, *fake.Clientset, store.Store, func()) {
	t.Helper()
	dir, err := os.MkdirTemp("", "kncc-runner-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	dbPath := filepath.Join(dir, "test.db")
	s, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}

	fakeClient := fake.NewSimpleClientset()
	cfg := config.DefaultConfig()
	cfg.K8s.ProbeTimeoutSeconds = 2

	runner := NewRunner(fakeClient, s, cfg)

	cleanup := func() {
		_ = s.Close()
		_ = os.RemoveAll(dir)
	}

	return runner, fakeClient, s, cleanup
}

func TestRunner_ExecuteCheck_Success(t *testing.T) {
	runner, fakeClient, s, cleanup := setupTestRunner(t)
	defer cleanup()

	ctx := context.Background()

	// 1. Add fake nodes
	node1 := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
		Status: corev1.NodeStatus{
			Addresses: []corev1.NodeAddress{
				{Type: corev1.NodeInternalIP, Address: "192.168.1.10"},
			},
		},
	}
	node2 := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-2"},
		Status: corev1.NodeStatus{
			Addresses: []corev1.NodeAddress{
				{Type: corev1.NodeInternalIP, Address: "192.168.1.11"},
			},
		},
	}
	_, _ = fakeClient.CoreV1().Nodes().Create(ctx, node1, metav1.CreateOptions{})
	_, _ = fakeClient.CoreV1().Nodes().Create(ctx, node2, metav1.CreateOptions{})

	// 2. Create report in store
	target := model.TargetService{
		Protocol: model.ProtocolTCP,
		Host:     "10.0.0.1",
		Port:     80,
	}
	report := &model.Report{
		ID:        "rep-test-1",
		UserID:    "user-1",
		Username:  "tester",
		Mode:      model.ModeHostNetwork,
		Namespace: "kncc",
		Targets:   []model.TargetService{target},
		Status:    model.StatusPending,
		CreatedAt: time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(1 * time.Hour),
	}
	if err := s.CreateReport(ctx, report); err != nil {
		t.Fatalf("CreateReport failed: %v", err)
	}

	// 3. Mock probe pod simulator in background
	// When pod is created, update its phase to Succeeded and set termination message
	stopSimulator := make(chan struct{})
	go func() {
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stopSimulator:
				return
			case <-ticker.C:
				pods, err := fakeClient.CoreV1().Pods("kncc").List(context.Background(), metav1.ListOptions{})
				if err != nil {
					continue
				}
				for _, p := range pods.Items {
					if p.Status.Phase == "" || p.Status.Phase == corev1.PodPending {
						probeOutputs := []model.ProbeOutput{
							{
								Target:     target,
								Success:    true,
								LatencyMs:  15,
								ResolvedIP: "10.0.0.1",
								Details:    "TCP connection established",
							},
						}
						outJSON, _ := json.Marshal(probeOutputs)
						p.Status.Phase = corev1.PodSucceeded
						p.Status.ContainerStatuses = []corev1.ContainerStatus{
							{
								Name: "kncc-probe",
								State: corev1.ContainerState{
									Terminated: &corev1.ContainerStateTerminated{
										ExitCode: 0,
										Message:  string(outJSON),
									},
								},
							},
						}
						_, _ = fakeClient.CoreV1().Pods("kncc").UpdateStatus(context.Background(), &p, metav1.UpdateOptions{})
					}
				}
			}
		}
	}()

	// 4. Run synchronous check
	runner.run(ctx, report)
	close(stopSimulator)

	// 5. Verify report in store
	updatedReport, err := s.GetReport(ctx, report.ID)
	if err != nil {
		t.Fatalf("GetReport failed: %v", err)
	}

	if updatedReport.Status != model.StatusCompleted {
		t.Errorf("expected status completed, got %s", updatedReport.Status)
	}
	if updatedReport.TotalNodes != 2 {
		t.Errorf("expected 2 total nodes, got %d", updatedReport.TotalNodes)
	}
	if updatedReport.CompletedNodes != 2 {
		t.Errorf("expected 2 completed nodes, got %d", updatedReport.CompletedNodes)
	}
	if len(updatedReport.NodeResults) != 2 {
		t.Errorf("expected 2 node results, got %d", len(updatedReport.NodeResults))
	}

	for nodeName, res := range updatedReport.NodeResults {
		if res.Status != "completed" {
			t.Errorf("node %s status = %s, want completed", nodeName, res.Status)
		}
		if len(res.ServiceResults) != 1 || !res.ServiceResults[0].Success {
			t.Errorf("node %s service results failed: %+v", nodeName, res.ServiceResults)
		}
	}
}

func TestGarbageCollector(t *testing.T) {
	fakeClient := fake.NewSimpleClientset()
	ctx := context.Background()

	// 1. Create stale finished pod
	stalePod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "stale-probe-1",
			Namespace:         "kncc",
			Labels:            map[string]string{"app.kubernetes.io/managed-by": "kncc"},
			CreationTimestamp: metav1.NewTime(time.Now().Add(-10 * time.Minute)),
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodSucceeded,
		},
	}
	_, _ = fakeClient.CoreV1().Pods("kncc").Create(ctx, stalePod, metav1.CreateOptions{})

	// 2. Create active pod of another app (should not be touched)
	otherPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "other-app",
			Namespace: "kncc",
			Labels:    map[string]string{"app": "other"},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}
	_, _ = fakeClient.CoreV1().Pods("kncc").Create(ctx, otherPod, metav1.CreateOptions{})

	// Run GC cleanup with staleThreshold 3m
	cleanStalePods(ctx, fakeClient, "kncc", false, 3*time.Minute)

	// Verify stalePod was deleted
	pods, _ := fakeClient.CoreV1().Pods("kncc").List(ctx, metav1.ListOptions{})
	if len(pods.Items) != 1 || pods.Items[0].Name != "other-app" {
		t.Errorf("expected only other-app to remain, got %d pods", len(pods.Items))
	}
}

func TestProcessCertInfo(t *testing.T) {
	now := time.Now().UTC()

	tests := []struct {
		name           string
		expiryStr      string
		expectedStatus model.CertStatus
		expectDaysLeft bool
		checkDays      func(days int) bool
	}{
		{
			name:           "Valid certificate expiring next year",
			expiryStr:      now.Add(180 * 24 * time.Hour).Format("2006-01-02 15:04:05Z"),
			expectedStatus: model.CertStatusValid,
			expectDaysLeft: true,
			checkDays:      func(d int) bool { return d >= 179 && d <= 181 },
		},
		{
			name:           "Certificate expiring soon (15 days)",
			expiryStr:      now.Add(15 * 24 * time.Hour).Format("2006-01-02 15:04:05Z"),
			expectedStatus: model.CertStatusExpiringSoon,
			expectDaysLeft: true,
			checkDays:      func(d int) bool { return d >= 14 && d <= 16 },
		},
		{
			name:           "Expired certificate (10 days ago)",
			expiryStr:      now.Add(-10 * 24 * time.Hour).Format("2006-01-02 15:04:05Z"),
			expectedStatus: model.CertStatusExpired,
			expectDaysLeft: true,
			checkDays:      func(d int) bool { return d < 0 },
		},
		{
			name:           "Empty expiry (e.g. non-HTTPS)",
			expiryStr:      "",
			expectedStatus: "",
			expectDaysLeft: false,
		},
		{
			name:           "RFC3339 format",
			expiryStr:      now.Add(60 * 24 * time.Hour).Format(time.RFC3339),
			expectedStatus: model.CertStatusValid,
			expectDaysLeft: true,
			checkDays:      func(d int) bool { return d >= 59 && d <= 61 },
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			probe := model.ProbeOutput{
				Target: model.TargetService{
					Protocol: model.ProtocolHTTPS,
					Host:     "example.com",
					Port:     443,
				},
				Success:    true,
				CertExpiry: tc.expiryStr,
			}

			ProcessCertInfo(&probe)

			if probe.CertStatus != tc.expectedStatus {
				t.Errorf("CertStatus = %q, want %q", probe.CertStatus, tc.expectedStatus)
			}

			if tc.expectDaysLeft {
				if probe.CertDaysLeft == nil {
					t.Fatalf("expected CertDaysLeft to be non-nil")
				}
				if tc.checkDays != nil && !tc.checkDays(*probe.CertDaysLeft) {
					t.Errorf("CertDaysLeft = %d does not satisfy expectation", *probe.CertDaysLeft)
				}
			} else {
				if probe.CertDaysLeft != nil {
					t.Errorf("expected CertDaysLeft to be nil, got %d", *probe.CertDaysLeft)
				}
			}
		})
	}
}

func TestExtractNodeRoles(t *testing.T) {
	tests := []struct {
		name     string
		labels   map[string]string
		expected []string
	}{
		{
			name:     "Nil labels",
			labels:   nil,
			expected: nil,
		},
		{
			name:     "Empty labels",
			labels:   map[string]string{},
			expected: nil,
		},
		{
			name: "Unrelated labels only",
			labels: map[string]string{
				"kubernetes.io/hostname": "node-1",
				"beta.kubernetes.io/os":  "linux",
			},
			expected: nil,
		},
		{
			name: "Single control-plane role",
			labels: map[string]string{
				"node-role.kubernetes.io/control-plane": "true",
			},
			expected: []string{"control-plane"},
		},
		{
			name: "Multiple roles with sorting",
			labels: map[string]string{
				"node-role.kubernetes.io/worker": "",
				"node-role.kubernetes.io/infra":  "",
			},
			expected: []string{"infra", "worker"},
		},
		{
			name: "Legacy kubernetes.io/role label",
			labels: map[string]string{
				"kubernetes.io/role": "master",
			},
			expected: []string{"master"},
		},
		{
			name: "Deduplication of modern and legacy label",
			labels: map[string]string{
				"node-role.kubernetes.io/worker": "",
				"kubernetes.io/role":            "worker",
			},
			expected: []string{"worker"},
		},
		{
			name: "Combined modern prefix and legacy label",
			labels: map[string]string{
				"node-role.kubernetes.io/control-plane": "true",
				"kubernetes.io/role":                    "master",
			},
			expected: []string{"control-plane", "master"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ExtractNodeRoles(tc.labels)
			if len(got) != len(tc.expected) {
				t.Fatalf("ExtractNodeRoles() = %v (len %d), want %v (len %d)", got, len(got), tc.expected, len(tc.expected))
			}
			for i := range got {
				if got[i] != tc.expected[i] {
					t.Errorf("ExtractNodeRoles()[%d] = %q, want %q", i, got[i], tc.expected[i])
				}
			}
		})
	}
}

func TestRunner_ExecuteCheck_NodeExclude(t *testing.T) {
	runner, fakeClient, s, cleanup := setupTestRunner(t)
	defer cleanup()

	ctx := context.Background()

	// Configure exclusion by selector and by name
	runner.cfg.K8s.NodeExcludeSelector = "node-role.kubernetes.io/control-plane=true"
	runner.cfg.K8s.NodeExcludeList = []string{"node-audit"}

	node1 := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "node-master",
			Labels: map[string]string{"node-role.kubernetes.io/control-plane": "true"},
		},
		Status: corev1.NodeStatus{
			Addresses: []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: "192.168.1.10"}},
		},
	}
	node2 := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "node-audit",
			Labels: map[string]string{"node-role.kubernetes.io/worker": "true"},
		},
		Status: corev1.NodeStatus{
			Addresses: []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: "192.168.1.11"}},
		},
	}
	node3 := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "node-worker",
			Labels: map[string]string{"node-role.kubernetes.io/worker": "true"},
		},
		Status: corev1.NodeStatus{
			Addresses: []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: "192.168.1.12"}},
		},
	}
	_, _ = fakeClient.CoreV1().Nodes().Create(ctx, node1, metav1.CreateOptions{})
	_, _ = fakeClient.CoreV1().Nodes().Create(ctx, node2, metav1.CreateOptions{})
	_, _ = fakeClient.CoreV1().Nodes().Create(ctx, node3, metav1.CreateOptions{})

	report := &model.Report{
		ID:        "rep-test-exclude",
		UserID:    "user-1",
		Username:  "tester",
		Mode:      model.ModeHostNetwork,
		Namespace: "kncc",
		Targets: []model.TargetService{
			{Protocol: model.ProtocolTCP, Host: "10.0.0.1", Port: 80},
		},
		Status:    model.StatusPending,
		CreatedAt: time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(1 * time.Hour),
	}
	if err := s.CreateReport(ctx, report); err != nil {
		t.Fatalf("CreateReport failed: %v", err)
	}

	stopSimulator := make(chan struct{})
	go func() {
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stopSimulator:
				return
			case <-ticker.C:
				pods, err := fakeClient.CoreV1().Pods("kncc").List(context.Background(), metav1.ListOptions{})
				if err != nil {
					continue
				}
				for _, p := range pods.Items {
					if p.Status.Phase == "" || p.Status.Phase == corev1.PodPending {
						probeOutputs := []model.ProbeOutput{
							{
								Target: model.TargetService{
									Protocol: model.ProtocolTCP,
									Host:     "10.0.0.1",
									Port:     80,
								},
								Success:   true,
								LatencyMs: 5,
							},
						}
						outJSON, _ := json.Marshal(probeOutputs)
						p.Status.Phase = corev1.PodSucceeded
						p.Status.ContainerStatuses = []corev1.ContainerStatus{
							{
								Name: "kncc-probe",
								State: corev1.ContainerState{
									Terminated: &corev1.ContainerStateTerminated{
										ExitCode: 0,
										Message:  string(outJSON),
									},
								},
							},
						}
						_, _ = fakeClient.CoreV1().Pods("kncc").UpdateStatus(context.Background(), &p, metav1.UpdateOptions{})
					}
				}
			}
		}
	}()

	runner.run(ctx, report)
	close(stopSimulator)

	updatedReport, err := s.GetReport(ctx, report.ID)
	if err != nil {
		t.Fatalf("GetReport failed: %v", err)
	}

	if updatedReport.Status != model.StatusCompleted {
		t.Errorf("expected status completed, got %s", updatedReport.Status)
	}
	// TotalNodes should only be 1 (node-worker)
	if updatedReport.TotalNodes != 1 {
		t.Errorf("expected 1 total node, got %d", updatedReport.TotalNodes)
	}
	if updatedReport.CompletedNodes != 1 {
		t.Errorf("expected 1 completed node, got %d", updatedReport.CompletedNodes)
	}
	if _, exists := updatedReport.NodeResults["node-worker"]; !exists {
		t.Errorf("expected node-worker in node results")
	}
	if _, exists := updatedReport.NodeResults["node-master"]; exists {
		t.Errorf("expected node-master to be excluded from results")
	}
	if _, exists := updatedReport.NodeResults["node-audit"]; exists {
		t.Errorf("expected node-audit to be excluded from results")
	}
}
