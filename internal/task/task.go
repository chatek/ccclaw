// Package task 定义任务状态机与核心数据结构
package task

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// State 任务状态
type State string

const (
	StateNew      State = "NEW"
	StateTriaged  State = "TRIAGED"
	StateRunning  State = "RUNNING"
	StateBlocked  State = "BLOCKED"
	StateFailed   State = "FAILED"
	StateDone     State = "DONE"
	StateArchived State = "ARCHIVED"
	StateDead     State = "DEAD" // 死信
)

// RiskLevel 风险等级
type RiskLevel string

const (
	RiskLow  RiskLevel = "low"
	RiskMed  RiskLevel = "med"
	RiskHigh RiskLevel = "high"
)

// Intent 任务意图
type Intent string

const (
	IntentResearch Intent = "research"
	IntentFix      Intent = "fix"
	IntentOps      Intent = "ops"
	IntentRelease  Intent = "release"
)

// Task 最小任务字段
type Task struct {
	TaskID         string    `json:"task_id"`         // issue# + comment_id
	IdempotencyKey string    `json:"idempotency_key"` // 防重复执行
	Intent         Intent    `json:"intent"`
	RiskLevel      RiskLevel `json:"risk_level"`
	Approved       bool      `json:"approved"` // high-risk 任务是否已获 approved 标签
	SkillRefs      []string  `json:"skill_refs"`
	TraceRefs       []string  `json:"trace_refs"`
	State           State     `json:"state"`
	RetryCount      int       `json:"retry_count"`
	IssueNumber     int       `json:"issue_number"`
	IssueTitle      string    `json:"issue_title"`
	IssueBody       string    `json:"issue_body"`
	Labels          []string  `json:"labels"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	ErrorMsg        string    `json:"error_msg,omitempty"`
}

const maxRetry = 3

// CanExecute 检查任务是否可执行（风险门禁）
func (t *Task) CanExecute() error {
	if t.RiskLevel == RiskHigh && !t.Approved {
		return fmt.Errorf("high-risk 任务需要 'approved' 标签，task_id=%s", t.TaskID)
	}
	if t.RetryCount >= maxRetry {
		return fmt.Errorf("已超过最大重试次数 %d，task_id=%s", maxRetry, t.TaskID)
	}
	return nil
}

// HasLabel 检查是否含有指定标签
func (t *Task) HasLabel(label string) bool {
	for _, l := range t.Labels {
		if l == label {
			return true
		}
	}
	return false
}

// Store 基于文件系统的任务状态存储
type Store struct {
	dir string
}

// NewStore 创建状态存储，dir 为状态目录
func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("创建状态目录失败: %w", err)
	}
	return &Store{dir: dir}, nil
}

func (s *Store) path(idempotencyKey string) string {
	// 将 # 替换为 _ 避免文件名问题
	safe := ""
	for _, c := range idempotencyKey {
		if c == '#' || c == '/' {
			safe += "_"
		} else {
			safe += string(c)
		}
	}
	return filepath.Join(s.dir, safe+".json")
}

// Save 持久化任务
func (s *Store) Save(t *Task) error {
	t.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化任务失败: %w", err)
	}
	return os.WriteFile(s.path(t.IdempotencyKey), data, 0o644)
}

// Load 加载任务，不存在返回 nil, nil
func (s *Store) Load(idempotencyKey string) (*Task, error) {
	data, err := os.ReadFile(s.path(idempotencyKey))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取任务文件失败: %w", err)
	}
	var t Task
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("反序列化任务失败: %w", err)
	}
	return &t, nil
}

// Exists 检查幂等键是否已存在
func (s *Store) Exists(idempotencyKey string) (bool, error) {
	t, err := s.Load(idempotencyKey)
	if err != nil {
		return false, err
	}
	return t != nil, nil
}

// List 列出所有任务
func (s *Store) List() ([]*Task, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, fmt.Errorf("读取状态目录失败: %w", err)
	}
	var tasks []*Task
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.dir, e.Name()))
		if err != nil {
			continue
		}
		var t Task
		if err := json.Unmarshal(data, &t); err != nil {
			continue
		}
		tasks = append(tasks, &t)
	}
	return tasks, nil
}
