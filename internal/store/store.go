package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"strings"
	"sync"
	"time"

	"kncc/internal/model"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

var (
	ErrNotFound      = errors.New("not found")
	ErrAlreadyExists = errors.New("already exists")
)

// Store defines the persistent storage operations.
type Store interface {
	// Reports
	CreateReport(ctx context.Context, report *model.Report) error
	GetReport(ctx context.Context, id string) (*model.Report, error)
	UpdateReportProgress(ctx context.Context, id string, totalNodes, completedNodes int, status model.ReportStatus, completedAt *time.Time) error
	SaveNodeResult(ctx context.Context, reportID string, result model.NodeResult) error
	ListReports(ctx context.Context, userID string, role model.Role, limit, offset int) ([]*model.Report, int, error)
	DeleteExpiredReports(ctx context.Context) (int64, error)

	// Users
	CreateUser(ctx context.Context, user *model.User) error
	GetUserByUsername(ctx context.Context, username string) (*model.User, error)
	GetUserByID(ctx context.Context, id string) (*model.User, error)
	ListUsers(ctx context.Context) ([]*model.User, error)
	UpdateUserRole(ctx context.Context, id string, role model.Role) error
	EnsureAdminUser(ctx context.Context, username, defaultPassword string) error

	// Stats
	RecordUserRequest(ctx context.Context, id, userID, username string, createdAt time.Time) error
	GetUserRequestStats(ctx context.Context) (*model.UserStatsResponse, error)

	// User Target History / Quick Add
	RecordUserTargets(ctx context.Context, userID string, targets []model.TargetService, usedAt time.Time) error
	GetUserTopTargets(ctx context.Context, userID string, limit int) ([]model.TargetService, error)

	// Settings
	GetSetting(ctx context.Context, key string) (string, error)
	SetSetting(ctx context.Context, key, value string) error

	// Scheduled Checks
	CreateScheduledCheck(ctx context.Context, check *model.ScheduledCheck) error
	GetScheduledCheck(ctx context.Context, id string) (*model.ScheduledCheck, error)
	ListScheduledChecks(ctx context.Context) ([]*model.ScheduledCheck, error)
	UpdateScheduledCheck(ctx context.Context, check *model.ScheduledCheck) error
	DeleteScheduledCheck(ctx context.Context, id string) error
	UpdateScheduledCheckLastReport(ctx context.Context, id, newReportID string) error
	PurgeEphemeralScheduledChecks(ctx context.Context) error
	UpsertSystemScheduledCheck(ctx context.Context, check *model.ScheduledCheck) error

	// Metrics & DB Helpers
	GetDatabaseSizeBytes() (int64, error)
	GetManualChecksCount(ctx context.Context) (int64, error)

	Ping(ctx context.Context) error
	Close() error
}

// SQLiteStore implements Store using modernc.org/sqlite (pure Go).
type SQLiteStore struct {
	db     *sql.DB
	dbPath string
	mu     sync.RWMutex
}

// NewSQLiteStore opens or creates an SQLite database at dbPath.
func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	// Enable WAL mode and busy timeout for concurrent read/write
	dsn := fmt.Sprintf("%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite at %s: %w", dbPath, err)
	}

	db.SetMaxOpenConns(1) // SQLite works best with 1 open writer in WAL mode
	db.SetMaxIdleConns(1)

	s := &SQLiteStore{db: db, dbPath: dbPath}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrating database: %w", err)
	}

	return s, nil
}

func (s *SQLiteStore) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS users (
		id TEXT PRIMARY KEY,
		username TEXT UNIQUE NOT NULL,
		password_hash TEXT,
		role TEXT NOT NULL,
		provider TEXT NOT NULL,
		email TEXT,
		created_at DATETIME NOT NULL
	);

	CREATE TABLE IF NOT EXISTS reports (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		username TEXT NOT NULL,
		mode TEXT NOT NULL,
		namespace TEXT NOT NULL,
		targets TEXT NOT NULL,
		status TEXT NOT NULL,
		total_nodes INTEGER NOT NULL DEFAULT 0,
		completed_nodes INTEGER NOT NULL DEFAULT 0,
		node_results TEXT NOT NULL DEFAULT '{}',
		created_at DATETIME NOT NULL,
		completed_at DATETIME,
		expires_at DATETIME NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_reports_user_id ON reports(user_id);
	CREATE INDEX IF NOT EXISTS idx_reports_created_at ON reports(created_at DESC);
	CREATE INDEX IF NOT EXISTS idx_reports_expires_at ON reports(expires_at);

	CREATE TABLE IF NOT EXISTS user_request_logs (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		username TEXT NOT NULL,
		created_at DATETIME NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_req_logs_created_at ON user_request_logs(created_at);
	CREATE INDEX IF NOT EXISTS idx_req_logs_user_id ON user_request_logs(user_id);

	INSERT OR IGNORE INTO user_request_logs (id, user_id, username, created_at)
	SELECT id, user_id, username, created_at FROM reports;

	CREATE TABLE IF NOT EXISTS user_target_history (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id TEXT NOT NULL,
		protocol TEXT NOT NULL,
		host TEXT NOT NULL,
		port INTEGER NOT NULL DEFAULT 0,
		path TEXT NOT NULL DEFAULT '',
		insecure_tls INTEGER NOT NULL DEFAULT 0,
		target_key TEXT NOT NULL,
		count INTEGER NOT NULL DEFAULT 1,
		first_used_at DATETIME NOT NULL,
		last_used_at DATETIME NOT NULL,
		UNIQUE(user_id, target_key)
	);

	CREATE INDEX IF NOT EXISTS idx_user_targets_user ON user_target_history(user_id, count DESC, last_used_at DESC);

	CREATE TABLE IF NOT EXISTS app_settings (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL,
		updated_at DATETIME NOT NULL
	);

	CREATE TABLE IF NOT EXISTS scheduled_checks (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		interval_str TEXT NOT NULL,
		mode TEXT NOT NULL,
		namespace TEXT NOT NULL,
		targets_json TEXT NOT NULL,
		enabled BOOLEAN NOT NULL DEFAULT 1,
		created_by_user_id TEXT NOT NULL,
		created_by_username TEXT NOT NULL,
		is_system BOOLEAN NOT NULL DEFAULT 0,
		is_ephemeral BOOLEAN NOT NULL DEFAULT 0,
		last_report_id TEXT,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		FOREIGN KEY (last_report_id) REFERENCES reports(id) ON DELETE SET NULL
	);

	CREATE INDEX IF NOT EXISTS idx_sched_checks_enabled ON scheduled_checks(enabled);
	`
	if _, err := s.db.Exec(schema); err != nil {
		return err
	}

	var historyCount int
	_ = s.db.QueryRow("SELECT COUNT(*) FROM user_target_history").Scan(&historyCount)
	if historyCount == 0 {
		s.backfillUserTargetHistory()
	}

	return nil
}

// Ping checks the connectivity and responsiveness of the SQLite database.
func (s *SQLiteStore) Ping(ctx context.Context) error {
	if s.db == nil {
		return errors.New("database connection is not initialized")
	}
	return s.db.PingContext(ctx)
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// CreateReport inserts a new report.
func (s *SQLiteStore) CreateReport(ctx context.Context, r *model.Report) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if r.ID == "" {
		r.ID = uuid.NewString()
	}
	if r.NodeResults == nil {
		r.NodeResults = make(map[string]model.NodeResult)
	}

	query := `
	INSERT INTO reports (
		id, user_id, username, mode, namespace, targets,
		status, total_nodes, completed_nodes, node_results,
		created_at, completed_at, expires_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := s.db.ExecContext(ctx, query,
		r.ID,
		r.UserID,
		r.Username,
		string(r.Mode),
		r.Namespace,
		r.TargetsJSON(),
		string(r.Status),
		r.TotalNodes,
		r.CompletedNodes,
		r.NodeResultsJSON(),
		r.CreatedAt.UTC(),
		r.CompletedAt,
		r.ExpiresAt.UTC(),
	)
	if err != nil {
		return err
	}

	// Also record in persistent user_request_logs
	_, _ = s.db.ExecContext(ctx, `
	INSERT OR IGNORE INTO user_request_logs (id, user_id, username, created_at)
	VALUES (?, ?, ?, ?)
	`, r.ID, r.UserID, r.Username, r.CreatedAt.UTC())

	return nil
}

// GetReport fetches a report by ID.
func (s *SQLiteStore) GetReport(ctx context.Context, id string) (*model.Report, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
	SELECT id, user_id, username, mode, namespace, targets,
	       status, total_nodes, completed_nodes, node_results,
	       created_at, completed_at, expires_at
	FROM reports WHERE id = ?
	`
	row := s.db.QueryRowContext(ctx, query, id)

	var (
		r              model.Report
		targetsJSON    string
		nodeResultsJSON string
		statusStr      string
		modeStr        string
	)

	err := row.Scan(
		&r.ID,
		&r.UserID,
		&r.Username,
		&modeStr,
		&r.Namespace,
		&targetsJSON,
		&statusStr,
		&r.TotalNodes,
		&r.CompletedNodes,
		&nodeResultsJSON,
		&r.CreatedAt,
		&r.CompletedAt,
		&r.ExpiresAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	r.Mode = model.CheckMode(modeStr)
	r.Status = model.ReportStatus(statusStr)

	if err := json.Unmarshal([]byte(targetsJSON), &r.Targets); err != nil {
		r.Targets = []model.TargetService{}
	}
	r.NodeResults = make(map[string]model.NodeResult)
	if err := json.Unmarshal([]byte(nodeResultsJSON), &r.NodeResults); err != nil {
		r.NodeResults = make(map[string]model.NodeResult)
	}

	return &r, nil
}

// UpdateReportProgress updates total node count, completed node count and overall status.
func (s *SQLiteStore) UpdateReportProgress(ctx context.Context, id string, totalNodes, completedNodes int, status model.ReportStatus, completedAt *time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `
	UPDATE reports
	SET total_nodes = CASE WHEN ? > 0 THEN ? ELSE total_nodes END,
	    completed_nodes = ?,
	    status = ?,
	    completed_at = ?
	WHERE id = ?
	`
	var compTime any
	if completedAt != nil {
		compTime = completedAt.UTC()
	}

	res, err := s.db.ExecContext(ctx, query, totalNodes, totalNodes, completedNodes, string(status), compTime, id)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// SaveNodeResult saves result for an individual node inside a report.
func (s *SQLiteStore) SaveNodeResult(ctx context.Context, reportID string, result model.NodeResult) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 1. Fetch current node_results
	var nodeResultsJSON string
	err := s.db.QueryRowContext(ctx, "SELECT node_results FROM reports WHERE id = ?", reportID).Scan(&nodeResultsJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}

	nodeResults := make(map[string]model.NodeResult)
	_ = json.Unmarshal([]byte(nodeResultsJSON), &nodeResults)

	nodeResults[result.NodeName] = result
	newJSON, err := json.Marshal(nodeResults)
	if err != nil {
		return err
	}

	_, err = s.db.ExecContext(ctx, "UPDATE reports SET node_results = ? WHERE id = ?", string(newJSON), reportID)
	return err
}

// ListReports returns list of reports with pagination. Users see only their own reports, Admins see all.
func (s *SQLiteStore) ListReports(ctx context.Context, userID string, role model.Role, limit, offset int) ([]*model.Report, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	var countQuery string
	var listQuery string
	var args []any

	if role == model.RoleAdmin {
		countQuery = "SELECT COUNT(*) FROM reports"
		listQuery = `
		SELECT id, user_id, username, mode, namespace, targets,
		       status, total_nodes, completed_nodes, node_results,
		       created_at, completed_at, expires_at
		FROM reports
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
		`
		args = []any{limit, offset}
	} else {
		countQuery = "SELECT COUNT(*) FROM reports WHERE user_id = ?"
		listQuery = `
		SELECT id, user_id, username, mode, namespace, targets,
		       status, total_nodes, completed_nodes, node_results,
		       created_at, completed_at, expires_at
		FROM reports
		WHERE user_id = ?
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
		`
		args = []any{userID, limit, offset}
	}

	var total int
	if role == model.RoleAdmin {
		if err := s.db.QueryRowContext(ctx, countQuery).Scan(&total); err != nil {
			return nil, 0, err
		}
	} else {
		if err := s.db.QueryRowContext(ctx, countQuery, userID).Scan(&total); err != nil {
			return nil, 0, err
		}
	}

	rows, err := s.db.QueryContext(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var reports []*model.Report
	for rows.Next() {
		var (
			r              model.Report
			targetsJSON    string
			nodeResultsJSON string
			statusStr      string
			modeStr        string
		)
		if err := rows.Scan(
			&r.ID,
			&r.UserID,
			&r.Username,
			&modeStr,
			&r.Namespace,
			&targetsJSON,
			&statusStr,
			&r.TotalNodes,
			&r.CompletedNodes,
			&nodeResultsJSON,
			&r.CreatedAt,
			&r.CompletedAt,
			&r.ExpiresAt,
		); err != nil {
			return nil, 0, err
		}

		r.Mode = model.CheckMode(modeStr)
		r.Status = model.ReportStatus(statusStr)
		_ = json.Unmarshal([]byte(targetsJSON), &r.Targets)
		r.NodeResults = make(map[string]model.NodeResult)
		_ = json.Unmarshal([]byte(nodeResultsJSON), &r.NodeResults)

		reports = append(reports, &r)
	}

	return reports, total, rows.Err()
}

// DeleteExpiredReports removes reports whose expires_at is before current time.
func (s *SQLiteStore) DeleteExpiredReports(ctx context.Context) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	res, err := s.db.ExecContext(ctx, "DELETE FROM reports WHERE expires_at < ?", time.Now().UTC())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// CreateUser inserts a new user.
func (s *SQLiteStore) CreateUser(ctx context.Context, u *model.User) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if u.ID == "" {
		u.ID = uuid.NewString()
	}
	if u.CreatedAt.IsZero() {
		u.CreatedAt = time.Now().UTC()
	}

	query := `
	INSERT INTO users (id, username, password_hash, role, provider, email, created_at)
	VALUES (?, ?, ?, ?, ?, ?, ?)
	`
	_, err := s.db.ExecContext(ctx, query,
		u.ID,
		u.Username,
		u.PasswordHash,
		string(u.Role),
		u.Provider,
		u.Email,
		u.CreatedAt.UTC(),
	)
	return err
}

// GetUserByUsername finds user by username.
func (s *SQLiteStore) GetUserByUsername(ctx context.Context, username string) (*model.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
	SELECT id, username, password_hash, role, provider, email, created_at
	FROM users WHERE username = ?
	`
	var (
		u       model.User
		roleStr string
	)
	err := s.db.QueryRowContext(ctx, query, username).Scan(
		&u.ID,
		&u.Username,
		&u.PasswordHash,
		&roleStr,
		&u.Provider,
		&u.Email,
		&u.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	u.Role = model.Role(roleStr)
	return &u, nil
}

// GetUserByID finds user by ID.
func (s *SQLiteStore) GetUserByID(ctx context.Context, id string) (*model.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
	SELECT id, username, password_hash, role, provider, email, created_at
	FROM users WHERE id = ?
	`
	var (
		u       model.User
		roleStr string
	)
	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&u.ID,
		&u.Username,
		&u.PasswordHash,
		&roleStr,
		&u.Provider,
		&u.Email,
		&u.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	u.Role = model.Role(roleStr)
	return &u, nil
}

// ListUsers returns all users in the system.
func (s *SQLiteStore) ListUsers(ctx context.Context) ([]*model.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
	SELECT id, username, password_hash, role, provider, email, created_at
	FROM users ORDER BY created_at ASC
	`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*model.User
	for rows.Next() {
		var (
			u       model.User
			roleStr string
		)
		if err := rows.Scan(
			&u.ID,
			&u.Username,
			&u.PasswordHash,
			&roleStr,
			&u.Provider,
			&u.Email,
			&u.CreatedAt,
		); err != nil {
			return nil, err
		}
		u.Role = model.Role(roleStr)
		users = append(users, &u)
	}
	return users, rows.Err()
}

// UpdateUserRole updates the role of an existing user.
func (s *SQLiteStore) UpdateUserRole(ctx context.Context, id string, role model.Role) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `UPDATE users SET role = ? WHERE id = ?`
	res, err := s.db.ExecContext(ctx, query, string(role), id)
	if err != nil {
		return fmt.Errorf("updating user role: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// EnsureAdminUser checks if admin exists; if not, creates default admin.
// If defaultPassword is empty, a cryptographically secure random password is generated.
func (s *SQLiteStore) EnsureAdminUser(ctx context.Context, username, defaultPassword string) error {
	_, err := s.GetUserByUsername(ctx, username)
	if err == nil {
		return nil // already exists
	}
	if !errors.Is(err, ErrNotFound) {
		return err
	}

	if defaultPassword == "" {
		defaultPassword = generateFallbackPassword(16)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(defaultPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hashing password: %w", err)
	}

	admin := &model.User{
		ID:           uuid.NewString(),
		Username:     username,
		PasswordHash: string(hash),
		Role:         model.RoleAdmin,
		Provider:     "local",
		Email:        "admin@kncc.local",
		CreatedAt:    time.Now().UTC(),
	}
	return s.CreateUser(ctx, admin)
}

func generateFallbackPassword(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!#%&*+-=?@"
	b := make([]byte, length)
	charsetLen := big.NewInt(int64(len(charset)))
	for i := range b {
		n, err := rand.Int(rand.Reader, charsetLen)
		if err != nil {
			b[i] = charset[i%len(charset)]
		} else {
			b[i] = charset[n.Int64()]
		}
	}
	return string(b)
}

// GetSetting retrieves a setting value by key.
func (s *SQLiteStore) GetSetting(ctx context.Context, key string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var val string
	err := s.db.QueryRowContext(ctx, "SELECT value FROM app_settings WHERE key = ?", key).Scan(&val)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", err
	}
	return val, nil
}

// SetSetting saves or updates a setting value by key.
func (s *SQLiteStore) SetSetting(ctx context.Context, key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `
	INSERT INTO app_settings (key, value, updated_at)
	VALUES (?, ?, ?)
	ON CONFLICT(key) DO UPDATE SET
		value = excluded.value,
		updated_at = excluded.updated_at
	`
	_, err := s.db.ExecContext(ctx, query, key, value, time.Now().UTC())
	return err
}

// RecordUserRequest logs a user network check request.
func (s *SQLiteStore) RecordUserRequest(ctx context.Context, id, userID, username string, createdAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if id == "" {
		id = uuid.NewString()
	}
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}

	query := `
	INSERT OR IGNORE INTO user_request_logs (id, user_id, username, created_at)
	VALUES (?, ?, ?, ?)
	`
	_, err := s.db.ExecContext(ctx, query, id, userID, username, createdAt.UTC())
	return err
}

func parseSQLiteTime(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	formats := []string{
		"2006-01-02 15:04:05.999999999 -0700 MST",
		"2006-01-02 15:04:05.999999999 -0700 -0700",
		"2006-01-02 15:04:05 -0700 MST",
		"2006-01-02 15:04:05 -0700 -0700",
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05-07:00",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot parse sqlite time %q", s)
}

// GetUserRequestStats aggregates request statistics across users and time periods.
func (s *SQLiteStore) GetUserRequestStats(ctx context.Context) (*model.UserStatsResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	now := time.Now().UTC()
	dayAgo := now.Add(-24 * time.Hour)
	weekAgo := now.Add(-7 * 24 * time.Hour)
	monthAgo := now.Add(-30 * 24 * time.Hour)
	yearAgo := now.Add(-365 * 24 * time.Hour)

	query := `
	SELECT 
		u.id,
		u.username,
		u.role,
		u.provider,
		COALESCE(COUNT(CASE WHEN r.created_at >= ? THEN 1 END), 0) AS day_cnt,
		COALESCE(COUNT(CASE WHEN r.created_at >= ? THEN 1 END), 0) AS week_cnt,
		COALESCE(COUNT(CASE WHEN r.created_at >= ? THEN 1 END), 0) AS month_cnt,
		COALESCE(COUNT(CASE WHEN r.created_at >= ? THEN 1 END), 0) AS year_cnt,
		COALESCE(COUNT(r.id), 0) AS total_cnt,
		MAX(r.created_at) AS last_active
	FROM users u
	LEFT JOIN user_request_logs r ON u.id = r.user_id
	GROUP BY u.id, u.username, u.role, u.provider
	ORDER BY total_cnt DESC, u.username ASC
	`

	rows, err := s.db.QueryContext(ctx, query, dayAgo, weekAgo, monthAgo, yearAgo)
	if err != nil {
		return nil, fmt.Errorf("querying user request stats: %w", err)
	}
	defer rows.Close()

	var usersStats []model.UserStats
	var summary model.UserStatsSummary

	for rows.Next() {
		var (
			stat          model.UserStats
			roleStr       string
			lastActiveStr sql.NullString
		)

		if err := rows.Scan(
			&stat.UserID,
			&stat.Username,
			&roleStr,
			&stat.Provider,
			&stat.DayCount,
			&stat.WeekCount,
			&stat.MonthCount,
			&stat.YearCount,
			&stat.TotalCount,
			&lastActiveStr,
		); err != nil {
			return nil, fmt.Errorf("scanning user stats row: %w", err)
		}

		stat.Role = model.Role(roleStr)
		if lastActiveStr.Valid && lastActiveStr.String != "" {
			if t, err := parseSQLiteTime(lastActiveStr.String); err == nil {
				stat.LastActive = &t
			}
		}

		summary.DayCount += stat.DayCount
		summary.WeekCount += stat.WeekCount
		summary.MonthCount += stat.MonthCount
		summary.YearCount += stat.YearCount
		summary.TotalCount += stat.TotalCount

		usersStats = append(usersStats, stat)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating user stats rows: %w", err)
	}

	if usersStats == nil {
		usersStats = []model.UserStats{}
	}

	return &model.UserStatsResponse{
		Users: usersStats,
		Total: summary,
	}, nil
}

func (s *SQLiteStore) backfillUserTargetHistory() {
	rows, err := s.db.Query("SELECT user_id, targets, created_at FROM reports ORDER BY created_at ASC")
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var (
			userID      string
			targetsJSON string
			createdAt   time.Time
		)
		if err := rows.Scan(&userID, &targetsJSON, &createdAt); err != nil {
			continue
		}
		var targets []model.TargetService
		if err := json.Unmarshal([]byte(targetsJSON), &targets); err != nil {
			continue
		}
		for _, target := range targets {
			_ = s.recordSingleTarget(context.Background(), userID, target, createdAt)
		}
	}
}

func (s *SQLiteStore) recordSingleTarget(ctx context.Context, userID string, target model.TargetService, usedAt time.Time) error {
	proto := strings.ToLower(strings.TrimSpace(string(target.Protocol)))
	host := strings.ToLower(strings.TrimSpace(target.Host))
	if proto == "" || host == "" {
		return nil
	}
	port := target.Port
	if proto == "icmp" {
		port = 0
	}
	path := strings.TrimSpace(target.Path)
	insecureTLS := 0
	if target.InsecureTLS {
		insecureTLS = 1
	}

	targetKey := fmt.Sprintf("%s|%s|%d|%s", proto, host, port, path)

	query := `
	INSERT INTO user_target_history (
		user_id, protocol, host, port, path, insecure_tls, target_key, count, first_used_at, last_used_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, 1, ?, ?)
	ON CONFLICT(user_id, target_key) DO UPDATE SET
		count = count + 1,
		last_used_at = excluded.last_used_at;
	`
	_, err := s.db.ExecContext(ctx, query,
		userID, proto, host, port, path, insecureTLS, targetKey, usedAt, usedAt,
	)
	return err
}

// RecordUserTargets records or increments frequency counters for targets used by a user.
func (s *SQLiteStore) RecordUserTargets(ctx context.Context, userID string, targets []model.TargetService, usedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, target := range targets {
		if err := s.recordSingleTarget(ctx, userID, target, usedAt); err != nil {
			return err
		}
	}
	return nil
}

// GetUserTopTargets returns the top most frequently used targets for a user.
func (s *SQLiteStore) GetUserTopTargets(ctx context.Context, userID string, limit int) ([]model.TargetService, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 {
		limit = 3
	}

	query := `
	SELECT protocol, host, port, path, insecure_tls
	FROM user_target_history
	WHERE user_id = ?
	ORDER BY count DESC, last_used_at DESC, id DESC
	LIMIT ?
	`
	rows, err := s.db.QueryContext(ctx, query, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("querying user top targets: %w", err)
	}
	defer rows.Close()

	var results []model.TargetService
	for rows.Next() {
		var (
			proto       string
			host        string
			port        int
			path        string
			insecureTLS int
		)
		if err := rows.Scan(&proto, &host, &port, &path, &insecureTLS); err != nil {
			return nil, fmt.Errorf("scanning target row: %w", err)
		}
		t := model.TargetService{
			Protocol:       model.Protocol(proto),
			Host:           host,
			Port:           port,
			Path:           path,
			InsecureTLS:    insecureTLS == 1,
			TimeoutSeconds: 5,
		}
		results = append(results, t)
	}

	return results, rows.Err()
}

// CreateScheduledCheck inserts a new scheduled check.
func (s *SQLiteStore) CreateScheduledCheck(ctx context.Context, check *model.ScheduledCheck) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if check.ID == "" {
		check.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	if check.CreatedAt.IsZero() {
		check.CreatedAt = now
	}
	if check.UpdatedAt.IsZero() {
		check.UpdatedAt = now
	}

	query := `
	INSERT INTO scheduled_checks (
		id, name, interval_str, mode, namespace, targets_json,
		enabled, created_by_user_id, created_by_username, is_system,
		is_ephemeral, last_report_id, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := s.db.ExecContext(ctx, query,
		check.ID,
		check.Name,
		check.IntervalStr,
		string(check.Mode),
		check.Namespace,
		check.TargetsJSON(),
		check.Enabled,
		check.CreatedByUserID,
		check.CreatedByUsername,
		check.IsSystem,
		check.IsEphemeral,
		check.LastReportID,
		check.CreatedAt.UTC(),
		check.UpdatedAt.UTC(),
	)
	return err
}

// GetScheduledCheck returns a scheduled check by ID with last report info if available.
func (s *SQLiteStore) GetScheduledCheck(ctx context.Context, id string) (*model.ScheduledCheck, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
	SELECT s.id, s.name, s.interval_str, s.mode, s.namespace, s.targets_json,
	       s.enabled, s.created_by_user_id, s.created_by_username, s.is_system,
	       s.is_ephemeral, s.last_report_id, s.created_at, s.updated_at,
	       r.status, r.completed_at
	FROM scheduled_checks s
	LEFT JOIN reports r ON s.last_report_id = r.id
	WHERE s.id = ?
	`
	row := s.db.QueryRowContext(ctx, query, id)

	var (
		c                model.ScheduledCheck
		modeStr          string
		targetsJSON      string
		lastReportID     sql.NullString
		lastReportStatus sql.NullString
		lastCompletedAt  sql.NullTime
	)

	err := row.Scan(
		&c.ID,
		&c.Name,
		&c.IntervalStr,
		&modeStr,
		&c.Namespace,
		&targetsJSON,
		&c.Enabled,
		&c.CreatedByUserID,
		&c.CreatedByUsername,
		&c.IsSystem,
		&c.IsEphemeral,
		&lastReportID,
		&c.CreatedAt,
		&c.UpdatedAt,
		&lastReportStatus,
		&lastCompletedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	c.Mode = model.CheckMode(modeStr)
	if lastReportID.Valid && lastReportID.String != "" {
		c.LastReportID = &lastReportID.String
	}
	if lastReportStatus.Valid && lastReportStatus.String != "" {
		st := model.ReportStatus(lastReportStatus.String)
		c.LastReportStatus = &st
	}
	if lastCompletedAt.Valid {
		c.LastRunAt = &lastCompletedAt.Time
	}
	if err := json.Unmarshal([]byte(targetsJSON), &c.Targets); err != nil {
		c.Targets = []model.TargetService{}
	}

	return &c, nil
}

// ListScheduledChecks returns all scheduled checks with last report info.
func (s *SQLiteStore) ListScheduledChecks(ctx context.Context) ([]*model.ScheduledCheck, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
	SELECT s.id, s.name, s.interval_str, s.mode, s.namespace, s.targets_json,
	       s.enabled, s.created_by_user_id, s.created_by_username, s.is_system,
	       s.is_ephemeral, s.last_report_id, s.created_at, s.updated_at,
	       r.status, r.completed_at
	FROM scheduled_checks s
	LEFT JOIN reports r ON s.last_report_id = r.id
	ORDER BY s.is_system DESC, s.name ASC
	`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var checks []*model.ScheduledCheck
	for rows.Next() {
		var (
			c                model.ScheduledCheck
			modeStr          string
			targetsJSON      string
			lastReportID     sql.NullString
			lastReportStatus sql.NullString
			lastCompletedAt  sql.NullTime
		)

		if err := rows.Scan(
			&c.ID,
			&c.Name,
			&c.IntervalStr,
			&modeStr,
			&c.Namespace,
			&targetsJSON,
			&c.Enabled,
			&c.CreatedByUserID,
			&c.CreatedByUsername,
			&c.IsSystem,
			&c.IsEphemeral,
			&lastReportID,
			&c.CreatedAt,
			&c.UpdatedAt,
			&lastReportStatus,
			&lastCompletedAt,
		); err != nil {
			return nil, err
		}

		c.Mode = model.CheckMode(modeStr)
		if lastReportID.Valid && lastReportID.String != "" {
			c.LastReportID = &lastReportID.String
		}
		if lastReportStatus.Valid && lastReportStatus.String != "" {
			st := model.ReportStatus(lastReportStatus.String)
			c.LastReportStatus = &st
		}
		if lastCompletedAt.Valid {
			c.LastRunAt = &lastCompletedAt.Time
		}
		if err := json.Unmarshal([]byte(targetsJSON), &c.Targets); err != nil {
			c.Targets = []model.TargetService{}
		}

		checks = append(checks, &c)
	}
	return checks, rows.Err()
}

// UpdateScheduledCheck updates a scheduled check.
func (s *SQLiteStore) UpdateScheduledCheck(ctx context.Context, check *model.ScheduledCheck) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	check.UpdatedAt = time.Now().UTC()
	query := `
	UPDATE scheduled_checks
	SET name = ?, interval_str = ?, mode = ?, namespace = ?,
	    targets_json = ?, enabled = ?, updated_at = ?
	WHERE id = ?
	`
	res, err := s.db.ExecContext(ctx, query,
		check.Name,
		check.IntervalStr,
		string(check.Mode),
		check.Namespace,
		check.TargetsJSON(),
		check.Enabled,
		check.UpdatedAt.UTC(),
		check.ID,
	)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteScheduledCheck deletes a scheduled check and cleans up its last report.
func (s *SQLiteStore) DeleteScheduledCheck(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var lastReportID sql.NullString
	_ = s.db.QueryRowContext(ctx, "SELECT last_report_id FROM scheduled_checks WHERE id = ?", id).Scan(&lastReportID)

	res, err := s.db.ExecContext(ctx, "DELETE FROM scheduled_checks WHERE id = ?", id)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}

	if lastReportID.Valid && lastReportID.String != "" {
		_, _ = s.db.ExecContext(ctx, "DELETE FROM reports WHERE id = ?", lastReportID.String)
	}

	return nil
}

// UpdateScheduledCheckLastReport updates last report ID and purges the previous report for this check.
func (s *SQLiteStore) UpdateScheduledCheckLastReport(ctx context.Context, id, newReportID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var oldReportID sql.NullString
	err := s.db.QueryRowContext(ctx, "SELECT last_report_id FROM scheduled_checks WHERE id = ?", id).Scan(&oldReportID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	query := `UPDATE scheduled_checks SET last_report_id = ?, updated_at = ? WHERE id = ?`
	if _, err := s.db.ExecContext(ctx, query, newReportID, time.Now().UTC(), id); err != nil {
		return err
	}

	if oldReportID.Valid && oldReportID.String != "" && oldReportID.String != newReportID {
		_, _ = s.db.ExecContext(ctx, "DELETE FROM reports WHERE id = ?", oldReportID.String)
	}

	return nil
}

// PurgeEphemeralScheduledChecks removes all ephemeral checks created via UI and their reports.
func (s *SQLiteStore) PurgeEphemeralScheduledChecks(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Delete reports belonging to ephemeral checks
	rows, err := s.db.QueryContext(ctx, "SELECT last_report_id FROM scheduled_checks WHERE is_ephemeral = 1 AND last_report_id IS NOT NULL")
	if err == nil {
		defer rows.Close()
		var reportIDs []string
		for rows.Next() {
			var rID string
			if err := rows.Scan(&rID); err == nil && rID != "" {
				reportIDs = append(reportIDs, rID)
			}
		}
		for _, rID := range reportIDs {
			_, _ = s.db.ExecContext(ctx, "DELETE FROM reports WHERE id = ?", rID)
		}
	}

	_, err = s.db.ExecContext(ctx, "DELETE FROM scheduled_checks WHERE is_ephemeral = 1")
	return err
}

// UpsertSystemScheduledCheck inserts or updates a declarative system check.
func (s *SQLiteStore) UpsertSystemScheduledCheck(ctx context.Context, check *model.ScheduledCheck) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	if check.CreatedAt.IsZero() {
		check.CreatedAt = now
	}
	check.UpdatedAt = now
	check.IsSystem = true
	check.IsEphemeral = false

	query := `
	INSERT INTO scheduled_checks (
		id, name, interval_str, mode, namespace, targets_json,
		enabled, created_by_user_id, created_by_username, is_system,
		is_ephemeral, last_report_id, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1, 0, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		name = excluded.name,
		interval_str = excluded.interval_str,
		mode = excluded.mode,
		namespace = excluded.namespace,
		targets_json = excluded.targets_json,
		is_system = 1,
		is_ephemeral = 0,
		updated_at = excluded.updated_at
	`
	_, err := s.db.ExecContext(ctx, query,
		check.ID,
		check.Name,
		check.IntervalStr,
		string(check.Mode),
		check.Namespace,
		check.TargetsJSON(),
		check.Enabled,
		check.CreatedByUserID,
		check.CreatedByUsername,
		check.LastReportID,
		check.CreatedAt.UTC(),
		check.UpdatedAt.UTC(),
	)
	return err
}

// GetDatabaseSizeBytes returns the size in bytes of the SQLite database file on disk.
func (s *SQLiteStore) GetDatabaseSizeBytes() (int64, error) {
	if s.dbPath == "" {
		return 0, nil
	}
	info, err := os.Stat(s.dbPath)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	return info.Size(), nil
}

// GetManualChecksCount returns the total number of manual user check requests recorded.
func (s *SQLiteStore) GetManualChecksCount(ctx context.Context) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var count int64
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM user_request_logs").Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}



