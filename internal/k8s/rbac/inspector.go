package rbac

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	authenticationv1 "k8s.io/api/authentication/v1"
	authorizationv1 "k8s.io/api/authorization/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	inClusterNamespaceFile = "/var/run/secrets/kubernetes.io/serviceaccount/namespace"
)

// InspectCurrentPermissions evaluates the effective RBAC policy rules of the current client/ServiceAccount.
func InspectCurrentPermissions(ctx context.Context, client kubernetes.Interface, defaultNamespace string, isMock bool) *RBACReport {
	if isMock || client == nil {
		return GetMockRBACReport(defaultNamespace)
	}

	subject := discoverSubject(ctx, client, defaultNamespace)

	// Primary Strategy: try full RBAC objects traversal (Roles & RoleBindings) for detailed OriginatedFrom tracking.
	rules, err := resolveViaRBACBindings(ctx, client, subject, defaultNamespace)
	if err == nil && len(rules) > 0 {
		return &RBACReport{
			Subject:     subject,
			Rules:       rules,
			Source:      "cluster-rbac",
			GeneratedAt: time.Now().UTC(),
		}
	}

	// Secondary Strategy: use SelfSubjectRulesReview which is supported natively for any authenticated subject.
	rules, errReview := resolveViaSelfSubjectRulesReview(ctx, client, subject, defaultNamespace)
	if errReview == nil && len(rules) > 0 {
		var evalErr string
		if err != nil {
			evalErr = fmt.Sprintf("Cluster RBAC listing failed (%v), resolved via SelfSubjectRulesReview", err)
		}
		return &RBACReport{
			Subject:         subject,
			Rules:           rules,
			Source:          "self-subject-rules-review",
			EvaluationError: evalErr,
			GeneratedAt:     time.Now().UTC(),
		}
	}

	// Fallback if both fail: return mock report with error note
	mockReport := GetMockRBACReport(defaultNamespace)
	mockReport.EvaluationError = fmt.Sprintf("Could not evaluate live RBAC (RBAC: %v; SelfSubjectRulesReview: %v)", err, errReview)
	return mockReport
}

// discoverSubject finds out who the current client is authenticated as.
func discoverSubject(ctx context.Context, client kubernetes.Interface, defaultNamespace string) SubjectInfo {
	// Try SelfSubjectReview (K8s 1.27+)
	review, err := client.AuthenticationV1().SelfSubjectReviews().Create(ctx, &authenticationv1.SelfSubjectReview{}, metav1.CreateOptions{})
	if err == nil && review != nil && review.Status.UserInfo.Username != "" {
		username := review.Status.UserInfo.Username
		if strings.HasPrefix(username, "system:serviceaccount:") {
			parts := strings.Split(username, ":")
			if len(parts) >= 4 {
				return SubjectInfo{
					Kind:        "ServiceAccount",
					Namespace:   parts[2],
					Name:        parts[3],
					RawUsername: username,
				}
			}
		}
		return SubjectInfo{
			Kind:        "User",
			Name:        username,
			RawUsername: username,
		}
	}

	// In-cluster service account detection
	ns := defaultNamespace
	if data, err := os.ReadFile(inClusterNamespaceFile); err == nil {
		if val := strings.TrimSpace(string(data)); val != "" {
			ns = val
		}
	}
	if ns == "" {
		ns = "kncc"
	}

	return SubjectInfo{
		Kind:        "ServiceAccount",
		Namespace:   ns,
		Name:        "kncc",
		RawUsername: fmt.Sprintf("system:serviceaccount:%s:kncc", ns),
	}
}

// resolveViaRBACBindings attempts to traverse Roles, ClusterRoles, and RoleBindings.
func resolveViaRBACBindings(ctx context.Context, client kubernetes.Interface, subject SubjectInfo, defaultNamespace string) ([]PolicyRule, error) {
	var rules []PolicyRule
	subStr := subject.Name
	if subject.Kind == "ServiceAccount" && subject.Namespace != "" {
		subStr = fmt.Sprintf("%s/%s", subject.Namespace, subject.Name)
	}

	// 1. ClusterRoleBindings
	crbList, err := client.RbacV1().ClusterRoleBindings().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	for _, crb := range crbList.Items {
		if !subjectMatchesRBAC(crb.Subjects, subject) {
			continue
		}

		cr, err := client.RbacV1().ClusterRoles().Get(ctx, crb.RoleRef.Name, metav1.GetOptions{})
		if err != nil {
			continue
		}

		origFrom := fmt.Sprintf("ClusterRole/%s", cr.Name)
		for _, r := range cr.Rules {
			expanded := expandPolicyRule(subject.Kind, subStr, "*", origFrom, r.Verbs, r.APIGroups, r.Resources, r.ResourceNames, r.NonResourceURLs)
			rules = append(rules, expanded...)
		}
	}

	// 2. Namespaced RoleBindings in target namespace
	targetNs := subject.Namespace
	if targetNs == "" {
		targetNs = defaultNamespace
	}
	if targetNs != "" {
		rbList, err := client.RbacV1().RoleBindings(targetNs).List(ctx, metav1.ListOptions{})
		if err == nil {
			for _, rb := range rbList.Items {
				if !subjectMatchesRBAC(rb.Subjects, subject) {
					continue
				}

				if rb.RoleRef.Kind == "ClusterRole" {
					cr, err := client.RbacV1().ClusterRoles().Get(ctx, rb.RoleRef.Name, metav1.GetOptions{})
					if err == nil {
						origFrom := fmt.Sprintf("RoleBinding/%s (ClusterRole/%s)", rb.Name, cr.Name)
						for _, r := range cr.Rules {
							expanded := expandPolicyRule(subject.Kind, subStr, targetNs, origFrom, r.Verbs, r.APIGroups, r.Resources, r.ResourceNames, r.NonResourceURLs)
							rules = append(rules, expanded...)
						}
					}
				} else if rb.RoleRef.Kind == "Role" {
					role, err := client.RbacV1().Roles(targetNs).Get(ctx, rb.RoleRef.Name, metav1.GetOptions{})
					if err == nil {
						origFrom := fmt.Sprintf("Role/%s/%s", targetNs, role.Name)
						for _, r := range role.Rules {
							expanded := expandPolicyRule(subject.Kind, subStr, targetNs, origFrom, r.Verbs, r.APIGroups, r.Resources, r.ResourceNames, r.NonResourceURLs)
							rules = append(rules, expanded...)
						}
					}
				}
			}
		}
	}

	sortPolicyRules(rules)
	return rules, nil
}

// resolveViaSelfSubjectRulesReview uses the standard SelfSubjectRulesReview API.
func resolveViaSelfSubjectRulesReview(ctx context.Context, client kubernetes.Interface, subject SubjectInfo, defaultNamespace string) ([]PolicyRule, error) {
	targetNs := subject.Namespace
	if targetNs == "" {
		targetNs = defaultNamespace
	}
	if targetNs == "" {
		targetNs = "default"
	}

	review, err := client.AuthorizationV1().SelfSubjectRulesReviews().Create(ctx, &authorizationv1.SelfSubjectRulesReview{
		Spec: authorizationv1.SelfSubjectRulesReviewSpec{
			Namespace: targetNs,
		},
	}, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}

	var rules []PolicyRule
	subStr := subject.Name
	if subject.Kind == "ServiceAccount" && subject.Namespace != "" {
		subStr = fmt.Sprintf("%s/%s", subject.Namespace, subject.Name)
	}

	origFrom := "SelfSubjectRulesReview"

	// Resource rules
	for _, r := range review.Status.ResourceRules {
		expanded := expandPolicyRule(subject.Kind, subStr, targetNs, origFrom, r.Verbs, r.APIGroups, r.Resources, r.ResourceNames, nil)
		rules = append(rules, expanded...)
	}

	// Non-resource rules
	for _, nr := range review.Status.NonResourceRules {
		expanded := expandPolicyRule(subject.Kind, subStr, "*", origFrom, nr.Verbs, nil, nil, nil, nr.NonResourceURLs)
		rules = append(rules, expanded...)
	}

	sortPolicyRules(rules)
	return rules, nil
}

func subjectMatchesRBAC(subjects []rbacv1.Subject, target SubjectInfo) bool {
	for _, s := range subjects {
		if s.Kind == target.Kind && s.Name == target.Name {
			if target.Kind == "ServiceAccount" && s.Namespace != "" && target.Namespace != "" {
				if s.Namespace == target.Namespace {
					return true
				}
				continue
			}
			return true
		}
	}
	return false
}

func expandPolicyRule(subType, subject, namespace, origFrom string, verbs, apiGroups, resources, resourceNames, nonResourceURLs []string) []PolicyRule {
	var result []PolicyRule

	cleanVerbs := make([]string, len(verbs))
	copy(cleanVerbs, verbs)
	sort.Strings(cleanVerbs)

	if len(nonResourceURLs) > 0 {
		for _, url := range nonResourceURLs {
			result = append(result, PolicyRule{
				Type:            subType,
				Subject:         subject,
				Verbs:           cleanVerbs,
				Namespace:       "*",
				APIGroup:        "",
				Resource:        "",
				ResourceNames:   nil,
				NonResourceURLs: []string{url},
				OriginatedFrom:  origFrom,
			})
		}
		return result
	}

	groups := apiGroups
	if len(groups) == 0 {
		groups = []string{"core"}
	} else {
		groups = make([]string, len(apiGroups))
		for i, g := range apiGroups {
			if g == "" {
				groups[i] = "core"
			} else {
				groups[i] = g
			}
		}
	}

	resList := resources
	if len(resList) == 0 {
		resList = []string{"*"}
	}

	resNames := resourceNames
	if len(resNames) == 0 {
		resNames = []string{"*"}
	}

	for _, g := range groups {
		for _, res := range resList {
			result = append(result, PolicyRule{
				Type:           subType,
				Subject:        subject,
				Verbs:          cleanVerbs,
				Namespace:      namespace,
				APIGroup:       g,
				Resource:       res,
				ResourceNames:  resNames,
				OriginatedFrom: origFrom,
			})
		}
	}

	return result
}

func sortPolicyRules(rules []PolicyRule) {
	sort.Slice(rules, func(i, j int) bool {
		if rules[i].Namespace != rules[j].Namespace {
			if rules[i].Namespace == "*" {
				return true
			}
			if rules[j].Namespace == "*" {
				return false
			}
			return rules[i].Namespace < rules[j].Namespace
		}
		if rules[i].APIGroup != rules[j].APIGroup {
			return rules[i].APIGroup < rules[j].APIGroup
		}
		if rules[i].Resource != rules[j].Resource {
			return rules[i].Resource < rules[j].Resource
		}
		return strings.Join(rules[i].Verbs, ",") < strings.Join(rules[j].Verbs, ",")
	})
}
