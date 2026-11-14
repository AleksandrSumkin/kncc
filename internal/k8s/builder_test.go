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

func TestBuildProbePod_CustomAgentConfig(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.K8s.ProbeImagePullPolicy = "Always"
	cfg.K8s.ProbeImagePullSecrets = []string{"regcred-1", "regcred-2"}
	cfg.K8s.ProbePriorityClassName = "high-priority"
	cfg.K8s.ProbeExtraLabels = map[string]string{
		"team":        "netops",
		"environment": "testing",
	}
	cfg.K8s.ProbeExtraAnnotations = map[string]string{
		"sidecar.istio.io/inject": "false",
	}
	cfg.K8s.ProbeResources = &config.ProbeResourceRequirements{
		Requests: map[string]string{"cpu": "20m", "memory": "64Mi"},
		Limits:   map[string]string{"cpu": "200m", "memory": "256Mi"},
	}
	uid := int64(10001)
	gid := int64(10001)
	nonRoot := true
	cfg.K8s.ProbePodSecurityContext = &config.ProbePodSecurityContextConfig{
		RunAsUser:    &uid,
		RunAsGroup:   &gid,
		RunAsNonRoot: &nonRoot,
		FSGroup:      &gid,
	}
	readOnly := true
	priv := false
	cfg.K8s.ProbeSecurityContext = &config.ProbeSecurityContextConfig{
		ReadOnlyRootFilesystem: &readOnly,
		Privileged:             &priv,
	}

	report := &model.Report{
		ID:        "rep-custom-cfg",
		Mode:      model.ModeHostNetwork,
		Namespace: "kncc",
		Targets: []model.TargetService{
			{Protocol: model.ProtocolTCP, Host: "10.0.0.1", Port: 80},
		},
	}

	pod, err := BuildProbePod(report, "node-custom", "kncc", cfg)
	if err != nil {
		t.Fatalf("BuildProbePod failed: %v", err)
	}

	// 1. Labels
	if pod.Labels["team"] != "netops" || pod.Labels["environment"] != "testing" {
		t.Errorf("missing extra labels: %v", pod.Labels)
	}
	if pod.Labels["app.kubernetes.io/name"] != "kncc-probe" {
		t.Errorf("expected default label to be preserved")
	}

	// 2. Annotations
	if pod.Annotations["sidecar.istio.io/inject"] != "false" {
		t.Errorf("missing extra annotation: %v", pod.Annotations)
	}

	// 3. PriorityClassName
	if pod.Spec.PriorityClassName != "high-priority" {
		t.Errorf("expected PriorityClassName high-priority, got %s", pod.Spec.PriorityClassName)
	}

	// 4. ImagePullSecrets
	if len(pod.Spec.ImagePullSecrets) != 2 || pod.Spec.ImagePullSecrets[0].Name != "regcred-1" {
		t.Errorf("expected ImagePullSecrets regcred-1, got %v", pod.Spec.ImagePullSecrets)
	}

	// 5. Pod SecurityContext
	if pod.Spec.SecurityContext == nil || *pod.Spec.SecurityContext.RunAsUser != 10001 {
		t.Errorf("expected PodSecurityContext RunAsUser 10001, got %v", pod.Spec.SecurityContext)
	}

	// 6. Container
	c := pod.Spec.Containers[0]
	if c.ImagePullPolicy != corev1.PullAlways {
		t.Errorf("expected PullAlways, got %s", c.ImagePullPolicy)
	}
	if c.Resources.Requests.Cpu().String() != "20m" || c.Resources.Limits.Memory().String() != "256Mi" {
		t.Errorf("expected custom resources 20m/256Mi, got %v", c.Resources)
	}
	if c.SecurityContext == nil || *c.SecurityContext.ReadOnlyRootFilesystem != true {
		t.Errorf("expected ReadOnlyRootFilesystem true, got %v", c.SecurityContext)
	}
}

func TestBuildProbePod_ConflictOverridesAndWarnings(t *testing.T) {
	cfg := config.DefaultConfig()
	// Override a default template label
	cfg.K8s.ProbeExtraLabels = map[string]string{
		"app.kubernetes.io/name": "custom-probe-agent",
	}
	// Override default resources
	cfg.K8s.ProbeResources = &config.ProbeResourceRequirements{
		Requests: map[string]string{"cpu": "50m"},
		Limits:   map[string]string{"memory": "512Mi"},
	}
	// Override container SecurityContext
	f := false
	cfg.K8s.ProbeSecurityContext = &config.ProbeSecurityContextConfig{
		AllowPrivilegeEscalation: &f,
		Capabilities: &config.ProbeCapabilitiesConfig{
			Add:  []string{"NET_ADMIN"},
			Drop: []string{"KILL"},
		},
	}

	report := &model.Report{
		ID:        "rep-conflict-test",
		Mode:      model.ModeHostNetwork,
		Namespace: "kncc",
	}

	pod, err := BuildProbePod(report, "node-conflict", "kncc", cfg)
	if err != nil {
		t.Fatalf("BuildProbePod failed: %v", err)
	}

	// Custom label overrides template label
	if pod.Labels["app.kubernetes.io/name"] != "custom-probe-agent" {
		t.Errorf("expected custom label to override template, got %s", pod.Labels["app.kubernetes.io/name"])
	}

	// Custom resources override default
	c := pod.Spec.Containers[0]
	if c.Resources.Requests.Cpu().String() != "50m" {
		t.Errorf("expected cpu 50m, got %s", c.Resources.Requests.Cpu().String())
	}
	if c.Resources.Limits.Memory().String() != "512Mi" {
		t.Errorf("expected memory 512Mi, got %s", c.Resources.Limits.Memory().String())
	}
	// Default memory request should remain intact (32Mi)
	if c.Resources.Requests.Memory().String() != "32Mi" {
		t.Errorf("expected default memory request 32Mi, got %s", c.Resources.Requests.Memory().String())
	}

	// SecurityContext overrides
	if c.SecurityContext.AllowPrivilegeEscalation == nil || *c.SecurityContext.AllowPrivilegeEscalation != false {
		t.Errorf("expected AllowPrivilegeEscalation false, got %v", c.SecurityContext.AllowPrivilegeEscalation)
	}
	if len(c.SecurityContext.Capabilities.Add) != 1 || c.SecurityContext.Capabilities.Add[0] != corev1.Capability("NET_ADMIN") {
		t.Errorf("expected NET_ADMIN capability, got %v", c.SecurityContext.Capabilities.Add)
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
