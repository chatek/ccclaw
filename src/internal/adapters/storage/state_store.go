package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/41490/ccclaw/internal/core"
)

type stateFile struct {
	Tasks map[string]*core.Task `json:"tasks"`
}

type StateStore struct {
	varDir string
}

func newStateStore(varDir string) *StateStore {
	return &StateStore{varDir: varDir}
}

func (s *StateStore) GetByIdempotency(idempotencyKey string) (*core.Task, error) {
	state, err := s.load()
	if err != nil {
		return nil, err
	}
	for _, task := range state.Tasks {
		if task != nil && task.IdempotencyKey == idempotencyKey {
			return cloneTask(task), nil
		}
	}
	return nil, nil
}

func (s *StateStore) GetTask(taskID string) (*core.Task, error) {
	state, err := s.load()
	if err != nil {
		return nil, err
	}
	task := state.Tasks[taskID]
	if task == nil {
		return nil, nil
	}
	return cloneTask(task), nil
}

func (s *StateStore) UpsertTask(task *core.Task) error {
	if task == nil {
		return errors.New("task 不能为空")
	}
	return withFileLock(s.lockPath(), func() error {
		state, err := s.loadUnlocked()
		if err != nil {
			return err
		}
		if state.Tasks == nil {
			state.Tasks = map[string]*core.Task{}
		}
		now := time.Now().UTC()
		existing := cloneTask(state.Tasks[task.TaskID])
		if task.CreatedAt.IsZero() {
			if existing != nil && !existing.CreatedAt.IsZero() {
				task.CreatedAt = existing.CreatedAt
			} else {
				task.CreatedAt = now
			}
		}
		if existing == nil {
			if task.UpdatedAt.IsZero() {
				task.UpdatedAt = now
			}
		} else if taskChanged(existing, task) || task.UpdatedAt.IsZero() {
			task.UpdatedAt = now
		}
		state.Tasks[task.TaskID] = cloneTask(task)
		return s.saveUnlocked(state)
	})
}

func (s *StateStore) ApplyTask(taskID string, fn func(*core.Task) (*core.Task, error)) (*core.Task, error) {
	if stringsTrimSpace(taskID) == "" {
		return nil, errors.New("taskID 不能为空")
	}
	if fn == nil {
		return nil, errors.New("task 更新函数不能为空")
	}
	var updated *core.Task
	err := withFileLock(s.lockPath(), func() error {
		state, err := s.loadUnlocked()
		if err != nil {
			return err
		}
		if state.Tasks == nil {
			state.Tasks = map[string]*core.Task{}
		}
		existing := cloneTask(state.Tasks[taskID])
		next, err := fn(existing)
		if err != nil {
			return err
		}
		if next == nil {
			delete(state.Tasks, taskID)
			updated = nil
			return s.saveUnlocked(state)
		}
		now := time.Now().UTC()
		if next.TaskID == "" {
			next.TaskID = taskID
		}
		if next.CreatedAt.IsZero() {
			if existing != nil && !existing.CreatedAt.IsZero() {
				next.CreatedAt = existing.CreatedAt
			} else {
				next.CreatedAt = now
			}
		}
		if existing == nil {
			if next.UpdatedAt.IsZero() {
				next.UpdatedAt = now
			}
		} else if taskChanged(existing, next) || next.UpdatedAt.IsZero() {
			next.UpdatedAt = now
		}
		state.Tasks[taskID] = cloneTask(next)
		updated = cloneTask(next)
		return s.saveUnlocked(state)
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

func (s *StateStore) ListTasks() ([]*core.Task, error) {
	state, err := s.load()
	if err != nil {
		return nil, err
	}
	tasks := make([]*core.Task, 0, len(state.Tasks))
	for _, task := range state.Tasks {
		if task != nil {
			tasks = append(tasks, cloneTask(task))
		}
	}
	return tasks, nil
}

func (s *StateStore) Snapshot() (map[string]*core.Task, error) {
	state, err := s.load()
	if err != nil {
		return nil, err
	}
	result := make(map[string]*core.Task, len(state.Tasks))
	for key, task := range state.Tasks {
		result[key] = cloneTask(task)
	}
	return result, nil
}

func (s *StateStore) load() (*stateFile, error) {
	var state *stateFile
	err := withFileLock(s.lockPath(), func() error {
		loaded, err := s.loadUnlocked()
		if err != nil {
			return err
		}
		state = loaded
		return nil
	})
	return state, err
}

func (s *StateStore) loadUnlocked() (*stateFile, error) {
	path := s.statePath()
	payload := &stateFile{Tasks: map[string]*core.Task{}}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return payload, nil
		}
		return nil, fmt.Errorf("读取状态缓存失败: %w", err)
	}
	if len(data) == 0 {
		return payload, nil
	}
	if err := json.Unmarshal(data, payload); err != nil {
		return nil, fmt.Errorf("解析状态缓存失败: %w", err)
	}
	if payload.Tasks == nil {
		payload.Tasks = map[string]*core.Task{}
	}
	return payload, nil
}

func (s *StateStore) saveUnlocked(state *stateFile) error {
	if state == nil {
		state = &stateFile{Tasks: map[string]*core.Task{}}
	}
	if state.Tasks == nil {
		state.Tasks = map[string]*core.Task{}
	}
	if err := os.MkdirAll(s.varDir, 0o755); err != nil {
		return fmt.Errorf("创建状态目录失败: %w", err)
	}
	encoded, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化状态缓存失败: %w", err)
	}
	tmp, err := os.CreateTemp(s.varDir, "state-*.json")
	if err != nil {
		return fmt.Errorf("创建状态临时文件失败: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(encoded); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("写入状态临时文件失败: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("关闭状态临时文件失败: %w", err)
	}
	if err := os.Rename(tmpPath, s.statePath()); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("替换状态缓存失败: %w", err)
	}
	return nil
}

func (s *StateStore) statePath() string {
	return filepath.Join(s.varDir, "state.json")
}

func (s *StateStore) lockPath() string {
	return filepath.Join(s.varDir, ".state.lock")
}

func cloneTask(task *core.Task) *core.Task {
	if task == nil {
		return nil
	}
	copy := *task
	copy.Labels = append([]string(nil), task.Labels...)
	return &copy
}

func taskChanged(existing, next *core.Task) bool {
	if existing == nil || next == nil {
		return existing != next
	}
	left := cloneTask(existing)
	right := cloneTask(next)
	left.UpdatedAt = time.Time{}
	right.UpdatedAt = time.Time{}
	return !reflect.DeepEqual(left, right)
}

func stringsTrimSpace(value string) string {
	return strings.TrimSpace(value)
}

func withFileLock(path string, fn func() error) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("创建锁目录失败: %w", err)
	}
	lockFile, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return fmt.Errorf("打开锁文件失败: %w", err)
	}
	defer lockFile.Close()
	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("获取文件锁失败: %w", err)
	}
	defer func() { _ = syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN) }()
	return fn()
}

func sortTasksByUpdatedDesc(tasks []*core.Task) {
	sort.Slice(tasks, func(i, j int) bool {
		left := tasks[i]
		right := tasks[j]
		if !left.UpdatedAt.Equal(right.UpdatedAt) {
			return left.UpdatedAt.After(right.UpdatedAt)
		}
		return left.IssueNumber > right.IssueNumber
	})
}

func sortTasksByUpdatedAsc(tasks []*core.Task) {
	sort.Slice(tasks, func(i, j int) bool {
		left := tasks[i]
		right := tasks[j]
		if !left.UpdatedAt.Equal(right.UpdatedAt) {
			return left.UpdatedAt.Before(right.UpdatedAt)
		}
		return left.IssueNumber < right.IssueNumber
	})
}

func sortTasksByCreatedAsc(tasks []*core.Task) {
	sort.Slice(tasks, func(i, j int) bool {
		left := tasks[i]
		right := tasks[j]
		if !left.CreatedAt.Equal(right.CreatedAt) {
			return left.CreatedAt.Before(right.CreatedAt)
		}
		return left.IssueNumber < right.IssueNumber
	})
}
