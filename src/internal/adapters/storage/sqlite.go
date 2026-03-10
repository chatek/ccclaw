package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/41490/ccclaw/internal/core"
	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

type TokenUsageRecord struct {
	TaskID     string
	SessionID  string
	Usage      core.TokenUsage
	CostUSD    float64
	DurationMS int64
	RTKEnabled bool
	RecordedAt time.Time
}

type TokenStatsSummary struct {
	Runs         int
	Sessions     int
	InputTokens  int
	OutputTokens int
	CacheCreate  int
	CacheRead    int
	CostUSD      float64
	FirstUsedAt  time.Time
	LastUsedAt   time.Time
}

type TaskTokenStat struct {
	TaskID         string
	IssueNumber    int
	IssueTitle     string
	LastSessionID  string
	Runs           int
	InputTokens    int
	OutputTokens   int
	CacheCreate    int
	CacheRead      int
	CostUSD        float64
	LastRecordedAt time.Time
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("创建 SQLite 目录失败: %w", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("打开 SQLite 失败: %w", err)
	}
	store := &Store{db: db}
	if err := store.init(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Ping() error {
	return s.db.Ping()
}

func (s *Store) GetByIdempotency(idempotencyKey string) (*core.Task, error) {
	row := s.db.QueryRow(`
		SELECT task_id, idempotency_key, control_repo, target_repo, last_session_id, issue_number, issue_title, issue_body,
		issue_author, issue_author_permission, labels, intent, risk_level, approved, approval_command,
		approval_actor, approval_comment_id, state, retry_count, error_msg, report_path, created_at, updated_at
		FROM tasks WHERE idempotency_key = ?
	`, idempotencyKey)
	return scanTask(row)
}

func (s *Store) UpsertTask(task *core.Task) error {
	labels, err := json.Marshal(task.Labels)
	if err != nil {
		return fmt.Errorf("序列化 labels 失败: %w", err)
	}
	now := time.Now()
	if task.CreatedAt.IsZero() {
		task.CreatedAt = now
	}
	task.UpdatedAt = now
	_, err = s.db.Exec(`
		INSERT INTO tasks (
			task_id, idempotency_key, control_repo, target_repo, last_session_id, issue_number, issue_title, issue_body,
			issue_author, issue_author_permission, labels, intent, risk_level, approved, approval_command,
			approval_actor, approval_comment_id, state, retry_count, error_msg, report_path, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(task_id) DO UPDATE SET
			target_repo = excluded.target_repo,
			last_session_id = excluded.last_session_id,
			issue_title = excluded.issue_title,
			issue_body = excluded.issue_body,
			issue_author = excluded.issue_author,
			issue_author_permission = excluded.issue_author_permission,
			labels = excluded.labels,
			intent = excluded.intent,
			risk_level = excluded.risk_level,
			approved = excluded.approved,
			approval_command = excluded.approval_command,
			approval_actor = excluded.approval_actor,
			approval_comment_id = excluded.approval_comment_id,
			state = excluded.state,
			retry_count = excluded.retry_count,
			error_msg = excluded.error_msg,
			report_path = excluded.report_path,
			updated_at = excluded.updated_at
	`,
		task.TaskID, task.IdempotencyKey, task.ControlRepo, task.TargetRepo, task.LastSessionID, task.IssueNumber, task.IssueTitle, task.IssueBody,
		task.IssueAuthor, task.IssueAuthorPermission, string(labels), string(task.Intent), string(task.RiskLevel), boolToInt(task.Approved), task.ApprovalCommand,
		task.ApprovalActor, task.ApprovalCommentID, string(task.State), task.RetryCount, task.ErrorMsg, task.ReportPath, task.CreatedAt, task.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("写入任务失败: %w", err)
	}
	return nil
}

func (s *Store) AppendEvent(taskID string, eventType core.EventType, detail string) error {
	_, err := s.db.Exec(`INSERT INTO task_events (task_id, event_type, detail) VALUES (?, ?, ?)`, taskID, string(eventType), detail)
	if err != nil {
		return fmt.Errorf("写入任务事件失败: %w", err)
	}
	return nil
}

func (s *Store) RecordTokenUsage(record TokenUsageRecord) error {
	recordedAt := record.RecordedAt
	if recordedAt.IsZero() {
		recordedAt = time.Now()
	}
	_, err := s.db.Exec(`
		INSERT INTO token_usage (
			task_id, session_id, input_tokens, output_tokens, cache_create, cache_read,
			cost_usd, duration_ms, rtk_enabled, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		record.TaskID,
		record.SessionID,
		record.Usage.InputTokens,
		record.Usage.OutputTokens,
		record.Usage.CacheCreationInputTokens,
		record.Usage.CacheReadInputTokens,
		record.CostUSD,
		record.DurationMS,
		boolToInt(record.RTKEnabled),
		recordedAt,
	)
	if err != nil {
		return fmt.Errorf("写入 token 使用记录失败: %w", err)
	}
	return nil
}

func (s *Store) TokenStats() (*TokenStatsSummary, error) {
	row := s.db.QueryRow(`
		SELECT
			COUNT(*) AS runs,
			COUNT(DISTINCT NULLIF(session_id, '')) AS sessions,
			COALESCE(SUM(input_tokens), 0),
			COALESCE(SUM(output_tokens), 0),
			COALESCE(SUM(cache_create), 0),
			COALESCE(SUM(cache_read), 0),
			COALESCE(SUM(cost_usd), 0),
			COALESCE(MIN(created_at), ''),
			COALESCE(MAX(created_at), '')
		FROM token_usage
	`)
	var (
		summary   TokenStatsSummary
		firstUsed string
		lastUsed  string
	)
	if err := row.Scan(
		&summary.Runs,
		&summary.Sessions,
		&summary.InputTokens,
		&summary.OutputTokens,
		&summary.CacheCreate,
		&summary.CacheRead,
		&summary.CostUSD,
		&firstUsed,
		&lastUsed,
	); err != nil {
		return nil, fmt.Errorf("读取 token 汇总失败: %w", err)
	}
	if firstUsed != "" {
		parsed, err := parseSQLiteTime(firstUsed)
		if err != nil {
			return nil, err
		}
		summary.FirstUsedAt = parsed
	}
	if lastUsed != "" {
		parsed, err := parseSQLiteTime(lastUsed)
		if err != nil {
			return nil, err
		}
		summary.LastUsedAt = parsed
	}
	return &summary, nil
}

func (s *Store) TaskTokenStats(limit int) ([]TaskTokenStat, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.Query(`
		SELECT
			t.task_id,
			t.issue_number,
			t.issue_title,
			t.last_session_id,
			COUNT(u.id) AS runs,
			COALESCE(SUM(u.input_tokens), 0),
			COALESCE(SUM(u.output_tokens), 0),
			COALESCE(SUM(u.cache_create), 0),
			COALESCE(SUM(u.cache_read), 0),
			COALESCE(SUM(u.cost_usd), 0),
			COALESCE(MAX(u.created_at), '')
		FROM token_usage u
		JOIN tasks t ON t.task_id = u.task_id
		GROUP BY t.task_id, t.issue_number, t.issue_title, t.last_session_id
		ORDER BY MAX(u.created_at) DESC, t.issue_number DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("读取任务 token 统计失败: %w", err)
	}
	defer rows.Close()

	var stats []TaskTokenStat
	for rows.Next() {
		var (
			item     TaskTokenStat
			lastUsed string
		)
		if err := rows.Scan(
			&item.TaskID,
			&item.IssueNumber,
			&item.IssueTitle,
			&item.LastSessionID,
			&item.Runs,
			&item.InputTokens,
			&item.OutputTokens,
			&item.CacheCreate,
			&item.CacheRead,
			&item.CostUSD,
			&lastUsed,
		); err != nil {
			return nil, fmt.Errorf("扫描任务 token 统计失败: %w", err)
		}
		if lastUsed != "" {
			parsed, err := parseSQLiteTime(lastUsed)
			if err != nil {
				return nil, err
			}
			item.LastRecordedAt = parsed
		}
		stats = append(stats, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历任务 token 统计失败: %w", err)
	}
	return stats, nil
}

func (s *Store) ListTasks() ([]*core.Task, error) {
	rows, err := s.db.Query(`
		SELECT task_id, idempotency_key, control_repo, target_repo, last_session_id, issue_number, issue_title, issue_body,
		issue_author, issue_author_permission, labels, intent, risk_level, approved, approval_command,
		approval_actor, approval_comment_id, state, retry_count, error_msg, report_path, created_at, updated_at
		FROM tasks ORDER BY updated_at DESC, issue_number DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("读取任务列表失败: %w", err)
	}
	defer rows.Close()

	var tasks []*core.Task
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历任务列表失败: %w", err)
	}
	return tasks, nil
}

func (s *Store) ListRunnable(limit int) ([]*core.Task, error) {
	rows, err := s.db.Query(`
		SELECT task_id, idempotency_key, control_repo, target_repo, last_session_id, issue_number, issue_title, issue_body,
		issue_author, issue_author_permission, labels, intent, risk_level, approved, approval_command,
		approval_actor, approval_comment_id, state, retry_count, error_msg, report_path, created_at, updated_at
		FROM tasks
		WHERE state IN ('NEW', 'FAILED', 'BLOCKED')
		ORDER BY updated_at ASC, issue_number ASC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("读取待执行任务失败: %w", err)
	}
	defer rows.Close()

	var tasks []*core.Task
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历待执行任务失败: %w", err)
	}
	return tasks, nil
}

func (s *Store) ListRunning() ([]*core.Task, error) {
	rows, err := s.db.Query(`
		SELECT task_id, idempotency_key, control_repo, target_repo, last_session_id, issue_number, issue_title, issue_body,
		issue_author, issue_author_permission, labels, intent, risk_level, approved, approval_command,
		approval_actor, approval_comment_id, state, retry_count, error_msg, report_path, created_at, updated_at
		FROM tasks
		WHERE state = 'RUNNING'
		ORDER BY updated_at ASC, issue_number ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("读取运行中任务失败: %w", err)
	}
	defer rows.Close()

	var tasks []*core.Task
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历运行中任务失败: %w", err)
	}
	return tasks, nil
}

func (s *Store) init() error {
	ddl := []string{
		`CREATE TABLE IF NOT EXISTS tasks (
			task_id TEXT PRIMARY KEY,
			idempotency_key TEXT UNIQUE NOT NULL,
			control_repo TEXT NOT NULL,
			target_repo TEXT NOT NULL,
			last_session_id TEXT NOT NULL DEFAULT '',
			issue_number INTEGER NOT NULL,
			issue_title TEXT,
			issue_body TEXT,
			issue_author TEXT,
			issue_author_permission TEXT,
			labels TEXT NOT NULL,
			intent TEXT,
			risk_level TEXT,
			approved INTEGER DEFAULT 0,
			approval_command TEXT,
			approval_actor TEXT,
			approval_comment_id INTEGER DEFAULT 0,
			state TEXT NOT NULL DEFAULT 'NEW',
			retry_count INTEGER DEFAULT 0,
			error_msg TEXT,
			report_path TEXT,
			created_at DATETIME,
			updated_at DATETIME
		)`,
		`CREATE TABLE IF NOT EXISTS task_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			task_id TEXT NOT NULL,
			event_type TEXT NOT NULL,
			detail TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY(task_id) REFERENCES tasks(task_id)
		)`,
		`CREATE TABLE IF NOT EXISTS token_usage (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			task_id TEXT NOT NULL,
			session_id TEXT,
			input_tokens INTEGER DEFAULT 0,
			output_tokens INTEGER DEFAULT 0,
			cache_create INTEGER DEFAULT 0,
			cache_read INTEGER DEFAULT 0,
			cost_usd REAL DEFAULT 0,
			duration_ms INTEGER DEFAULT 0,
			rtk_enabled INTEGER DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY(task_id) REFERENCES tasks(task_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_tasks_state_updated_at ON tasks(state, updated_at)`,
		`CREATE INDEX IF NOT EXISTS idx_token_usage_task_created_at ON token_usage(task_id, created_at DESC)`,
	}
	for _, stmt := range ddl {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("初始化 SQLite 表失败: %w", err)
		}
	}
	if err := s.ensureTaskColumn("last_session_id", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	return nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanTask(row scanner) (*core.Task, error) {
	var (
		task       core.Task
		labelsJSON string
		intent     string
		risk       string
		state      string
		approved   int
	)
	if err := row.Scan(
		&task.TaskID, &task.IdempotencyKey, &task.ControlRepo, &task.TargetRepo, &task.LastSessionID, &task.IssueNumber, &task.IssueTitle, &task.IssueBody,
		&task.IssueAuthor, &task.IssueAuthorPermission, &labelsJSON, &intent, &risk, &approved, &task.ApprovalCommand,
		&task.ApprovalActor, &task.ApprovalCommentID, &state, &task.RetryCount, &task.ErrorMsg, &task.ReportPath, &task.CreatedAt, &task.UpdatedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("扫描任务失败: %w", err)
	}
	if err := json.Unmarshal([]byte(labelsJSON), &task.Labels); err != nil {
		return nil, fmt.Errorf("解析 labels 失败: %w", err)
	}
	task.Intent = core.Intent(intent)
	task.RiskLevel = core.RiskLevel(risk)
	task.State = core.State(state)
	task.Approved = approved == 1
	return &task, nil
}

func (s *Store) ensureTaskColumn(name, columnDef string) error {
	rows, err := s.db.Query(`PRAGMA table_info(tasks)`)
	if err != nil {
		return fmt.Errorf("读取 tasks 表结构失败: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			cid        int
			columnName string
			columnType string
			notNull    int
			defaultVal any
			pk         int
		)
		if err := rows.Scan(&cid, &columnName, &columnType, &notNull, &defaultVal, &pk); err != nil {
			return fmt.Errorf("扫描 tasks 表结构失败: %w", err)
		}
		if columnName == name {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("遍历 tasks 表结构失败: %w", err)
	}
	if _, err := s.db.Exec(fmt.Sprintf("ALTER TABLE tasks ADD COLUMN %s %s", name, columnDef)); err != nil {
		return fmt.Errorf("补充 tasks.%s 字段失败: %w", name, err)
	}
	return nil
}

func parseSQLiteTime(value string) (time.Time, error) {
	if idx := strings.Index(value, " m="); idx >= 0 {
		value = value[:idx]
	}
	layouts := []string{
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04:05 -0700 MST",
		time.RFC3339Nano,
	}
	for _, layout := range layouts {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("解析 SQLite 时间失败: %s", value)
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
