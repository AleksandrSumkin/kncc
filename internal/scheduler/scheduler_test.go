package scheduler

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"kncc/internal/config"
	"kncc/internal/metrics"
	"kncc/internal/model"
	"kncc/internal/store"

	dto "github.com/prometheus/client_model/go"
)

type dummyRunner struct {
	store store.Store
}

func (d *dummyRunner) ExecuteCheck(ctx context.Context, report *model.Report) {
	go func() {
		time.Sleep(50 * time.Millisecond)
		nodeRes := model.NodeResult{
			NodeName: "worker-01",
			Status:   "completed",
			ServiceResults: []model.ProbeOutput{
				{
					Target:         report.Targets[0],
					Success:        true,
					LatencyMs:      15,
					HTTPStatusCode: 200,
				},
			},
		}
		_ = d.store.SaveNodeResult(context.Background(), report.ID, nodeRes)
		now := time.Now().UTC()
		_ = d.store.UpdateReportProgress(context.Background(), report.ID, 1, 1, model.StatusCompleted, &now)
	}()
}

func setupTestScheduler(t *testing.T) (*Scheduler, store.Store, func()) {
	t.Helper()
	dir, err := os.MkdirTemp("", "kncc-sched-test-*")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	dbPath := filepath.Join(dir, "sched.db")
	s, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("sqlite store: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.Monitoring.MinInterval = "30s"
	cfg.Monitoring.AllowedIntervals = []string{"30s", "1m", "5m"}

	runner := &dummyRunner{store: s}
	m := metrics.NewMetrics(s)
	sched := NewScheduler(cfg, s, runner, m)

	cleanup := func() {
		sched.Stop()
		_ = s.Close()
		_ = os.RemoveAll(dir)
	}

	return sched, s, cleanup
}

func TestValidateInterval(t *testing.T) {
	cfg := &config.MonitoringConfig{
		MinInterval:      "30s",
		AllowedIntervals: []string{"30s", "1m", "5m", "15m", "1h"},
	}

	// 1. Valid intervals
	d, err := ValidateInterval("1m", cfg)
	if err != nil || d != time.Minute {
		t.Errorf("expected 1m to be valid, got err: %v, d: %v", err, d)
	}

	d, err = ValidateInterval("30s", cfg)
	if err != nil || d != 30*time.Second {
		t.Errorf("expected 30s to be valid, got err: %v", err)
	}

	// 2. Reject interval below min_interval
	cfgWithLowerAllowed := &config.MonitoringConfig{
		MinInterval:      "30s",
		AllowedIntervals: []string{"10s", "30s", "1m"},
	}
	_, err = ValidateInterval("10s", cfgWithLowerAllowed)
	if err == nil {
		t.Error("expected 10s to be rejected because it is < min_interval 30s")
	}

	// 3. Reject interval not in allowed list
	_, err = ValidateInterval("2m", cfg)
	if err == nil {
		t.Error("expected 2m to be rejected because it is not in allowed_intervals")
	}

	// 4. Invalid duration string
	_, err = ValidateInterval("invalid", cfg)
	if err == nil {
		t.Error("expected invalid duration string to fail")
	}
}

func TestScheduler_StartupAndTriggerJobNow(t *testing.T) {
	sched, s, cleanup := setupTestScheduler(t)
	defer cleanup()

	ctx := context.Background()

	// 1. Add ephemeral check into store prior to Start()
	ephCheck := &model.ScheduledCheck{
		ID:                "eph-startup-check",
		Name:              "Old Ephemeral",
		IntervalStr:       "30s",
		Mode:              model.ModeHostNetwork,
		Namespace:         "kncc",
		Enabled:           true,
		CreatedByUserID:   "user-1",
		CreatedByUsername: "tester",
		IsSystem:          false,
		IsEphemeral:       true,
	}
	_ = s.CreateScheduledCheck(ctx, ephCheck)

	// Configure system check in cfg
	sched.cfg.ScheduledChecks = []config.ScheduledCheckConfig{
		{
			ID:       "sys-forgejo",
			Name:     "Forgejo Connectivity",
			Interval: "30s",
			Mode:     model.ModeHostNetwork,
			Targets: []model.TargetService{
				{Protocol: model.ProtocolHTTP, Host: "forgejo.local", Port: 3000},
			},
		},
	}

	// 2. Start scheduler
	if err := sched.Start(ctx); err != nil {
		t.Fatalf("Scheduler Start failed: %v", err)
	}

	// 3. Verify ephemeral was purged
	_, err := s.GetScheduledCheck(ctx, "eph-startup-check")
	if err != store.ErrNotFound {
		t.Errorf("expected ephemeral check to be purged on startup, got %v", err)
	}

	// 4. Verify system check exists
	sys, err := s.GetScheduledCheck(ctx, "sys-forgejo")
	if err != nil {
		t.Fatalf("expected sys-forgejo to be loaded: %v", err)
	}
	if !sys.IsSystem || sys.IsEphemeral {
		t.Errorf("unexpected sys flags: is_system=%v, is_ephemeral=%v", sys.IsSystem, sys.IsEphemeral)
	}

	// 5. Trigger job now
	rep, err := sched.TriggerJobNow(ctx, "sys-forgejo")
	if err != nil {
		t.Fatalf("TriggerJobNow failed: %v", err)
	}
	if rep.Status != model.StatusCompleted {
		t.Errorf("expected completed status, got %s", rep.Status)
	}

	// Verify last_report_id was updated on the check
	updatedSys, err := s.GetScheduledCheck(ctx, "sys-forgejo")
	if err != nil {
		t.Fatalf("GetScheduledCheck failed: %v", err)
	}
	if updatedSys.LastReportID == nil || *updatedSys.LastReportID != rep.ID {
		t.Errorf("expected last_report_id to be %s, got %v", rep.ID, updatedSys.LastReportID)
	}
}

func TestScheduler_AddRemoveJob(t *testing.T) {
	sched, s, cleanup := setupTestScheduler(t)
	defer cleanup()

	ctx := context.Background()
	_ = sched.Start(ctx)

	check := &model.ScheduledCheck{
		ID:                "dynamic-check-1",
		Name:              "Dynamic DNS",
		IntervalStr:       "30s",
		Mode:              model.ModeHostNetwork,
		Namespace:         "kncc",
		Targets:           []model.TargetService{{Protocol: model.ProtocolICMP, Host: "8.8.8.8"}},
		Enabled:           true,
		CreatedByUserID:   "user-1",
		CreatedByUsername: "tester",
		IsSystem:          false,
		IsEphemeral:       true,
	}

	_ = s.CreateScheduledCheck(ctx, check)
	sched.AddOrUpdateJob(check)

	nextRun := sched.GetJobNextRun(check.ID)
	if nextRun == nil {
		t.Fatal("expected next run to be set for active job")
	}

	sched.RemoveJob(check.ID)
	if sched.GetJobNextRun(check.ID) != nil {
		t.Error("expected job to be removed")
	}
}

func TestScheduler_ReconciliationOnStartup(t *testing.T) {
	sched, s, cleanup := setupTestScheduler(t)
	defer cleanup()

	ctx := context.Background()

	// 1. Pre-populate SQLite with an old system check (e.g. from previous deployment/session)
	oldSysCheck := &model.ScheduledCheck{
		ID:                "sys-old-example",
		Name:              "Old Example Check",
		IntervalStr:       "30s",
		Mode:              model.ModeHostNetwork,
		Namespace:         "kncc",
		Targets:           []model.TargetService{{Protocol: model.ProtocolHTTP, Host: "10.0.0.1", Port: 3000}},
		Enabled:           true,
		CreatedByUserID:   "system",
		CreatedByUsername: "system",
		IsSystem:          true,
		IsEphemeral:       false,
	}
	if err := s.CreateScheduledCheck(ctx, oldSysCheck); err != nil {
		t.Fatalf("CreateScheduledCheck oldSysCheck failed: %v", err)
	}

	// Active configuration does NOT contain sys-old-example (empty scheduled checks list)
	sched.cfg.ScheduledChecks = []config.ScheduledCheckConfig{}

	// 2. Start scheduler
	if err := sched.Start(ctx); err != nil {
		t.Fatalf("Scheduler Start failed: %v", err)
	}

	// 3. Verify old system check was purged from DB on startup
	_, err := s.GetScheduledCheck(ctx, "sys-old-example")
	if err != store.ErrNotFound {
		t.Errorf("expected sys-old-example to be purged on startup, got %v", err)
	}

	// 4. Verify no job was scheduled in memory
	if sched.GetJobNextRun("sys-old-example") != nil {
		t.Errorf("expected sys-old-example not to be running in scheduler")
	}

	// 5. Test reconciliation with a new system check: old one purged, new one registered
	oldSysCheck2 := &model.ScheduledCheck{
		ID:                "sys-orphan-2",
		Name:              "Orphan 2",
		IntervalStr:       "30s",
		Mode:              model.ModeHostNetwork,
		Namespace:         "kncc",
		Targets:           []model.TargetService{{Protocol: model.ProtocolICMP, Host: "1.1.1.1"}},
		Enabled:           true,
		CreatedByUserID:   "system",
		CreatedByUsername: "system",
		IsSystem:          true,
		IsEphemeral:       false,
	}
	_ = s.CreateScheduledCheck(ctx, oldSysCheck2)

	sched.cfg.ScheduledChecks = []config.ScheduledCheckConfig{
		{
			ID:       "sys-active-check",
			Name:     "Active System Check",
			Interval: "30s",
			Mode:     model.ModeHostNetwork,
			Targets: []model.TargetService{
				{Protocol: model.ProtocolICMP, Host: "8.8.8.8"},
			},
		},
	}

	sched.Stop()
	sched2 := NewScheduler(sched.cfg, s, sched.runner, sched.metrics)
	if err := sched2.Start(ctx); err != nil {
		t.Fatalf("Scheduler Start 2 failed: %v", err)
	}
	defer sched2.Stop()

	// sys-orphan-2 should be purged, sys-active-check should exist and run
	if _, err := s.GetScheduledCheck(ctx, "sys-orphan-2"); err != store.ErrNotFound {
		t.Errorf("expected sys-orphan-2 to be purged, got %v", err)
	}
	if _, err := s.GetScheduledCheck(ctx, "sys-active-check"); err != nil {
		t.Errorf("expected sys-active-check to exist, got %v", err)
	}
	if sched2.GetJobNextRun("sys-active-check") == nil {
		t.Errorf("expected sys-active-check to have an active job in scheduler")
	}
}

func hasJobMetric(mfs []*dto.MetricFamily, metricName, jobID string) bool {
	for _, mf := range mfs {
		if mf.GetName() == metricName {
			for _, m := range mf.GetMetric() {
				for _, label := range m.GetLabel() {
					if label.GetName() == "job" && label.GetValue() == jobID {
						return true
					}
				}
			}
		}
	}
	return false
}

func TestScheduler_MetricsCleanupOnRemoveAndToggle(t *testing.T) {
	sched, s, cleanup := setupTestScheduler(t)
	defer cleanup()

	ctx := context.Background()

	// 1. Create a system check
	check := &model.ScheduledCheck{
		ID:                "sys-cleanup-test",
		Name:              "System Cleanup Check",
		IntervalStr:       "30s",
		Mode:              model.ModeHostNetwork,
		Namespace:         "kncc",
		Targets:           []model.TargetService{{Protocol: model.ProtocolTCP, Host: "1.1.1.1", Port: 53}},
		Enabled:           true,
		IsSystem:          true,
		IsEphemeral:       false,
		CreatedByUserID:   "system",
		CreatedByUsername: "system",
	}

	_ = s.CreateScheduledCheck(ctx, check)
	sched.AddOrUpdateJob(check)

	// Trigger run
	rep, err := sched.TriggerJobNow(ctx, check.ID)
	if err != nil || rep == nil {
		t.Fatalf("TriggerJobNow failed: %v", err)
	}

	// Verify metrics are present in exporter
	mfs, err := sched.metrics.Registry.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	if !hasJobMetric(mfs, "kncc_probe_success", check.ID) {
		t.Errorf("expected kncc_probe_success for %s to be present", check.ID)
	}
	if !hasJobMetric(mfs, "kncc_scheduled_job_total_nodes", check.ID) {
		t.Errorf("expected kncc_scheduled_job_total_nodes for %s to be present", check.ID)
	}

	// 2. Toggle to disabled -> metrics should be immediately purged
	check.Enabled = false
	sched.AddOrUpdateJob(check)

	mfsAfterPause, _ := sched.metrics.Registry.Gather()
	if hasJobMetric(mfsAfterPause, "kncc_probe_success", check.ID) {
		t.Errorf("expected kncc_probe_success for %s to be deleted when paused", check.ID)
	}
	if hasJobMetric(mfsAfterPause, "kncc_scheduled_job_total_nodes", check.ID) {
		t.Errorf("expected kncc_scheduled_job_total_nodes for %s to be deleted when paused", check.ID)
	}

	// 3. Re-enable -> metrics re-initialized
	check.Enabled = true
	sched.AddOrUpdateJob(check)
	mfsAfterResume, _ := sched.metrics.Registry.Gather()
	if !hasJobMetric(mfsAfterResume, "kncc_scheduled_job_total_nodes", check.ID) {
		t.Errorf("expected kncc_scheduled_job_total_nodes for %s to be restored upon resume", check.ID)
	}

	// 4. RemoveJob -> metrics purged
	sched.RemoveJob(check.ID)
	mfsAfterRemove, _ := sched.metrics.Registry.Gather()
	if hasJobMetric(mfsAfterRemove, "kncc_scheduled_job_total_nodes", check.ID) {
		t.Errorf("expected kncc_scheduled_job_total_nodes for %s to be deleted upon RemoveJob", check.ID)
	}
}

func TestScheduler_EphemeralMetricsFiltering(t *testing.T) {
	sched, s, cleanup := setupTestScheduler(t)
	defer cleanup()

	ctx := context.Background()

	// 1. Ephemeral check with default config (ExportEphemeralMetrics = false)
	ephemeralCheck := &model.ScheduledCheck{
		ID:                "2e54dc09-febd-4251-ae7d-6f21a7f676ba",
		Name:              "Ad-hoc Ephemeral Check",
		IntervalStr:       "30s",
		Mode:              model.ModeHostNetwork,
		Namespace:         "kncc",
		Targets:           []model.TargetService{{Protocol: model.ProtocolTCP, Host: "10.0.0.1", Port: 80}},
		Enabled:           true,
		IsSystem:          false,
		IsEphemeral:       true,
		CreatedByUserID:   "user-1",
		CreatedByUsername: "tester",
	}

	_ = s.CreateScheduledCheck(ctx, ephemeralCheck)
	sched.AddOrUpdateJob(ephemeralCheck)

	rep, err := sched.TriggerJobNow(ctx, ephemeralCheck.ID)
	if err != nil || rep == nil {
		t.Fatalf("TriggerJobNow failed: %v", err)
	}

	mfs, _ := sched.metrics.Registry.Gather()
	if hasJobMetric(mfs, "kncc_probe_success", ephemeralCheck.ID) {
		t.Errorf("ephemeral check should NOT export kncc_probe_success when ExportEphemeralMetrics is false")
	}
	if hasJobMetric(mfs, "kncc_scheduled_job_total_nodes", ephemeralCheck.ID) {
		t.Errorf("ephemeral check should NOT export kncc_scheduled_job_total_nodes when ExportEphemeralMetrics is false")
	}

	// 2. Enable ExportEphemeralMetrics -> now it should export
	sched.cfg.Metrics.ExportEphemeralMetrics = true
	rep2, err := sched.TriggerJobNow(ctx, ephemeralCheck.ID)
	if err != nil || rep2 == nil {
		t.Fatalf("TriggerJobNow failed: %v", err)
	}

	mfsAllowed, _ := sched.metrics.Registry.Gather()
	if !hasJobMetric(mfsAllowed, "kncc_probe_success", ephemeralCheck.ID) {
		t.Errorf("ephemeral check should export kncc_probe_success when ExportEphemeralMetrics is true")
	}
}
