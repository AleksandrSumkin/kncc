package k8s

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
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

	// 1. Labels & Annotations
	labels := map[string]string{
		"app.kubernetes.io/name":       "kncc-probe",
		"app.kubernetes.io/managed-by": "kncc",
		"kncc.io/report-id":            report.ID,
		"kncc.io/node":                 sanitizeLabelValue(nodeName),
		"kncc.io/mode":                 string(report.Mode),
	}
	if cfg != nil && len(cfg.K8s.ProbeExtraLabels) > 0 {
		for k, v := range cfg.K8s.ProbeExtraLabels {
			if existing, exists := labels[k]; exists && existing != v {
				log.Printf("[WARN] probe pod label conflict: overriding template label %q: %q -> %q", k, existing, v)
			}
			labels[k] = v
		}
	}

	var annotations map[string]string
	if cfg != nil && len(cfg.K8s.ProbeExtraAnnotations) > 0 {
		annotations = make(map[string]string, len(cfg.K8s.ProbeExtraAnnotations))
		for k, v := range cfg.K8s.ProbeExtraAnnotations {
			annotations[k] = v
		}
	}

	// 2. Resources (Requests & Limits)
	reqMap := corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("10m"),
		corev1.ResourceMemory: resource.MustParse("32Mi"),
	}
	limMap := corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("100m"),
		corev1.ResourceMemory: resource.MustParse("128Mi"),
	}
	if cfg != nil && cfg.K8s.ProbeResources != nil {
		if cfg.K8s.ProbeResources.Requests != nil {
			for rName, rVal := range cfg.K8s.ProbeResources.Requests {
				q, err := resource.ParseQuantity(rVal)
				if err != nil {
					log.Printf("[WARN] probe pod invalid custom requests.%s quantity %q: %v", rName, rVal, err)
					continue
				}
				resKey := corev1.ResourceName(rName)
				if oldQ, exists := reqMap[resKey]; exists && oldQ.String() != q.String() {
					log.Printf("[WARN] probe pod resource conflict: overriding template requests.%s: %s -> %s", rName, oldQ.String(), q.String())
				}
				reqMap[resKey] = q
			}
		}
		if cfg.K8s.ProbeResources.Limits != nil {
			for rName, rVal := range cfg.K8s.ProbeResources.Limits {
				q, err := resource.ParseQuantity(rVal)
				if err != nil {
					log.Printf("[WARN] probe pod invalid custom limits.%s quantity %q: %v", rName, rVal, err)
					continue
				}
				resKey := corev1.ResourceName(rName)
				if oldQ, exists := limMap[resKey]; exists && oldQ.String() != q.String() {
					log.Printf("[WARN] probe pod resource conflict: overriding template limits.%s: %s -> %s", rName, oldQ.String(), q.String())
				}
				limMap[resKey] = q
			}
		}
	}

	// 3. SecurityContexts
	secCtx := &corev1.SecurityContext{
		AllowPrivilegeEscalation: &allowPrivilegeEscalation,
		ReadOnlyRootFilesystem:   &readOnlyRootFilesystem,
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{"ALL"},
			Add:  []corev1.Capability{"NET_RAW"},
		},
	}
	if cfg != nil && cfg.K8s.ProbeSecurityContext != nil {
		cSec := cfg.K8s.ProbeSecurityContext
		if cSec.AllowPrivilegeEscalation != nil {
			if *cSec.AllowPrivilegeEscalation != allowPrivilegeEscalation {
				log.Printf("[WARN] probe pod securityContext conflict: overriding template allowPrivilegeEscalation: %v -> %v",
					allowPrivilegeEscalation, *cSec.AllowPrivilegeEscalation)
			}
			secCtx.AllowPrivilegeEscalation = cSec.AllowPrivilegeEscalation
		}
		if cSec.ReadOnlyRootFilesystem != nil {
			if *cSec.ReadOnlyRootFilesystem != readOnlyRootFilesystem {
				log.Printf("[WARN] probe pod securityContext conflict: overriding template readOnlyRootFilesystem: %v -> %v",
					readOnlyRootFilesystem, *cSec.ReadOnlyRootFilesystem)
			}
			secCtx.ReadOnlyRootFilesystem = cSec.ReadOnlyRootFilesystem
		}
		if cSec.RunAsUser != nil {
			secCtx.RunAsUser = cSec.RunAsUser
		}
		if cSec.RunAsGroup != nil {
			secCtx.RunAsGroup = cSec.RunAsGroup
		}
		if cSec.RunAsNonRoot != nil {
			secCtx.RunAsNonRoot = cSec.RunAsNonRoot
		}
		if cSec.Privileged != nil {
			secCtx.Privileged = cSec.Privileged
		}
		if cSec.Capabilities != nil {
			if cSec.Capabilities.Add != nil {
				adds := make([]corev1.Capability, len(cSec.Capabilities.Add))
				for i, a := range cSec.Capabilities.Add {
					adds[i] = corev1.Capability(a)
				}
				log.Printf("[WARN] probe pod securityContext conflict: overriding template capabilities.add: %v -> %v",
					secCtx.Capabilities.Add, adds)
				secCtx.Capabilities.Add = adds
			}
			if cSec.Capabilities.Drop != nil {
				drops := make([]corev1.Capability, len(cSec.Capabilities.Drop))
				for i, d := range cSec.Capabilities.Drop {
					drops[i] = corev1.Capability(d)
				}
				log.Printf("[WARN] probe pod securityContext conflict: overriding template capabilities.drop: %v -> %v",
					secCtx.Capabilities.Drop, drops)
				secCtx.Capabilities.Drop = drops
			}
		}
	}

	var podSecCtx *corev1.PodSecurityContext
	if cfg != nil && cfg.K8s.ProbePodSecurityContext != nil {
		customPSC := cfg.K8s.ProbePodSecurityContext
		podSecCtx = &corev1.PodSecurityContext{}
		if customPSC.RunAsUser != nil {
			podSecCtx.RunAsUser = customPSC.RunAsUser
		}
		if customPSC.RunAsGroup != nil {
			podSecCtx.RunAsGroup = customPSC.RunAsGroup
		}
		if customPSC.RunAsNonRoot != nil {
			podSecCtx.RunAsNonRoot = customPSC.RunAsNonRoot
		}
		if customPSC.FSGroup != nil {
			podSecCtx.FSGroup = customPSC.FSGroup
		}
	}

	// 4. ImagePullSecrets
	var imagePullSecrets []corev1.LocalObjectReference
	if cfg != nil && len(cfg.K8s.ProbeImagePullSecrets) > 0 {
		imagePullSecrets = make([]corev1.LocalObjectReference, 0, len(cfg.K8s.ProbeImagePullSecrets))
		for _, sec := range cfg.K8s.ProbeImagePullSecrets {
			trimmed := strings.TrimSpace(sec)
			if trimmed != "" {
				imagePullSecrets = append(imagePullSecrets, corev1.LocalObjectReference{Name: trimmed})
			}
		}
	}

	// 5. ImagePullPolicy
	var pullPolicy corev1.PullPolicy
	if cfg != nil && cfg.K8s.ProbeImagePullPolicy != "" {
		pullPolicy = corev1.PullPolicy(cfg.K8s.ProbeImagePullPolicy)
	}

	// 6. PriorityClassName
	priorityClassName := ""
	if cfg != nil && cfg.K8s.ProbePriorityClassName != "" {
		priorityClassName = cfg.K8s.ProbePriorityClassName
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        podName,
			Namespace:   namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: corev1.PodSpec{
			NodeName:              nodeName,
			HostNetwork:           isHostNetwork,
			RestartPolicy:         corev1.RestartPolicyNever,
			ActiveDeadlineSeconds: &activeDeadline,
			PriorityClassName:     priorityClassName,
			ImagePullSecrets:      imagePullSecrets,
			SecurityContext:       podSecCtx,
			Tolerations: []corev1.Toleration{
				{
					// Tolerate all taints (master/control-plane, GPU, custom)
					Operator: corev1.TolerationOpExists,
				},
			},
			Containers: []corev1.Container{
				{
					Name:                     "kncc-probe",
					Image:                    probeImage,
					ImagePullPolicy:          pullPolicy,
					Command:                  []string{"/bin/sh", "-c", script},
					Resources:                corev1.ResourceRequirements{Requests: reqMap, Limits: limMap},
					SecurityContext:          secCtx,
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
