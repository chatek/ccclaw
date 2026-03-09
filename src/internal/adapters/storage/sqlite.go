package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/41490/ccclaw/internal/core"
	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
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
		SELECT task_id, idempotency_key, control_repo, target_repo, issue_number, issue_title, issue_body,
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
			task_id, idempotency_key, control_repo, target_repo, issue_number, issue_title, issue_body,
			issue_author, issue_author_permission, labels, intent, risk_level, approved, approval_command,
			approval_actor, approval_comment_id, state, retry_count, error_msg, report_path, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(task_id) DO UPDATE SET
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
		task.TaskID, task.IdempotencyKey, task.ControlRepo, task.TargetRepo, task.IssueNumber, task.IssueTitle, task.IssueBody,
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

func (s *Store) ListTasks() ([]*core.Task, error) {
	rows, err := s.db.Query(`
		SELECT task_id, idempotency_key, control_repo, target_repo, issue_number, issue_title, issue_body,
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
		SELECT task_id, idempotency_key, control_repo, target_repo, issue_number, issue_title, issue_body,
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

func (s *Store) init() error {
	ddl := []string{
		`CREATE TABLE IF NOT EXISTS tasks (
			task_id TEXT PRIMARY KEY,
			idempotency_key TEXT UNIQUE NOT NULL,
			control_repo TEXT NOT NULL,
			target_repo TEXT NOT NULL,
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
		`CREATE INDEX IF NOT EXISTS idx_tasks_state_updated_at ON tasks(state, updated_at)`,
	}
	for _, stmt := range ddl {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("初始化 SQLite 表失败: %w", err)
		}
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
		&task.TaskID, &task.IdempotencyKey, &task.ControlRepo, &task.TargetRepo, &task.IssueNumber, &task.IssueTitle, &task.IssueBody,
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

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
