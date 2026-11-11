package k8s

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"

	"kncc/internal/config"
	"kncc/internal/model"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var dns1123SubdomainRegex = regexp.MustCompile(`[^a-z0-9\-.]`)

// BuildProbePod creates the Kubernetes Pod manifest for a single node probe.
func BuildProbePod(report *model.Report, nodeName, namespace string, cfg *config.Config) (*corev1.Pod, error) {
	customCA := ""
	if cfg != nil {
		if cfg.K8s.CustomCABundle != "" {
			customCA = cfg.K8s.CustomCABundle
		} else if cfg.CACerts.PEM != "" {
			customCA = cfg.CACerts.PEM
		}
	}
	script, err := BuildProbeScriptWithCA(report.Targets, customCA)
	if err != nil {
		return nil, fmt.Errorf("building probe script: %w", err)
	}

	podName := generateProbePodName(report.ID, nodeName)
	isHostNetwork := report.Mode == model.ModeHostNetwork

	var activeDeadline int64 = 60
	probeImage := "wbitt/network-multitool:alpine-extra"
	allowPrivilegeEscalation := true

	if cfg != nil {
		if cfg.K8s.ProbeActiveDeadlineSeconds > 0 {
			activeDeadline = cfg.K8s.ProbeActiveDeadlineSeconds
		}
		if cfg.K8s.ProbeImage != "" {
			probeImage = cfg.K8s.ProbeImage
		}
		if cfg.K8s.ProbeAllowPrivilegeEscalation != nil {
			allowPrivilegeEscalation = *cfg.K8s.ProbeAllowPrivilegeEscalation
		}
	}

	readOnlyRootFilesystem := false // container needs to write to /dev/termination-log and /tmp

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "kncc-probe",
				"app.kubernetes.io/managed-by": "kncc",
				"kncc.io/report-id":            report.ID,
				"kncc.io/node":                 sanitizeLabelValue(nodeName),
				"kncc.io/mode":                 string(report.Mode),
			},
		},
		Spec: corev1.PodSpec{
			NodeName:              nodeName,
			HostNetwork:           isHostNetwork,
			RestartPolicy:         corev1.RestartPolicyNever,
			ActiveDeadlineSeconds: &activeDeadline,
			Tolerations: []corev1.Toleration{
				{
					// Tolerate all taints (master/control-plane, GPU, custom)
					Operator: corev1.TolerationOpExists,
				},
			},
			Containers: []corev1.Container{
				{
					Name:    "kncc-probe",
					Image:   probeImage,
					Command: []string{"/bin/sh", "-c", script},
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("10m"),
							corev1.ResourceMemory: resource.MustParse("32Mi"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("128Mi"),
						},
					},
					SecurityContext: &corev1.SecurityContext{
						AllowPrivilegeEscalation: &allowPrivilegeEscalation,
						ReadOnlyRootFilesystem:   &readOnlyRootFilesystem,
						Capabilities: &corev1.Capabilities{
							Drop: []corev1.Capability{"ALL"},
							Add:  []corev1.Capability{"NET_RAW"},
						},
					},
					TerminationMessagePath:   "/dev/termination-log",
					TerminationMessagePolicy: corev1.TerminationMessageFallbackToLogsOnError,
				},
			},
		},
	}

	return pod, nil
}

// generateProbePodName creates a valid DNS-1123 pod name <= 63 chars.
func generateProbePodName(reportID, nodeName string) string {
	reportPrefix := reportID
	if len(reportPrefix) > 8 {
		reportPrefix = reportPrefix[:8]
	}

	h := sha256.New()
	h.Write([]byte(nodeName))
	nodeHash := hex.EncodeToString(h.Sum(nil))[:8]

	cleanNode := dns1123SubdomainRegex.ReplaceAllString(strings.ToLower(nodeName), "-")
	if len(cleanNode) > 20 {
		cleanNode = cleanNode[:20]
	}
	cleanNode = strings.Trim(cleanNode, "-.")

	name := fmt.Sprintf("kncc-%s-%s-%s", reportPrefix, cleanNode, nodeHash)
	if len(name) > 63 {
		name = name[:63]
	}
	return strings.Trim(name, "-.")
}

func sanitizeLabelValue(s string) string {
	clean := dns1123SubdomainRegex.ReplaceAllString(strings.ToLower(s), "-")
	if len(clean) > 63 {
		clean = clean[:63]
	}
	return strings.Trim(clean, "-.")
}
