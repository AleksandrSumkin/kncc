package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"kncc/internal/model"
)

func setupTestStore(t *testing.T) (*SQLiteStore, func()) {
	t.Helper()
	dir, err := os.MkdirTemp("", "kncc-store-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	dbPath := filepath.Join(dir, "test.db")

	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("failed to open sqlite store: %v", err)
	}

	cleanup := func() {
		_ = store.Close()
		_ = os.RemoveAll(dir)
	}
	return store, cleanup
}

func TestStore_ReportLifecycle(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	report := &model.Report{
		ID:        "report-123",
		UserID:    "user-1",
		Username:  "alice",
		Mode:      model.ModeHostNetwork,
		Namespace: "kncc",
		Targets: []model.TargetService{
			{Protocol: model.ProtocolTCP, Host: "10.0.0.1", Port: 80},
			{Protocol: model.ProtocolHTTP, Host: "api.corp.local", Port: 80, Path: "/healthz"},
		},
		Status:         model.StatusRunning,
		TotalNodes:     2,
		CompletedNodes: 0,
		CreatedAt:      time.Now().UTC(),
		ExpiresAt:      time.Now().UTC().Add(24 * time.Hour),
	}

	// 1. Create Report
	if err := s.CreateReport(ctx, report); err != nil {
		t.Fatalf("CreateReport failed: %v", err)
	}

	// 2. Get Report
	got, err := s.GetReport(ctx, report.ID)
	if err != nil {
		t.Fatalf("GetReport failed: %v", err)
	}
	if got.Username != "alice" {
		t.Errorf("expected username alice, got %s", got.Username)
	}
	if len(got.Targets) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(got.Targets))
	}

	// 3. Save Node Result
	nodeRes := model.NodeResult{
		NodeName: "node-1",
		Status:   "completed",
		ServiceResults: []model.ProbeOutput{
			{
				Target:    report.Targets[0],
				Success:   true,
				LatencyMs: 12,
			},
		},
		FinishedAt: time.Now().UTC(),
	}
	if err := s.SaveNodeResult(ctx, report.ID, nodeRes); err != nil {
		t.Fatalf("SaveNodeResult failed: %v", err)
	}

	// 4. Update Progress
	now := time.Now().UTC()
	if err := s.UpdateReportProgress(ctx, report.ID, 2, 1, model.StatusCompleted, &now); err != nil {
		t.Fatalf("UpdateReportProgress failed: %v", err)
	}

	// 5. Verify Updated Report
	got, err = s.GetReport(ctx, report.ID)
	if err != nil {
		t.Fatalf("GetReport after update failed: %v", err)
	}
	if got.CompletedNodes != 1 {
		t.Errorf("expected 1 completed node, got %d", got.CompletedNodes)
	}
	if got.Status != model.StatusCompleted {
		t.Errorf("expected status completed, got %s", got.Status)
	}
	if len(got.NodeResults) != 1 {
		t.Fatalf("expected 1 node result, got %d", len(got.NodeResults))
	}
	if got.NodeResults["node-1"].ServiceResults[0].LatencyMs != 12 {
		t.Errorf("expected latency 12, got %d", got.NodeResults["node-1"].ServiceResults[0].LatencyMs)
	}
}

func TestStore_ListReports_RBAC(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Alice report
	_ = s.CreateReport(ctx, &model.Report{
		ID:        "rep-alice",
		UserID:    "alice-id",
		Username:  "alice",
		Mode:      model.ModeHostNetwork,
		Namespace: "kncc",
		Status:    model.StatusCompleted,
		CreatedAt: time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(1 * time.Hour),
	})

	// Bob report
	_ = s.CreateReport(ctx, &model.Report{
		ID:        "rep-bob",
		UserID:    "bob-id",
		Username:  "bob",
		Mode:      model.ModeHostNetwork,
		Namespace: "kncc",
		Status:    model.StatusCompleted,
		CreatedAt: time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(1 * time.Hour),
	})

	// User role: Alice should only see her report
	aliceReports, total, err := s.ListReports(ctx, "alice-id", model.RoleUser, 10, 0)
	if err != nil {
		t.Fatalf("ListReports for Alice failed: %v", err)
	}
	if total != 1 || len(aliceReports) != 1 || aliceReports[0].ID != "rep-alice" {
		t.Errorf("expected Alice to see 1 report (rep-alice), got %d reports (total %d)", len(aliceReports), total)
	}

	// Admin role: Admin should see both reports
	allReports, total, err := s.ListReports(ctx, "admin-id", model.RoleAdmin, 10, 0)
	if err != nil {
		t.Fatalf("ListReports for Admin failed: %v", err)
	}
	if total != 2 || len(allReports) != 2 {
		t.Errorf("expected Admin to see 2 reports, got %d (total %d)", len(allReports), total)
	}
}

func TestStore_DeleteExpiredReports(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Expired report
	_ = s.CreateReport(ctx, &model.Report{
		ID:        "rep-expired",
		UserID:    "u1",
		Username:  "u1",
		Mode:      model.ModeHostNetwork,
		Namespace: "kncc",
		Status:    model.StatusCompleted,
		CreatedAt: time.Now().UTC().Add(-2 * time.Hour),
		ExpiresAt: time.Now().UTC().Add(-1 * time.Hour),
	})

	// Active report
	_ = s.CreateReport(ctx, &model.Report{
		ID:        "rep-active",
		UserID:    "u1",
		Username:  "u1",
		Mode:      model.ModeHostNetwork,
		Namespace: "kncc",
		Status:    model.StatusCompleted,
		CreatedAt: time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(1 * time.Hour),
	})

	deleted, err := s.DeleteExpiredReports(ctx)
	if err != nil {
		t.Fatalf("DeleteExpiredReports failed: %v", err)
	}
	if deleted != 1 {
		t.Errorf("expected 1 deleted report, got %d", deleted)
	}

	// rep-active should remain
	if _, err := s.GetReport(ctx, "rep-active"); err != nil {
		t.Errorf("expected rep-active to still exist, got: %v", err)
	}
	// rep-expired should be gone
	if _, err := s.GetReport(ctx, "rep-expired"); err != ErrNotFound {
		t.Errorf("expected ErrNotFound for rep-expired, got: %v", err)
	}
}

func TestStore_UsersAndEnsureAdmin(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// EnsureAdminUser
	if err := s.EnsureAdminUser(ctx, "admin", "secret123"); err != nil {
		t.Fatalf("EnsureAdminUser failed: %v", err)
	}

	// Idempotency check
	if err := s.EnsureAdminUser(ctx, "admin", "secret123"); err != nil {
		t.Fatalf("EnsureAdminUser second call failed: %v", err)
	}

	admin, err := s.GetUserByUsername(ctx, "admin")
	if err != nil {
		t.Fatalf("GetUserByUsername failed: %v", err)
	}
	if admin.Role != model.RoleAdmin {
		t.Errorf("expected role admin, got %s", admin.Role)
	}

	users, err := s.ListUsers(ctx)
	if err != nil {
		t.Fatalf("ListUsers failed: %v", err)
	}
	if len(users) != 1 {
		t.Errorf("expected 1 user, got %d", len(users))
	}
}

func TestStore_EnsureAdminUser_EmptyPasswordAutoGenerates(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// EnsureAdminUser with empty password should auto-generate a valid hash
	if err := s.EnsureAdminUser(ctx, "admin-auto", ""); err != nil {
		t.Fatalf("EnsureAdminUser with empty password failed: %v", err)
	}

	admin, err := s.GetUserByUsername(ctx, "admin-auto")
	if err != nil {
		t.Fatalf("GetUserByUsername failed: %v", err)
	}
	if admin.PasswordHash == "" {
		t.Errorf("expected non-empty PasswordHash for auto-generated admin")
	}
	if admin.Role != model.RoleAdmin {
		t.Errorf("expected role admin, got %s", admin.Role)
	}
}

func TestStore_UpdateUserRole(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	user := &model.User{
		ID:        "ldap-user-1",
		Username:  "bob",
		Role:      model.RoleUser,
		Provider:  "ldap",
		Email:     "bob@example.com",
		CreatedAt: time.Now().UTC(),
	}
	if err := s.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	// 1. Promote to Admin
	if err := s.UpdateUserRole(ctx, "ldap-user-1", model.RoleAdmin); err != nil {
		t.Fatalf("UpdateUserRole to admin failed: %v", err)
	}

	updated, err := s.GetUserByID(ctx, "ldap-user-1")
	if err != nil {
		t.Fatalf("GetUserByID failed: %v", err)
	}
	if updated.Role != model.RoleAdmin {
		t.Errorf("expected role admin after promotion, got %s", updated.Role)
	}

	// 2. Demote to User
	if err := s.UpdateUserRole(ctx, "ldap-user-1", model.RoleUser); err != nil {
		t.Fatalf("UpdateUserRole to user failed: %v", err)
	}

	demoted, err := s.GetUserByID(ctx, "ldap-user-1")
	if err != nil {
		t.Fatalf("GetUserByID failed: %v", err)
	}
	if demoted.Role != model.RoleUser {
		t.Errorf("expected role user after demotion, got %s", demoted.Role)
	}

	// 3. Non-existent user returns ErrNotFound
	if err := s.UpdateUserRole(ctx, "non-existent-user-id", model.RoleAdmin); err != ErrNotFound {
		t.Errorf("expected ErrNotFound for non-existent user, got: %v", err)
	}
}

func TestStore_Settings(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Not found initially
	_, err := s.GetSetting(ctx, "default_theme")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}

	// Set value
	if err := s.SetSetting(ctx, "default_theme", "dark"); err != nil {
		t.Fatalf("SetSetting failed: %v", err)
	}

	// Retrieve value
	val, err := s.GetSetting(ctx, "default_theme")
	if err != nil {
		t.Fatalf("GetSetting failed: %v", err)
	}
	if val != "dark" {
		t.Errorf("expected 'dark', got '%s'", val)
	}

	// Update existing setting
	if err := s.SetSetting(ctx, "default_theme", "light"); err != nil {
		t.Fatalf("SetSetting update failed: %v", err)
	}
	val, err = s.GetSetting(ctx, "default_theme")
	if err != nil {
		t.Fatalf("GetSetting after update failed: %v", err)
	}
	if val != "light" {
		t.Errorf("expected 'light', got '%s'", val)
	}
}

func TestStore_UserRequestStats(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// 1. Create two users: alice and bob
	alice := &model.User{
		ID:        "user-alice",
		Username:  "alice",
		Role:      model.RoleAdmin,
		Provider:  "local",
		CreatedAt: time.Now().UTC(),
	}
	bob := &model.User{
		ID:        "user-bob",
		Username:  "bob",
		Role:      model.RoleUser,
		Provider:  "ldap",
		CreatedAt: time.Now().UTC(),
	}
	if err := s.CreateUser(ctx, alice); err != nil {
		t.Fatalf("failed to create alice: %v", err)
	}
	if err := s.CreateUser(ctx, bob); err != nil {
		t.Fatalf("failed to create bob: %v", err)
	}

	now := time.Now().UTC()

	// 2. Add requests for Alice:
	// - 1 request 2 hours ago (Day, Week, Month, Year, Total)
	// - 1 request 3 days ago (Week, Month, Year, Total)
	// - 1 request 15 days ago (Month, Year, Total)
	// - 1 request 60 days ago (Year, Total)
	// - 1 request 400 days ago (Total only)
	_ = s.RecordUserRequest(ctx, "req-a1", alice.ID, alice.Username, now.Add(-2*time.Hour))
	_ = s.RecordUserRequest(ctx, "req-a2", alice.ID, alice.Username, now.Add(-3*24*time.Hour))
	_ = s.RecordUserRequest(ctx, "req-a3", alice.ID, alice.Username, now.Add(-15*24*time.Hour))
	_ = s.RecordUserRequest(ctx, "req-a4", alice.ID, alice.Username, now.Add(-60*24*time.Hour))
	_ = s.RecordUserRequest(ctx, "req-a5", alice.ID, alice.Username, now.Add(-400*24*time.Hour))

	// 3. Add requests for Bob via CreateReport (verifying automatic mirroring):
	// - 1 report 1 hour ago (Day, Week, Month, Year, Total)
	repBob := &model.Report{
		ID:        "rep-bob-1",
		UserID:    bob.ID,
		Username:  bob.Username,
		Mode:      model.ModeHostNetwork,
		Namespace: "kncc",
		Status:    model.StatusCompleted,
		CreatedAt: now.Add(-1 * time.Hour),
		ExpiresAt: now.Add(24 * time.Hour),
	}
	if err := s.CreateReport(ctx, repBob); err != nil {
		t.Fatalf("CreateReport failed: %v", err)
	}

	// 4. Query stats
	resp, err := s.GetUserRequestStats(ctx)
	if err != nil {
		t.Fatalf("GetUserRequestStats failed: %v", err)
	}

	if len(resp.Users) != 2 {
		t.Fatalf("expected 2 users in stats, got %d", len(resp.Users))
	}

	// Alice should be first since she has 5 total requests vs Bob's 1
	aliceStat := resp.Users[0]
	if aliceStat.Username != "alice" {
		t.Errorf("expected alice first, got %s", aliceStat.Username)
	}
	if aliceStat.DayCount != 1 {
		t.Errorf("expected alice day count 1, got %d", aliceStat.DayCount)
	}
	if aliceStat.WeekCount != 2 {
		t.Errorf("expected alice week count 2, got %d", aliceStat.WeekCount)
	}
	if aliceStat.MonthCount != 3 {
		t.Errorf("expected alice month count 3, got %d", aliceStat.MonthCount)
	}
	if aliceStat.YearCount != 4 {
		t.Errorf("expected alice year count 4, got %d", aliceStat.YearCount)
	}
	if aliceStat.TotalCount != 5 {
		t.Errorf("expected alice total count 5, got %d", aliceStat.TotalCount)
	}
	if aliceStat.LastActive == nil {
		t.Errorf("expected alice last_active to be set")
	}

	// Bob stats
	bobStat := resp.Users[1]
	if bobStat.Username != "bob" {
		t.Errorf("expected bob second, got %s", bobStat.Username)
	}
	if bobStat.DayCount != 1 || bobStat.WeekCount != 1 || bobStat.MonthCount != 1 || bobStat.YearCount != 1 || bobStat.TotalCount != 1 {
		t.Errorf("expected bob to have all counts = 1, got %+v", bobStat)
	}

	// Total Summary
	if resp.Total.DayCount != 2 {
		t.Errorf("expected total day count 2, got %d", resp.Total.DayCount)
	}
	if resp.Total.WeekCount != 3 {
		t.Errorf("expected total week count 3, got %d", resp.Total.WeekCount)
	}
	if resp.Total.MonthCount != 4 {
		t.Errorf("expected total month count 4, got %d", resp.Total.MonthCount)
	}
	if resp.Total.YearCount != 5 {
		t.Errorf("expected total year count 5, got %d", resp.Total.YearCount)
	}
	if resp.Total.TotalCount != 6 {
		t.Errorf("expected total total count 6, got %d", resp.Total.TotalCount)
	}
}

func TestStore_UserTargetHistory(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()
	userID := "user-123"

	// Initially empty
	targets, err := s.GetUserTopTargets(ctx, userID, 3)
	if err != nil {
		t.Fatalf("GetUserTopTargets failed: %v", err)
	}
	if len(targets) != 0 {
		t.Errorf("expected 0 targets initially, got %d", len(targets))
	}

	now := time.Now().UTC()

	// 1. First check: target A
	targetA := model.TargetService{Protocol: model.ProtocolTCP, Host: "10.0.0.1", Port: 80}
	if err := s.RecordUserTargets(ctx, userID, []model.TargetService{targetA}, now); err != nil {
		t.Fatalf("RecordUserTargets failed: %v", err)
	}

	targets, err = s.GetUserTopTargets(ctx, userID, 3)
	if err != nil {
		t.Fatalf("GetUserTopTargets failed: %v", err)
	}
	if len(targets) != 1 || targets[0].Host != "10.0.0.1" {
		t.Fatalf("expected 1 target [10.0.0.1], got %+v", targets)
	}

	// 2. Second check: target B and target C
	targetB := model.TargetService{Protocol: model.ProtocolHTTP, Host: "api.example.com", Port: 8080, Path: "/health"}
	targetC := model.TargetService{Protocol: model.ProtocolICMP, Host: "1.1.1.1"}
	if err := s.RecordUserTargets(ctx, userID, []model.TargetService{targetB, targetC}, now.Add(time.Minute)); err != nil {
		t.Fatalf("RecordUserTargets failed: %v", err)
	}

	targets, err = s.GetUserTopTargets(ctx, userID, 3)
	if err != nil {
		t.Fatalf("GetUserTopTargets failed: %v", err)
	}
	if len(targets) != 3 {
		t.Fatalf("expected 3 targets, got %d", len(targets))
	}

	// 3. Increment target B multiple times so it becomes the most frequent
	for i := 0; i < 5; i++ {
		_ = s.RecordUserTargets(ctx, userID, []model.TargetService{targetB}, now.Add(time.Duration(i+2)*time.Minute))
	}

	targets, err = s.GetUserTopTargets(ctx, userID, 3)
	if err != nil {
		t.Fatalf("GetUserTopTargets failed: %v", err)
	}
	if len(targets) != 3 {
		t.Fatalf("expected 3 targets, got %d", len(targets))
	}
	// targetB should be first due to highest frequency
	if targets[0].Host != "api.example.com" {
		t.Errorf("expected top target to be api.example.com, got %s", targets[0].Host)
	}
}

func TestStore_Ping(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := s.Ping(ctx); err != nil {
		t.Fatalf("expected ping to succeed on open store, got: %v", err)
	}

	// Close store and verify ping fails
	if err := s.Close(); err != nil {
		t.Fatalf("closing store failed: %v", err)
	}

	if err := s.Ping(ctx); err == nil {
		t.Fatalf("expected ping to fail on closed store, got nil")
	}
}

func TestStore_ScheduledChecksLifecycle(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// 1. Create a dynamic scheduled check
	check := &model.ScheduledCheck{
		ID:                "sched-1",
		Name:              "Daily DNS Check",
		IntervalStr:       "5m",
		Mode:              model.ModeHostNetwork,
		Namespace:         "kncc",
		Targets:           []model.TargetService{{Protocol: model.ProtocolICMP, Host: "8.8.8.8"}},
		Enabled:           true,
		CreatedByUserID:   "user-1",
		CreatedByUsername: "bob",
		IsSystem:          false,
		IsEphemeral:       true,
	}

	if err := s.CreateScheduledCheck(ctx, check); err != nil {
		t.Fatalf("CreateScheduledCheck failed: %v", err)
	}

	// 2. Get the check
	got, err := s.GetScheduledCheck(ctx, "sched-1")
	if err != nil {
		t.Fatalf("GetScheduledCheck failed: %v", err)
	}
	if got.Name != "Daily DNS Check" || got.IntervalStr != "5m" || !got.IsEphemeral {
		t.Errorf("unexpected check data: %+v", got)
	}

	// 3. Update check
	got.IntervalStr = "15m"
	got.Enabled = false
	if err := s.UpdateScheduledCheck(ctx, got); err != nil {
		t.Fatalf("UpdateScheduledCheck failed: %v", err)
	}

	updated, err := s.GetScheduledCheck(ctx, "sched-1")
	if err != nil {
		t.Fatalf("GetScheduledCheck failed: %v", err)
	}
	if updated.IntervalStr != "15m" || updated.Enabled != false {
		t.Errorf("expected interval 15m and enabled false, got %+v", updated)
	}

	// 4. Upsert system check
	sysCheck := &model.ScheduledCheck{
		ID:                "sys-1",
		Name:              "Forgejo Check",
		IntervalStr:       "1m",
		Mode:              model.ModeHostNetwork,
		Namespace:         "kncc",
		Targets:           []model.TargetService{{Protocol: model.ProtocolHTTP, Host: "forgejo.local", Port: 3000}},
		Enabled:           true,
		CreatedByUserID:   "system",
		CreatedByUsername: "system",
	}
	if err := s.UpsertSystemScheduledCheck(ctx, sysCheck); err != nil {
		t.Fatalf("UpsertSystemScheduledCheck failed: %v", err)
	}

	list, err := s.ListScheduledChecks(ctx)
	if err != nil {
		t.Fatalf("ListScheduledChecks failed: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 checks, got %d", len(list))
	}

	// 5. Delete dynamic check
	if err := s.DeleteScheduledCheck(ctx, "sched-1"); err != nil {
		t.Fatalf("DeleteScheduledCheck failed: %v", err)
	}
	_, err = s.GetScheduledCheck(ctx, "sched-1")
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestStore_UpdateScheduledCheckLastReport_PurgeOld(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	check := &model.ScheduledCheck{
		ID:                "sched-purge-test",
		Name:              "Purge Test",
		IntervalStr:       "1m",
		Mode:              model.ModeHostNetwork,
		Namespace:         "kncc",
		Targets:           []model.TargetService{{Protocol: model.ProtocolTCP, Host: "1.1.1.1", Port: 53}},
		Enabled:           true,
		CreatedByUserID:   "user-1",
		CreatedByUsername: "tester",
	}
	if err := s.CreateScheduledCheck(ctx, check); err != nil {
		t.Fatalf("CreateScheduledCheck failed: %v", err)
	}

	// Create first report
	rep1 := &model.Report{
		ID:        "rep-1",
		UserID:    "user-1",
		Username:  "tester",
		Mode:      model.ModeHostNetwork,
		Namespace: "kncc",
		Status:    model.StatusCompleted,
		CreatedAt: time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}
	if err := s.CreateReport(ctx, rep1); err != nil {
		t.Fatalf("CreateReport 1 failed: %v", err)
	}

	if err := s.UpdateScheduledCheckLastReport(ctx, check.ID, rep1.ID); err != nil {
		t.Fatalf("UpdateScheduledCheckLastReport rep1 failed: %v", err)
	}

	// Create second report
	rep2 := &model.Report{
		ID:        "rep-2",
		UserID:    "user-1",
		Username:  "tester",
		Mode:      model.ModeHostNetwork,
		Namespace: "kncc",
		Status:    model.StatusCompleted,
		CreatedAt: time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}
	if err := s.CreateReport(ctx, rep2); err != nil {
		t.Fatalf("CreateReport 2 failed: %v", err)
	}

	// Updating to rep2 should purge rep1!
	if err := s.UpdateScheduledCheckLastReport(ctx, check.ID, rep2.ID); err != nil {
		t.Fatalf("UpdateScheduledCheckLastReport rep2 failed: %v", err)
	}

	// rep1 should be gone from reports table
	_, err := s.GetReport(ctx, "rep-1")
	if err != ErrNotFound {
		t.Errorf("expected rep-1 to be deleted from store, got %v", err)
	}

	// rep2 should still exist
	gotRep2, err := s.GetReport(ctx, "rep-2")
	if err != nil {
		t.Errorf("expected rep-2 to exist, got %v", err)
	} else if gotRep2.ID != "rep-2" {
		t.Errorf("expected rep-2, got %s", gotRep2.ID)
	}
}

func TestStore_PurgeEphemeralScheduledChecks(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create 1 system check and 2 ephemeral checks
	sysCheck := &model.ScheduledCheck{
		ID:                "sys-check",
		Name:              "System Check",
		IntervalStr:       "1m",
		Mode:              model.ModeHostNetwork,
		Namespace:         "kncc",
		Enabled:           true,
		CreatedByUserID:   "system",
		CreatedByUsername: "system",
		IsSystem:          true,
		IsEphemeral:       false,
	}
	if err := s.CreateScheduledCheck(ctx, sysCheck); err != nil {
		t.Fatalf("Create system check failed: %v", err)
	}

	ephCheck1 := &model.ScheduledCheck{
		ID:                "eph-1",
		Name:              "Ephemeral 1",
		IntervalStr:       "30s",
		Mode:              model.ModeHostNetwork,
		Namespace:         "kncc",
		Enabled:           true,
		CreatedByUserID:   "user-1",
		CreatedByUsername: "alice",
		IsSystem:          false,
		IsEphemeral:       true,
	}
	if err := s.CreateScheduledCheck(ctx, ephCheck1); err != nil {
		t.Fatalf("Create ephCheck1 failed: %v", err)
	}

	// Purge ephemeral checks
	if err := s.PurgeEphemeralScheduledChecks(ctx); err != nil {
		t.Fatalf("PurgeEphemeralScheduledChecks failed: %v", err)
	}

	checks, err := s.ListScheduledChecks(ctx)
	if err != nil {
		t.Fatalf("ListScheduledChecks failed: %v", err)
	}
	if len(checks) != 1 || checks[0].ID != "sys-check" {
		t.Errorf("expected only sys-check to remain, got %+v", checks)
	}
}

func TestStore_MetricsHelpers(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	size, err := s.GetDatabaseSizeBytes()
	if err != nil {
		t.Fatalf("GetDatabaseSizeBytes failed: %v", err)
	}
	if size <= 0 {
		t.Errorf("expected database size > 0, got %d", size)
	}

	count, err := s.GetManualChecksCount(ctx)
	if err != nil {
		t.Fatalf("GetManualChecksCount failed: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 manual checks initially, got %d", count)
	}

	_ = s.RecordUserRequest(ctx, "req-1", "user-1", "alice", time.Now().UTC())
	count, err = s.GetManualChecksCount(ctx)
	if err != nil {
		t.Fatalf("GetManualChecksCount after record failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 manual check, got %d", count)
	}
}

func TestStore_PurgeOrphanedSystemScheduledChecks(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// 1. Create two system checks and an ephemeral check
	sysCheck1 := &model.ScheduledCheck{
		ID:                "sys-check-1",
		Name:              "System Check 1",
		IntervalStr:       "1m",
		Mode:              model.ModeHostNetwork,
		Namespace:         "kncc",
		Targets:           []model.TargetService{{Protocol: model.ProtocolHTTP, Host: "10.0.0.1", Port: 80}},
		Enabled:           true,
		CreatedByUserID:   "system",
		CreatedByUsername: "system",
		IsSystem:          true,
		IsEphemeral:       false,
	}
	sysCheck2 := &model.ScheduledCheck{
		ID:                "sys-check-2",
		Name:              "System Check 2",
		IntervalStr:       "1m",
		Mode:              model.ModeHostNetwork,
		Namespace:         "kncc",
		Targets:           []model.TargetService{{Protocol: model.ProtocolHTTP, Host: "10.0.0.2", Port: 80}},
		Enabled:           true,
		CreatedByUserID:   "system",
		CreatedByUsername: "system",
		IsSystem:          true,
		IsEphemeral:       false,
	}
	ephCheck := &model.ScheduledCheck{
		ID:                "eph-check",
		Name:              "User Ephemeral",
		IntervalStr:       "5m",
		Mode:              model.ModeHostNetwork,
		Namespace:         "kncc",
		Targets:           []model.TargetService{{Protocol: model.ProtocolHTTP, Host: "10.0.0.3", Port: 80}},
		Enabled:           true,
		CreatedByUserID:   "user-1",
		CreatedByUsername: "user",
		IsSystem:          false,
		IsEphemeral:       true,
	}

	if err := s.CreateScheduledCheck(ctx, sysCheck1); err != nil {
		t.Fatalf("Create sysCheck1 failed: %v", err)
	}
	if err := s.CreateScheduledCheck(ctx, sysCheck2); err != nil {
		t.Fatalf("Create sysCheck2 failed: %v", err)
	}
	if err := s.CreateScheduledCheck(ctx, ephCheck); err != nil {
		t.Fatalf("Create ephCheck failed: %v", err)
	}

	// 2. Attach a report to sysCheck2
	rep := &model.Report{
		ID:        "rep-sys-2",
		UserID:    "system",
		Username:  "system",
		Mode:      model.ModeHostNetwork,
		Namespace: "kncc",
		Status:    model.StatusCompleted,
		CreatedAt: time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}
	if err := s.CreateReport(ctx, rep); err != nil {
		t.Fatalf("CreateReport failed: %v", err)
	}
	if err := s.UpdateScheduledCheckLastReport(ctx, "sys-check-2", rep.ID); err != nil {
		t.Fatalf("UpdateScheduledCheckLastReport failed: %v", err)
	}

	// 3. Purge orphaned: activeIDs contains only sys-check-1
	if err := s.PurgeOrphanedSystemScheduledChecks(ctx, []string{"sys-check-1"}); err != nil {
		t.Fatalf("PurgeOrphanedSystemScheduledChecks failed: %v", err)
	}

	// sys-check-2 should be gone, its report should be gone, sys-check-1 and eph-check must remain
	if _, err := s.GetScheduledCheck(ctx, "sys-check-2"); err != ErrNotFound {
		t.Errorf("expected sys-check-2 to be deleted, got err: %v", err)
	}
	if _, err := s.GetReport(ctx, "rep-sys-2"); err != ErrNotFound {
		t.Errorf("expected rep-sys-2 report to be deleted, got err: %v", err)
	}
	if _, err := s.GetScheduledCheck(ctx, "sys-check-1"); err != nil {
		t.Errorf("expected sys-check-1 to remain, got err: %v", err)
	}
	if _, err := s.GetScheduledCheck(ctx, "eph-check"); err != nil {
		t.Errorf("expected eph-check to remain, got err: %v", err)
	}

	// 4. Purge with empty activeIDs: all system checks should be purged, ephemeral remains
	if err := s.PurgeOrphanedSystemScheduledChecks(ctx, []string{}); err != nil {
		t.Fatalf("PurgeOrphanedSystemScheduledChecks with empty activeIDs failed: %v", err)
	}
	if _, err := s.GetScheduledCheck(ctx, "sys-check-1"); err != ErrNotFound {
		t.Errorf("expected sys-check-1 to be deleted when activeIDs is empty, got err: %v", err)
	}
	if _, err := s.GetScheduledCheck(ctx, "eph-check"); err != nil {
		t.Errorf("expected eph-check to still remain, got err: %v", err)
	}
}

