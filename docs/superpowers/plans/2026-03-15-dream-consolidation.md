# DreamCleaner v2 — dream 记忆整合命令实施计划

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 新增 `ccclaw dream` 子命令，每日 02:42 自动读取近期 journal/assay 内容，调用 Claude executor 提炼 skill 草稿到 `kb/stuff/skills/`，并创建 GitHub Issue 等待人工审核批准后安装。

**Architecture:** `ccclaw dream` 是纯 Go 子命令（同 sevolver 模式），读取 `kb/skills/L1/dream-consolidation.md` 作为提示词，追加近期 journal 内容，直调 Claude executor（`ccclaude` wrapper），由 Claude 负责写入草稿文件和创建 review Issue。调度通过新增 `ccclaw-dream.timer`（02:42 Asia/Shanghai）实现。

**Tech Stack:** Go 1.25 (module `github.com/41490/ccclaw`), Cobra CLI, systemd oneshot timer, GitHub CLI (`gh`)，`ccclaude` wrapper。

---

## Chunk 1: Config 层与 Scheduler 单元生成

### 改动范围

- Modify: `src/internal/config/config.go:76-81` — `SchedulerTimersConfig` 新增 `Sevolver`/`Archive`/`Dream` 字段
- Modify: `src/internal/scheduler/systemd_units.go:22-86` — 常量改为可配置，新增 dream 定义
- Modify: `src/internal/scheduler/systemd_test.go:46-60` — 补充 dream timer 测试
- Modify: `src/dist/ops/config/config.example.toml:49-52` — 补全缺失 timer 条目

---

- [ ] **Step 1.1: 为 SchedulerTimersConfig 增加三个字段**

修改 `src/internal/config/config.go` 中的 `SchedulerTimersConfig` struct：

```go
type SchedulerTimersConfig struct {
	Ingest   string `mapstructure:"ingest"   toml:"ingest"`
	Run      string `mapstructure:"run"      toml:"run"`
	Patrol   string `mapstructure:"patrol"   toml:"patrol"`
	Journal  string `mapstructure:"journal"  toml:"journal"`
	Sevolver string `mapstructure:"sevolver" toml:"sevolver"`
	Archive  string `mapstructure:"archive"  toml:"archive"`
	Dream    string `mapstructure:"dream"    toml:"dream"`
}
```

- [ ] **Step 1.2: 在 systemd_units.go 中用配置值覆盖默认常量**

修改 `src/internal/scheduler/systemd_units.go`，执行以下三步：

**① 将旧常量块替换（重命名 + 新增 dream）：**

旧：
```go
const (
	archiveCalendar  = "Mon 02:00:00"
	sevolverCalendar = "*-*-* 22:00:00"
)
```
新：
```go
const (
	defaultArchiveCalendar  = "Mon 02:00:00"
	defaultSevolverCalendar = "*-*-* 22:00:00"
	defaultDreamCalendar    = "*-*-* 02:42:00"
)
```

**② 在常量块下方插入 `resolveCalendar` 函数：**

```go
func resolveCalendar(configured, defaultVal string) string {
	if strings.TrimSpace(configured) != "" {
		return configured
	}
	return defaultVal
}
```

**③ 替换整个 `ManagedTimerDefinitions` 函数（完整代码）：**

```go
func ManagedTimerDefinitions(cfg *config.Config) ([]TimerDefinition, error) {
	if cfg == nil {
		return nil, fmt.Errorf("配置不能为空")
	}
	location, err := schedulerLocation(cfg)
	if err != nil {
		return nil, err
	}
	timers := cfg.Scheduler.Timers
	defs := []TimerDefinition{
		{
			Key:            "ingest",
			TimerUnit:      "ccclaw-ingest.timer",
			ServiceUnit:    "ccclaw-ingest.service",
			ServiceCommand: "ingest",
			ServiceDesc:    "ccclaw ingest service",
			TimerDesc:      "Run ccclaw ingest on schedule",
			Calendar:       timers.Ingest,
		},
		{
			Key:            "patrol",
			TimerUnit:      "ccclaw-patrol.timer",
			ServiceUnit:    "ccclaw-patrol.service",
			ServiceCommand: "patrol",
			ServiceDesc:    "ccclaw patrol service",
			TimerDesc:      "Run ccclaw patrol on schedule",
			Calendar:       timers.Patrol,
		},
		{
			Key:            "journal",
			TimerUnit:      "ccclaw-journal.timer",
			ServiceUnit:    "ccclaw-journal.service",
			ServiceCommand: "journal",
			ServiceDesc:    "ccclaw journal service",
			TimerDesc:      "Run ccclaw journal on schedule",
			Calendar:       timers.Journal,
		},
		{
			Key:            "archive",
			TimerUnit:      "ccclaw-archive.timer",
			ServiceUnit:    "ccclaw-archive.service",
			ServiceCommand: "archive",
			ServiceDesc:    "ccclaw archive service",
			TimerDesc:      "Run ccclaw weekly archive on schedule",
			Calendar:       resolveCalendar(timers.Archive, defaultArchiveCalendar),
		},
		{
			Key:            "sevolver",
			TimerUnit:      "ccclaw-sevolver.timer",
			ServiceUnit:    "ccclaw-sevolver.service",
			ServiceCommand: "sevolver",
			ServiceDesc:    "ccclaw sevolver service",
			TimerDesc:      "Run ccclaw daily sevolver on schedule",
			Calendar:       resolveCalendar(timers.Sevolver, defaultSevolverCalendar),
		},
		{
			Key:            "dream",
			TimerUnit:      "ccclaw-dream.timer",
			ServiceUnit:    "ccclaw-dream.service",
			ServiceCommand: "dream",
			ServiceDesc:    "ccclaw dream service",
			TimerDesc:      "Run ccclaw daily dream consolidation",
			Calendar:       resolveCalendar(timers.Dream, defaultDreamCalendar),
		},
	}
	for idx := range defs {
		defs[idx].CalendarWithTZ = calendarWithTimezone(defs[idx].Calendar, location.String())
	}
	return defs, nil
}
```

- [ ] **Step 1.3: 先写测试，再运行确认失败**

在 `src/internal/scheduler/systemd_test.go` 中做两处修改：

**① 在 `testSystemdConfig()` 的 `SchedulerTimersConfig` 中追加 `Dream` 字段**（此时 Step 1.1 已添加该字段到 struct）：

```go
Timers: config.SchedulerTimersConfig{
    Ingest:  "*:0/5",
    Run:     "*:0/10",
    Patrol:  "*:0/2",
    Journal: "*-*-* 01:01:42",
    Dream:   "*-*-* 02:42:00",   // ← 新增，测试可配置覆盖路径
},
```

**② 在 `TestGenerateSystemdUnitContentsIncludesCalendarTimezone` 末尾追加 dream 断言：**

```go
if !strings.Contains(units["ccclaw-dream.timer"], "OnCalendar=*-*-* 02:42:00 Asia/Shanghai") {
    t.Fatalf("unexpected dream timer unit: %q", units["ccclaw-dream.timer"])
}
```

运行：

```bash
cd src && go test ./internal/scheduler/... -run TestGenerateSystemdUnitContents -v
```

预期：FAIL — `ccclaw-dream.timer` key 不存在（此时 Step 1.2 尚未实施）。

- [ ] **Step 1.4: 执行 Step 1.2 的代码修改，再运行测试**

```bash
cd src && go test ./internal/scheduler/... -run TestGenerateSystemdUnitContents -v
```

预期：PASS。

- [ ] **Step 1.5: 运行全量 scheduler 测试确认无回归**

```bash
cd src && go test ./internal/scheduler/... -v
```

预期：全部 PASS。

- [ ] **Step 1.6: 补全 config.example.toml**

在 `src/dist/ops/config/config.example.toml` 中，找到已有的 `[scheduler.timers]` 段（其中已有 `ingest`/`patrol`/`journal` 三行），在该段末尾追加以下**三行新条目**（不要重复已有条目，避免 TOML 重复 key 错误）：

```toml
sevolver = "*-*-* 22:00:00"   # 每日 22:00 Asia/Shanghai（默认值）
archive  = "Mon 02:00:00"     # 每周一 02:00 Asia/Shanghai（默认值）
dream    = "*-*-* 02:42:00"   # 每日 02:42 Asia/Shanghai，journal 结束后约 3h
```

验证无重复 key：
```bash
grep -n 'sevolver\|archive\|dream' src/dist/ops/config/config.example.toml
```
预期：每个 key 各出现一次。

- [ ] **Step 1.7: 确认 config.toml Load 能解析新字段**

在 `src/internal/config/config_test.go` 中新增测试：

```go
func TestSchedulerTimersIncludesDreamField(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	content := `[github]
control_repo = "41490/ccclaw"

[paths]
app_dir    = "~/.ccclaw"
home_repo  = "/opt/ccclaw"
var_dir    = "~/.ccclaw/var"
log_dir    = "~/.ccclaw/log"
kb_dir     = "/opt/ccclaw/kb"
env_file   = "~/.ccclaw/.env"

[executor]
provider = "claude-code"
command  = ["~/.ccclaw/bin/ccclaude"]
timeout  = "30m"

[approval]
words        = ["approve"]
reject_words = ["reject"]
minimum_permission = "maintain"

[scheduler.timers]
ingest   = "*:0/4"
sevolver = "*-*-* 21:00:00"
dream    = "*-*-* 03:00:00"
`
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Scheduler.Timers.Sevolver != "*-*-* 21:00:00" {
		t.Fatalf("expected sevolver timer, got %q", cfg.Scheduler.Timers.Sevolver)
	}
	if cfg.Scheduler.Timers.Dream != "*-*-* 03:00:00" {
		t.Fatalf("expected dream timer, got %q", cfg.Scheduler.Timers.Dream)
	}
}
```

运行：

```bash
cd src && go test ./internal/config/... -run TestSchedulerTimers -v
```

预期：PASS（新字段正确解析）。

- [ ] **Step 1.8: 在 install.sh 中追加 dream timer 到所有 timer 列表**

`src/dist/install.sh` 中共有 **4 处** timer 列表需要更新，均在 `ccclaw-sevolver.timer` 后追加 `ccclaw-dream.timer`：

**位置 1 — 约 1834 行，simulate enable 日志行：**

旧：
```bash
log "[simulate] systemctl --user enable --now ccclaw-ingest.timer ccclaw-patrol.timer ccclaw-journal.timer ccclaw-archive.timer ccclaw-sevolver.timer"
```
新：
```bash
log "[simulate] systemctl --user enable --now ccclaw-ingest.timer ccclaw-patrol.timer ccclaw-journal.timer ccclaw-archive.timer ccclaw-sevolver.timer ccclaw-dream.timer"
```

**位置 2 — 约 1836 行，simulate restart 日志行：**

旧：
```bash
log "[simulate] systemctl --user restart ccclaw-ingest.timer ccclaw-patrol.timer ccclaw-journal.timer ccclaw-archive.timer ccclaw-sevolver.timer"
```
新：
```bash
log "[simulate] systemctl --user restart ccclaw-ingest.timer ccclaw-patrol.timer ccclaw-journal.timer ccclaw-archive.timer ccclaw-sevolver.timer ccclaw-dream.timer"
```

**位置 3 — 约 1841/1843 行，实际 enable/restart 命令：**

旧（1841）：
```bash
systemctl --user enable --now ccclaw-ingest.timer ccclaw-patrol.timer ccclaw-journal.timer ccclaw-archive.timer ccclaw-sevolver.timer
```
新：
```bash
systemctl --user enable --now ccclaw-ingest.timer ccclaw-patrol.timer ccclaw-journal.timer ccclaw-archive.timer ccclaw-sevolver.timer ccclaw-dream.timer
```

旧（1843）：
```bash
systemctl --user restart ccclaw-ingest.timer ccclaw-patrol.timer ccclaw-journal.timer ccclaw-archive.timer ccclaw-sevolver.timer
```
新：
```bash
systemctl --user restart ccclaw-ingest.timer ccclaw-patrol.timer ccclaw-journal.timer ccclaw-archive.timer ccclaw-sevolver.timer ccclaw-dream.timer
```

**位置 4 — 约 2238/2250 行，手工提示字符串（两处相同替换）：**

旧：
```bash
systemctl --user enable --now ccclaw-ingest.timer ccclaw-patrol.timer ccclaw-journal.timer ccclaw-archive.timer ccclaw-sevolver.timer
```
新：
```bash
systemctl --user enable --now ccclaw-ingest.timer ccclaw-patrol.timer ccclaw-journal.timer ccclaw-archive.timer ccclaw-sevolver.timer ccclaw-dream.timer
```

验证所有位置已更新：
```bash
grep -c 'ccclaw-dream.timer' src/dist/install.sh
```
预期：输出 `6`（四处实际命令 + 两处手工提示）。

- [ ] **Step 1.9: Commit Chunk 1**

```bash
cd /opt/src/ccclaw
git add src/internal/config/config.go \
        src/internal/scheduler/systemd_units.go \
        src/internal/scheduler/systemd_test.go \
        src/internal/config/config_test.go \
        src/dist/ops/config/config.example.toml \
        src/dist/install.sh
git commit -m "feat(#70): 补全 SchedulerTimersConfig sevolver/archive/dream 字段，新增 dream timer 定义"
```

---

## Chunk 2: dream 内部包

### 改动范围

- Create: `src/internal/dream/dream.go` — 读取 journal delta + 调 executor 的核心逻辑
- Create: `src/internal/dream/dream_test.go` — 单元测试

---

- [ ] **Step 2.1: 先写 dream 包的接口测试（TDD）**

创建 `src/internal/dream/dream_test.go`：

```go
package dream

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCollectJournalDelta_ReturnsFilesNewerThanCutoff(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "2026-03-10.md")
	recent := filepath.Join(dir, "2026-03-14.md")
	if err := os.WriteFile(old, []byte("old entry"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(recent, []byte("recent entry"), 0o644); err != nil {
		t.Fatal(err)
	}
	// 让 old 文件的 mtime 比 recent 旧
	cutoff := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(old, cutoff.Add(-24*time.Hour), cutoff.Add(-24*time.Hour)); err != nil {
		t.Fatal(err)
	}

	files, err := collectJournalDelta(dir, cutoff)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d: %v", len(files), files)
	}
	if filepath.Base(files[0].Path) != "2026-03-14.md" {
		t.Fatalf("unexpected file: %v", files[0].Path)
	}
}

func TestBuildPrompt_InjectsJournalContent(t *testing.T) {
	skillContent := "你是整合 Agent。\n\n## Journal\n\n{{JOURNAL_CONTENT}}"
	entries := []journalEntry{
		{Path: "kb/journal/2026-03-14.md", Content: "今天解决了 cron 问题"},
	}
	prompt := buildPrompt(skillContent, entries)
	if !strings.Contains(prompt, "今天解决了 cron 问题") {
		t.Fatalf("prompt missing journal content: %q", prompt)
	}
}
```

运行（预期 FAIL — 包不存在）：

```bash
cd src && go test ./internal/dream/... -v 2>&1 | head -10
```

- [ ] **Step 2.2: 创建 dream.go 实现骨架**

创建 `src/internal/dream/dream.go`：

```go
package dream

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Config 是 dream 命令的运行配置
type Config struct {
	KBDir       string        // kb 根目录（含 skills/L1/、stuff/skills/、journal/）
	JournalDir  string        // 覆盖默认 KBDir/journal/
	AssayDir    string        // 覆盖默认 KBDir/assay/
	StuffDir    string        // 覆盖默认 KBDir/stuff/skills/
	PromptFile  string        // dream-consolidation.md 路径
	ExecutorCmd []string      // executor 命令（如 ["/home/user/.ccclaw/bin/ccclaude"]）
	Timeout     time.Duration // executor 超时
	DeltaDays   int           // 扫描最近 N 天的 journal（默认 1）
	Now         time.Time
}

type journalEntry struct {
	Path    string
	Content string
}

// Run 执行一次 dream 整合
func Run(cfg Config, out io.Writer) error {
	now := cfg.Now
	if now.IsZero() {
		now = time.Now()
	}
	deltaDays := cfg.DeltaDays
	if deltaDays <= 0 {
		deltaDays = 1
	}

	journalDir := cfg.JournalDir
	if journalDir == "" {
		journalDir = filepath.Join(cfg.KBDir, "journal")
	}
	assayDir := cfg.AssayDir
	if assayDir == "" {
		assayDir = filepath.Join(cfg.KBDir, "assay")
	}
	stuffDir := cfg.StuffDir
	if stuffDir == "" {
		stuffDir = filepath.Join(cfg.KBDir, "stuff", "skills")
	}
	promptFile := cfg.PromptFile
	if promptFile == "" {
		promptFile = filepath.Join(cfg.KBDir, "skills", "L1", "dream-consolidation.md")
	}

	// 1. 读取 dream 提示词
	promptTemplate, err := os.ReadFile(promptFile)
	if err != nil {
		return fmt.Errorf("读取 dream 提示词失败 (%s): %w", promptFile, err)
	}

	// 2. 收集近期 journal + assay delta
	cutoff := now.Add(-time.Duration(deltaDays) * 24 * time.Hour)
	journalEntries, err := collectJournalDelta(journalDir, cutoff)
	if err != nil {
		return fmt.Errorf("扫描 journal delta 失败: %w", err)
	}
	assayEntries, err := collectJournalDelta(assayDir, cutoff)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("扫描 assay delta 失败: %w", err)
	}
	allEntries := append(journalEntries, assayEntries...)

	if len(allEntries) == 0 {
		_, _ = fmt.Fprintln(out, "dream: 无近期 journal/assay 变更，跳过本次整合")
		return nil
	}
	_, _ = fmt.Fprintf(out, "dream: 发现 %d 个近期条目，开始整合...\n", len(allEntries))

	// 3. 构建完整提示词（替换 {{JOURNAL_CONTENT}} 和 {{KB_DIR}}）
	prompt := buildPrompt(string(promptTemplate), allEntries)
	prompt = strings.ReplaceAll(prompt, "{{KB_DIR}}", cfg.KBDir)

	// 4. 确保 stuff/skills/ 目录存在
	if err := os.MkdirAll(stuffDir, 0o755); err != nil {
		return fmt.Errorf("创建 stuff/skills/ 目录失败: %w", err)
	}

	// 5. 调用 executor
	if len(cfg.ExecutorCmd) == 0 {
		return fmt.Errorf("executor 命令未配置")
	}
	return runExecutor(cfg.ExecutorCmd, prompt, cfg.Timeout, out)
}

// collectJournalDelta 返回 dir 下修改时间晚于 cutoff 的 .md 文件
func collectJournalDelta(dir string, cutoff time.Time) ([]journalEntry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var result []journalEntry
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			continue
		}
		fullPath := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}
		result = append(result, journalEntry{Path: fullPath, Content: string(data)})
	}
	return result, nil
}

// buildPrompt 将 journal entries 注入提示词模板
// 模板中 {{JOURNAL_CONTENT}} 会被替换为拼接后的 journal 内容
func buildPrompt(template string, entries []journalEntry) string {
	var sb strings.Builder
	for _, e := range entries {
		sb.WriteString("\n\n### ")
		sb.WriteString(filepath.Base(e.Path))
		sb.WriteString("\n\n")
		sb.WriteString(e.Content)
	}
	journalBlock := sb.String()
	if strings.Contains(template, "{{JOURNAL_CONTENT}}") {
		return strings.ReplaceAll(template, "{{JOURNAL_CONTENT}}", journalBlock)
	}
	// fallback: 直接追加到模板尾部
	return template + "\n\n## 近期 Journal/Assay 内容\n" + journalBlock
}
```

- [ ] **Step 2.3: 在 dream.go 末尾追加 executor 调用封装**

在 `src/internal/dream/dream.go` 文件末尾追加（所需 import 已在 Step 2.2 的 import 块中）：

```go
// runExecutor 用 -p flag 调用 executor，传入 prompt（OS 参数长度上限约 2MB；正常 journal delta 远小于此限）
func runExecutor(cmd []string, prompt string, timeout time.Duration, out io.Writer) error {
	if timeout <= 0 {
		timeout = 30 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	args := append(cmd[1:], "-p", prompt, "--output-format", "stream-json")
	c := exec.CommandContext(ctx, cmd[0], args...)
	c.Stdout = out
	c.Stderr = out
	if err := c.Run(); err != nil {
		return fmt.Errorf("executor 执行失败: %w", err)
	}
	return nil
}
```

- [ ] **Step 2.4: 运行测试，确认 PASS**

```bash
cd src && go test ./internal/dream/... -v
```

预期：PASS（两个测试用例通过）。

- [ ] **Step 2.5: 运行全量测试确认无回归**

```bash
cd src && go test ./... 2>&1 | tail -20
```

预期：全部 PASS 或仅已知失败项。

- [ ] **Step 2.6: Commit Chunk 2**

```bash
cd /opt/src/ccclaw
git add src/internal/dream/
git commit -m "feat(#70): 新增 internal/dream 包，实现 journal delta 收集与 executor 调用"
```

---

## Chunk 3: dream CLI 命令

### 改动范围

- Create: `src/cmd/ccclaw/dream.go` — Cobra 子命令注册
- Modify: `src/cmd/ccclaw/main.go` — 注册 dream 命令（找到其他命令注册行，仿照追加）

---

- [ ] **Step 3.1: 创建 dream.go CLI 命令**

创建 `src/cmd/ccclaw/dream.go`，参照 `sevolver.go` 结构：

```go
package main

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/41490/ccclaw/internal/config"
	"github.com/41490/ccclaw/internal/dream"
	"github.com/spf13/cobra"
)

func addDreamCommand(rootCmd *cobra.Command, configPath, envFile *string) {
	var deltaDays int
	cmd := &cobra.Command{
		Use:   "dream",
		Short: "执行一次 kb 记忆整合，提炼 skill 草稿到 kb/stuff/skills/",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(*configPath)
			if err != nil {
				return err
			}
			_ = envFile // 保留签名一致性

			journalDir := filepath.Join(cfg.Paths.HomeRepo, "kb", "journal")
			if _, err := os.Stat(journalDir); err != nil {
				journalDir = filepath.Join(cfg.Paths.KBDir, "journal")
			}

			executorCmd := cfg.Executor.Command
			if len(executorCmd) == 0 {
				executorCmd = []string{filepath.Join(cfg.Paths.AppDir, "bin", "ccclaude")}
			}
			// 展开 ~ 路径
			expanded := make([]string, len(executorCmd))
			for i, c := range executorCmd {
				expanded[i] = config.ExpandPath(c)
			}

			timeout, _ := time.ParseDuration(strings.TrimSpace(cfg.Executor.Timeout))
			if timeout <= 0 {
				timeout = 30 * time.Minute
			}

			return dream.Run(dream.Config{
				KBDir:       cfg.Paths.KBDir,
				JournalDir:  journalDir,
				ExecutorCmd: expanded,
				Timeout:     timeout,
				DeltaDays:   deltaDays,
			}, cmd.OutOrStdout())
		},
	}
	cmd.Flags().IntVar(&deltaDays, "delta-days", 1, "扫描最近 N 天的 journal delta（默认 1）")
	rootCmd.AddCommand(cmd)
}
```

- [ ] **Step 3.2: 在 main.go 注册 dream 命令**

在 `src/cmd/ccclaw/main.go` 中找到其他命令注册行（如 `addSevolverCommand(rootCmd, ...)`），在其后追加：

```go
addDreamCommand(rootCmd, &configPath, &envFile)
```

- [ ] **Step 3.3: 编译确认**

```bash
cd src && go build ./cmd/ccclaw/...
```

预期：编译通过，无错误。

- [ ] **Step 3.4: 运行全量测试**

```bash
cd src && go test ./... 2>&1 | grep -E "FAIL|ok"
```

预期：全部 PASS 或仅已知失败。

- [ ] **Step 3.5: 验证 ccclaw dream --help**

```bash
cd /opt/src/ccclaw/src && go run ./cmd/ccclaw dream --help
```

预期：显示 "执行一次 kb 记忆整合..." 帮助文本。

- [ ] **Step 3.6: Commit Chunk 3**

```bash
cd /opt/src/ccclaw
git add src/cmd/ccclaw/dream.go src/cmd/ccclaw/main.go
git commit -m "feat(#70): 新增 ccclaw dream 子命令"
```

---

## Chunk 4: KB 文件 + 安装层

### 改动范围

- Create: `src/dist/kb/stuff/skills/.gitkeep` — 暂存区目录占位
- Create: `src/dist/kb/skills/L1/dream-consolidation.md` — dream 整合提示词（skill）
- Modify: `src/dist/install.sh:1911` — `seed_home_repo_tree` 新增 `stuff/skills/` 目录创建

---

- [ ] **Step 4.1: 创建 stuff/skills/ 目录占位文件**

```bash
mkdir -p /opt/src/ccclaw/src/dist/kb/stuff/skills
touch /opt/src/ccclaw/src/dist/kb/stuff/skills/.gitkeep
```

- [ ] **Step 4.2: 在 install.sh seed_home_repo_tree 中新增 stuff/skills/ 目录**

先定位实际行号（避免前序 chunk 导致偏移）：
```bash
grep -n 'kb/designs.*kb/assay' src/dist/install.sh
```

在对应 `mkdir -p` 命令中追加 `"$HOME_REPO/kb/stuff/skills"`（约 1911 行）：

原行：
```bash
mkdir -p "$HOME_REPO/kb/designs" "$HOME_REPO/kb/assay" "$HOME_REPO/kb/journal" "$HOME_REPO/kb/skills/L1" "$HOME_REPO/kb/skills/L2" "$HOME_REPO/docs/reports" "$HOME_REPO/docs/plans" "$HOME_REPO/docs/rfcs"
```

改为：
```bash
mkdir -p "$HOME_REPO/kb/designs" "$HOME_REPO/kb/assay" "$HOME_REPO/kb/journal" \
          "$HOME_REPO/kb/skills/L1" "$HOME_REPO/kb/skills/L2" \
          "$HOME_REPO/kb/stuff/skills" \
          "$HOME_REPO/docs/reports" "$HOME_REPO/docs/plans" "$HOME_REPO/docs/rfcs"
```

- [ ] **Step 4.3: 创建 dream-consolidation.md 提示词 skill**

创建 `src/dist/kb/skills/L1/dream-consolidation.md`：

```markdown
---
name: dream-consolidation
description: DreamCleaner 夜间记忆整合提示词，由 ccclaw dream 自动注入并执行
trigger: 由 ccclaw dream 命令触发（每日 02:42 或手工执行）
keywords: [dream, memory, consolidation, skill, stuff]
---

你是 CCClaw 的夜间记忆整合 Agent（DreamCleaner v2）。

## 你的任务

分析以下注入的近期 `kb/journal/` 和 `kb/assay/` 内容，识别值得提炼为可复用技能的模式。

## 分析标准

只提炼满足以下条件的内容：
- 出现 ≥2 次的重复操作模式或解决方案
- 有明确的"触发场景"和"操作步骤"
- 有长期复用价值（非一次性操作）

## 输出步骤

对每个候选技能，执行以下操作：

**1. 使用 Write 工具写入草稿文件**

路径：`{{KB_DIR}}/stuff/skills/YYYYMMDD-slug.md`

其中 `{{KB_DIR}}` 由 buildPrompt 在运行时注入（实际 KBDir 路径），YYYYMMDD 替换为今日日期（`date +%Y%m%d`），slug 替换为技能英文名。

文件格式：

```markdown
---
name: {技能名（英文 slug）}
description: {一句话：何时触发此技能}
keywords: [{关键词1}, {关键词2}]
---

## 场景

{什么情况下用到这个技能}

## 操作

{具体操作步骤，精简可复用}

## 来源

{来自哪些 journal/assay 条目}
```

**2. 所有草稿写完后，用 Bash 工具创建 review Issue**

注：`{{KB_DIR}}` 是由 `buildPrompt()` 在运行时替换为实际 KBDir 路径的占位符（参见 dream.go Step 2.2）。

```bash
TODAY=$(date +%Y%m%d)
STUFF_DIR="{{KB_DIR}}/stuff/skills"

SKILLS=$(ls "${STUFF_DIR}/${TODAY}-"*.md 2>/dev/null | while IFS= read -r f; do
  name=$(basename "$f")
  desc=$(grep '^description:' "$f" 2>/dev/null | head -1 | sed 's/^description:[[:space:]]*//')
  printf -- '- `%s`: %s\n' "$name" "$desc"
done)

COUNT=$(ls "${STUFF_DIR}/${TODAY}-"*.md 2>/dev/null | wc -l | tr -d ' ')

gh issue create \
  --repo 41490/ccclaw \
  --label ccclaw \
  --title "dream(${TODAY}): ${COUNT} 个技能候选待审核" \
  --body "## DreamCleaner 整合结果

**日期**: $(date +%Y-%m-%d)
**候选技能**:

${SKILLS}

---

审核通过请回复 \`/ccclaw approve\`，拒绝请回复 \`/ccclaw NIL\`。
批准后 Claude 将把通过的技能从 \`kb/stuff/skills/\` 移动到 \`kb/skills/L1/\`。"
```

## 无内容时的处理

若分析后未发现值得提炼的技能，输出一行说明并**不创建 Issue**：

```
DreamCleaner: 未发现值得提炼的新技能模式，本次整合无产出。
```

## 注意事项

- 每个技能文件只包含一个技能
- 不记录已经在 `kb/skills/` 中存在的技能
- 技能描述必须从调用者视角写（"当你需要...时使用此技能"）

---

{{JOURNAL_CONTENT}}
```

- [ ] **Step 4.4: 验证 dist/kb 结构与 install.sh 修改**

```bash
find /opt/src/ccclaw/src/dist/kb -type f | sort
```

预期：包含 `stuff/skills/.gitkeep` 和 `skills/L1/dream-consolidation.md`。

验证 seed_home_repo_tree 已包含 `stuff/skills`：
```bash
grep 'stuff/skills' /opt/src/ccclaw/src/dist/install.sh
```
预期：至少两行（seed_home_repo_tree + enable 列表）。

- [ ] **Step 4.5: 运行全量测试再次确认**

```bash
cd /opt/src/ccclaw/src && go test ./... 2>&1 | grep -E "FAIL|ok"
```

预期：全部 PASS。

- [ ] **Step 4.6: 构建 release 验证二进制包含 dream 命令（依赖 Chunk 3 已完成）**

此步骤需要 Chunk 3（dream CLI 注册）已完成后方可执行。若按顺序实施，此时 dream 子命令已注册：

```bash
cd /opt/src/ccclaw/src && go build -o /tmp/ccclaw-dream-test ./cmd/ccclaw/ && /tmp/ccclaw-dream-test --help | grep dream
```

预期：输出包含 `dream` 子命令描述。

- [ ] **Step 4.7: Commit Chunk 4**

```bash
cd /opt/src/ccclaw
git add src/dist/kb/stuff/skills/.gitkeep \
        src/dist/kb/skills/L1/dream-consolidation.md \
        src/dist/install.sh
git commit -m "feat(#70): 新增 kb/stuff/skills/ 暂存区，dream-consolidation.md 提示词 skill，更新 install.sh"
```

---

## 验收检查单

实施完成后，逐项验证：

- [ ] `ccclaw dream --help` 有正确描述
- [ ] `ccclaw dream --delta-days 7` 可运行（executor 不存在时有明确报错）
- [ ] `go test ./...` 全部通过
- [ ] `src/dist/kb/stuff/skills/.gitkeep` 存在
- [ ] `src/dist/kb/skills/L1/dream-consolidation.md` 包含 `{{JOURNAL_CONTENT}}` 占位
- [ ] `GenerateSystemdUnitContents()` 生成 `ccclaw-dream.timer` 内容
- [ ] `ccclaw-dream.timer` 周期为 `*-*-* 02:42:00 Asia/Shanghai`
- [ ] `config.example.toml` 包含 `dream = "*-*-* 02:42:00"`
- [ ] `install.sh` 在 enable --now 列表中包含 `ccclaw-dream.timer`
- [ ] `install.sh` 在 `seed_home_repo_tree` 中创建 `kb/stuff/skills/`
