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
