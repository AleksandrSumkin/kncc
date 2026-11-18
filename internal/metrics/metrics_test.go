package metrics

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"kncc/internal/config"
	"kncc/internal/model"

	dto "github.com/prometheus/client_model/go"
)

type mockStore struct {
	dbSize int64
	count  int64
}

func (m *mockStore) GetDatabaseSizeBytes() (int64, error) {
	return m.dbSize, nil
}

func (m *mockStore) GetManualChecksCount(ctx context.Context) (int64, error) {
	return m.count, nil
}

func (m *mockStore) CreateReport(ctx context.Context, report *model.Report) error { return nil }
func (m *mockStore) GetReport(ctx context.Context, id string) (*model.Report, error) { return nil, nil }
func (m *mockStore) UpdateReportProgress(ctx context.Context, id string, totalNodes, completedNodes int, status model.ReportStatus, completedAt *time.Time) error {
	return nil
}
func (m *mockStore) SaveNodeResult(ctx context.Context, reportID string, result model.NodeResult) error {
	return nil
}
func (m *mockStore) ListReports(ctx context.Context, userID string, role model.Role, limit, offset int) ([]*model.Report, int, error) {
	return nil, 0, nil
}
func (m *mockStore) DeleteExpiredReports(ctx context.Context) (int64, error) { return 0, nil }
func (m *mockStore) CreateUser(ctx context.Context, user *model.User) error   { return nil }
func (m *mockStore) GetUserByUsername(ctx context.Context, username string) (*model.User, error) {
	return nil, nil
}
func (m *mockStore) GetUserByID(ctx context.Context, id string) (*model.User, error) { return nil, nil }
func (m *mockStore) ListUsers(ctx context.Context) ([]*model.User, error)             { return nil, nil }
func (m *mockStore) UpdateUserRole(ctx context.Context, id string, role model.Role) error {
	return nil
}
func (m *mockStore) EnsureAdminUser(ctx context.Context, username, defaultPassword string) error {
	return nil
}
func (m *mockStore) RecordUserRequest(ctx context.Context, id, userID, username string, createdAt time.Time) error {
	return nil
}
func (m *mockStore) GetUserRequestStats(ctx context.Context) (*model.UserStatsResponse, error) {
	return nil, nil
}
func (m *mockStore) RecordUserTargets(ctx context.Context, userID string, targets []model.TargetService, usedAt time.Time) error {
	return nil
}
func (m *mockStore) GetUserTopTargets(ctx context.Context, userID string, limit int) ([]model.TargetService, error) {
	return nil, nil
}
func (m *mockStore) GetSetting(ctx context.Context, key string) (string, error) { return "", nil }
func (m *mockStore) SetSetting(ctx context.Context, key, value string) error    { return nil }
func (m *mockStore) CreateScheduledCheck(ctx context.Context, check *model.ScheduledCheck) error {
	return nil
}
func (m *mockStore) GetScheduledCheck(ctx context.Context, id string) (*model.ScheduledCheck, error) {
	return nil, nil
}
func (m *mockStore) ListScheduledChecks(ctx context.Context) ([]*model.ScheduledCheck, error) {
	return nil, nil
}
func (m *mockStore) UpdateScheduledCheck(ctx context.Context, check *model.ScheduledCheck) error {
	return nil
}
func (m *mockStore) DeleteScheduledCheck(ctx context.Context, id string) error { return nil }
func (m *mockStore) UpdateScheduledCheckLastReport(ctx context.Context, id, newReportID string) error {
	return nil
}
func (m *mockStore) PurgeEphemeralScheduledChecks(ctx context.Context) error { return nil }
func (m *mockStore) PurgeOrphanedSystemScheduledChecks(ctx context.Context, activeIDs []string) error {
	return nil
}
func (m *mockStore) UpsertSystemScheduledCheck(ctx context.Context, check *model.ScheduledCheck) error {
	return nil
}
func (m *mockStore) Ping(ctx context.Context) error { return nil }
func (m *mockStore) Close() error                   { return nil }

func TestMetrics_RegistrationAndRecording(t *testing.T) {
	ms := &mockStore{dbSize: 1048576, count: 5}
	m := NewMetrics(ms)

	if m.Registry == nil {
		t.Fatal("expected registry to be initialized")
	}

	// 1. Manual check recording
	m.RecordManualCheck()

	// 2. Scheduled job results recording
	certDays := 45
	report := &model.Report{
		ID:         "rep-1",
		TotalNodes: 2,
		Mode:       model.ModeHostNetwork,
		Namespace:  "kncc",
		NodeResults: map[string]model.NodeResult{
			"node-1": {
				NodeName: "node-1",
				ServiceResults: []model.ProbeOutput{
					{
						Target: model.TargetService{
							Name:     "K8s API",
							Protocol: model.ProtocolTCP,
							Host:     "10.96.0.1",
							Port:     443,
						},
						Success:        true,
						LatencyMs:      12,
						HTTPStatusCode: 200,
						CertDaysLeft:   &certDays,
					},
				},
			},
		},
	}

	m.RecordScheduledJobResults("test-job", report, 2*time.Second)

	// Gather metrics and verify presence of custom metrics
	metricFamilies, err := m.Registry.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}

	foundMetrics := make(map[string]bool)
	for _, mf := range metricFamilies {
		foundMetrics[mf.GetName()] = true
	}

	expectedMetrics := []string{
		"kncc_database_size_bytes",
		"kncc_manual_checks_total",
		"kncc_probe_success",
		"kncc_probe_duration_seconds",
		"kncc_probe_http_status_code",
		"kncc_probe_cert_days_left",
		"kncc_scheduled_job_total_nodes",
		"kncc_scheduled_job_healthy_nodes",
		"kncc_scheduled_job_last_run_timestamp_seconds",
		"kncc_scheduled_job_last_run_duration_seconds",
		"go_goroutines",
	}

	for _, exp := range expectedMetrics {
		if !foundMetrics[exp] {
			t.Errorf("expected metric family %s to be registered and gathered", exp)
		}
	}
}

func TestMetrics_DeleteJobMetrics(t *testing.T) {
	ms := &mockStore{dbSize: 1048576, count: 5}
	m := NewMetrics(ms)

	certDays := 30
	report1 := &model.Report{
		ID:         "rep-1",
		TotalNodes: 1,
		Mode:       model.ModeHostNetwork,
		Namespace:  "default",
		NodeResults: map[string]model.NodeResult{
			"node-1": {
				NodeName: "node-1",
				ServiceResults: []model.ProbeOutput{
					{
						Target:         model.TargetService{Name: "Target-A", Protocol: model.ProtocolTCP, Host: "10.0.0.1", Port: 80},
						Success:        true,
						LatencyMs:      10,
						HTTPStatusCode: 200,
						CertDaysLeft:   &certDays,
					},
				},
			},
		},
	}
	report2 := &model.Report{
		ID:         "rep-2",
		TotalNodes: 1,
		Mode:       model.ModeHostNetwork,
		Namespace:  "default",
		NodeResults: map[string]model.NodeResult{
			"node-1": {
				NodeName: "node-1",
				ServiceResults: []model.ProbeOutput{
					{
						Target:    model.TargetService{Name: "Target-B", Protocol: model.ProtocolTCP, Host: "10.0.0.2", Port: 443},
						Success:   true,
						LatencyMs: 15,
					},
				},
			},
		},
	}

	m.RecordScheduledJobResults("job-to-delete", report1, time.Second)
	m.RecordScheduledJobResults("job-to-keep", report2, time.Second)

	// Verify both exist
	mfs, err := m.Registry.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	hasJob := func(mfs []*dto.MetricFamily, name, jobID string) bool {
		for _, mf := range mfs {
			if mf.GetName() == name {
				for _, metric := range mf.GetMetric() {
					for _, label := range metric.GetLabel() {
						if label.GetName() == "job" && label.GetValue() == jobID {
							return true
						}
					}
				}
			}
		}
		return false
	}

	if !hasJob(mfs, "kncc_probe_success", "job-to-delete") {
		t.Fatalf("expected job-to-delete to be present before delete")
	}
	if !hasJob(mfs, "kncc_probe_success", "job-to-keep") {
		t.Fatalf("expected job-to-keep to be present before delete")
	}

	// Delete job-to-delete
	m.DeleteJobMetrics("job-to-delete")

	mfsAfter, err := m.Registry.Gather()
	if err != nil {
		t.Fatalf("gather after delete: %v", err)
	}

	jobMetricsToCheck := []string{
		"kncc_probe_success",
		"kncc_probe_duration_seconds",
		"kncc_probe_http_status_code",
		"kncc_probe_cert_days_left",
		"kncc_scheduled_job_total_nodes",
		"kncc_scheduled_job_healthy_nodes",
		"kncc_scheduled_job_last_run_timestamp_seconds",
		"kncc_scheduled_job_last_run_duration_seconds",
	}

	for _, metricName := range jobMetricsToCheck {
		if hasJob(mfsAfter, metricName, "job-to-delete") {
			t.Errorf("metric %s still contains deleted job 'job-to-delete'", metricName)
		}
	}

	// Verify job-to-keep still exists
	if !hasJob(mfsAfter, "kncc_probe_success", "job-to-keep") {
		t.Errorf("kncc_probe_success should still contain 'job-to-keep'")
	}
	if !hasJob(mfsAfter, "kncc_scheduled_job_total_nodes", "job-to-keep") {
		t.Errorf("kncc_scheduled_job_total_nodes should still contain 'job-to-keep'")
	}
}

func TestMetricsAuthMiddleware_Disabled(t *testing.T) {
	cfg := &config.MetricsAuthConfig{Enabled: false}
	mw := MetricsAuthMiddleware(cfg)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))

	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 OK when auth disabled, got %d", rec.Code)
	}
}

func TestMetricsAuthMiddleware_BasicAuth(t *testing.T) {
	cfg := &config.MetricsAuthConfig{
		Enabled:  true,
		Type:     "basic",
		Username: "prom-user",
		Password: "prom-secret-password",
	}
	mw := MetricsAuthMiddleware(cfg)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("metrics-data"))
	}))

	// Case 1: No auth headers -> 401
	req1 := httptest.NewRequest("GET", "/metrics", nil)
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized, got %d", rec1.Code)
	}

	// Case 2: Wrong credentials -> 401
	req2 := httptest.NewRequest("GET", "/metrics", nil)
	req2.SetBasicAuth("prom-user", "wrong-password")
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for wrong credentials, got %d", rec2.Code)
	}

	// Case 3: Valid credentials -> 200
	req3 := httptest.NewRequest("GET", "/metrics", nil)
	req3.SetBasicAuth("prom-user", "prom-secret-password")
	rec3 := httptest.NewRecorder()
	handler.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusOK {
		t.Errorf("expected 200 OK for valid basic auth, got %d", rec3.Code)
	}
}

func TestMetricsAuthMiddleware_BearerAuth(t *testing.T) {
	cfg := &config.MetricsAuthConfig{
		Enabled: true,
		Type:    "bearer",
		Token:   "super-secret-bearer-token",
	}
	mw := MetricsAuthMiddleware(cfg)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("metrics-data"))
	}))

	// Case 1: No token -> 401
	req1 := httptest.NewRequest("GET", "/metrics", nil)
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized, got %d", rec1.Code)
	}

	// Case 2: Invalid token -> 401
	req2 := httptest.NewRequest("GET", "/metrics", nil)
	req2.Header.Set("Authorization", "Bearer invalid-token")
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized for invalid token, got %d", rec2.Code)
	}

	// Case 3: Valid token -> 200
	req3 := httptest.NewRequest("GET", "/metrics", nil)
	req3.Header.Set("Authorization", "Bearer super-secret-bearer-token")
	rec3 := httptest.NewRecorder()
	handler.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusOK {
		t.Errorf("expected 200 OK for valid bearer token, got %d", rec3.Code)
	}
}
