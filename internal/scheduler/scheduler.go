package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"kncc/internal/config"
	"kncc/internal/metrics"
	"kncc/internal/model"
	"kncc/internal/store"

	"github.com/google/uuid"
)

// CheckRunner matches the runner interface needed to execute checks.
type CheckRunner interface {
	ExecuteCheck(ctx context.Context, report *model.Report)
}

// Scheduler coordinates periodic background connectivity checks and metrics updates.
type Scheduler struct {
	cfg     *config.Config
	store   store.Store
	runner  CheckRunner
	metrics *metrics.Metrics

	jobs   map[string]*activeJob
	mu     sync.RWMutex
	stopCh chan struct{}
}

type activeJob struct {
	check     *model.ScheduledCheck
	duration  time.Duration
	stopCh    chan struct{}
	isRunning atomic.Bool
	lastRun   time.Time
	nextRun   time.Time
}

// NewScheduler creates an instance of Scheduler.
func NewScheduler(cfg *config.Config, s store.Store, runner CheckRunner, m *metrics.Metrics) *Scheduler {
	return &Scheduler{
		cfg:     cfg,
		store:   s,
		runner:  runner,
		metrics: m,
		jobs:    make(map[string]*activeJob),
		stopCh:  make(chan struct{}),
	}
}

// ValidateInterval checks that interval string is valid, meets minimum threshold, and is in allowed list.
func ValidateInterval(intervalStr string, cfg *config.MonitoringConfig) (time.Duration, error) {
	if intervalStr == "" {
		return 0, errors.New("interval cannot be empty")
	}

	d, err := time.ParseDuration(intervalStr)
	if err != nil {
		return 0, fmt.Errorf("invalid interval duration format %q: %w", intervalStr, err)
	}

	if cfg == nil {
		if d < 10*time.Second {
			return 0, errors.New("interval must be at least 10s")
		}
		return d, nil
	}

	// 1. Check allowed list if specified
	if len(cfg.AllowedIntervals) > 0 {
		allowed := false
		for _, a := range cfg.AllowedIntervals {
			if a == intervalStr {
				allowed = true
				break
			}
		}
		if !allowed {
			return 0, fmt.Errorf("interval %q is not in allowed intervals list: %v", intervalStr, cfg.AllowedIntervals)
		}
	}

	// 2. Check min interval
	if cfg.MinInterval != "" {
		minDur, err := time.ParseDuration(cfg.MinInterval)
		if err == nil && d < minDur {
			return 0, fmt.Errorf("interval %q cannot be less than minimum allowed interval %q", intervalStr, cfg.MinInterval)
		}
	}

	return d, nil
}

// Start bootstraps declarative checks, purges old ephemeral checks, and starts all active scheduled checks.
func (s *Scheduler) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 1. Purge ephemeral checks from previous server runs
	if err := s.store.PurgeEphemeralScheduledChecks(ctx); err != nil {
		slog.Warn("Failed to purge ephemeral scheduled checks on startup", "error", err)
	} else {
		slog.Info("Purged ephemeral scheduled checks from previous sessions")
	}

	// 2. Load and upsert system checks from declarative config
	for _, sc := range s.cfg.ScheduledChecks {
		if sc.ID == "" {
			sc.ID = "sys-" + uuid.NewString()[:8]
		}
		enabled := true
		if sc.Enabled != nil {
			enabled = *sc.Enabled
		}
		mode := sc.Mode
		if mode == "" {
			mode = model.ModeHostNetwork
		}
		ns := sc.Namespace
		if ns == "" {
			ns = s.cfg.K8s.Namespace
		}

		check := &model.ScheduledCheck{
			ID:                sc.ID,
			Name:              sc.Name,
			IntervalStr:       sc.Interval,
			Mode:              mode,
			Namespace:         ns,
			Targets:           sc.Targets,
			Enabled:           enabled,
			CreatedByUserID:   "system",
			CreatedByUsername: "system",
			IsSystem:          true,
			IsEphemeral:       false,
		}

		if err := s.store.UpsertSystemScheduledCheck(ctx, check); err != nil {
			slog.Error("Failed to upsert system scheduled check", "id", sc.ID, "error", err)
		} else {
			slog.Info("Loaded persistent scheduled check", "id", sc.ID, "name", sc.Name, "interval", sc.Interval)
		}
	}

	// 3. Query all scheduled checks and start tickers for enabled checks
	checks, err := s.store.ListScheduledChecks(ctx)
	if err != nil {
		return fmt.Errorf("loading scheduled checks: %w", err)
	}

	for _, check := range checks {
		s.metrics.EnsureJobMetrics(check.ID)
		if check.Enabled {
			s.startJobLocked(check)
		}
	}

	return nil
}

// Stop stops all running job tickers and halts the scheduler.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	close(s.stopCh)
	for id, job := range s.jobs {
		close(job.stopCh)
		delete(s.jobs, id)
	}
	slog.Info("Scheduler stopped cleanly")
}

// AddOrUpdateJob registers or restarts a scheduled check.
func (s *Scheduler) AddOrUpdateJob(check *model.ScheduledCheck) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Stop existing ticker if running
	if existing, exists := s.jobs[check.ID]; exists {
		close(existing.stopCh)
		delete(s.jobs, check.ID)
	}

	s.metrics.EnsureJobMetrics(check.ID)

	if check.Enabled {
		s.startJobLocked(check)
	}
}

// RemoveJob stops and removes a job from scheduler.
func (s *Scheduler) RemoveJob(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if job, exists := s.jobs[id]; exists {
		close(job.stopCh)
		delete(s.jobs, id)
	}
}

// GetJobNextRun returns the expected next run time for a job if scheduled.
func (s *Scheduler) GetJobNextRun(id string) *time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if job, exists := s.jobs[id]; exists {
		t := job.nextRun
		return &t
	}
	return nil
}

func (s *Scheduler) startJobLocked(check *model.ScheduledCheck) {
	dur, err := ValidateInterval(check.IntervalStr, &s.cfg.Monitoring)
	if err != nil {
		slog.Error("Cannot start scheduled check due to invalid interval", "id", check.ID, "interval", check.IntervalStr, "error", err)
		return
	}

	stop := make(chan struct{})
	job := &activeJob{
		check:    check,
		duration: dur,
		stopCh:   stop,
		nextRun:  time.Now().Add(dur),
	}
	s.jobs[check.ID] = job

	go func() {
		ticker := time.NewTicker(dur)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				s.mu.Lock()
				job.nextRun = time.Now().Add(dur)
				s.mu.Unlock()

				go s.executeCheckRun(job)
			case <-stop:
				return
			case <-s.stopCh:
				return
			}
		}
	}()

	slog.Info("Scheduled background check started", "id", check.ID, "name", check.Name, "interval", dur.String())
}

func (s *Scheduler) executeCheckRun(job *activeJob) (*model.Report, error) {
	// Overlap protection
	if !job.isRunning.CompareAndSwap(false, true) {
		slog.Warn("Skipping scheduled check run because previous run is still in progress", "job", job.check.ID, "name", job.check.Name)
		return nil, errors.New("job execution already in progress")
	}
	defer job.isRunning.Store(false)

	startTime := time.Now()
	job.lastRun = startTime

	reportID := uuid.NewString()
	report := &model.Report{
		ID:          reportID,
		UserID:      job.check.CreatedByUserID,
		Username:    "scheduler/" + job.check.Name,
		Mode:        job.check.Mode,
		Namespace:   job.check.Namespace,
		Targets:     job.check.Targets,
		Status:      model.StatusPending,
		CreatedAt:   startTime.UTC(),
		ExpiresAt:   startTime.UTC().Add(s.cfg.Storage.ReportTTL),
		NodeResults: make(map[string]model.NodeResult),
	}

	ctx := context.Background()
	if err := s.store.CreateReport(ctx, report); err != nil {
		slog.Error("Scheduler failed to create report in store", "job", job.check.ID, "error", err)
		return nil, err
	}

	// Trigger runner execution
	s.runner.ExecuteCheck(ctx, report)

	// Poll store until report completes or fails (timeout: 60s)
	pollTimeout := 60 * time.Second
	pollCtx, cancel := context.WithTimeout(ctx, pollTimeout)
	defer cancel()

	var finalReport *model.Report
	for {
		select {
		case <-pollCtx.Done():
			slog.Warn("Scheduled check run timed out waiting for report completion", "job", job.check.ID, "report", reportID)
			now := time.Now().UTC()
			_ = s.store.UpdateReportProgress(ctx, reportID, 0, 0, model.StatusFailed, &now)
			finalReport, _ = s.store.GetReport(ctx, reportID)
			break
		case <-time.After(300 * time.Millisecond):
			r, err := s.store.GetReport(pollCtx, reportID)
			if err == nil && (r.Status == model.StatusCompleted || r.Status == model.StatusFailed) {
				finalReport = r
				break
			}
		}
		if finalReport != nil {
			break
		}
	}

	if finalReport == nil {
		finalReport, _ = s.store.GetReport(ctx, reportID)
	}

	elapsed := time.Since(startTime)

	// Update last report on scheduled check and purge prior report
	if err := s.store.UpdateScheduledCheckLastReport(ctx, job.check.ID, reportID); err != nil {
		slog.Error("Failed to update scheduled check last report in store", "job", job.check.ID, "error", err)
	}

	// Publish metrics to Prometheus
	if finalReport != nil {
		s.metrics.RecordScheduledJobResults(job.check.ID, finalReport, elapsed)
	}

	slog.Info("Scheduled check completed", "job", job.check.ID, "status", report.Status, "duration", elapsed.String())
	return finalReport, nil
}

// TriggerJobNow forces immediate execution of a scheduled check.
func (s *Scheduler) TriggerJobNow(ctx context.Context, id string) (*model.Report, error) {
	check, err := s.store.GetScheduledCheck(ctx, id)
	if err != nil {
		return nil, err
	}

	s.mu.RLock()
	job, exists := s.jobs[id]
	s.mu.RUnlock()

	if !exists {
		// Temporary activeJob for execution
		dur, _ := time.ParseDuration(check.IntervalStr)
		job = &activeJob{
			check:    check,
			duration: dur,
		}
	}

	return s.executeCheckRun(job)
}
