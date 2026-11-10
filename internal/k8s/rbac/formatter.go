package rbac

import (
	"bytes"
	"fmt"
	"strings"
	"time"
)

// GetMockRBACReport returns realistic RBAC rules for demo/mock mode matching the kncc Helm chart permissions.
func GetMockRBACReport(defaultNamespace string) *RBACReport {
	if defaultNamespace == "" {
		defaultNamespace = "kncc"
	}

	subject := SubjectInfo{
		Kind:        "ServiceAccount",
		Namespace:   defaultNamespace,
		Name:        "kncc",
		RawUsername: fmt.Sprintf("system:serviceaccount:%s:kncc", defaultNamespace),
	}

	rules := []PolicyRule{
		{
			Type:           "ServiceAccount",
			Subject:        fmt.Sprintf("%s/kncc", defaultNamespace),
			Verbs:          []string{"get", "list", "watch"},
			Namespace:      "*",
			APIGroup:       "core",
			Resource:       "nodes",
			ResourceNames:  []string{"*"},
			OriginatedFrom: "ClusterRole/kncc-node-viewer",
		},
		{
			Type:           "ServiceAccount",
			Subject:        fmt.Sprintf("%s/kncc", defaultNamespace),
			Verbs:          []string{"get", "list", "watch"},
			Namespace:      "*",
			APIGroup:       "core",
			Resource:       "namespaces",
			ResourceNames:  []string{"*"},
			OriginatedFrom: "ClusterRole/kncc-node-viewer",
		},
		{
			Type:           "ServiceAccount",
			Subject:        fmt.Sprintf("%s/kncc", defaultNamespace),
			Verbs:          []string{"create", "delete", "get", "list", "watch"},
			Namespace:      defaultNamespace,
			APIGroup:       "core",
			Resource:       "pods",
			ResourceNames:  []string{"*"},
			OriginatedFrom: fmt.Sprintf("Role/%s/kncc-probe-executor", defaultNamespace),
		},
		{
			Type:           "ServiceAccount",
			Subject:        fmt.Sprintf("%s/kncc", defaultNamespace),
			Verbs:          []string{"create", "delete", "get", "list", "watch"},
			Namespace:      defaultNamespace,
			APIGroup:       "core",
			Resource:       "pods/status",
			ResourceNames:  []string{"*"},
			OriginatedFrom: fmt.Sprintf("Role/%s/kncc-probe-executor", defaultNamespace),
		},
		{
			Type:           "ServiceAccount",
			Subject:        fmt.Sprintf("%s/kncc", defaultNamespace),
			Verbs:          []string{"get"},
			Namespace:      defaultNamespace,
			APIGroup:       "core",
			Resource:       "pods/log",
			ResourceNames:  []string{"*"},
			OriginatedFrom: fmt.Sprintf("Role/%s/kncc-probe-executor", defaultNamespace),
		},
	}

	sortPolicyRules(rules)

	return &RBACReport{
		Subject:     subject,
		Rules:       rules,
		Source:      "mock",
		GeneratedAt: time.Now().UTC(),
	}
}

// FormatPolicyRulesTable renders an aligned ASCII table formatted identically to rbac-tool policy-rules.
func FormatPolicyRulesTable(report *RBACReport) string {
	if report == nil || len(report.Rules) == 0 {
		return "No RBAC policy rules discovered."
	}

	headers := []string{
		"TYPE",
		"SUBJECT",
		"VERBS",
		"NAMESPACE",
		"API GROUP",
		"KIND",
		"NAMES",
		"NONRESOURCEURI",
		"ORIGINATED FROM",
	}

	rows := make([][]string, len(report.Rules))
	for i, r := range report.Rules {
		verbsStr := strings.Join(r.Verbs, ",")
		namesStr := strings.Join(r.ResourceNames, ",")
		nonResourceStr := strings.Join(r.NonResourceURLs, ",")

		rows[i] = []string{
			r.Type,
			r.Subject,
			verbsStr,
			r.Namespace,
			r.APIGroup,
			r.Resource,
			namesStr,
			nonResourceStr,
			r.OriginatedFrom,
		}
	}

	// Calculate maximum width for each column
	widths := make([]int, len(headers))
	for c, h := range headers {
		widths[c] = len(h)
	}
	for _, row := range rows {
		for c, val := range row {
			if len(val) > widths[c] {
				widths[c] = len(val)
			}
		}
	}

	var buf bytes.Buffer

	// Border builder
	border := func() string {
		var b strings.Builder
		b.WriteString("+")
		for _, w := range widths {
			b.WriteString(strings.Repeat("-", w+2))
			b.WriteString("+")
		}
		return b.String()
	}

	bLine := border()
	buf.WriteString(bLine)
	buf.WriteString("\n")

	// Header row
	buf.WriteString("|")
	for c, h := range headers {
		buf.WriteString(fmt.Sprintf(" %-*s |", widths[c], h))
	}
	buf.WriteString("\n")
	buf.WriteString(bLine)
	buf.WriteString("\n")

	// Data rows
	for _, row := range rows {
		buf.WriteString("|")
		for c, val := range row {
			buf.WriteString(fmt.Sprintf(" %-*s |", widths[c], val))
		}
		buf.WriteString("\n")
	}

	buf.WriteString(bLine)
	return buf.String()
}
