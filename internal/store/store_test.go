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
