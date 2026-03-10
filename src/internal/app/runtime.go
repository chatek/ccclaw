package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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

const (
	claudeStateMissingCLI     = "missing_cli"
	claudeStateInstallBlocked = "install_channel_blocked"
	claudeStateInstalledBare  = "installed_unconfigured"
	claudeStateInstalledSetup = "installed_setup_token_ready"
	claudeStateInstalledProxy = "installed_proxy_ready"
	claudeInstallScriptURL    = "https://claude.ai/install.sh"
	issueTargetRepoPattern    = `(?mi)^\s*target_repo\s*:\s*([A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+)\s*$`
	manualSudoGuide           = "sudo 无口令未开启，请按文档手工执行 `sudo visudo` 并为当前用户配置 `NOPASSWD` 规则"
)

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
		target, err := rt.cfg.EnabledTargetByRepo(fresh.TargetRepo)
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
	_, _ = fmt.Fprintln(w, "ISSUE\tSTATE\tTARGET\tAUTHOR\tAPPROVED\tRETRY\tTITLE")
	for _, task := range tasks {
		title := task.IssueTitle
		if len(title) > 46 {
			title = title[:43] + "..."
		}
		targetRepo := task.TargetRepo
		if targetRepo == "" {
			targetRepo = "-"
		}
		_, _ = fmt.Fprintf(w, "#%d\t%s\t%s\t%s\t%t\t%d\t%s\n", task.IssueNumber, task.State, targetRepo, task.IssueAuthor, task.Approved, task.RetryCount, title)
	}
	return w.Flush()
}

func (rt *Runtime) ShowConfig(out io.Writer) error {
	defer rt.store.Close()
	targets := make([]map[string]string, 0, len(rt.cfg.Targets))
	for _, target := range rt.cfg.Targets {
		item := map[string]string{
			"repo":       target.Repo,
			"local_path": target.LocalPath,
			"kb_path":    target.KBPath,
		}
		if target.Disabled {
			item["status"] = "disabled"
		} else {
			item["status"] = "enabled"
		}
		targets = append(targets, item)
	}
	payload := map[string]any{
		"default_target": rt.cfg.DefaultTarget,
		"github":         rt.cfg.GitHub,
		"paths":          rt.cfg.Paths,
		"executor":       rt.cfg.Executor,
		"scheduler":      rt.cfg.Scheduler,
		"approval":       rt.cfg.Approval,
		"targets":        targets,
		"env_file":       rt.secrets.Path,
		"secret_keys":    sortedKeys(rt.secrets.Values),
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
		run  func() (string, error)
	}{
		{name: "配置文件", run: func() (string, error) { return rt.describeTargetRouting(), rt.cfg.Validate() }},
		{name: ".env 权限", run: func() (string, error) { return rt.secrets.Path, config.ValidateEnvFile(rt.secrets.Path) }},
		{name: "claude CLI", run: func() (string, error) {
			return rt.describeExecutor(), commandExists(rt.cfg.Executor.Command, rt.cfg.Executor.Binary)
		}},
		{name: "Claude 安装通道", run: func() (string, error) { return claudeInstallScriptURL, rt.checkClaudeInstallChannel(ctx) }},
		{name: "Claude 运行态", run: func() (string, error) { return rt.detectClaudeState(ctx) }},
		{name: "Go 工具链", run: func() (string, error) { return commandVersion("go", "version") }},
		{name: "sudo 无口令", run: rt.checkPasswordlessSudo},
		{name: "rtk CLI", run: func() (string, error) { return commandVersion("rtk", "--version") }},
		{name: "rg CLI", run: func() (string, error) { return commandVersion("rg", "--version") }},
		{name: "sqlite3 CLI", run: func() (string, error) { return commandVersion("sqlite3", "--version") }},
		{name: "git CLI", run: func() (string, error) { return commandVersion("git", "--version") }},
		{name: "gh CLI", run: func() (string, error) { return commandVersion("gh", "--version") }},
		{name: "GitHub 网络", run: func() (string, error) { return "rate_limit", rt.gh.NetworkCheck(ctx) }},
		{name: "SQLite 读写", run: func() (string, error) { return rt.cfg.Paths.StateDB, rt.store.Ping() }},
		{name: "调度器", run: rt.describeSchedulerStatus},
		{name: "磁盘空间", run: func() (string, error) { return rt.cfg.Paths.LogDir, rt.checkDisk() }},
	}
	var failed []string
	for _, check := range checks {
		detail, err := check.run()
		if err != nil {
			failed = append(failed, fmt.Sprintf("%s: %v", check.name, err))
			if detail != "" {
				_, _ = fmt.Fprintf(out, "[FAIL] %s: %v (%s)\n", check.name, err, detail)
			} else {
				_, _ = fmt.Fprintf(out, "[FAIL] %s: %v\n", check.name, err)
			}
			continue
		}
		if detail != "" {
			_, _ = fmt.Fprintf(out, "[ OK ] %s: %s\n", check.name, detail)
		} else {
			_, _ = fmt.Fprintf(out, "[ OK ] %s\n", check.name)
		}
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
	targetRepo, routeReasons := rt.resolveTargetRepo(issue.Body)
	blockReasons := make([]string, 0, 2)
	if !approved {
		blockReasons = append(blockReasons, fmt.Sprintf("发起者 `%s` 权限为 `%s`，等待管理员评论 `%s`", issue.User.Login, permission, rt.cfg.Approval.Command))
	}
	blockReasons = append(blockReasons, routeReasons...)
	labels := github.LabelNames(issue.Labels)
	if existing == nil {
		existing = &core.Task{
			TaskID:         core.TaskID(issue.Number),
			IdempotencyKey: core.IdempotencyKey(issue.Number),
			ControlRepo:    rt.cfg.GitHub.ControlRepo,
			TargetRepo:     targetRepo,
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
	existing.TargetRepo = targetRepo
	existing.ApprovalCommand = rt.cfg.Approval.Command
	existing.ApprovalActor = approval.Actor
	existing.ApprovalCommentID = approval.CommentID

	switch {
	case existing.State == core.StateDone || existing.State == core.StateDead:
		// 保持终态，只更新元数据。
	case len(blockReasons) == 0:
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
		detail := fmt.Sprintf("任务入队，author=%s permission=%s approved=%t target=%s", existing.IssueAuthor, permission, approved, displayTargetRepo(targetRepo))
		if existing.State == core.StateBlocked {
			eventType = core.EventBlocked
			detail = blockReasonText(blockReasons)
		}
		_ = rt.store.AppendEvent(existing.TaskID, eventType, detail)
		if existing.State == core.StateBlocked && !fromRun {
			_ = rt.rep.ReportBlocked(existing, blockReasonText(blockReasons))
		}
		return nil
	}
	if previousState != existing.State {
		eventType := core.EventUpdated
		detail := fmt.Sprintf("状态变更: %s -> %s", previousState, existing.State)
		if existing.State == core.StateBlocked {
			eventType = core.EventBlocked
			detail = blockReasonText(blockReasons)
		}
		_ = rt.store.AppendEvent(existing.TaskID, eventType, detail)
		if existing.State == core.StateBlocked && !fromRun {
			_ = rt.rep.ReportBlocked(existing, blockReasonText(blockReasons))
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

type schedulerProbe struct {
	Requested        string
	SystemdUserDir   string
	SystemdInstalled bool
	SystemdActive    bool
	SystemdReason    string
	CronActive       bool
	CronReason       string
}

type schedulerDiagnosis struct {
	Effective string
	Reason    string
	Repair    string
	Context   []string
}

func (rt *Runtime) describeSchedulerStatus() (string, error) {
	probe := rt.inspectScheduler()
	return summarizeScheduler(probe)
}

func (rt *Runtime) inspectScheduler() schedulerProbe {
	probe := schedulerProbe{
		Requested:      strings.TrimSpace(rt.cfg.Scheduler.Mode),
		SystemdUserDir: rt.cfg.Scheduler.SystemdUserDir,
	}
	if probe.Requested == "" {
		probe.Requested = "none"
	}
	probe.SystemdInstalled = rt.hasSystemdUnitFiles(probe.SystemdUserDir)
	probe.SystemdActive, probe.SystemdReason = rt.detectSystemdTimers()
	probe.CronActive, probe.CronReason = rt.detectCronEntries()
	return probe
}

func summarizeScheduler(probe schedulerProbe) (string, error) {
	requested := probe.Requested
	if requested == "" {
		requested = "none"
	}
	diagnosis := diagnoseScheduler(probe, requested)

	detail := fmt.Sprintf("request=%s effective=%s reason=%s", requested, diagnosis.Effective, diagnosis.Reason)
	for _, item := range diagnosis.Context {
		detail += " " + item
	}
	if diagnosis.Repair != "" {
		detail += fmt.Sprintf(" repair=%s", diagnosis.Repair)
	}

	switch requested {
	case "none":
		if diagnosis.Effective != "none" {
			return detail, fmt.Errorf("配置要求 none，但当前检测到 %s 调度仍在生效", diagnosis.Effective)
		}
		return detail, nil
	case "systemd":
		if diagnosis.Effective != "systemd" {
			return detail, fmt.Errorf("配置要求 systemd，但当前检测到 %s", diagnosis.Effective)
		}
		return detail, nil
	case "cron":
		if diagnosis.Effective != "cron" {
			return detail, fmt.Errorf("配置要求 cron，但当前检测到 %s", diagnosis.Effective)
		}
		return detail, nil
	case "auto":
		if diagnosis.Effective == "none" || diagnosis.Effective == "systemd+cron" {
			return detail, fmt.Errorf("自动调度未处于单一可用状态")
		}
		return detail, nil
	default:
		return detail, fmt.Errorf("未知调度模式: %s", requested)
	}
}

func diagnoseScheduler(probe schedulerProbe, requested string) schedulerDiagnosis {
	diagnosis := schedulerDiagnosis{
		Effective: "none",
		Reason:    "未检测到生效中的 systemd timer 或受控 crontab",
	}

	switch {
	case probe.SystemdActive && probe.CronActive:
		diagnosis.Effective = "systemd+cron"
		diagnosis.Reason = "同时检测到 systemd 与 cron 调度，存在重复执行风险"
		diagnosis.Repair = "请只保留一种调度方式；若保留 systemd，请删除 crontab 中的 ccclaw 规则"
	case probe.SystemdActive:
		diagnosis.Effective = "systemd"
		diagnosis.Reason = fallbackReason(probe.SystemdReason, "已检测到启用中的 user systemd timer")
	case probe.CronActive:
		diagnosis.Effective = "cron"
		diagnosis.Reason = fallbackReason(probe.CronReason, "已检测到受控 crontab ingest/run 规则")
	case probe.SystemdInstalled:
		diagnosis.Reason = summarizeInstalledButInactiveSystemd(probe.SystemdUserDir, probe.SystemdReason)
		diagnosis.Repair = systemdRepairHint(probe.SystemdReason, true)
	default:
		diagnosis.Reason = summarizeUnavailableScheduler(requested, probe.SystemdReason, probe.CronReason)
		switch requested {
		case "cron":
			diagnosis.Repair = cronRepairHint(probe.CronReason)
		case "systemd", "auto":
			diagnosis.Repair = systemdRepairHint(probe.SystemdReason, false)
		}
	}

	if shouldAppendSchedulerContext(probe, requested, diagnosis.Effective) {
		diagnosis.Context = schedulerContext(probe)
	}
	return diagnosis
}

func fallbackReason(reason, fallback string) string {
	if strings.TrimSpace(reason) == "" {
		return fallback
	}
	return reason
}

func summarizeInstalledButInactiveSystemd(unitDir, reason string) string {
	if isUserBusUnavailable(reason) {
		return fmt.Sprintf("已写入 user systemd 单元目录 %s，但当前会话无法连接 user bus", unitDir)
	}
	if strings.Contains(reason, "is-enabled") {
		return fmt.Sprintf("已写入 user systemd 单元目录 %s，但 timer 尚未启用", unitDir)
	}
	if strings.Contains(reason, "is-active") {
		return fmt.Sprintf("已写入 user systemd 单元目录 %s，但 timer 当前未处于运行态", unitDir)
	}
	return fmt.Sprintf("已写入 user systemd 单元目录 %s，但未检测到启用中的 timer", unitDir)
}

func summarizeUnavailableScheduler(requested, systemdReason, cronReason string) string {
	switch requested {
	case "systemd", "auto":
		switch {
		case systemdReason != "":
			if isUserBusUnavailable(systemdReason) {
				return "当前会话无法连接 user systemd 总线"
			}
			if strings.Contains(systemdReason, "未找到 systemctl") {
				return "当前环境缺少 systemctl，无法使用 user systemd"
			}
			return systemdReason
		case cronReason != "":
			return cronReason
		default:
			return "未检测到可用的 user systemd 或受控 crontab"
		}
	case "cron":
		switch {
		case cronReason != "":
			if strings.Contains(cronReason, "未找到 crontab") {
				return "当前环境缺少 crontab，无法使用 cron 调度"
			}
			return cronReason
		case systemdReason != "":
			return systemdReason
		default:
			return "未检测到受控 crontab 规则"
		}
	default:
		switch {
		case systemdReason != "":
			return systemdReason
		case cronReason != "":
			return cronReason
		default:
			return "未检测到生效中的 systemd timer 或受控 crontab"
		}
	}
}

func systemdRepairHint(reason string, installed bool) string {
	switch {
	case strings.TrimSpace(reason) == "":
		if installed {
			return "请执行 systemctl --user daemon-reload && systemctl --user enable --now ccclaw-ingest.timer ccclaw-run.timer"
		}
		return "请检查 user systemd 可用性，并按需执行 systemctl --user daemon-reload && systemctl --user enable --now ccclaw-ingest.timer ccclaw-run.timer"
	case strings.Contains(reason, "未找到 systemctl"):
		return "当前环境缺少 systemctl；请安装 systemd 组件，或改用 cron/none"
	case isUserBusUnavailable(reason):
		return "请在用户登录会话中执行 systemctl --user daemon-reload && systemctl --user enable --now ccclaw-ingest.timer ccclaw-run.timer；若仍失败，请先确认 user bus 已建立"
	case strings.Contains(reason, "is-enabled"), strings.Contains(reason, "disabled"), strings.Contains(reason, "not-found"):
		return "请执行 systemctl --user daemon-reload && systemctl --user enable --now ccclaw-ingest.timer ccclaw-run.timer"
	case strings.Contains(reason, "is-active"), strings.Contains(reason, "inactive"), strings.Contains(reason, "failed"):
		return "请执行 systemctl --user restart ccclaw-ingest.timer ccclaw-run.timer，并检查 journalctl --user -u ccclaw-ingest.service -u ccclaw-run.service"
	default:
		if installed {
			return "请检查 user systemd 状态后重新执行 daemon-reload / enable --now"
		}
		return "请检查 user systemd 可用性，并按需执行 systemctl --user daemon-reload && systemctl --user enable --now ccclaw-ingest.timer ccclaw-run.timer"
	}
}

func cronRepairHint(reason string) string {
	switch {
	case strings.TrimSpace(reason) == "":
		return "请补充受控 crontab 规则，确保 ingest/run 两条任务都已写入"
	case strings.Contains(reason, "未找到 crontab"):
		return "当前环境缺少 crontab；请安装 cron/cronie，或改用 systemd/none"
	case strings.Contains(reason, "未配置 crontab"), strings.Contains(reason, "未检测到受控 crontab 规则"):
		return "请按安装摘要中的 cron 样板补充 ingest/run 两条受控规则"
	default:
		return "请检查当前用户 crontab，并补齐 ccclaw ingest/run 两条受控规则"
	}
}

func shouldAppendSchedulerContext(probe schedulerProbe, requested, effective string) bool {
	if effective == "systemd+cron" {
		return true
	}
	if probe.SystemdInstalled && !probe.SystemdActive {
		return true
	}
	return requested != effective
}

func schedulerContext(probe schedulerProbe) []string {
	context := make([]string, 0, 2)
	if reason := strings.TrimSpace(probe.SystemdReason); reason != "" {
		context = append(context, "systemd="+reason)
	}
	if reason := strings.TrimSpace(probe.CronReason); reason != "" {
		context = append(context, "cron="+reason)
	}
	return context
}

func isUserBusUnavailable(reason string) bool {
	return strings.Contains(reason, "No medium found") ||
		strings.Contains(reason, "Failed to connect to bus") ||
		strings.Contains(reason, "连接到总线失败") ||
		strings.Contains(reason, "user bus")
}

func (rt *Runtime) hasSystemdUnitFiles(unitDir string) bool {
	if unitDir == "" {
		return false
	}
	required := []string{
		"ccclaw-ingest.service",
		"ccclaw-ingest.timer",
		"ccclaw-run.service",
		"ccclaw-run.timer",
	}
	for _, name := range required {
		if _, err := os.Stat(filepath.Join(unitDir, name)); err != nil {
			return false
		}
	}
	return true
}

func (rt *Runtime) detectSystemdTimers() (bool, string) {
	if err := requireCommand("systemctl"); err != nil {
		return false, err.Error()
	}
	units := []string{"ccclaw-ingest.timer", "ccclaw-run.timer"}
	for _, unit := range units {
		if _, err := systemctlUser("is-enabled", unit); err != nil {
			return false, err.Error()
		}
		if _, err := systemctlUser("is-active", unit); err != nil {
			return false, err.Error()
		}
	}
	return true, "ccclaw-ingest.timer, ccclaw-run.timer 已启用且运行中"
}

func (rt *Runtime) detectCronEntries() (bool, string) {
	if err := requireCommand("crontab"); err != nil {
		return false, err.Error()
	}
	cmd := exec.Command("crontab", "-l")
	output, err := cmd.CombinedOutput()
	if err != nil {
		text := strings.TrimSpace(string(output))
		if strings.Contains(text, "no crontab for") || strings.Contains(text, "没有 crontab") {
			return false, "未配置 crontab"
		}
		if text == "" {
			text = err.Error()
		}
		return false, text
	}
	content := string(output)
	appBin := filepath.ToSlash(filepath.Join(rt.cfg.Paths.AppDir, "bin", "ccclaw"))
	configPath := filepath.ToSlash(filepath.Join(rt.cfg.Paths.AppDir, "ops", "config", "config.toml"))
	envFile := filepath.ToSlash(rt.cfg.Paths.EnvFile)
	ingestLine := fmt.Sprintf("%s ingest --config %s --env-file %s", appBin, configPath, envFile)
	runLine := fmt.Sprintf("%s run --config %s --env-file %s", appBin, configPath, envFile)
	if strings.Contains(content, ingestLine) && strings.Contains(content, runLine) {
		return true, "已检测到受控 crontab ingest/run 规则"
	}
	return false, "未检测到受控 crontab 规则"
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

func systemctlUser(action, unit string) (string, error) {
	cmd := exec.Command("systemctl", "--user", action, unit)
	output, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(output))
	if err == nil {
		if text == "" {
			text = unit
		}
		return text, nil
	}
	if text == "" {
		text = err.Error()
	}
	return "", fmt.Errorf("systemctl --user %s %s 失败: %s", action, unit, text)
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

func (rt *Runtime) resolveTargetRepo(body string) (string, []string) {
	explicitTarget := parseTargetRepo(body)
	if explicitTarget != "" {
		if _, err := rt.cfg.EnabledTargetByRepo(explicitTarget); err != nil {
			return "", []string{fmt.Sprintf("Issue 指定的 `target_repo: %s` 不可用: %v", explicitTarget, err)}
		}
		return explicitTarget, nil
	}
	if rt.cfg.DefaultTarget != "" {
		if _, err := rt.cfg.EnabledTargetByRepo(rt.cfg.DefaultTarget); err != nil {
			return "", []string{fmt.Sprintf("default_target `%s` 不可用: %v", rt.cfg.DefaultTarget, err)}
		}
		return rt.cfg.DefaultTarget, nil
	}
	if len(rt.cfg.EnabledTargets()) == 0 {
		return "", []string{"当前未绑定任何可用 target，请先执行 `ccclaw target add --repo owner/repo --path /path`"}
	}
	return "", []string{"Issue 未声明 `target_repo:`，且当前未配置 `default_target`"}
}

func parseTargetRepo(body string) string {
	matches := regexp.MustCompile(issueTargetRepoPattern).FindStringSubmatch(body)
	if len(matches) != 2 {
		return ""
	}
	return strings.TrimSpace(matches[1])
}

func blockReasonText(reasons []string) string {
	if len(reasons) == 0 {
		return "无"
	}
	return strings.Join(reasons, "；")
}

func displayTargetRepo(repo string) string {
	if repo == "" {
		return "-"
	}
	return repo
}

func (rt *Runtime) describeTargetRouting() string {
	enabled := rt.cfg.EnabledTargets()
	return fmt.Sprintf("enabled=%d default_target=%s", len(enabled), displayTargetRepo(rt.cfg.DefaultTarget))
}

func (rt *Runtime) describeExecutor() string {
	if len(rt.cfg.Executor.Command) > 0 {
		return strings.Join(rt.cfg.Executor.Command, " ")
	}
	return rt.cfg.Executor.Binary
}

func (rt *Runtime) checkClaudeInstallChannel(ctx context.Context) error {
	return probeHTTP(ctx, claudeInstallScriptURL, "")
}

func (rt *Runtime) detectClaudeState(ctx context.Context) (string, error) {
	if err := requireCommand("claude"); err != nil {
		if probeErr := rt.checkClaudeInstallChannel(ctx); probeErr != nil {
			return claudeStateInstallBlocked, fmt.Errorf("未找到 claude，且官方安装通道不可达: %w", probeErr)
		}
		return claudeStateMissingCLI, errors.New("未找到 claude CLI")
	}
	if err := rt.checkClaudeProxyMode(ctx); err == nil {
		return claudeStateInstalledProxy, nil
	}
	if err := rt.checkClaudeSetupToken(ctx); err == nil {
		return claudeStateInstalledSetup, nil
	}
	return claudeStateInstalledBare, errors.New("claude 已安装，但 `setup-token` 或代理模式均未就绪")
}

func (rt *Runtime) checkClaudeSetupToken(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "claude", "auth", "status", "--json")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("`claude auth status --json` 失败: %s", strings.TrimSpace(string(output)))
	}
	var payload map[string]any
	if err := json.Unmarshal(output, &payload); err != nil {
		return fmt.Errorf("解析 `claude auth status --json` 失败: %w", err)
	}
	return nil
}

func (rt *Runtime) checkClaudeProxyMode(ctx context.Context) error {
	baseURL := strings.TrimSpace(rt.secrets.Values["ANTHROPIC_BASE_URL"])
	token := strings.TrimSpace(rt.secrets.Values["ANTHROPIC_AUTH_TOKEN"])
	if baseURL == "" || token == "" {
		return errors.New("缺少 ANTHROPIC_BASE_URL 或 ANTHROPIC_AUTH_TOKEN")
	}
	if len(rt.cfg.Executor.Command) == 0 {
		return errors.New("executor.command 未配置代理包装器")
	}
	if err := requireCommand(rt.cfg.Executor.Command[0]); err != nil {
		return fmt.Errorf("代理包装器不可执行: %w", err)
	}
	return probeHTTP(ctx, baseURL, token)
}

func (rt *Runtime) checkPasswordlessSudo() (string, error) {
	if err := requireCommand("sudo"); err != nil {
		return "", err
	}
	cmd := exec.Command("sudo", "-n", "true")
	if output, err := cmd.CombinedOutput(); err != nil {
		msg := strings.TrimSpace(string(output))
		if msg == "" {
			msg = manualSudoGuide
		}
		return manualSudoGuide, errors.New(msg)
	}
	return "已开启", nil
}

func commandVersion(name string, args ...string) (string, error) {
	if err := requireCommand(name); err != nil {
		return "", err
	}
	cmd := exec.Command(name, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return name, nil
	}
	line := strings.TrimSpace(strings.SplitN(string(output), "\n", 2)[0])
	return line, nil
}

func probeHTTP(ctx context.Context, rawURL, bearerToken string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("URL 无效: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("URL 不完整: %s", rawURL)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	if bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+bearerToken)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 {
		return fmt.Errorf("上游返回 %d", resp.StatusCode)
	}
	return nil
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
