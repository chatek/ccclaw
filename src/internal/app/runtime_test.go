package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/41490/ccclaw/internal/adapters/storage"
	"github.com/41490/ccclaw/internal/config"
	"github.com/41490/ccclaw/internal/core"
	"github.com/41490/ccclaw/internal/executor"
)

func TestParseTargetRepo(t *testing.T) {
	body := "请处理这个问题\n\ntarget_repo: 41490/ccclaw\n"
	if got := parseTargetRepo(body); got != "41490/ccclaw" {
		t.Fatalf("unexpected target repo: %q", got)
	}
}

func TestResolveTargetRepoFallsBackToDefaultTarget(t *testing.T) {
	rt := &Runtime{
		cfg: &config.Config{
			DefaultTarget: "41490/ccclaw",
			Paths:         config.PathsConfig{KBDir: "/opt/ccclaw/kb"},
			Targets: []config.TargetConfig{{
				Repo:      "41490/ccclaw",
				LocalPath: "/opt/src/ccclaw",
			}},
		},
	}
	repo, reasons := rt.resolveTargetRepo("无显式 target")
	if repo != "41490/ccclaw" {
		t.Fatalf("unexpected repo: %q", repo)
	}
	if len(reasons) != 0 {
		t.Fatalf("expected no reasons, got %#v", reasons)
	}
}

func TestResolveTargetRepoBlocksWhenNoTargetConfigured(t *testing.T) {
	rt := &Runtime{
		cfg: &config.Config{
			Paths:   config.PathsConfig{KBDir: "/opt/ccclaw/kb"},
			Targets: nil,
		},
	}
	repo, reasons := rt.resolveTargetRepo("无显式 target")
	if repo != "" {
		t.Fatalf("expected empty repo, got %q", repo)
	}
	if len(reasons) != 1 {
		t.Fatalf("expected 1 reason, got %#v", reasons)
	}
}

func TestSummarizeSchedulerNoneModePasses(t *testing.T) {
	detail, err := summarizeScheduler(schedulerProbe{
		Requested:  "none",
		CronReason: "未检测到受控 crontab 规则",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if detail == "" {
		t.Fatal("expected scheduler detail")
	}
}

func TestSummarizeSchedulerAutoDegradeFails(t *testing.T) {
	detail, err := summarizeScheduler(schedulerProbe{
		Requested:     "auto",
		SystemdReason: "Failed to connect to bus: No medium found",
		CronReason:    "未检测到受控 crontab 规则",
	})
	if err == nil {
		t.Fatal("expected degrade error")
	}
	if detail == "" {
		t.Fatal("expected scheduler detail")
	}
	if want := "reason=当前会话无法连接 user systemd 总线"; !strings.Contains(detail, want) {
		t.Fatalf("expected %q in %q", want, detail)
	}
	if want := "repair=请在用户登录会话中执行 systemctl --user daemon-reload"; !strings.Contains(detail, want) {
		t.Fatalf("expected repair hint %q in %q", want, detail)
	}
}

func TestSummarizeSchedulerSystemdNeedsRepair(t *testing.T) {
	detail, err := summarizeScheduler(schedulerProbe{
		Requested:        "systemd",
		SystemdUserDir:   "/tmp/systemd-user",
		SystemdInstalled: true,
		SystemdReason:    "systemctl --user is-enabled ccclaw-ingest.timer 失败: disabled",
	})
	if err == nil {
		t.Fatal("expected systemd mismatch error")
	}
	if want := "repair=请执行 systemctl --user daemon-reload"; !strings.Contains(detail, want) {
		t.Fatalf("expected repair hint %q in %q", want, detail)
	}
	if want := "reason=已写入 user systemd 单元目录 /tmp/systemd-user，但 timer 尚未启用"; !strings.Contains(detail, want) {
		t.Fatalf("expected systemd reason %q in %q", want, detail)
	}
	if want := "systemd=systemctl --user is-enabled"; !strings.Contains(detail, want) {
		t.Fatalf("expected systemd context %q in %q", want, detail)
	}
}

func TestSummarizeSchedulerCronPasses(t *testing.T) {
	detail, err := summarizeScheduler(schedulerProbe{
		Requested:  "cron",
		CronActive: true,
		CronReason: "已检测到受控 crontab ingest/run 规则",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if want := "effective=cron"; !strings.Contains(detail, want) {
		t.Fatalf("expected %q in %q", want, detail)
	}
}

func TestSummarizeSchedulerDoubleSchedulingFails(t *testing.T) {
	detail, err := summarizeScheduler(schedulerProbe{
		Requested:     "auto",
		SystemdActive: true,
		SystemdReason: "systemd active",
		CronActive:    true,
		CronReason:    "cron active",
	})
	if err == nil {
		t.Fatal("expected duplicate scheduling error")
	}
	if want := "systemd=systemd active"; !strings.Contains(detail, want) {
		t.Fatalf("expected systemd context %q in %q", want, detail)
	}
	if want := "cron=cron active"; !strings.Contains(detail, want) {
		t.Fatalf("expected cron context %q in %q", want, detail)
	}
}

func TestSummarizeSchedulerCronMissingCommand(t *testing.T) {
	detail, err := summarizeScheduler(schedulerProbe{
		Requested:  "cron",
		CronReason: "未找到 crontab: exec: \"crontab\": executable file not found in $PATH",
	})
	if err == nil {
		t.Fatal("expected cron mismatch error")
	}
	if want := "reason=当前环境缺少 crontab，无法使用 cron 调度"; !strings.Contains(detail, want) {
		t.Fatalf("expected cron reason %q in %q", want, detail)
	}
	if want := "repair=当前环境缺少 crontab；请安装 cron/cronie，或改用 systemd/none"; !strings.Contains(detail, want) {
		t.Fatalf("expected cron repair %q in %q", want, detail)
	}
}

func TestStatsRendersSummaryAndTaskTable(t *testing.T) {
	store, err := storage.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("打开 store 失败: %v", err)
	}
	defer store.Close()

	task := &core.Task{
		TaskID:         "10#body",
		IdempotencyKey: "10#body",
		ControlRepo:    "41490/ccclaw",
		TargetRepo:     "41490/ccclaw",
		LastSessionID:  "sess-10",
		IssueNumber:    10,
		IssueTitle:     "phase1 统计命令",
		Labels:         []string{"ccclaw"},
		Intent:         core.IntentResearch,
		RiskLevel:      core.RiskLow,
		State:          core.StateDone,
	}
	if err := store.UpsertTask(task); err != nil {
		t.Fatalf("写入任务失败: %v", err)
	}
	if err := store.RecordTokenUsage(storage.TokenUsageRecord{
		TaskID:    task.TaskID,
		SessionID: "sess-10",
		Usage: core.TokenUsage{
			InputTokens:  30,
			OutputTokens: 12,
		},
		CostUSD:    0.09,
		DurationMS: 5000,
		RecordedAt: time.Date(2026, 3, 10, 15, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("写入 token 使用失败: %v", err)
	}

	rt := &Runtime{store: store}
	var out bytes.Buffer
	if err := rt.Stats(&out); err != nil {
		t.Fatalf("执行 stats 失败: %v", err)
	}
	text := out.String()
	for _, want := range []string{"执行次数: 1", "会话数: 1", "#10", "sess-10", "累计成本(USD): 0.0900"} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected %q in %q", want, text)
		}
	}
}

func TestPatrolFinalizesDeadTMuxSession(t *testing.T) {
	tmpDir := t.TempDir()
	fakeBin := filepath.Join(tmpDir, "bin")
	if err := os.MkdirAll(fakeBin, 0o755); err != nil {
		t.Fatalf("创建 fake bin 失败: %v", err)
	}
	fixtureDir := filepath.Join(tmpDir, "fixture")
	if err := os.MkdirAll(fixtureDir, 0o755); err != nil {
		t.Fatalf("创建 fixture 目录失败: %v", err)
	}
	tmuxScript := filepath.Join(fakeBin, "tmux")
	tmuxBody := `#!/usr/bin/env bash
set -euo pipefail
fixture_dir="${TMUX_FIXTURE_DIR:?}"
if [[ "${1:-}" == "list-panes" ]]; then
  cat "$fixture_dir/list-panes.txt"
  exit 0
fi
if [[ "${1:-}" == "kill-session" ]]; then
  printf '%s\n' "${3:-}" >> "$fixture_dir/killed.txt"
  exit 0
fi
if [[ "${1:-}" == "capture-pane" ]]; then
  cat "$fixture_dir/capture.txt"
  exit 0
fi
exit 0
`
	if err := os.WriteFile(tmuxScript, []byte(tmuxBody), 0o755); err != nil {
		t.Fatalf("写入 fake tmux 失败: %v", err)
	}

	oldPath := os.Getenv("PATH")
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+oldPath)
	t.Setenv("TMUX_FIXTURE_DIR", fixtureDir)

	stateDB := filepath.Join(tmpDir, "state.db")
	store, err := storage.Open(stateDB)
	if err != nil {
		t.Fatalf("打开 store 失败: %v", err)
	}

	task := &core.Task{
		TaskID:         "10#body",
		IdempotencyKey: "10#body",
		ControlRepo:    "41490/ccclaw",
		TargetRepo:     "41490/ccclaw",
		IssueNumber:    10,
		IssueTitle:     "tmux patrol",
		Labels:         []string{"ccclaw"},
		Intent:         core.IntentResearch,
		RiskLevel:      core.RiskLow,
		State:          core.StateRunning,
	}
	if err := store.UpsertTask(task); err != nil {
		t.Fatalf("写入任务失败: %v", err)
	}

	sessionName := executor.SessionName(task.TaskID)
	if err := os.WriteFile(filepath.Join(fixtureDir, "list-panes.txt"), []byte(sessionName+"\t"+strconv.FormatInt(time.Now().Add(-time.Minute).Unix(), 10)+"\t1\t0\t123\n"), 0o644); err != nil {
		t.Fatalf("写入 list-panes fixture 失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(fixtureDir, "capture.txt"), []byte(""), 0o644); err != nil {
		t.Fatalf("写入 capture fixture 失败: %v", err)
	}

	resultDir := filepath.Join(tmpDir, "app", "var", "results")
	if err := os.MkdirAll(resultDir, 0o755); err != nil {
		t.Fatalf("创建结果目录失败: %v", err)
	}
	resultPath := filepath.Join(resultDir, "10_body.json")
	resultJSON := `{"type":"result","subtype":"success","session_id":"sess-10","duration_ms":2000,"result":"任务完成","total_cost_usd":0.15,"usage":{"input_tokens":50,"output_tokens":20}}`
	if err := os.WriteFile(resultPath, []byte(resultJSON), 0o644); err != nil {
		t.Fatalf("写入结果文件失败: %v", err)
	}

	logDir := filepath.Join(tmpDir, "app", "log")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatalf("创建日志目录失败: %v", err)
	}
	rt := &Runtime{
		cfg: &config.Config{
			Paths: config.PathsConfig{
				AppDir:  filepath.Join(tmpDir, "app"),
				LogDir:  logDir,
				StateDB: stateDB,
				KBDir:   "/opt/ccclaw/kb",
			},
			Executor: config.ExecutorConfig{
				Command: []string{"/bin/sh"},
				Timeout: "30m",
			},
		},
		secrets: &config.Secrets{Values: map[string]string{}},
		store:   store,
	}

	var out bytes.Buffer
	if err := rt.Patrol(context.Background(), &out); err != nil {
		t.Fatalf("执行 patrol 失败: %v", err)
	}
	if !strings.Contains(out.String(), "completed=1") {
		t.Fatalf("unexpected patrol output: %q", out.String())
	}

	store, err = storage.Open(stateDB)
	if err != nil {
		t.Fatalf("重新打开 store 失败: %v", err)
	}
	defer store.Close()
	loaded, err := store.GetByIdempotency(task.IdempotencyKey)
	if err != nil {
		t.Fatalf("读取任务失败: %v", err)
	}
	if loaded == nil || loaded.State != core.StateDone || loaded.LastSessionID != "sess-10" {
		t.Fatalf("unexpected task after patrol: %#v", loaded)
	}
	summary, err := store.TokenStats()
	if err != nil {
		t.Fatalf("读取 token 汇总失败: %v", err)
	}
	if summary.Runs != 1 || summary.InputTokens != 50 {
		t.Fatalf("unexpected summary after patrol: %#v", summary)
	}
}
