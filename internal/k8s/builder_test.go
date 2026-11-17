package k8s

import (
	"strings"
	"testing"

	"kncc/internal/config"
	"kncc/internal/model"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/validation"
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
	if pod.Spec.AutomountServiceAccountToken == nil || *pod.Spec.AutomountServiceAccountToken != false {
		t.Errorf("expected AutomountServiceAccountToken to be false by default")
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
	if pod.Spec.AutomountServiceAccountToken == nil || *pod.Spec.AutomountServiceAccountToken != false {
		t.Errorf("expected AutomountServiceAccountToken to be false for kncc-agent image")
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

func TestBuildProbePod_AutomountServiceAccountTokenConfig(t *testing.T) {
	report := &model.Report{
		ID:        "rep-sa-test",
		Mode:      model.ModeHostNetwork,
		Namespace: "kncc",
		Targets: []model.TargetService{
			{Protocol: model.ProtocolTCP, Host: "1.1.1.1", Port: 80},
		},
	}

	// 1. Explicitly enabled via config
	tr := true
	cfg := config.DefaultConfig()
	cfg.K8s.ProbeAutomountServiceAccountToken = &tr

	podEnabled, err := BuildProbePod(report, "node-1", "kncc", cfg)
	if err != nil {
		t.Fatalf("BuildProbePod failed: %v", err)
	}
	if podEnabled.Spec.AutomountServiceAccountToken == nil || *podEnabled.Spec.AutomountServiceAccountToken != true {
		t.Errorf("expected AutomountServiceAccountToken to be true when explicitly configured")
	}

	// 2. Explicitly disabled via config
	fa := false
	cfg.K8s.ProbeAutomountServiceAccountToken = &fa

	podDisabled, err := BuildProbePod(report, "node-1", "kncc", cfg)
	if err != nil {
		t.Fatalf("BuildProbePod failed: %v", err)
	}
	if podDisabled.Spec.AutomountServiceAccountToken == nil || *podDisabled.Spec.AutomountServiceAccountToken != false {
		t.Errorf("expected AutomountServiceAccountToken to be false when explicitly configured")
	}

	// 3. nil cfg defaults to false (secure default)
	podNilCfg, err := BuildProbePod(report, "node-1", "kncc", nil)
	if err != nil {
		t.Fatalf("BuildProbePod with nil cfg failed: %v", err)
	}
	if podNilCfg.Spec.AutomountServiceAccountToken == nil || *podNilCfg.Spec.AutomountServiceAccountToken != false {
		t.Errorf("expected AutomountServiceAccountToken to be false when cfg is nil")
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

func TestBuildProbePod_ReadOnlyRootFilesystemDefault(t *testing.T) {
	report := &model.Report{
		ID:        "rep-ro-test",
		Mode:      model.ModeHostNetwork,
		Namespace: "kncc",
		Targets: []model.TargetService{
			{Protocol: model.ProtocolTCP, Host: "10.0.0.1", Port: 80},
		},
	}

	// 1. DefaultConfig: ReadOnlyRootFilesystem must be true by default
	cfgDefault := config.DefaultConfig()
	podDefault, err := BuildProbePod(report, "node-1", "kncc", cfgDefault)
	if err != nil {
		t.Fatalf("BuildProbePod failed: %v", err)
	}

	cDefault := podDefault.Spec.Containers[0]
	if cDefault.SecurityContext == nil || cDefault.SecurityContext.ReadOnlyRootFilesystem == nil {
		t.Fatalf("expected SecurityContext.ReadOnlyRootFilesystem to be set")
	}
	if !*cDefault.SecurityContext.ReadOnlyRootFilesystem {
		t.Errorf("expected ReadOnlyRootFilesystem to be true by default, got false")
	}

	// Verify emptyDir volume for /tmp is present in pod spec and container volume mounts
	foundVol := false
	for _, v := range podDefault.Spec.Volumes {
		if v.Name == "kncc-probe-tmp" && v.EmptyDir != nil {
			foundVol = true
			break
		}
	}
	if !foundVol {
		t.Errorf("expected kncc-probe-tmp emptyDir volume in pod spec")
	}

	foundMount := false
	for _, m := range cDefault.VolumeMounts {
		if m.Name == "kncc-probe-tmp" && m.MountPath == "/tmp" {
			foundMount = true
			break
		}
	}
	if !foundMount {
		t.Errorf("expected kncc-probe-tmp volume mount at /tmp in container spec")
	}

	// 2. nil Config: ReadOnlyRootFilesystem must also default to true
	podNil, err := BuildProbePod(report, "node-1", "kncc", nil)
	if err != nil {
		t.Fatalf("BuildProbePod with nil cfg failed: %v", err)
	}
	cNil := podNil.Spec.Containers[0]
	if cNil.SecurityContext == nil || cNil.SecurityContext.ReadOnlyRootFilesystem == nil || !*cNil.SecurityContext.ReadOnlyRootFilesystem {
		t.Errorf("expected ReadOnlyRootFilesystem to be true when cfg is nil")
	}

	// 3. Explicit override: ReadOnlyRootFilesystem = false
	f := false
	cfgOverride := config.DefaultConfig()
	cfgOverride.K8s.ProbeSecurityContext = &config.ProbeSecurityContextConfig{
		ReadOnlyRootFilesystem: &f,
	}
	podOverride, err := BuildProbePod(report, "node-1", "kncc", cfgOverride)
	if err != nil {
		t.Fatalf("BuildProbePod failed: %v", err)
	}
	cOverride := podOverride.Spec.Containers[0]
	if cOverride.SecurityContext == nil || cOverride.SecurityContext.ReadOnlyRootFilesystem == nil || *cOverride.SecurityContext.ReadOnlyRootFilesystem != false {
		t.Errorf("expected ReadOnlyRootFilesystem to be false when explicitly configured")
	}
}

func TestBuildProbePod_WithImageDigest(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.K8s.ProbeImage = "ghcr.io/aleksandrsumkin/kncc-agent:latest"
	cfg.K8s.ProbeImageDigest = "sha256:45b23dee08af5e43a7fa6c4e1aa9f7f0f6992d6790c308e9587e3b60f6e85071"

	report := &model.Report{
		ID:        "rep-digest-test",
		Mode:      model.ModeHostNetwork,
		Namespace: "kncc",
		Targets: []model.TargetService{
			{Protocol: model.ProtocolTCP, Host: "10.0.0.1", Port: 80},
		},
	}

	pod, err := BuildProbePod(report, "worker-node-1", "kncc", cfg)
	if err != nil {
		t.Fatalf("BuildProbePod failed: %v", err)
	}

	expectedImage := "ghcr.io/aleksandrsumkin/kncc-agent@sha256:45b23dee08af5e43a7fa6c4e1aa9f7f0f6992d6790c308e9587e3b60f6e85071"
	if pod.Spec.Containers[0].Image != expectedImage {
		t.Errorf("expected container image %q, got %q", expectedImage, pod.Spec.Containers[0].Image)
	}
}

func TestBuildProbePod_SecurityContextWithSeccompAndCamelCase(t *testing.T) {
	cfg := config.DefaultConfig()
	runAsUser := int64(10001)
	runAsGroup := int64(10001)
	runAsNonRoot := true
	fsGroup := int64(10001)
	fsGroupChangePolicy := "OnRootMismatch"
	readOnlyRoot := false
	allowEscalation := true

	cfg.K8s.ProbePodSecurityContext = &config.ProbePodSecurityContextConfig{
		RunAsUser:           &runAsUser,
		RunAsGroup:          &runAsGroup,
		RunAsNonRoot:        &runAsNonRoot,
		FSGroup:             &fsGroup,
		FSGroupChangePolicy: &fsGroupChangePolicy,
		SeccompProfile: &config.ProbeSeccompProfileConfig{
			Type: "RuntimeDefault",
		},
	}
	cfg.K8s.ProbeSecurityContext = &config.ProbeSecurityContextConfig{
		ReadOnlyRootFilesystem:   &readOnlyRoot,
		AllowPrivilegeEscalation: &allowEscalation,
		RunAsUser:                &runAsUser,
		RunAsGroup:               &runAsGroup,
		RunAsNonRoot:             &runAsNonRoot,
		Capabilities: &config.ProbeCapabilitiesConfig{
			Drop: []string{"ALL"},
			Add:  []string{"NET_RAW"},
		},
		SeccompProfile: &config.ProbeSeccompProfileConfig{
			Type: "RuntimeDefault",
		},
	}

	report := &model.Report{
		ID:        "rep-sec-test",
		Mode:      model.ModePodNetwork,
		Namespace: "kncc",
	}

	pod, err := BuildProbePod(report, "node-sec", "kncc", cfg)
	if err != nil {
		t.Fatalf("BuildProbePod failed: %v", err)
	}

	// PodSecurityContext assertions
	if pod.Spec.SecurityContext == nil {
		t.Fatal("expected pod.Spec.SecurityContext not nil")
	}
	if pod.Spec.SecurityContext.RunAsUser == nil || *pod.Spec.SecurityContext.RunAsUser != 10001 {
		t.Errorf("expected pod RunAsUser 10001, got %v", pod.Spec.SecurityContext.RunAsUser)
	}
	if pod.Spec.SecurityContext.FSGroupChangePolicy == nil || *pod.Spec.SecurityContext.FSGroupChangePolicy != corev1.FSGroupChangeOnRootMismatch {
		t.Errorf("expected pod FSGroupChangePolicy OnRootMismatch, got %v", pod.Spec.SecurityContext.FSGroupChangePolicy)
	}
	if pod.Spec.SecurityContext.SeccompProfile == nil || pod.Spec.SecurityContext.SeccompProfile.Type != corev1.SeccompProfileTypeRuntimeDefault {
		t.Errorf("expected pod SeccompProfile RuntimeDefault, got %v", pod.Spec.SecurityContext.SeccompProfile)
	}

	// Container SecurityContext assertions
	c := pod.Spec.Containers[0]
	if c.SecurityContext == nil {
		t.Fatal("expected container SecurityContext not nil")
	}
	if c.SecurityContext.AllowPrivilegeEscalation == nil || *c.SecurityContext.AllowPrivilegeEscalation != true {
		t.Errorf("expected AllowPrivilegeEscalation true, got %v", c.SecurityContext.AllowPrivilegeEscalation)
	}
	if c.SecurityContext.ReadOnlyRootFilesystem == nil || *c.SecurityContext.ReadOnlyRootFilesystem != false {
		t.Errorf("expected ReadOnlyRootFilesystem false, got %v", c.SecurityContext.ReadOnlyRootFilesystem)
	}
	if c.SecurityContext.SeccompProfile == nil || c.SecurityContext.SeccompProfile.Type != corev1.SeccompProfileTypeRuntimeDefault {
		t.Errorf("expected container SeccompProfile RuntimeDefault, got %v", c.SecurityContext.SeccompProfile)
	}
	if len(c.SecurityContext.Capabilities.Add) != 1 || c.SecurityContext.Capabilities.Add[0] != corev1.Capability("NET_RAW") {
		t.Errorf("expected NET_RAW capability, got %v", c.SecurityContext.Capabilities.Add)
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

func TestGenerateProbePodName_RFC1123Compliance(t *testing.T) {
	testCases := []struct {
		reportID string
		nodeName string
		desc     string
	}{
		{
			reportID: "a1b2c3d4-e5f6-7890",
			nodeName: "worker-01.us-east-1.compute.internal",
			desc:     "AWS EKS style FQDN node name",
		},
		{
			reportID: "rep-12345",
			nodeName: "k8s-master.homelab.local",
			desc:     "Homelab FQDN node name",
		},
		{
			reportID: "report-abcdef-123456",
			nodeName: "node1.sub.domain.corp.example.com",
			desc:     "Deep nested subdomain FQDN",
		},
		{
			reportID: "short",
			nodeName: "node_01.internal_zone:8080",
			desc:     "Node name with underscores and colons",
		},
		{
			reportID: "rep-multi-dot",
			nodeName: "node..01...test",
			desc:     "Consecutive dots and hyphens",
		},
		{
			reportID: "rep-edges",
			nodeName: ".leading-and-trailing-dots.",
			desc:     "Leading and trailing dots",
		},
		{
			reportID: "rep-dots-only",
			nodeName: ".....",
			desc:     "Node name with only dots",
		},
		{
			reportID: "rep-symbols-only",
			nodeName: "____---....",
			desc:     "Node name with only special characters",
		},
		{
			reportID: "rep-long",
			nodeName: "very-long-node-name-with-many-subdomains.cluster.eu-central-1.cloud.provider.internal.corp",
			desc:     "Very long FQDN exceeding limits",
		},
		{
			reportID: "standard",
			nodeName: "k3s-worker-01",
			desc:     "Standard non-FQDN node name",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			podName := generateProbePodName(tc.reportID, tc.nodeName)

			// 1. Pod name must not contain dots
			if strings.Contains(podName, ".") {
				t.Errorf("pod name %q contains dot (.)", podName)
			}

			// 2. Length must not exceed 63 characters
			if len(podName) > 63 {
				t.Errorf("pod name %q length %d exceeds 63 characters", podName, len(podName))
			}

			// 3. Must not have consecutive hyphens from sanitization
			if strings.Contains(podName, "--") {
				t.Errorf("pod name %q contains consecutive hyphens", podName)
			}

			// 4. Must strictly adhere to Kubernetes RFC 1123 DNS Label
			errs := validation.IsDNS1123Label(podName)
			if len(errs) > 0 {
				t.Errorf("pod name %q is not a valid DNS-1123 label: %v", podName, errs)
			}
		})
	}
}

func TestBuildProbePod_FQDNNode(t *testing.T) {
	cfg := config.DefaultConfig()
	report := &model.Report{
		ID:        "rep-fqdn-check-1234",
		Mode:      model.ModeHostNetwork,
		Namespace: "kncc",
		Targets: []model.TargetService{
			{Protocol: model.ProtocolTCP, Host: "10.0.0.1", Port: 80},
		},
	}

	fqdnNode := "worker-01.us-east-1.compute.internal"
	pod, err := BuildProbePod(report, fqdnNode, "kncc", cfg)
	if err != nil {
		t.Fatalf("BuildProbePod failed: %v", err)
	}

	// Invariant 1: Spec.NodeName must match the EXACT original node name in the cluster
	if pod.Spec.NodeName != fqdnNode {
		t.Errorf("expected pod.Spec.NodeName %q, got %q", fqdnNode, pod.Spec.NodeName)
	}

	// Invariant 2: pod.Name must be sanitized (no dots) and valid DNS-1123 Label
	if strings.Contains(pod.Name, ".") {
		t.Errorf("expected pod.Name to not contain dots, got %q", pod.Name)
	}
	errs := validation.IsDNS1123Label(pod.Name)
	if len(errs) > 0 {
		t.Errorf("pod.Name %q failed DNS-1123 label validation: %v", pod.Name, errs)
	}

	// Invariant 3: kncc.io/node label must be sanitized and <= 63 chars
	nodeLabel := pod.Labels["kncc.io/node"]
	if len(nodeLabel) > 63 {
		t.Errorf("node label %q exceeds 63 characters", nodeLabel)
	}
	if strings.HasPrefix(nodeLabel, "-") || strings.HasSuffix(nodeLabel, "-") {
		t.Errorf("node label %q has leading/trailing hyphens", nodeLabel)
	}
}

func TestSanitizeLabelValue(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"worker-01.corp.internal", "worker-01-corp-internal"},
		{"node..01__test", "node-01-test"},
		{"---leading-trailing---", "leading-trailing"},
		{"valid-name-123", "valid-name-123"},
	}

	for _, tc := range cases {
		got := sanitizeLabelValue(tc.input)
		if got != tc.want {
			t.Errorf("sanitizeLabelValue(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
