package claude

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	HookEventSessionStart = "SessionStart"
	HookEventPreCompact   = "PreCompact"
	HookEventStop         = "Stop"
)

type HookPayload struct {
	HookEventName string `json:"hook_event_name"`
	SessionID     string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
	Cwd           string `json:"cwd"`
	Matcher       string `json:"matcher"`
	Model         any    `json:"model"`
	ContextWindow *struct {
		ContextWindowSize int `json:"context_window_size"`
	} `json:"context_window,omitempty"`
}

type HookState struct {
	TaskID            string    `json:"task_id"`
	SessionID         string    `json:"session_id,omitempty"`
	TranscriptPath    string    `json:"transcript_path,omitempty"`
	Cwd               string    `json:"cwd,omitempty"`
	Model             string    `json:"model,omitempty"`
	ContextWindowSize int       `json:"context_window_size,omitempty"`
	PreCompactMatcher string    `json:"pre_compact_matcher,omitempty"`
	PreCompactAt      time.Time `json:"pre_compact_at,omitempty"`
	StoppedAt         time.Time `json:"stopped_at,omitempty"`
	UpdatedAt         time.Time `json:"updated_at"`
}

func DefaultHookStateDir(appDir string) string {
	return filepath.Join(strings.TrimSpace(appDir), "var", "claude-hooks")
}

func HookStatePath(stateDir, taskID string) string {
	return filepath.Join(strings.TrimSpace(stateDir), safeTaskID(taskID)+".json")
}

func LoadHookState(stateDir, taskID string) (*HookState, error) {
	path := HookStatePath(stateDir, taskID)
	payload, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("读取 Claude hook 状态失败: %w", err)
	}
	var state HookState
	if err := json.Unmarshal(payload, &state); err != nil {
		return nil, fmt.Errorf("解析 Claude hook 状态失败: %w", err)
	}
	if strings.TrimSpace(state.TaskID) == "" {
		state.TaskID = taskID
	}
	return &state, nil
}

func SaveHookState(stateDir string, state *HookState) error {
	if state == nil {
		return nil
	}
	if err := os.MkdirAll(strings.TrimSpace(stateDir), 0o755); err != nil {
		return fmt.Errorf("创建 Claude hook 状态目录失败: %w", err)
	}
	state.UpdatedAt = time.Now()
	payload, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化 Claude hook 状态失败: %w", err)
	}
	path := HookStatePath(stateDir, state.TaskID)
	tmp, err := os.CreateTemp(strings.TrimSpace(stateDir), "hook-*.json")
	if err != nil {
		return fmt.Errorf("创建 Claude hook 状态临时文件失败: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(payload); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("写入 Claude hook 状态临时文件失败: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("关闭 Claude hook 状态临时文件失败: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("替换 Claude hook 状态文件失败: %w", err)
	}
	return nil
}

func ClearHookState(stateDir, taskID string) error {
	path := HookStatePath(stateDir, taskID)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("清理 Claude hook 状态失败: %w", err)
	}
	return nil
}

func HandleHook(stateDir, taskID string, payload HookPayload) error {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return fmt.Errorf("缺少 CCCLAW_TASK_ID")
	}
	state, err := LoadHookState(stateDir, taskID)
	if err != nil {
		return err
	}
	if state == nil {
		state = &HookState{TaskID: taskID}
	}
	if sessionID := strings.TrimSpace(payload.SessionID); sessionID != "" {
		state.SessionID = sessionID
	}
	if transcriptPath := strings.TrimSpace(payload.TranscriptPath); transcriptPath != "" {
		state.TranscriptPath = transcriptPath
	}
	if cwd := strings.TrimSpace(payload.Cwd); cwd != "" {
		state.Cwd = cwd
	}
	if model := normalizeModel(payload.Model); model != "" {
		state.Model = model
	}
	if payload.ContextWindow != nil && payload.ContextWindow.ContextWindowSize > 0 {
		state.ContextWindowSize = payload.ContextWindow.ContextWindowSize
	}
	switch strings.TrimSpace(payload.HookEventName) {
	case HookEventPreCompact:
		state.PreCompactAt = time.Now()
		state.PreCompactMatcher = strings.TrimSpace(payload.Matcher)
	case HookEventStop:
		state.StoppedAt = time.Now()
	}
	return SaveHookState(stateDir, state)
}

func ManagedHookSettings(ccclawBin string) map[string]any {
	buildGroup := func(name, matcher string) map[string]any {
		group := map[string]any{
			"hooks": []any{
				map[string]any{
					"type":    "command",
					"command": strings.TrimSpace(ccclawBin) + " claude-hook " + name,
				},
			},
		}
		if strings.TrimSpace(matcher) != "" {
			group["matcher"] = strings.TrimSpace(matcher)
		}
		return group
	}
	return map[string]any{
		"hooks": map[string]any{
			HookEventSessionStart: []any{buildGroup("session-start", "")},
			HookEventPreCompact:   []any{buildGroup("pre-compact", "auto")},
			HookEventStop:         []any{buildGroup("stop", "")},
		},
	}
}

func EnsureManagedHookSettings(repoPath, ccclawBin string) error {
	repoPath = strings.TrimSpace(repoPath)
	if repoPath == "" {
		return nil
	}
	settingsDir := filepath.Join(repoPath, ".claude")
	if err := os.MkdirAll(settingsDir, 0o755); err != nil {
		return fmt.Errorf("创建项目 Claude 配置目录失败: %w", err)
	}
	settingsPath := filepath.Join(settingsDir, "settings.local.json")
	current := map[string]any{}
	if payload, err := os.ReadFile(settingsPath); err == nil && len(strings.TrimSpace(string(payload))) > 0 {
		if err := json.Unmarshal(payload, &current); err != nil {
			return fmt.Errorf("解析项目 Claude 本地配置失败: %w", err)
		}
	}
	mergeManagedHooks(current, strings.TrimSpace(ccclawBin))
	encoded, err := json.MarshalIndent(current, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化项目 Claude 本地配置失败: %w", err)
	}
	if err := os.WriteFile(settingsPath, encoded, 0o644); err != nil {
		return fmt.Errorf("写入项目 Claude 本地配置失败: %w", err)
	}
	return nil
}

func mergeManagedHooks(current map[string]any, ccclawBin string) {
	managed := ManagedHookSettings(ccclawBin)
	hooks, _ := current["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
		current["hooks"] = hooks
	}
	managedHooks, _ := managed["hooks"].(map[string]any)
	for eventName, rawEntries := range managedHooks {
		entries, _ := hooks[eventName].([]any)
		filtered := make([]any, 0, len(entries)+1)
		for _, entry := range entries {
			asMap, _ := entry.(map[string]any)
			if cleaned, keep := stripManagedHookEntries(asMap, ccclawBin); keep {
				filtered = append(filtered, cleaned)
				continue
			}
			if len(asMap) == 0 {
				filtered = append(filtered, entry)
			}
		}
		if managedEntries, ok := rawEntries.([]any); ok {
			filtered = append(filtered, managedEntries...)
		}
		hooks[eventName] = filtered
	}
}

func stripManagedHookEntries(entry map[string]any, ccclawBin string) (map[string]any, bool) {
	if len(entry) == 0 {
		return nil, false
	}
	rawHooks, ok := entry["hooks"].([]any)
	if !ok {
		return nil, false
	}
	filteredHooks := make([]any, 0, len(rawHooks))
	for _, rawHook := range rawHooks {
		asMap, _ := rawHook.(map[string]any)
		if isManagedHookEntry(asMap, ccclawBin) {
			continue
		}
		filteredHooks = append(filteredHooks, rawHook)
	}
	if len(filteredHooks) == 0 {
		return nil, false
	}
	cleaned := make(map[string]any, len(entry))
	for key, value := range entry {
		if key == "hooks" {
			cleaned[key] = filteredHooks
			continue
		}
		cleaned[key] = value
	}
	return cleaned, true
}

func isManagedHookEntry(entry map[string]any, ccclawBin string) bool {
	if len(entry) == 0 {
		return false
	}
	command, _ := entry["command"].(string)
	command = strings.TrimSpace(command)
	if command == "" {
		return false
	}
	if strings.Contains(command, strings.TrimSpace(ccclawBin)+" claude-hook ") {
		return true
	}
	return strings.Contains(command, "ccclaw claude-hook ")
}

func normalizeModel(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case map[string]any:
		parts := make([]string, 0, 2)
		if id, _ := typed["id"].(string); strings.TrimSpace(id) != "" {
			parts = append(parts, strings.TrimSpace(id))
		}
		if displayName, _ := typed["display_name"].(string); strings.TrimSpace(displayName) != "" {
			parts = append(parts, strings.TrimSpace(displayName))
		}
		return strings.TrimSpace(strings.Join(parts, " "))
	default:
		return ""
	}
}

func safeTaskID(taskID string) string {
	replacer := strings.NewReplacer("#", "_", "/", "_", ":", "_", ".", "_", " ", "_")
	return replacer.Replace(taskID)
}
