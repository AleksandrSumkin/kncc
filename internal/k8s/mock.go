package k8s

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"kncc/internal/config"
	"kncc/internal/model"
	"kncc/internal/store"
)

// MockRunner simulates Kubernetes node probe execution without a real cluster.
type MockRunner struct {
	store store.Store
	cfg   *config.Config
}

// NewMockRunner creates a simulated runner.
func NewMockRunner(s store.Store, cfg *config.Config) *MockRunner {
	return &MockRunner{
		store: s,
		cfg:   cfg,
	}
}

// MockNode holds mock node metadata.
type MockNode struct {
	Name   string
	IP     string
	Labels map[string]string
	Roles  []string
}

var defaultMockNodes = []MockNode{
	{
		Name:   "k8s-control-plane-01",
		IP:     "10.0.0.1",
		Labels: map[string]string{"node-role.kubernetes.io/control-plane": "true"},
		Roles:  []string{"control-plane"},
	},
	{
		Name:   "k8s-worker-01",
		IP:     "10.0.0.1",
		Labels: map[string]string{"node-role.kubernetes.io/worker": "true"},
		Roles:  []string{"worker"},
	},
	{
		Name:   "k8s-worker-02",
		IP:     "10.0.0.1",
		Labels: map[string]string{"node-role.kubernetes.io/worker": "true"},
		Roles:  []string{"worker"},
	},
	{
		Name:   "k8s-worker-03",
		IP:     "10.0.0.1",
		Labels: map[string]string{"node-role.kubernetes.io/worker": "true", "security.zone": "audit"},
		Roles:  []string{"worker"},
	},
}

// ExecuteCheck simulates asynchronous probe execution across mock cluster nodes.
func (m *MockRunner) ExecuteCheck(ctx context.Context, report *model.Report) {
	go func() {
		nodes := FilterMockNodes(m.cfg, defaultMockNodes)
		if len(nodes) == 0 {
			now := time.Now().UTC()
			_ = m.store.UpdateReportProgress(context.Background(), report.ID, 0, 0, model.StatusFailed, &now)
			return
		}

		totalNodes := len(nodes)
		report.TotalNodes = totalNodes
		report.Status = model.StatusRunning
		_ = m.store.UpdateReportProgress(context.Background(), report.ID, totalNodes, 0, model.StatusRunning, nil)

		completed := 0
		for _, node := range nodes {
			// Simulate pod scheduling & execution delay
			time.Sleep(600 * time.Millisecond)

			nodeRes := m.simulateNode(node, report)
			_ = m.store.SaveNodeResult(context.Background(), report.ID, nodeRes)

			completed++
			_ = m.store.UpdateReportProgress(context.Background(), report.ID, totalNodes, completed, model.StatusRunning, nil)
		}

		now := time.Now().UTC()
		_ = m.store.UpdateReportProgress(context.Background(), report.ID, totalNodes, completed, model.StatusCompleted, &now)
	}()
}

func (m *MockRunner) simulateNode(node MockNode, report *model.Report) model.NodeResult {
	start := time.Now().UTC()
	var serviceResults []model.ProbeOutput

	for _, target := range report.Targets {
		out := model.ProbeOutput{
			Target: target,
		}

		// Node 3 simulates a firewall or routing block for demonstration
		isWorker3Blocked := node.Name == "k8s-worker-03" && (target.Port == 5432 || target.Port == 6379 || strings.Contains(target.Host, "corp"))

		if isWorker3Blocked {
			out.Success = false
			out.LatencyMs = int64(1500 + rand.Intn(500))
			out.ResolvedIP = "10.40.12.5"
			out.Error = fmt.Sprintf("Connection timed out after 3s (SYN sent, no ACK received - check Calico/Cilium NetworkPolicy or SecurityGroup on %s)", node.Name)
		} else {
			out.Success = true
			out.LatencyMs = int64(8 + rand.Intn(22))
			if target.Protocol == model.ProtocolHTTP || target.Protocol == model.ProtocolHTTPS {
				out.HTTPStatusCode = 200
				out.Details = fmt.Sprintf("HTTP 200 OK via %s", node.Name)
				out.ResolvedIP = "10.0.4.15"
			} else if target.Protocol == model.ProtocolICMP {
				out.Details = fmt.Sprintf("ICMP Echo Reply (avg rtt: %dms) via %s", out.LatencyMs, node.Name)
				out.ResolvedIP = "8.8.8.8"
			} else {
				out.Details = fmt.Sprintf("TCP handshake completed (SYN/ACK) via %s", node.Name)
				out.ResolvedIP = "10.20.30.40"
			}
		}

		serviceResults = append(serviceResults, out)
	}

	return model.NodeResult{
		NodeName:       node.Name,
		NodeIP:         node.IP,
		NodeRoles:      node.Roles,
		Status:         "completed",
		ServiceResults: serviceResults,
		DurationMs:     time.Since(start).Milliseconds(),
		FinishedAt:     time.Now().UTC(),
	}
}

// SeedDemoData populates the store with demo reports for immediate exploration.
func SeedDemoData(s store.Store, cfg *config.Config) error {
	ctx := context.Background()

	// 1. Demo Report 1: All green (Healthy API & DNS)
	rep1ID := "demo-report-01-healthy"
	if _, err := s.GetReport(ctx, rep1ID); err != nil {
		now := time.Now().UTC().Add(-25 * time.Minute)
		comp := now.Add(2 * time.Second)
		rep1 := &model.Report{
			ID:             rep1ID,
			UserID:         "user-admin",
			Username:       "admin",
			Mode:           model.ModeHostNetwork,
			Namespace:      "kncc",
			Targets: []model.TargetService{
				{Protocol: model.ProtocolHTTPS, Host: "api.github.com", Port: 443, Path: "/healthz", TimeoutSeconds: 5},
				{Protocol: model.ProtocolICMP, Host: "8.8.8.8", TimeoutSeconds: 2},
			},
			Status:         model.StatusCompleted,
			TotalNodes:     4,
			CompletedNodes: 4,
			NodeResults: map[string]model.NodeResult{
				"k8s-control-plane-01": {
					NodeName: "k8s-control-plane-01", NodeIP: "10.0.0.1", NodeRoles: []string{"control-plane"}, Status: "completed", DurationMs: 140, FinishedAt: comp,
					ServiceResults: []model.ProbeOutput{
						{Target: model.TargetService{Protocol: model.ProtocolHTTPS, Host: "api.github.com", Port: 443, Path: "/healthz"}, Success: true, LatencyMs: 24, ResolvedIP: "140.82.121.4", HTTPStatusCode: 200, Details: "HTTP 200 OK"},
						{Target: model.TargetService{Protocol: model.ProtocolICMP, Host: "8.8.8.8"}, Success: true, LatencyMs: 12, ResolvedIP: "8.8.8.8", Details: "ICMP Echo Reply"},
					},
				},
				"k8s-worker-01": {
					NodeName: "k8s-worker-01", NodeIP: "10.0.0.1", NodeRoles: []string{"worker"}, Status: "completed", DurationMs: 110, FinishedAt: comp,
					ServiceResults: []model.ProbeOutput{
						{Target: model.TargetService{Protocol: model.ProtocolHTTPS, Host: "api.github.com", Port: 443, Path: "/healthz"}, Success: true, LatencyMs: 18, ResolvedIP: "140.82.121.4", HTTPStatusCode: 200, Details: "HTTP 200 OK"},
						{Target: model.TargetService{Protocol: model.ProtocolICMP, Host: "8.8.8.8"}, Success: true, LatencyMs: 9, ResolvedIP: "8.8.8.8", Details: "ICMP Echo Reply"},
					},
				},
				"k8s-worker-02": {
					NodeName: "k8s-worker-02", NodeIP: "10.0.0.1", NodeRoles: []string{"worker"}, Status: "completed", DurationMs: 115, FinishedAt: comp,
					ServiceResults: []model.ProbeOutput{
						{Target: model.TargetService{Protocol: model.ProtocolHTTPS, Host: "api.github.com", Port: 443, Path: "/healthz"}, Success: true, LatencyMs: 21, ResolvedIP: "140.82.121.4", HTTPStatusCode: 200, Details: "HTTP 200 OK"},
						{Target: model.TargetService{Protocol: model.ProtocolICMP, Host: "8.8.8.8"}, Success: true, LatencyMs: 11, ResolvedIP: "8.8.8.8", Details: "ICMP Echo Reply"},
					},
				},
				"k8s-worker-03": {
					NodeName: "k8s-worker-03", NodeIP: "10.0.0.1", NodeRoles: []string{"worker"}, Status: "completed", DurationMs: 125, FinishedAt: comp,
					ServiceResults: []model.ProbeOutput{
						{Target: model.TargetService{Protocol: model.ProtocolHTTPS, Host: "api.github.com", Port: 443, Path: "/healthz"}, Success: true, LatencyMs: 25, ResolvedIP: "140.82.121.4", HTTPStatusCode: 200, Details: "HTTP 200 OK"},
						{Target: model.TargetService{Protocol: model.ProtocolICMP, Host: "8.8.8.8"}, Success: true, LatencyMs: 14, ResolvedIP: "8.8.8.8", Details: "ICMP Echo Reply"},
					},
				},
			},
			CreatedAt: now,
			CompletedAt: &comp,
			ExpiresAt: now.Add(cfg.Storage.ReportTTL),
		}
		_ = s.CreateReport(ctx, rep1)
	}

	// 2. Demo Report 2: NetworkPolicy blocked demonstration on worker-03
	rep2ID := "demo-report-02-blocked"
	if _, err := s.GetReport(ctx, rep2ID); err != nil {
		now := time.Now().UTC().Add(-10 * time.Minute)
		comp := now.Add(3 * time.Second)
		rep2 := &model.Report{
			ID:             rep2ID,
			UserID:         "user-admin",
			Username:       "admin",
			Mode:           model.ModePodNetwork,
			Namespace:      "billing-app",
			Targets: []model.TargetService{
				{Protocol: model.ProtocolTCP, Host: "postgres.corp.internal", Port: 5432, TimeoutSeconds: 3},
				{Protocol: model.ProtocolTCP, Host: "redis.corp.internal", Port: 6379, TimeoutSeconds: 3},
			},
			Status:         model.StatusCompleted,
			TotalNodes:     4,
			CompletedNodes: 4,
			NodeResults: map[string]model.NodeResult{
				"k8s-control-plane-01": {
					NodeName: "k8s-control-plane-01", NodeIP: "10.0.0.1", NodeRoles: []string{"control-plane"}, Status: "completed", DurationMs: 95, FinishedAt: comp,
					ServiceResults: []model.ProbeOutput{
						{Target: model.TargetService{Protocol: model.ProtocolTCP, Host: "postgres.corp.internal", Port: 5432}, Success: true, LatencyMs: 8, ResolvedIP: "10.50.1.20", Details: "TCP connection established"},
						{Target: model.TargetService{Protocol: model.ProtocolTCP, Host: "redis.corp.internal", Port: 6379}, Success: true, LatencyMs: 6, ResolvedIP: "10.50.1.30", Details: "TCP connection established"},
					},
				},
				"k8s-worker-01": {
					NodeName: "k8s-worker-01", NodeIP: "10.0.0.1", NodeRoles: []string{"worker"}, Status: "completed", DurationMs: 80, FinishedAt: comp,
					ServiceResults: []model.ProbeOutput{
						{Target: model.TargetService{Protocol: model.ProtocolTCP, Host: "postgres.corp.internal", Port: 5432}, Success: true, LatencyMs: 12, ResolvedIP: "10.50.1.20", Details: "TCP connection established"},
						{Target: model.TargetService{Protocol: model.ProtocolTCP, Host: "redis.corp.internal", Port: 6379}, Success: true, LatencyMs: 7, ResolvedIP: "10.50.1.30", Details: "TCP connection established"},
					},
				},
				"k8s-worker-02": {
					NodeName: "k8s-worker-02", NodeIP: "10.0.0.1", NodeRoles: []string{"worker"}, Status: "completed", DurationMs: 85, FinishedAt: comp,
					ServiceResults: []model.ProbeOutput{
						{Target: model.TargetService{Protocol: model.ProtocolTCP, Host: "postgres.corp.internal", Port: 5432}, Success: true, LatencyMs: 10, ResolvedIP: "10.50.1.20", Details: "TCP connection established"},
						{Target: model.TargetService{Protocol: model.ProtocolTCP, Host: "redis.corp.internal", Port: 6379}, Success: true, LatencyMs: 9, ResolvedIP: "10.50.1.30", Details: "TCP connection established"},
					},
				},
				"k8s-worker-03": {
					NodeName: "k8s-worker-03", NodeIP: "10.0.0.1", NodeRoles: []string{"worker"}, Status: "completed", DurationMs: 3100, FinishedAt: comp,
					ServiceResults: []model.ProbeOutput{
						{
							Target: model.TargetService{Protocol: model.ProtocolTCP, Host: "postgres.corp.internal", Port: 5432},
							Success: false, LatencyMs: 3000, ResolvedIP: "10.50.1.20",
							Error: "Connection timed out (SYN sent, no ACK received - check Calico/Cilium NetworkPolicy or SecurityGroup on k8s-worker-03)",
						},
						{
							Target: model.TargetService{Protocol: model.ProtocolTCP, Host: "redis.corp.internal", Port: 6379},
							Success: true, LatencyMs: 11, ResolvedIP: "10.50.1.30",
							Details: "TCP connection established",
						},
					},
				},
			},
			CreatedAt: now,
			CompletedAt: &comp,
			ExpiresAt: now.Add(cfg.Storage.ReportTTL),
		}
		_ = s.CreateReport(ctx, rep2)
	}

	return nil
}
