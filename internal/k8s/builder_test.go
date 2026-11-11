package k8s

import (
	"testing"

	"kncc/internal/config"
	"kncc/internal/model"

	corev1 "k8s.io/api/core/v1"
)

func TestBuildProbePod(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.K8s.ProbeImage = "custom/netshoot:latest"
	cfg.K8s.ProbeActiveDeadlineSeconds = 45

	report := &model.Report{
		ID:   "a1b2c3d4-e5f6-7890",
		Mode: model.ModeHostNetwork,
		Targets: []model.TargetService{
			{Protocol: model.ProtocolTCP, Host: "1.1.1.1", Port: 80, TimeoutSeconds: 2},
		},
	}

	pod, err := BuildProbePod(report, "node-worker-01", "kncc", cfg)
	if err != nil {
		t.Fatalf("BuildProbePod failed: %v", err)
	}

	if pod.Spec.NodeName != "node-worker-01" {
		t.Errorf("expected nodeName node-worker-01, got %s", pod.Spec.NodeName)
	}
	if !pod.Spec.HostNetwork {
		t.Errorf("expected HostNetwork to be true for host mode")
	}
	if *pod.Spec.ActiveDeadlineSeconds != 45 {
		t.Errorf("expected ActiveDeadlineSeconds 45, got %d", *pod.Spec.ActiveDeadlineSeconds)
	}
	if len(pod.Spec.Tolerations) == 0 || pod.Spec.Tolerations[0].Operator != corev1.TolerationOpExists {
		t.Errorf("expected operator Exists in tolerations")
	}

	container := pod.Spec.Containers[0]
	if container.Image != "custom/netshoot:latest" {
		t.Errorf("expected image custom/netshoot:latest, got %s", container.Image)
	}
	if container.TerminationMessagePolicy != corev1.TerminationMessageFallbackToLogsOnError {
		t.Errorf("expected FallbackToLogsOnError termination policy")
	}

	// Verify pod name length adheres to Kubernetes 63 char limit
	if len(pod.Name) > 63 {
		t.Errorf("pod name length %d exceeds 63 characters: %s", len(pod.Name), pod.Name)
	}
}

func TestBuildProbePod_PodNetwork(t *testing.T) {
	cfg := config.DefaultConfig()
	report := &model.Report{
		ID:        "rep-1234",
		Mode:      model.ModePodNetwork,
		Namespace: "tenant-dev",
		Targets: []model.TargetService{
			{Protocol: model.ProtocolTCP, Host: "10.0.0.1", Port: 80},
		},
	}

	pod, err := BuildProbePod(report, "worker-node", "tenant-dev", cfg)
	if err != nil {
		t.Fatalf("BuildProbePod failed: %v", err)
	}

	if pod.Namespace != "tenant-dev" {
		t.Errorf("expected namespace tenant-dev, got %s", pod.Namespace)
	}
	if pod.Spec.HostNetwork {
		t.Errorf("expected HostNetwork to be false in pod network mode")
	}
}

func TestBuildProbePod_WithCustomCA(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.K8s.CustomCABundle = "-----BEGIN CERTIFICATE-----\nTEST_CORP_BUNDLE\n-----END CERTIFICATE-----"

	report := &model.Report{
		ID:        "rep-ca-test",
		Mode:      model.ModePodNetwork,
		Namespace: "kncc",
		Targets: []model.TargetService{
			{Protocol: model.ProtocolHTTPS, Host: "int.corp.com", Port: 443},
		},
	}

	pod, err := BuildProbePod(report, "worker-ca", "kncc", cfg)
	if err != nil {
		t.Fatalf("BuildProbePod failed: %v", err)
	}

	container := pod.Spec.Containers[0]
	cmdJoined := ""
	for _, c := range container.Command {
		cmdJoined += c + " "
	}

	if !corev1HasSubstring(cmdJoined, "TEST_CORP_BUNDLE") {
		t.Errorf("expected probe pod command script to contain custom CA bundle")
	}
}

func TestBuildProbePod_WithKnccAgentImage(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.K8s.ProbeImage = "ghcr.io/AleksandrSumkin/kncc-agent:latest"

	report := &model.Report{
		ID:        "rep-agent-test",
		Mode:      model.ModeHostNetwork,
		Namespace: "kncc",
		Targets: []model.TargetService{
			{Protocol: model.ProtocolTCP, Host: "10.0.0.1", Port: 80},
			{Protocol: model.ProtocolHTTPS, Host: "secure.example.com", Port: 443},
			{Protocol: model.ProtocolICMP, Host: "8.8.8.8", Port: 0},
		},
	}

	pod, err := BuildProbePod(report, "worker-node-1", "kncc", cfg)
	if err != nil {
		t.Fatalf("BuildProbePod failed: %v", err)
	}

	if pod.Spec.Containers[0].Image != "ghcr.io/AleksandrSumkin/kncc-agent:latest" {
		t.Errorf("expected image ghcr.io/AleksandrSumkin/kncc-agent:latest, got %s", pod.Spec.Containers[0].Image)
	}
	if !pod.Spec.HostNetwork {
		t.Errorf("expected HostNetwork to be true")
	}
	if len(pod.Spec.Containers[0].SecurityContext.Capabilities.Add) == 0 ||
		pod.Spec.Containers[0].SecurityContext.Capabilities.Add[0] != corev1.Capability("NET_RAW") {
		t.Errorf("expected NET_RAW capability to be added")
	}
	if pod.Spec.Containers[0].SecurityContext.AllowPrivilegeEscalation == nil ||
		!*pod.Spec.Containers[0].SecurityContext.AllowPrivilegeEscalation {
		t.Errorf("expected AllowPrivilegeEscalation to be true by default")
	}
}

func TestBuildProbePod_AllowPrivilegeEscalationConfig(t *testing.T) {
	report := &model.Report{
		ID:        "rep-priv-test",
		Mode:      model.ModeHostNetwork,
		Namespace: "kncc",
		Targets: []model.TargetService{
			{Protocol: model.ProtocolICMP, Host: "8.8.8.8"},
		},
	}

	// 1. Explicitly disabled via config
	f := false
	cfg := config.DefaultConfig()
	cfg.K8s.ProbeAllowPrivilegeEscalation = &f

	podDisabled, err := BuildProbePod(report, "node-1", "kncc", cfg)
	if err != nil {
		t.Fatalf("BuildProbePod failed: %v", err)
	}
	if podDisabled.Spec.Containers[0].SecurityContext.AllowPrivilegeEscalation == nil ||
		*podDisabled.Spec.Containers[0].SecurityContext.AllowPrivilegeEscalation != false {
		t.Errorf("expected AllowPrivilegeEscalation to be false when configured")
	}

	// 2. nil cfg defaults to true
	podNilCfg, err := BuildProbePod(report, "node-1", "kncc", nil)
	if err != nil {
		t.Fatalf("BuildProbePod with nil cfg failed: %v", err)
	}
	if podNilCfg.Spec.Containers[0].SecurityContext.AllowPrivilegeEscalation == nil ||
		*podNilCfg.Spec.Containers[0].SecurityContext.AllowPrivilegeEscalation != true {
		t.Errorf("expected AllowPrivilegeEscalation to be true when cfg is nil")
	}
}

func corev1HasSubstring(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
