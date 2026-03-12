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
	PromptFile string
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
	IssueRepo      string
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

type RTKComparison struct {
	RTKRuns         int
	PlainRuns       int
	RTKAvgCostUSD   float64
	PlainAvgCostUSD float64
	RTKAvgTokens    float64
	PlainAvgTokens  float64
	SavingsPercent  float64
}

type DailyTokenStat struct {
	Day          time.Time
	Runs         int
	Sessions     int
	InputTokens  int
	OutputTokens int
	CacheCreate  int
	CacheRead    int
	CostUSD      float64
}

type DailyRTKComparison struct {
	Day             time.Time
	RTKRuns         int
	PlainRuns       int
	RTKAvgCostUSD   float64
	PlainAvgCostUSD float64
	RTKAvgTokens    float64
	PlainAvgTokens  float64
	SavingsPercent  float64
}

type JournalDaySummary struct {
	TasksTouched int
	Started      int
	Done         int
	Failed       int
	Dead         int
	InputTokens  int
	OutputTokens int
	CacheCreate  int
	CacheRead    int
	CostUSD      float64
}

type JournalTaskSummary struct {
	TaskID       string
	IssueRepo    string
	IssueNumber  int
	IssueTitle   string
	State        core.State
	Runs         int
	InputTokens  int
	OutputTokens int
	CacheCreate  int
	CacheRead    int
	CostUSD      float64
	UpdatedAt    time.Time
}

type EventRecord struct {
	TaskID    string
	EventType core.EventType
	Detail    string
	CreatedAt time.Time
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
		SELECT task_id, idempotency_key, control_repo, issue_repo, target_repo, last_session_id, issue_number, issue_title, issue_body,
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
			task_id, idempotency_key, control_repo, issue_repo, target_repo, last_session_id, issue_number, issue_title, issue_body,
			issue_author, issue_author_permission, labels, intent, risk_level, approved, approval_command,
			approval_actor, approval_comment_id, state, retry_count, error_msg, report_path, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(task_id) DO UPDATE SET
			issue_repo = excluded.issue_repo,
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
		task.TaskID, task.IdempotencyKey, task.ControlRepo, task.IssueRepo, task.TargetRepo, task.LastSessionID, task.IssueNumber, task.IssueTitle, task.IssueBody,
		task.IssueAuthor, task.IssueAuthorPermission, string(labels), string(task.Intent), string(task.RiskLevel), boolToInt(task.Approved), task.ApprovalCommand,
		task.ApprovalActor, task.ApprovalCommentID, string(task.State), task.RetryCount, task.ErrorMsg, task.ReportPath, task.CreatedAt, task.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("写入任务失败: %w", err)
	}
	return nil
}

func (s *Store) AppendEvent(taskID string, eventType core.EventType, detail string) error {
	return s.AppendEventAt(taskID, eventType, detail, time.Time{})
}

func (s *Store) AppendEventAt(taskID string, eventType core.EventType, detail string, createdAt time.Time) error {
	if createdAt.IsZero() {
		_, err := s.db.Exec(`INSERT INTO task_events (task_id, event_type, detail) VALUES (?, ?, ?)`, taskID, string(eventType), detail)
		if err != nil {
			return fmt.Errorf("写入任务事件失败: %w", err)
		}
		return nil
	}
	_, err := s.db.Exec(`INSERT INTO task_events (task_id, event_type, detail, created_at) VALUES (?, ?, ?, ?)`, taskID, string(eventType), detail, createdAt)
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
			cost_usd, duration_ms, rtk_enabled, prompt_file, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
		record.PromptFile,
		recordedAt,
	)
	if err != nil {
		return fmt.Errorf("写入 token 使用记录失败: %w", err)
	}
	return nil
}

func (s *Store) TokenStats() (*TokenStatsSummary, error) {
	return s.TokenStatsBetween(time.Time{}, time.Time{})
}

func (s *Store) TokenStatsBetween(start, end time.Time) (*TokenStatsSummary, error) {
	where, args := buildTimeRangeClause("created_at", start, end)
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
		FROM token_usage`+where, args...)
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

func (s *Store) RTKComparisonBetween(start, end time.Time) (*RTKComparison, error) {
	where, args := buildTimeRangeClause("created_at", start, end)
	row := s.db.QueryRow(`
		SELECT
			COALESCE(SUM(CASE WHEN rtk_enabled = 1 THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN rtk_enabled = 0 THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN rtk_enabled = 1 THEN cost_usd ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN rtk_enabled = 0 THEN cost_usd ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN rtk_enabled = 1 THEN input_tokens + output_tokens + cache_create + cache_read ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN rtk_enabled = 0 THEN input_tokens + output_tokens + cache_create + cache_read ELSE 0 END), 0)
		FROM token_usage`+where, args...)
	var (
		item             RTKComparison
		rtkTotalCost     float64
		plainTotalCost   float64
		rtkTotalTokens   int
		plainTotalTokens int
	)
	if err := row.Scan(
		&item.RTKRuns,
		&item.PlainRuns,
		&rtkTotalCost,
		&plainTotalCost,
		&rtkTotalTokens,
		&plainTotalTokens,
	); err != nil {
		return nil, fmt.Errorf("读取 rtk 对比统计失败: %w", err)
	}
	if item.RTKRuns > 0 {
		item.RTKAvgCostUSD = rtkTotalCost / float64(item.RTKRuns)
		item.RTKAvgTokens = float64(rtkTotalTokens) / float64(item.RTKRuns)
	}
	if item.PlainRuns > 0 {
		item.PlainAvgCostUSD = plainTotalCost / float64(item.PlainRuns)
		item.PlainAvgTokens = float64(plainTotalTokens) / float64(item.PlainRuns)
	}
	if item.RTKRuns > 0 && item.PlainRuns > 0 && item.PlainAvgCostUSD > 0 {
		item.SavingsPercent = (item.PlainAvgCostUSD - item.RTKAvgCostUSD) / item.PlainAvgCostUSD * 100
	}
	return &item, nil
}

func (s *Store) TaskTokenStats(limit int) ([]TaskTokenStat, error) {
	return s.TaskTokenStatsBetween(time.Time{}, time.Time{}, limit)
}

func (s *Store) TaskTokenStatsBetween(start, end time.Time, limit int) ([]TaskTokenStat, error) {
	if limit <= 0 {
		limit = 20
	}
	where, args := buildTimeRangeClause("u.created_at", start, end)
	args = append(args, limit)
	rows, err := s.db.Query(`
		SELECT
			t.task_id,
			t.issue_repo,
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
	`+where+`
		GROUP BY t.task_id, t.issue_repo, t.issue_number, t.issue_title, t.last_session_id
		ORDER BY MAX(u.created_at) DESC, t.issue_number DESC
		LIMIT ?
	`, args...)
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
			&item.IssueRepo,
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

func (s *Store) DailyTokenStatsBetween(start, end time.Time) ([]DailyTokenStat, error) {
	where, args := buildTimeRangeClause("created_at", start, end)
	rows, err := s.db.Query(`
		SELECT session_id, input_tokens, output_tokens, cache_create, cache_read, cost_usd, created_at
		FROM token_usage
	`+where+`
		ORDER BY created_at ASC, id ASC
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("读取每日 token 统计失败: %w", err)
	}
	defer rows.Close()

	type aggregate struct {
		item     DailyTokenStat
		sessions map[string]struct{}
	}

	loc := time.Local
	if !start.IsZero() {
		loc = start.Location()
	}
	orderedKeys := make([]string, 0)
	items := make(map[string]*aggregate)
	for rows.Next() {
		var (
			sessionID    string
			inputTokens  int
			outputTokens int
			cacheCreate  int
			cacheRead    int
			costUSD      float64
			createdAt    string
		)
		if err := rows.Scan(&sessionID, &inputTokens, &outputTokens, &cacheCreate, &cacheRead, &costUSD, &createdAt); err != nil {
			return nil, fmt.Errorf("扫描每日 token 统计失败: %w", err)
		}
		parsed, err := parseSQLiteTime(createdAt)
		if err != nil {
			return nil, err
		}
		day := parsed.In(loc)
		day = time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, loc)
		key := day.Format("2006-01-02")
		entry, exists := items[key]
		if !exists {
			entry = &aggregate{
				item:     DailyTokenStat{Day: day},
				sessions: make(map[string]struct{}),
			}
			items[key] = entry
			orderedKeys = append(orderedKeys, key)
		}
		entry.item.Runs++
		entry.item.InputTokens += inputTokens
		entry.item.OutputTokens += outputTokens
		entry.item.CacheCreate += cacheCreate
		entry.item.CacheRead += cacheRead
		entry.item.CostUSD += costUSD
		if strings.TrimSpace(sessionID) != "" {
			entry.sessions[sessionID] = struct{}{}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历每日 token 统计失败: %w", err)
	}

	daily := make([]DailyTokenStat, 0, len(orderedKeys))
	for _, key := range orderedKeys {
		entry := items[key]
		entry.item.Sessions = len(entry.sessions)
		daily = append(daily, entry.item)
	}
	return daily, nil
}

func (s *Store) DailyRTKComparisonBetween(start, end time.Time) ([]DailyRTKComparison, error) {
	where, args := buildTimeRangeClause("created_at", start, end)
	rows, err := s.db.Query(`
		SELECT rtk_enabled, input_tokens, output_tokens, cache_create, cache_read, cost_usd, created_at
		FROM token_usage
	`+where+`
		ORDER BY created_at ASC, id ASC
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("读取每日 rtk 对比失败: %w", err)
	}
	defer rows.Close()

	type bucket struct {
		runs       int
		totalCost  float64
		totalToken int
	}
	type aggregate struct {
		day   time.Time
		rtk   bucket
		plain bucket
	}

	loc := time.Local
	if !start.IsZero() {
		loc = start.Location()
	}
	orderedKeys := make([]string, 0)
	items := make(map[string]*aggregate)
	for rows.Next() {
		var (
			rtkEnabled   int
			inputTokens  int
			outputTokens int
			cacheCreate  int
			cacheRead    int
			costUSD      float64
			createdAt    string
		)
		if err := rows.Scan(&rtkEnabled, &inputTokens, &outputTokens, &cacheCreate, &cacheRead, &costUSD, &createdAt); err != nil {
			return nil, fmt.Errorf("扫描每日 rtk 对比失败: %w", err)
		}
		parsed, err := parseSQLiteTime(createdAt)
		if err != nil {
			return nil, err
		}
		day := parsed.In(loc)
		day = time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, loc)
		key := day.Format("2006-01-02")
		entry, exists := items[key]
		if !exists {
			entry = &aggregate{day: day}
			items[key] = entry
			orderedKeys = append(orderedKeys, key)
		}
		target := &entry.plain
		if rtkEnabled == 1 {
			target = &entry.rtk
		}
		target.runs++
		target.totalCost += costUSD
		target.totalToken += inputTokens + outputTokens + cacheCreate + cacheRead
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历每日 rtk 对比失败: %w", err)
	}

	itemsByDay := make([]DailyRTKComparison, 0, len(orderedKeys))
	for _, key := range orderedKeys {
		entry := items[key]
		item := DailyRTKComparison{
			Day:       entry.day,
			RTKRuns:   entry.rtk.runs,
			PlainRuns: entry.plain.runs,
		}
		if entry.rtk.runs > 0 {
			item.RTKAvgCostUSD = entry.rtk.totalCost / float64(entry.rtk.runs)
			item.RTKAvgTokens = float64(entry.rtk.totalToken) / float64(entry.rtk.runs)
		}
		if entry.plain.runs > 0 {
			item.PlainAvgCostUSD = entry.plain.totalCost / float64(entry.plain.runs)
			item.PlainAvgTokens = float64(entry.plain.totalToken) / float64(entry.plain.runs)
		}
		if item.RTKRuns > 0 && item.PlainRuns > 0 && item.PlainAvgCostUSD > 0 {
			item.SavingsPercent = (item.PlainAvgCostUSD - item.RTKAvgCostUSD) / item.PlainAvgCostUSD * 100
		}
		itemsByDay = append(itemsByDay, item)
	}
	return itemsByDay, nil
}

func (s *Store) ListTasks() ([]*core.Task, error) {
	rows, err := s.db.Query(`
		SELECT task_id, idempotency_key, control_repo, issue_repo, target_repo, last_session_id, issue_number, issue_title, issue_body,
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
		SELECT task_id, idempotency_key, control_repo, issue_repo, target_repo, last_session_id, issue_number, issue_title, issue_body,
		issue_author, issue_author_permission, labels, intent, risk_level, approved, approval_command,
		approval_actor, approval_comment_id, state, retry_count, error_msg, report_path, created_at, updated_at
		FROM tasks
		WHERE state IN ('NEW', 'FAILED')
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
		SELECT task_id, idempotency_key, control_repo, issue_repo, target_repo, last_session_id, issue_number, issue_title, issue_body,
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

func (s *Store) JournalDaySummary(day time.Time) (*JournalDaySummary, error) {
	start, end := dayBounds(day)
	return s.JournalSummaryBetween(start, end)
}

func (s *Store) JournalDaySummaryByTarget(day time.Time, targetRepo string) (*JournalDaySummary, error) {
	start, end := dayBounds(day)
	targetRepo = strings.TrimSpace(targetRepo)
	if targetRepo == "" {
		return s.JournalDaySummary(day)
	}
	tokenRow := s.db.QueryRow(`
		SELECT
			COALESCE(SUM(u.input_tokens), 0),
			COALESCE(SUM(u.output_tokens), 0),
			COALESCE(SUM(u.cache_create), 0),
			COALESCE(SUM(u.cache_read), 0),
			COALESCE(SUM(u.cost_usd), 0)
		FROM token_usage u
		JOIN tasks t ON t.task_id = u.task_id
		WHERE t.target_repo = ? AND u.created_at >= ? AND u.created_at < ?
	`, targetRepo, start, end)
	summary := &JournalDaySummary{}
	if err := tokenRow.Scan(
		&summary.InputTokens,
		&summary.OutputTokens,
		&summary.CacheCreate,
		&summary.CacheRead,
		&summary.CostUSD,
	); err != nil {
		return nil, fmt.Errorf("读取目标仓库 journal token 汇总失败: %w", err)
	}
	row := s.db.QueryRow(`
		SELECT
			COUNT(DISTINCT task_id),
			COUNT(DISTINCT CASE WHEN event_type = 'STARTED' THEN task_id END),
			COUNT(DISTINCT CASE WHEN event_type = 'DONE' THEN task_id END),
			COUNT(DISTINCT CASE WHEN event_type = 'FAILED' THEN task_id END),
			COUNT(DISTINCT CASE WHEN event_type = 'DEAD' THEN task_id END)
		FROM (
			SELECT e.task_id, e.event_type
			FROM task_events e
			JOIN tasks t ON t.task_id = e.task_id
			WHERE t.target_repo = ? AND e.created_at >= ? AND e.created_at < ?
			UNION ALL
			SELECT u.task_id, '' AS event_type
			FROM token_usage u
			JOIN tasks t ON t.task_id = u.task_id
			WHERE t.target_repo = ? AND u.created_at >= ? AND u.created_at < ?
		)
	`, targetRepo, start, end, targetRepo, start, end)
	if err := row.Scan(&summary.TasksTouched, &summary.Started, &summary.Done, &summary.Failed, &summary.Dead); err != nil {
		return nil, fmt.Errorf("读取目标仓库 journal 日汇总失败: %w", err)
	}
	return summary, nil
}

func (s *Store) JournalSummaryBetween(start, end time.Time) (*JournalDaySummary, error) {
	tokenSummary, err := s.TokenStatsBetween(start, end)
	if err != nil {
		return nil, err
	}
	row := s.db.QueryRow(`
		SELECT
			COUNT(DISTINCT task_id),
			COUNT(DISTINCT CASE WHEN event_type = 'STARTED' THEN task_id END),
			COUNT(DISTINCT CASE WHEN event_type = 'DONE' THEN task_id END),
			COUNT(DISTINCT CASE WHEN event_type = 'FAILED' THEN task_id END),
			COUNT(DISTINCT CASE WHEN event_type = 'DEAD' THEN task_id END)
		FROM (
			SELECT task_id, event_type FROM task_events WHERE created_at >= ? AND created_at < ?
			UNION ALL
			SELECT task_id, '' AS event_type FROM token_usage WHERE created_at >= ? AND created_at < ?
		)
	`, start, end, start, end)
	summary := &JournalDaySummary{
		InputTokens:  tokenSummary.InputTokens,
		OutputTokens: tokenSummary.OutputTokens,
		CacheCreate:  tokenSummary.CacheCreate,
		CacheRead:    tokenSummary.CacheRead,
		CostUSD:      tokenSummary.CostUSD,
	}
	if err := row.Scan(&summary.TasksTouched, &summary.Started, &summary.Done, &summary.Failed, &summary.Dead); err != nil {
		return nil, fmt.Errorf("读取 journal 日汇总失败: %w", err)
	}
	return summary, nil
}

func (s *Store) JournalTaskSummaries(day time.Time) ([]JournalTaskSummary, error) {
	start, end := dayBounds(day)
	rows, err := s.db.Query(`
		SELECT
			t.task_id,
			t.issue_repo,
			t.issue_number,
			t.issue_title,
			t.state,
			COALESCE(COUNT(u.id), 0),
			COALESCE(SUM(u.input_tokens), 0),
			COALESCE(SUM(u.output_tokens), 0),
			COALESCE(SUM(u.cache_create), 0),
			COALESCE(SUM(u.cache_read), 0),
			COALESCE(SUM(u.cost_usd), 0),
			COALESCE(MAX(t.updated_at), '')
		FROM tasks t
		LEFT JOIN token_usage u
			ON t.task_id = u.task_id
			AND u.created_at >= ? AND u.created_at < ?
		WHERE t.task_id IN (
			SELECT task_id FROM task_events WHERE created_at >= ? AND created_at < ?
			UNION
			SELECT task_id FROM token_usage WHERE created_at >= ? AND created_at < ?
			UNION
			SELECT task_id FROM tasks WHERE updated_at >= ? AND updated_at < ?
		)
		GROUP BY t.task_id, t.issue_repo, t.issue_number, t.issue_title, t.state
		ORDER BY MAX(t.updated_at) DESC, t.issue_number DESC
	`, start, end, start, end, start, end, start, end)
	if err != nil {
		return nil, fmt.Errorf("读取 journal 任务汇总失败: %w", err)
	}
	defer rows.Close()

	var items []JournalTaskSummary
	for rows.Next() {
		var (
			item      JournalTaskSummary
			state     string
			updatedAt string
		)
		if err := rows.Scan(
			&item.TaskID,
			&item.IssueRepo,
			&item.IssueNumber,
			&item.IssueTitle,
			&state,
			&item.Runs,
			&item.InputTokens,
			&item.OutputTokens,
			&item.CacheCreate,
			&item.CacheRead,
			&item.CostUSD,
			&updatedAt,
		); err != nil {
			return nil, fmt.Errorf("扫描 journal 任务汇总失败: %w", err)
		}
		item.State = core.State(state)
		if updatedAt != "" {
			parsed, err := parseSQLiteTime(updatedAt)
			if err != nil {
				return nil, err
			}
			item.UpdatedAt = parsed
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历 journal 任务汇总失败: %w", err)
	}
	return items, nil
}

func (s *Store) JournalTaskSummariesByTarget(day time.Time, targetRepo string) ([]JournalTaskSummary, error) {
	start, end := dayBounds(day)
	targetRepo = strings.TrimSpace(targetRepo)
	if targetRepo == "" {
		return s.JournalTaskSummaries(day)
	}
	rows, err := s.db.Query(`
		SELECT
			t.task_id,
			t.issue_repo,
			t.issue_number,
			t.issue_title,
			t.state,
			COALESCE(COUNT(u.id), 0),
			COALESCE(SUM(u.input_tokens), 0),
			COALESCE(SUM(u.output_tokens), 0),
			COALESCE(SUM(u.cache_create), 0),
			COALESCE(SUM(u.cache_read), 0),
			COALESCE(SUM(u.cost_usd), 0),
			COALESCE(MAX(t.updated_at), '')
		FROM tasks t
		LEFT JOIN token_usage u
			ON t.task_id = u.task_id
			AND u.created_at >= ? AND u.created_at < ?
		WHERE t.target_repo = ?
			AND t.task_id IN (
				SELECT e.task_id
				FROM task_events e
				JOIN tasks te ON te.task_id = e.task_id
				WHERE te.target_repo = ? AND e.created_at >= ? AND e.created_at < ?
				UNION
				SELECT u.task_id
				FROM token_usage u
				JOIN tasks tu ON tu.task_id = u.task_id
				WHERE tu.target_repo = ? AND u.created_at >= ? AND u.created_at < ?
				UNION
				SELECT task_id FROM tasks WHERE target_repo = ? AND updated_at >= ? AND updated_at < ?
			)
		GROUP BY t.task_id, t.issue_repo, t.issue_number, t.issue_title, t.state
		ORDER BY MAX(t.updated_at) DESC, t.issue_number DESC
	`, start, end, targetRepo, targetRepo, start, end, targetRepo, start, end, targetRepo, start, end)
	if err != nil {
		return nil, fmt.Errorf("读取目标仓库 journal 任务汇总失败: %w", err)
	}
	defer rows.Close()

	var items []JournalTaskSummary
	for rows.Next() {
		var (
			item      JournalTaskSummary
			state     string
			updatedAt string
		)
		if err := rows.Scan(
			&item.TaskID,
			&item.IssueRepo,
			&item.IssueNumber,
			&item.IssueTitle,
			&state,
			&item.Runs,
			&item.InputTokens,
			&item.OutputTokens,
			&item.CacheCreate,
			&item.CacheRead,
			&item.CostUSD,
			&updatedAt,
		); err != nil {
			return nil, fmt.Errorf("扫描目标仓库 journal 任务汇总失败: %w", err)
		}
		item.State = core.State(state)
		if updatedAt != "" {
			parsed, err := parseSQLiteTime(updatedAt)
			if err != nil {
				return nil, err
			}
			item.UpdatedAt = parsed
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历目标仓库 journal 任务汇总失败: %w", err)
	}
	return items, nil
}

func (s *Store) ListTaskEventsBetween(start, end time.Time, limit int) ([]EventRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`
		SELECT task_id, event_type, detail, created_at
		FROM task_events
		WHERE created_at >= ? AND created_at < ?
		ORDER BY created_at DESC, id DESC
		LIMIT ?
	`, start, end, limit)
	if err != nil {
		return nil, fmt.Errorf("读取任务事件失败: %w", err)
	}
	defer rows.Close()

	var items []EventRecord
	for rows.Next() {
		var (
			item      EventRecord
			eventType string
			createdAt string
		)
		if err := rows.Scan(&item.TaskID, &eventType, &item.Detail, &createdAt); err != nil {
			return nil, fmt.Errorf("扫描任务事件失败: %w", err)
		}
		item.EventType = core.EventType(eventType)
		parsed, err := parseSQLiteTime(createdAt)
		if err != nil {
			return nil, err
		}
		item.CreatedAt = parsed
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历任务事件失败: %w", err)
	}
	return items, nil
}

func (s *Store) ListTaskEventsBetweenByTarget(start, end time.Time, limit int, targetRepo string) ([]EventRecord, error) {
	targetRepo = strings.TrimSpace(targetRepo)
	if targetRepo == "" {
		return s.ListTaskEventsBetween(start, end, limit)
	}
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`
		SELECT e.task_id, e.event_type, e.detail, e.created_at
		FROM task_events e
		JOIN tasks t ON t.task_id = e.task_id
		WHERE t.target_repo = ? AND e.created_at >= ? AND e.created_at < ?
		ORDER BY e.created_at DESC, e.id DESC
		LIMIT ?
	`, targetRepo, start, end, limit)
	if err != nil {
		return nil, fmt.Errorf("读取目标仓库任务事件失败: %w", err)
	}
	defer rows.Close()

	var items []EventRecord
	for rows.Next() {
		var (
			item      EventRecord
			eventType string
			createdAt string
		)
		if err := rows.Scan(&item.TaskID, &eventType, &item.Detail, &createdAt); err != nil {
			return nil, fmt.Errorf("扫描目标仓库任务事件失败: %w", err)
		}
		item.EventType = core.EventType(eventType)
		parsed, err := parseSQLiteTime(createdAt)
		if err != nil {
			return nil, err
		}
		item.CreatedAt = parsed
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历目标仓库任务事件失败: %w", err)
	}
	return items, nil
}

func (s *Store) RTKComparisonBetweenByTarget(start, end time.Time, targetRepo string) (*RTKComparison, error) {
	targetRepo = strings.TrimSpace(targetRepo)
	if targetRepo == "" {
		return s.RTKComparisonBetween(start, end)
	}
	row := s.db.QueryRow(`
		SELECT
			COALESCE(SUM(CASE WHEN u.rtk_enabled = 1 THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN u.rtk_enabled = 0 THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN u.rtk_enabled = 1 THEN u.cost_usd ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN u.rtk_enabled = 0 THEN u.cost_usd ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN u.rtk_enabled = 1 THEN u.input_tokens + u.output_tokens + u.cache_create + u.cache_read ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN u.rtk_enabled = 0 THEN u.input_tokens + u.output_tokens + u.cache_create + u.cache_read ELSE 0 END), 0)
		FROM token_usage u
		JOIN tasks t ON t.task_id = u.task_id
		WHERE t.target_repo = ? AND u.created_at >= ? AND u.created_at < ?
	`, targetRepo, start, end)
	var (
		item             RTKComparison
		rtkTotalCost     float64
		plainTotalCost   float64
		rtkTotalTokens   int
		plainTotalTokens int
	)
	if err := row.Scan(
		&item.RTKRuns,
		&item.PlainRuns,
		&rtkTotalCost,
		&plainTotalCost,
		&rtkTotalTokens,
		&plainTotalTokens,
	); err != nil {
		return nil, fmt.Errorf("读取目标仓库 rtk 对比统计失败: %w", err)
	}
	if item.RTKRuns > 0 {
		item.RTKAvgCostUSD = rtkTotalCost / float64(item.RTKRuns)
		item.RTKAvgTokens = float64(rtkTotalTokens) / float64(item.RTKRuns)
	}
	if item.PlainRuns > 0 {
		item.PlainAvgCostUSD = plainTotalCost / float64(item.PlainRuns)
		item.PlainAvgTokens = float64(plainTotalTokens) / float64(item.PlainRuns)
	}
	if item.RTKRuns > 0 && item.PlainRuns > 0 && item.PlainAvgCostUSD > 0 {
		item.SavingsPercent = (item.PlainAvgCostUSD - item.RTKAvgCostUSD) / item.PlainAvgCostUSD * 100
	}
	return &item, nil
}

func (s *Store) ListTaskEvents(limit int) ([]EventRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`
		SELECT task_id, event_type, detail, created_at
		FROM task_events
		ORDER BY created_at DESC, id DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("读取任务事件失败: %w", err)
	}
	defer rows.Close()

	var items []EventRecord
	for rows.Next() {
		var (
			item      EventRecord
			eventType string
			createdAt string
		)
		if err := rows.Scan(&item.TaskID, &eventType, &item.Detail, &createdAt); err != nil {
			return nil, fmt.Errorf("扫描任务事件失败: %w", err)
		}
		item.EventType = core.EventType(eventType)
		parsed, err := parseSQLiteTime(createdAt)
		if err != nil {
			return nil, err
		}
		item.CreatedAt = parsed
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历任务事件失败: %w", err)
	}
	return items, nil
}

func (s *Store) init() error {
	ddl := []string{
		`CREATE TABLE IF NOT EXISTS tasks (
			task_id TEXT PRIMARY KEY,
			idempotency_key TEXT UNIQUE NOT NULL,
			control_repo TEXT NOT NULL,
			issue_repo TEXT NOT NULL DEFAULT '',
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
			prompt_file TEXT NOT NULL DEFAULT '',
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
	if err := s.ensureTaskColumn("issue_repo", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if _, err := s.db.Exec(`UPDATE tasks SET issue_repo = control_repo WHERE issue_repo = ''`); err != nil {
		return fmt.Errorf("回填 issue_repo 失败: %w", err)
	}
	if err := s.ensureTokenUsageColumn("prompt_file", "TEXT NOT NULL DEFAULT ''"); err != nil {
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
		&task.TaskID, &task.IdempotencyKey, &task.ControlRepo, &task.IssueRepo, &task.TargetRepo, &task.LastSessionID, &task.IssueNumber, &task.IssueTitle, &task.IssueBody,
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
	if strings.TrimSpace(task.IssueRepo) == "" {
		task.IssueRepo = task.ControlRepo
	}
	return &task, nil
}

func (s *Store) ensureTaskColumn(name, columnDef string) error {
	return s.ensureTableColumn("tasks", name, columnDef)
}

func (s *Store) ensureTokenUsageColumn(name, columnDef string) error {
	return s.ensureTableColumn("token_usage", name, columnDef)
}

func (s *Store) ensureTableColumn(table, name, columnDef string) error {
	rows, err := s.db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return fmt.Errorf("读取 %s 表结构失败: %w", table, err)
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
			return fmt.Errorf("扫描 %s 表结构失败: %w", table, err)
		}
		if columnName == name {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("遍历 %s 表结构失败: %w", table, err)
	}
	if _, err := s.db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, name, columnDef)); err != nil {
		return fmt.Errorf("补充 %s.%s 字段失败: %w", table, name, err)
	}
	return nil
}

func buildTimeRangeClause(column string, start, end time.Time) (string, []any) {
	clauses := make([]string, 0, 2)
	args := make([]any, 0, 2)
	if !start.IsZero() {
		clauses = append(clauses, column+" >= ?")
		args = append(args, start)
	}
	if !end.IsZero() {
		clauses = append(clauses, column+" < ?")
		args = append(args, end)
	}
	if len(clauses) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

func dayBounds(day time.Time) (time.Time, time.Time) {
	loc := day.Location()
	start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, loc)
	return start, start.Add(24 * time.Hour)
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
