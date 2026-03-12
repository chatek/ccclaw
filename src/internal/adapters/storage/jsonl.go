package storage

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/41490/ccclaw/internal/core"
)

const timeLayout = time.RFC3339Nano

type tokenJSONRecord struct {
	Seq          int64     `json:"seq"`
	TaskID       string    `json:"task_id"`
	SessionID    string    `json:"session_id"`
	InputTokens  int       `json:"input_tokens"`
	OutputTokens int       `json:"output_tokens"`
	CacheCreate  int       `json:"cache_create"`
	CacheRead    int       `json:"cache_read"`
	CostUSD      float64   `json:"cost_usd"`
	DurationMS   int64     `json:"duration_ms"`
	RTKEnabled   bool      `json:"rtk_enabled"`
	PromptFile   string    `json:"prompt_file,omitempty"`
	RecordedAt   time.Time `json:"ts"`
	PrevHash     string    `json:"prev_hash"`
	Hash         string    `json:"hash"`
}

type JSONLStore struct {
	varDir string
}

func newJSONLStore(varDir string) *JSONLStore {
	return &JSONLStore{varDir: varDir}
}

func (s *JSONLStore) AppendEvent(taskID string, eventType core.EventType, detail string) error {
	return s.AppendEventAt(taskID, eventType, detail, time.Time{})
}

func (s *JSONLStore) AppendEventAt(taskID string, eventType core.EventType, detail string, createdAt time.Time) error {
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	year, week := createdAt.ISOWeek()
	path := s.eventFile(year, week)
	lockPath := path + ".lock"
	return withFileLock(lockPath, func() error {
		lastSeq, lastHash, err := lastJSONLState[EventRecord](path)
		if err != nil {
			return err
		}
		prevHash := lastHash
		if prevHash == "" {
			prevHash = GenesisHash(year, week)
		}
		record := EventRecord{
			Seq:       lastSeq + 1,
			TaskID:    taskID,
			EventType: eventType,
			Detail:    detail,
			CreatedAt: createdAt.UTC(),
			PrevHash:  prevHash,
		}
		record.Hash, err = ComputeHash(taskID, string(eventType), record.CreatedAt.UTC().Format(timeLayout), detail, prevHash)
		if err != nil {
			return err
		}
		return appendJSONLine(path, record)
	})
}

func (s *JSONLStore) AppendToken(record TokenUsageRecord) error {
	recordedAt := record.RecordedAt
	if recordedAt.IsZero() {
		recordedAt = time.Now().UTC()
	}
	year, week := recordedAt.ISOWeek()
	path := s.tokenFile(year, week)
	lockPath := path + ".lock"
	return withFileLock(lockPath, func() error {
		lastSeq, lastHash, err := lastJSONLState[tokenJSONRecord](path)
		if err != nil {
			return err
		}
		prevHash := lastHash
		if prevHash == "" {
			prevHash = GenesisHash(year, week)
		}
		jsonRecord := tokenJSONRecord{
			Seq:          lastSeq + 1,
			TaskID:       record.TaskID,
			SessionID:    record.SessionID,
			InputTokens:  record.Usage.InputTokens,
			OutputTokens: record.Usage.OutputTokens,
			CacheCreate:  record.Usage.CacheCreationInputTokens,
			CacheRead:    record.Usage.CacheReadInputTokens,
			CostUSD:      record.CostUSD,
			DurationMS:   record.DurationMS,
			RTKEnabled:   record.RTKEnabled,
			PromptFile:   record.PromptFile,
			RecordedAt:   recordedAt.UTC(),
			PrevHash:     prevHash,
		}
		jsonRecord.Hash, err = ComputeTokenHash(jsonRecord, prevHash)
		if err != nil {
			return err
		}
		return appendJSONLine(path, jsonRecord)
	})
}

func (s *JSONLStore) ReadEvents() ([]EventRecord, error) {
	paths, err := filepath.Glob(filepath.Join(s.varDir, "events-*.jsonl"))
	if err != nil {
		return nil, fmt.Errorf("枚举事件流失败: %w", err)
	}
	sort.Strings(paths)
	records := make([]EventRecord, 0)
	for _, path := range paths {
		items, err := readJSONL[EventRecord](path)
		if err != nil {
			return nil, err
		}
		records = append(records, items...)
	}
	return records, nil
}

func (s *JSONLStore) ReadTokens() ([]tokenJSONRecord, error) {
	paths, err := filepath.Glob(filepath.Join(s.varDir, "token-*.jsonl"))
	if err != nil {
		return nil, fmt.Errorf("枚举 token 流失败: %w", err)
	}
	sort.Strings(paths)
	records := make([]tokenJSONRecord, 0)
	for _, path := range paths {
		items, err := readJSONL[tokenJSONRecord](path)
		if err != nil {
			return nil, err
		}
		records = append(records, items...)
	}
	return records, nil
}

func (s *JSONLStore) eventFile(year, week int) string {
	return filepath.Join(s.varDir, fmt.Sprintf("events-%d-W%02d.jsonl", year, week))
}

func (s *JSONLStore) tokenFile(year, week int) string {
	return filepath.Join(s.varDir, fmt.Sprintf("token-%d-W%02d.jsonl", year, week))
}

func appendJSONLine(path string, payload any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("创建 JSONL 目录失败: %w", err)
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("序列化 JSONL 记录失败: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("打开 JSONL 文件失败: %w", err)
	}
	defer file.Close()
	line := append(encoded, '\n')
	if _, err := file.Write(line); err != nil {
		return fmt.Errorf("写入 JSONL 文件失败: %w", err)
	}
	return nil
}

func readJSONL[T any](path string) ([]T, error) {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("打开 JSONL 文件失败: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	items := make([]T, 0)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var item T
		if err := json.Unmarshal([]byte(line), &item); err != nil {
			return nil, fmt.Errorf("解析 JSONL 记录失败: %w", err)
		}
		items = append(items, item)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("读取 JSONL 文件失败: %w", err)
	}
	return items, nil
}

func lastJSONLState[T interface{ GetSeq() int64; GetHash() string }](path string) (int64, string, error) {
	items, err := readJSONL[T](path)
	if err != nil {
		return 0, "", err
	}
	if len(items) == 0 {
		return 0, "", nil
	}
	last := items[len(items)-1]
	return last.GetSeq(), last.GetHash(), nil
}
