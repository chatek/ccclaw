package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/41490/ccclaw/internal/adapters/github"
	"github.com/41490/ccclaw/internal/adapters/reporter"
	"github.com/41490/ccclaw/internal/adapters/storage"
	"github.com/41490/ccclaw/internal/config"
	"github.com/41490/ccclaw/internal/core"
	"github.com/41490/ccclaw/internal/executor"
	"github.com/41490/ccclaw/internal/memory"
)

type Runtime struct {
	cfg     *config.Config
	secrets *config.Secrets
	store   *storage.Store
	gh      *github.Client
	rep     *reporter.Reporter
	mem     *memory.Index
}

func NewRuntime(configPath, envFile string) (*Runtime, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, err
	}
	if envFile == "" {
		envFile = cfg.Paths.EnvFile
	}
	secrets, err := config.LoadSecrets(envFile)
	if err != nil {
		return nil, err
	}
	store, err := storage.Open(cfg.Paths.StateDB)
	if err != nil {
		return nil, err
	}
	mem, err := memory.Build(cfg.Paths.KBDir)
	if err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("构建 kb 索引失败: %w", err)
	}
	ghClient := github.NewClient(cfg.GitHub.ControlRepo, secrets.Values)
	return &Runtime{cfg: cfg, secrets: secrets, store: store, gh: ghClient, rep: reporter.New(ghClient), mem: mem}, nil
}

func (rt *Runtime) Ingest(ctx context.Context) error {
	defer rt.store.Close()
	issues, err := rt.gh.ListOpenIssues(rt.cfg.GitHub.IssueLabel, rt.cfg.GitHub.Limit)
	if err != nil {
		return err
	}
	for _, issue := range issues {
		if err := rt.syncIssue(ctx, issue, false); err != nil {
			return err
		}
	}
	return nil
}

func (rt *Runtime) Run(ctx context.Context, limit int) error {
	defer rt.store.Close()
	tasks, err := rt.store.ListRunnable(limit)
	if err != nil {
		return err
	}
	if len(tasks) == 0 {
		fmt.Println("暂无待执行任务")
		return nil
	}
	execEngine, err := rt.newExecutor()
	if err != nil {
		return err
	}
	for _, task := range tasks {
		issue, err := rt.gh.GetIssue(task.IssueNumber)
		if err != nil {
			return err
		}
		if err := rt.syncIssue(ctx, *issue, true); err != nil {
			return err
		}
		fresh, err := rt.store.GetByIdempotency(task.IdempotencyKey)
		if err != nil {
			return err
		}
		if fresh == nil || fresh.State == core.StateBlocked || fresh.State == core.StateDone || fresh.State == core.StateDead {
			continue
		}
		target, err := rt.cfg.TargetByRepo(fresh.TargetRepo)
		if err != nil {
			return err
		}
		prompt := rt.buildPrompt(fresh, target)
		fresh.State = core.StateRunning
		if err := rt.store.UpsertTask(fresh); err != nil {
			return err
		}
		_ = rt.store.AppendEvent(fresh.TaskID, core.EventStarted, "开始执行任务")

		result, runErr := execEngine.Run(ctx, target.LocalPath, fresh.TaskID, prompt)
		if runErr != nil {
			fresh.RetryCount++
			fresh.ErrorMsg = runErr.Error()
			fresh.State = core.StateFailed
			eventType := core.EventFailed
			if fresh.RetryCount >= core.MaxRetry {
				fresh.State = core.StateDead
				eventType = core.EventDead
			}
			if err := rt.store.UpsertTask(fresh); err != nil {
				return err
			}
			_ = rt.store.AppendEvent(fresh.TaskID, eventType, fresh.ErrorMsg)
			_ = rt.rep.ReportFailure(fresh)
			continue
		}

		fresh.State = core.StateDone
		fresh.ErrorMsg = ""
		fresh.ReportPath = filepath.ToSlash(filepath.Join("docs", "reports", core.SafeReportFileName(time.Now(), fresh.IssueNumber, fresh.IssueTitle)))
		if err := rt.store.UpsertTask(fresh); err != nil {
			return err
		}
		_ = rt.store.AppendEvent(fresh.TaskID, core.EventDone, "任务执行完成")
		_ = rt.rep.ReportSuccess(fresh, result.Duration, result.LogFile)
	}
	return nil
}

func (rt *Runtime) Status(out io.Writer) error {
	defer rt.store.Close()
	tasks, err := rt.store.ListTasks()
	if err != nil {
		return err
	}
	if len(tasks) == 0 {
		_, _ = fmt.Fprintln(out, "暂无任务")
		return nil
	}
	counts := map[core.State]int{}
	for _, task := range tasks {
		counts[task.State]++
	}
	_, _ = fmt.Fprintf(out, "任务总数: %d\n", len(tasks))
	for _, state := range []core.State{core.StateNew, core.StateRunning, core.StateBlocked, core.StateFailed, core.StateDone, core.StateDead} {
		if counts[state] > 0 {
			_, _ = fmt.Fprintf(out, "  %-8s %d\n", state, counts[state])
		}
	}
	_, _ = fmt.Fprintln(out)
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "ISSUE\tSTATE\tAUTHOR\tAPPROVED\tRETRY\tTITLE")
	for _, task := range tasks {
		title := task.IssueTitle
		if len(title) > 46 {
			title = title[:43] + "..."
		}
		_, _ = fmt.Fprintf(w, "#%d\t%s\t%s\t%t\t%d\t%s\n", task.IssueNumber, task.State, task.IssueAuthor, task.Approved, task.RetryCount, title)
	}
	return w.Flush()
}

func (rt *Runtime) ShowConfig(out io.Writer) error {
	defer rt.store.Close()
	targets := make([]map[string]string, 0, len(rt.cfg.Targets))
	for _, target := range rt.cfg.Targets {
		targets = append(targets, map[string]string{
			"repo":       target.Repo,
			"local_path": target.LocalPath,
			"kb_path":    target.KBPath,
		})
	}
	payload := map[string]any{
		"github":      rt.cfg.GitHub,
		"paths":       rt.cfg.Paths,
		"executor":    rt.cfg.Executor,
		"approval":    rt.cfg.Approval,
		"targets":     targets,
		"env_file":    rt.secrets.Path,
		"secret_keys": sortedKeys(rt.secrets.Values),
	}
	encoded, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, string(encoded))
	return err
}

func (rt *Runtime) Doctor(ctx context.Context, out io.Writer) error {
	defer rt.store.Close()
	checks := []struct {
		name string
		run  func() error
	}{
		{name: "配置文件", run: func() error { return rt.cfg.Validate() }},
		{name: ".env 权限", run: func() error { return config.ValidateEnvFile(rt.secrets.Path) }},
		{name: "claude CLI", run: func() error { return commandExists(rt.cfg.Executor.Command, rt.cfg.Executor.Binary) }},
		{name: "rtk CLI", run: func() error { return requireCommand("rtk") }},
		{name: "rg CLI", run: func() error { return requireCommand("rg") }},
		{name: "sqlite3 CLI", run: func() error { return requireCommand("sqlite3") }},
		{name: "git CLI", run: func() error { return requireCommand("git") }},
		{name: "gh CLI", run: func() error { return requireCommand("gh") }},
		{name: "GitHub 网络", run: func() error { return rt.gh.NetworkCheck(ctx) }},
		{name: "SQLite 读写", run: func() error { return rt.store.Ping() }},
		{name: "systemd timers", run: rt.checkSystemd},
		{name: "磁盘空间", run: rt.checkDisk},
	}
	var failed []string
	for _, check := range checks {
		err := check.run()
		if err != nil {
			failed = append(failed, fmt.Sprintf("%s: %v", check.name, err))
			_, _ = fmt.Fprintf(out, "[FAIL] %s: %v\n", check.name, err)
			continue
		}
		_, _ = fmt.Fprintf(out, "[ OK ] %s\n", check.name)
	}
	if len(failed) > 0 {
		return fmt.Errorf("doctor 失败，共 %d 项异常", len(failed))
	}
	return nil
}

func (rt *Runtime) syncIssue(ctx context.Context, issue github.Issue, fromRun bool) error {
	_ = ctx
	existing, err := rt.store.GetByIdempotency(core.IdempotencyKey(issue.Number))
	if err != nil {
		return err
	}
	trusted, permission, err := rt.gh.IsTrustedActor(issue.User.Login, rt.cfg.Approval.MinimumPermission)
	if err != nil {
		return err
	}
	approval, err := rt.gh.FindApproval(issue.Number, rt.cfg.Approval.Command, rt.cfg.Approval.MinimumPermission)
	if err != nil {
		return err
	}
	approved := trusted || approval.Approved
	labels := github.LabelNames(issue.Labels)
	if existing == nil {
		existing = &core.Task{
			TaskID:         core.TaskID(issue.Number),
			IdempotencyKey: core.IdempotencyKey(issue.Number),
			ControlRepo:    rt.cfg.GitHub.ControlRepo,
			TargetRepo:     rt.cfg.GitHub.ControlRepo,
			CreatedAt:      issue.CreatedAt,
		}
	}
	previousState := existing.State
	existing.IssueNumber = issue.Number
	existing.IssueTitle = issue.Title
	existing.IssueBody = issue.Body
	existing.IssueAuthor = issue.User.Login
	existing.IssueAuthorPermission = permission
	existing.Labels = labels
	existing.Intent = core.InferIntent(labels)
	existing.RiskLevel = core.InferRisk(labels)
	existing.Approved = approved
	existing.ApprovalCommand = rt.cfg.Approval.Command
	existing.ApprovalActor = approval.Actor
	existing.ApprovalCommentID = approval.CommentID

	switch {
	case existing.State == core.StateDone || existing.State == core.StateDead:
		// 保持终态，只更新元数据。
	case approved:
		if existing.State == core.StateBlocked || existing.State == "" {
			existing.State = core.StateNew
		}
	default:
		existing.State = core.StateBlocked
	}

	if err := rt.store.UpsertTask(existing); err != nil {
		return err
	}
	if previousState == "" {
		eventType := core.EventCreated
		detail := fmt.Sprintf("任务入队，author=%s permission=%s approved=%t", existing.IssueAuthor, permission, approved)
		if existing.State == core.StateBlocked {
			eventType = core.EventBlocked
		}
		_ = rt.store.AppendEvent(existing.TaskID, eventType, detail)
		if existing.State == core.StateBlocked && !fromRun {
			_ = rt.rep.ReportBlocked(existing)
		}
		return nil
	}
	if previousState != existing.State {
		eventType := core.EventUpdated
		if existing.State == core.StateBlocked {
			eventType = core.EventBlocked
		}
		_ = rt.store.AppendEvent(existing.TaskID, eventType, fmt.Sprintf("状态变更: %s -> %s", previousState, existing.State))
		if existing.State == core.StateBlocked && !fromRun {
			_ = rt.rep.ReportBlocked(existing)
		}
	}
	return nil
}

func (rt *Runtime) buildPrompt(task *core.Task, target *config.TargetConfig) string {
	keywords := make([]string, 0, len(task.Labels)+3)
	keywords = append(keywords, string(task.Intent), string(task.RiskLevel), task.IssueTitle)
	keywords = append(keywords, task.Labels...)
	matches := rt.mem.Match(keywords, 6)
	reportFile := filepath.ToSlash(filepath.Join("docs", "reports", core.SafeReportFileName(time.Now(), task.IssueNumber, task.IssueTitle)))

	var sb strings.Builder
	sb.WriteString("你是 ccclaw 执行器，请在目标仓库中完成以下 Issue 任务。\n\n")
	sb.WriteString(fmt.Sprintf("## 控制仓库\n- %s\n\n", task.ControlRepo))
	sb.WriteString(fmt.Sprintf("## 目标仓库\n- repo: %s\n- local_path: %s\n\n", target.Repo, target.LocalPath))
	sb.WriteString(fmt.Sprintf("## Issue #%d: %s\n\n%s\n\n", task.IssueNumber, task.IssueTitle, task.IssueBody))
	sb.WriteString("## 元数据\n")
	sb.WriteString(fmt.Sprintf("- 发起者: %s\n- 发起者权限: %s\n- 风险等级: %s\n- 意图: %s\n- 标签: %s\n", task.IssueAuthor, task.IssueAuthorPermission, task.RiskLevel, task.Intent, strings.Join(task.Labels, ", ")))
	if task.ApprovalActor != "" {
		sb.WriteString(fmt.Sprintf("- 审批人: %s\n", task.ApprovalActor))
	}
	if len(matches) > 0 {
		sb.WriteString("\n## 相关 kb 记忆\n")
		for _, doc := range matches {
			sb.WriteString(fmt.Sprintf("\n### %s\n%s\n", doc.Path, doc.Content))
		}
	}
	sb.WriteString("\n## 交付要求\n")
	sb.WriteString("- 优先修改根因，不要做表面补丁\n")
	sb.WriteString("- 保持改动最小且可验证\n")
	sb.WriteString("- 所有文档和注释使用中文\n")
	sb.WriteString(fmt.Sprintf("- 完成后必须写工程报告到 `%s`\n", reportFile))
	sb.WriteString("- 工程报告文件命名遵循 `yymmdd_[Issue No.]_[Case Summary].md`\n")
	sb.WriteString("- 如需回复 Issue，请总结成果、测试结果和后续优化建议\n")
	return sb.String()
}

func (rt *Runtime) newExecutor() (*executor.Executor, error) {
	timeout, err := time.ParseDuration(rt.cfg.Executor.Timeout)
	if err != nil {
		return nil, fmt.Errorf("解析执行超时失败: %w", err)
	}
	return executor.New(rt.cfg.Executor.Command, rt.cfg.Executor.Binary, timeout, rt.cfg.Paths.LogDir, rt.secrets.Values)
}

func (rt *Runtime) checkSystemd() error {
	units := []string{"ccclaw-ingest.timer", "ccclaw-run.timer"}
	for _, unit := range units {
		if err := systemctl("is-enabled", unit); err != nil {
			return err
		}
		if err := systemctl("is-active", unit); err != nil {
			return err
		}
	}
	return nil
}

func (rt *Runtime) checkDisk() error {
	paths := []string{filepath.Dir(rt.cfg.Paths.StateDB), rt.cfg.Paths.LogDir}
	for _, path := range paths {
		if err := os.MkdirAll(path, 0o755); err != nil {
			return err
		}
		var stat syscall.Statfs_t
		if err := syscall.Statfs(path, &stat); err != nil {
			return err
		}
		if stat.Blocks == 0 {
			continue
		}
		freePercent := float64(stat.Bavail) / float64(stat.Blocks) * 100
		if freePercent < 10 {
			return fmt.Errorf("磁盘剩余空间不足 10%%: %s (%.2f%%)", path, freePercent)
		}
	}
	return nil
}

func systemctl(action, unit string) error {
	variants := [][]string{
		{"systemctl", "--user", action, unit},
		{"systemctl", action, unit},
	}
	var errors []string
	for _, argv := range variants {
		cmd := exec.Command(argv[0], argv[1:]...)
		output, err := cmd.CombinedOutput()
		if err == nil {
			return nil
		}
		errors = append(errors, fmt.Sprintf("%s", strings.TrimSpace(string(output))))
	}
	return fmt.Errorf("systemctl %s %s 失败: %s", action, unit, strings.Join(errors, " | "))
}

func requireCommand(name string) error {
	_, err := exec.LookPath(name)
	if err != nil {
		return fmt.Errorf("未找到 %s: %w", name, err)
	}
	return nil
}

func commandExists(command []string, binary string) error {
	if len(command) > 0 {
		return requireCommand(command[0])
	}
	if binary == "" {
		binary = "claude"
	}
	return requireCommand(binary)
}

func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	if len(keys) < 2 {
		return keys
	}
	for i := 0; i < len(keys)-1; i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[j] < keys[i] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	return keys
}
