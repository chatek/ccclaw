package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestManagedHookSettingsUsesOfficialNestedShape(t *testing.T) {
	settings := ManagedHookSettings("/opt/ccclaw/bin/ccclaw")
	hooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		t.Fatalf("expected hooks map, got %#v", settings["hooks"])
	}
	sessionEntries, ok := hooks[HookEventSessionStart].([]any)
	if !ok || len(sessionEntries) != 1 {
		t.Fatalf("expected session start entries, got %#v", hooks[HookEventSessionStart])
	}
	entry, ok := sessionEntries[0].(map[string]any)
	if !ok {
		t.Fatalf("expected entry map, got %#v", sessionEntries[0])
	}
	if _, exists := entry["matcher"]; exists {
		t.Fatalf("session start should not force matcher: %#v", entry)
	}
	rawHooks, ok := entry["hooks"].([]any)
	if !ok || len(rawHooks) != 1 {
		t.Fatalf("expected nested hooks array, got %#v", entry["hooks"])
	}
	commandHook, ok := rawHooks[0].(map[string]any)
	if !ok {
		t.Fatalf("expected nested command hook, got %#v", rawHooks[0])
	}
	if commandHook["type"] != "command" {
		t.Fatalf("expected command type, got %#v", commandHook["type"])
	}
	if commandHook["command"] != "/opt/ccclaw/bin/ccclaw claude-hook session-start" {
		t.Fatalf("unexpected command hook: %#v", commandHook)
	}

	preCompactEntries, ok := hooks[HookEventPreCompact].([]any)
	if !ok || len(preCompactEntries) != 1 {
		t.Fatalf("expected pre-compact entries, got %#v", hooks[HookEventPreCompact])
	}
	preCompactEntry, ok := preCompactEntries[0].(map[string]any)
	if !ok || preCompactEntry["matcher"] != "auto" {
		t.Fatalf("expected pre-compact matcher auto, got %#v", preCompactEntries[0])
	}
}

func TestEnsureManagedHookSettingsPreservesCustomHooksAndReplacesManagedOnes(t *testing.T) {
	repoDir := t.TempDir()
	settingsDir := filepath.Join(repoDir, ".claude")
	if err := os.MkdirAll(settingsDir, 0o755); err != nil {
		t.Fatalf("创建 .claude 目录失败: %v", err)
	}
	current := map[string]any{
		"hooks": map[string]any{
			HookEventSessionStart: []any{
				map[string]any{
					"hooks": []any{
						map[string]any{
							"type":    "command",
							"command": "/custom/bin/session-start",
						},
						map[string]any{
							"type":    "command",
							"command": "/old/bin/ccclaw claude-hook session-start",
						},
					},
				},
			},
		},
	}
	payload, err := json.Marshal(current)
	if err != nil {
		t.Fatalf("序列化当前配置失败: %v", err)
	}
	settingsPath := filepath.Join(settingsDir, "settings.local.json")
	if err := os.WriteFile(settingsPath, payload, 0o644); err != nil {
		t.Fatalf("写入 settings.local.json 失败: %v", err)
	}

	if err := EnsureManagedHookSettings(repoDir, "/opt/ccclaw/bin/ccclaw"); err != nil {
		t.Fatalf("EnsureManagedHookSettings 失败: %v", err)
	}

	updatedPayload, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("读取更新后的 settings.local.json 失败: %v", err)
	}
	updated := map[string]any{}
	if err := json.Unmarshal(updatedPayload, &updated); err != nil {
		t.Fatalf("解析更新后的 settings.local.json 失败: %v", err)
	}
	hooks := updated["hooks"].(map[string]any)
	sessionEntries := hooks[HookEventSessionStart].([]any)
	if len(sessionEntries) != 2 {
		t.Fatalf("expected preserved custom group and managed group, got %#v", sessionEntries)
	}
	firstGroup := sessionEntries[0].(map[string]any)
	firstHooks := firstGroup["hooks"].([]any)
	if len(firstHooks) != 1 {
		t.Fatalf("expected old managed hook to be stripped, got %#v", firstHooks)
	}
	firstHook := firstHooks[0].(map[string]any)
	if firstHook["command"] != "/custom/bin/session-start" {
		t.Fatalf("expected custom hook to remain, got %#v", firstHook)
	}
	lastGroup := sessionEntries[1].(map[string]any)
	lastHooks := lastGroup["hooks"].([]any)
	lastHook := lastHooks[0].(map[string]any)
	if lastHook["command"] != "/opt/ccclaw/bin/ccclaw claude-hook session-start" {
		t.Fatalf("expected new managed hook to be appended, got %#v", lastHook)
	}
}
