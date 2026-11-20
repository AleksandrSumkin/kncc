package metrics

import (
	"context"
	"strconv"
	"time"

	"kncc/internal/model"
	"kncc/internal/store"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

// Metrics holds the Prometheus registry and metric collectors for kncc.
type Metrics struct {
	Registry *prometheus.Registry

	manualChecksTotal *prometheus.CounterVec

	probeSuccess     *prometheus.GaugeVec
	probeDuration    *prometheus.GaugeVec
	probeHTTPStatus  *prometheus.GaugeVec
	probeCertDaysLeft *prometheus.GaugeVec

	jobTotalNodes    *prometheus.GaugeVec
	jobHealthyNodes  *prometheus.GaugeVec
	jobLastTimestamp *prometheus.GaugeVec
	jobLastDuration  *prometheus.GaugeVec
}

var probeLabelNames = []string{
	"job", "target", "protocol", "target_host", "target_port", "node", "mode", "namespace",
}

// NewMetrics initializes the Prometheus registry and registers all kncc metrics.
func NewMetrics(s store.Store) *Metrics {
	reg := prometheus.NewRegistry()

	// 1. Standard Go & Process metrics
	reg.MustRegister(collectors.NewGoCollector())
	reg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

	// 2. Database size metric
	if s != nil {
		dbSizeGauge := prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "kncc_database_size_bytes",
			Help: "Current size of SQLite database file on disk in bytes.",
		}, func() float64 {
			size, err := s.GetDatabaseSizeBytes()
			if err != nil {
				return 0
			}
			return float64(size)
		})
		reg.MustRegister(dbSizeGauge)
		reg.MustRegister(newScheduledChecksCollector(s))
	}

	// 3. User manual check counter
	manualChecks := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "kncc_manual_checks_total",
		Help: "Total count of ad-hoc manual network checks initiated by users.",
	}, []string{})
	reg.MustRegister(manualChecks)

	// Initialize with historical count from database if available
	if s != nil {
		if count, err := s.GetManualChecksCount(context.Background()); err == nil && count > 0 {
			manualChecks.WithLabelValues().Add(float64(count))
		}
	}

	// 4. Probe metrics
	probeSuccess := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "kncc_probe_success",
		Help: "Status of probe: 1 for successful connectivity, 0 for failure.",
	}, probeLabelNames)
	reg.MustRegister(probeSuccess)

	probeDuration := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "kncc_probe_duration_seconds",
		Help: "Round-trip latency of target probe in seconds.",
	}, probeLabelNames)
	reg.MustRegister(probeDuration)

	probeHTTPStatus := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "kncc_probe_http_status_code",
		Help: "HTTP response status code returned by target service.",
	}, probeLabelNames)
	reg.MustRegister(probeHTTPStatus)

	probeCertDaysLeft := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "kncc_probe_cert_days_left",
		Help: "Number of days remaining before SSL/TLS certificate expires.",
	}, probeLabelNames)
	reg.MustRegister(probeCertDaysLeft)

	// 5. Scheduled job aggregated metrics
	jobTotalNodes := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "kncc_scheduled_job_total_nodes",
		Help: "Total number of cluster nodes participating in the scheduled check run.",
	}, []string{"job"})
	reg.MustRegister(jobTotalNodes)

	jobHealthyNodes := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "kncc_scheduled_job_healthy_nodes",
		Help: "Number of cluster nodes where all probe targets succeeded.",
	}, []string{"job"})
	reg.MustRegister(jobHealthyNodes)

	jobLastTimestamp := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "kncc_scheduled_job_last_run_timestamp_seconds",
		Help: "Unix timestamp (seconds) when the scheduled check last completed.",
	}, []string{"job"})
	reg.MustRegister(jobLastTimestamp)

	jobLastDuration := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "kncc_scheduled_job_last_run_duration_seconds",
		Help: "Total execution duration (seconds) of the scheduled check run.",
	}, []string{"job"})
	reg.MustRegister(jobLastDuration)

	return &Metrics{
		Registry:          reg,
		manualChecksTotal: manualChecks,
		probeSuccess:      probeSuccess,
		probeDuration:     probeDuration,
		probeHTTPStatus:   probeHTTPStatus,
		probeCertDaysLeft: probeCertDaysLeft,
		jobTotalNodes:     jobTotalNodes,
		jobHealthyNodes:   jobHealthyNodes,
		jobLastTimestamp:  jobLastTimestamp,
		jobLastDuration:   jobLastDuration,
	}
}

// RecordManualCheck increments the counter for ad-hoc checks.
func (m *Metrics) RecordManualCheck() {
	if m == nil || m.manualChecksTotal == nil {
		return
	}
	m.manualChecksTotal.WithLabelValues().Inc()
}

// RecordScheduledJobResults updates Prometheus metrics based on the results of a scheduled check run.
func (m *Metrics) RecordScheduledJobResults(jobID string, report *model.Report, duration time.Duration) {
	if m == nil || report == nil {
		return
	}

	modeStr := string(report.Mode)
	nsStr := report.Namespace
	healthyNodesCount := 0

	for _, nodeRes := range report.NodeResults {
		nodeHealthy := true
		for _, probe := range nodeRes.ServiceResults {
			targetName := probe.Target.Name
			if targetName == "" {
				targetName = probe.Target.TargetIdentifier()
			}
			protoStr := string(probe.Target.Protocol)
			hostStr := probe.Target.Host
			portStr := strconv.Itoa(probe.Target.Port)

			labels := prometheus.Labels{
				"job":         jobID,
				"target":      targetName,
				"protocol":    protoStr,
				"target_host": hostStr,
				"target_port": portStr,
				"node":        nodeRes.NodeName,
				"mode":        modeStr,
				"namespace":   nsStr,
			}

			if probe.Success {
				m.probeSuccess.With(labels).Set(1.0)
			} else {
				m.probeSuccess.With(labels).Set(0.0)
				nodeHealthy = false
			}

			durationSec := float64(probe.LatencyMs) / 1000.0
			m.probeDuration.With(labels).Set(durationSec)

			if probe.HTTPStatusCode > 0 {
				m.probeHTTPStatus.With(labels).Set(float64(probe.HTTPStatusCode))
			}

			if probe.CertDaysLeft != nil {
				m.probeCertDaysLeft.With(labels).Set(float64(*probe.CertDaysLeft))
			}
		}

		if nodeHealthy && len(nodeRes.ServiceResults) > 0 {
			healthyNodesCount++
		}
	}

	totalNodes := report.TotalNodes
	if totalNodes == 0 {
		totalNodes = len(report.NodeResults)
	}

	m.jobTotalNodes.WithLabelValues(jobID).Set(float64(totalNodes))
	m.jobHealthyNodes.WithLabelValues(jobID).Set(float64(healthyNodesCount))
	m.jobLastTimestamp.WithLabelValues(jobID).Set(float64(time.Now().Unix()))
	m.jobLastDuration.WithLabelValues(jobID).Set(duration.Seconds())
}

// EnsureJobMetrics ensures gauge entries exist with default zeros for new jobs.
func (m *Metrics) EnsureJobMetrics(jobID string) {
	if m == nil {
		return
	}
	m.jobTotalNodes.WithLabelValues(jobID).Set(0)
	m.jobHealthyNodes.WithLabelValues(jobID).Set(0)
	m.jobLastTimestamp.WithLabelValues(jobID).Set(0)
	m.jobLastDuration.WithLabelValues(jobID).Set(0)
}

// DeleteJobMetrics purges all probe and aggregated job metrics for the specified job ID.
func (m *Metrics) DeleteJobMetrics(jobID string) {
	if m == nil {
		return
	}
	match := prometheus.Labels{"job": jobID}
	if m.probeSuccess != nil {
		m.probeSuccess.DeletePartialMatch(match)
	}
	if m.probeDuration != nil {
		m.probeDuration.DeletePartialMatch(match)
	}
	if m.probeHTTPStatus != nil {
		m.probeHTTPStatus.DeletePartialMatch(match)
	}
	if m.probeCertDaysLeft != nil {
		m.probeCertDaysLeft.DeletePartialMatch(match)
	}

	if m.jobTotalNodes != nil {
		m.jobTotalNodes.DeleteLabelValues(jobID)
	}
	if m.jobHealthyNodes != nil {
		m.jobHealthyNodes.DeleteLabelValues(jobID)
	}
	if m.jobLastTimestamp != nil {
		m.jobLastTimestamp.DeleteLabelValues(jobID)
	}
	if m.jobLastDuration != nil {
		m.jobLastDuration.DeleteLabelValues(jobID)
	}
}

// scheduledChecksCollector dynamically counts scheduled checks by type (permanent/ephemeral) and status (enabled/disabled).
type scheduledChecksCollector struct {
	s    store.Store
	desc *prometheus.Desc
}

func newScheduledChecksCollector(s store.Store) *scheduledChecksCollector {
	return &scheduledChecksCollector{
		s: s,
		desc: prometheus.NewDesc(
			"kncc_scheduled_checks",
			"Current number of periodic connectivity checks configured in kncc.",
			[]string{"type", "enabled"},
			nil,
		),
	}
}

func (c *scheduledChecksCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.desc
}

func (c *scheduledChecksCollector) Collect(ch chan<- prometheus.Metric) {
	if c.s == nil {
		return
	}
	checks, err := c.s.ListScheduledChecks(context.Background())
	if err != nil {
		return
	}
	counts := map[[2]string]float64{
		{"permanent", "true"}:  0,
		{"permanent", "false"}: 0,
		{"ephemeral", "true"}:  0,
		{"ephemeral", "false"}: 0,
	}
	for _, chk := range checks {
		typ := "permanent"
		if chk.IsEphemeral {
			typ = "ephemeral"
		}
		en := "false"
		if chk.Enabled {
			en = "true"
		}
		counts[[2]string{typ, en}]++
	}
	for k, v := range counts {
		ch <- prometheus.MustNewConstMetric(c.desc, prometheus.GaugeValue, v, k[0], k[1])
	}
}
