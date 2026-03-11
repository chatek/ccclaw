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
	"github.com/41490/ccclaw/internal/tmux"
)

type Runtime struct {
	cfg      *config.Config
	secrets  *config.Secrets
	store    *storage.Store
	gh       *github.Client
	rep      *reporter.Reporter
	mem      *memory.Index
	memRoot  string
	memCache map[string]*memory.Index
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

type StatsOptions struct {
	Start             time.Time
	End               time.Time
	Daily             bool
	ShowRTKComparison bool
	Limit             int
}

const (
	defaultStatsLimit      = 20
	journalManagedStartTag = "<!-- ccclaw:managed:start -->"
	journalManagedEndTag   = "<!-- ccclaw:managed:end -->"
	journalUserStartTag    = "<!-- ccclaw:user:start -->"
	journalUserEndTag      = "<!-- ccclaw:user:end -->"
	journalUserPlaceholder = "<!-- 本区块留给本机人工补充；自动更新时会保留这里的内容。 -->"
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
	return &Runtime{
		cfg:      cfg,
		secrets:  secrets,
		store:    store,
		gh:       ghClient,
		rep:      reporter.New(ghClient),
		mem:      mem,
		memRoot:  cfg.Paths.KBDir,
		memCache: map[string]*memory.Index{cfg.Paths.KBDir: mem},
	}, nil
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
		runOpts := rt.buildExecutionOptions(fresh, target)
		fresh.State = core.StateRunning
		if err := rt.store.UpsertTask(fresh); err != nil {
			return err
		}
		_ = rt.store.AppendEvent(fresh.TaskID, core.EventStarted, "开始执行任务")
		if strings.TrimSpace(runOpts.ResumeSessionID) != "" {
			_ = rt.store.AppendEvent(fresh.TaskID, core.EventUpdated, fmt.Sprintf("检测到失败重试，尝试恢复 session %s", runOpts.ResumeSessionID))
		}

		result, runErr := execEngine.Run(ctx, target.LocalPath, fresh.TaskID, runOpts)
		if result != nil && result.Pending {
			_ = rt.store.AppendEvent(fresh.TaskID, core.EventUpdated, fmt.Sprintf("任务已挂入 tmux 会话 %s", result.SessionName))
			continue
		}
		if result == nil && runErr != nil && strings.TrimSpace(runOpts.ResumeSessionID) != "" {
			result = &executor.Result{ResumeSessionID: runOpts.ResumeSessionID}
		}
		if result != nil {
			if result.SessionID != "" {
				fresh.LastSessionID = result.SessionID
			}
		}
		if err := rt.finishTaskExecution(fresh, result, runErr); err != nil {
			return err
		}
	}
	return nil
}

func (rt *Runtime) Patrol(ctx context.Context, out io.Writer) error {
	defer rt.store.Close()
	execEngine, err := rt.newExecutor()
	if err != nil {
		return err
	}
	manager := execEngine.TMux()
	if manager == nil {
		_, _ = fmt.Fprintln(out, "当前未启用 tmux，会话巡查已跳过")
		return nil
	}

	timeout := execEngine.Timeout()
	runningTasks, err := rt.store.ListRunning()
	if err != nil {
		return err
	}
	sessions, err := manager.List("ccclaw-")
	if err != nil {
		return err
	}
	sessionMap := make(map[string]tmux.SessionStatus, len(sessions))
	for _, session := range sessions {
		sessionMap[session.Name] = session
	}

	var (
		completed int
		timedOut  int
		warnings  int
		orphaned  int
	)
	taskSessions := make(map[string]struct{}, len(runningTasks))
	for _, task := range runningTasks {
		artifacts := execEngine.ArtifactPaths(task.TaskID)
		taskSessions[artifacts.SessionName] = struct{}{}
		status, exists := sessionMap[artifacts.SessionName]
		if !exists {
			if handled, err := rt.finalizeMissingSession(task, execEngine); err != nil {
				return err
			} else if handled {
				completed++
				continue
			}
			if err := rt.markTaskDead(task, fmt.Sprintf("tmux 会话 %s 已丢失，且未找到结果文件", artifacts.SessionName), artifacts.LogFile); err != nil {
				return err
			}
			timedOut++
			continue
		}
		if status.PaneDead {
			if err := rt.finalizePatrolSession(task, execEngine, status); err != nil {
				return err
			}
			completed++
			if killErr := manager.Kill(status.Name); killErr != nil {
				_ = rt.store.AppendEvent(task.TaskID, core.EventWarning, fmt.Sprintf("清理 tmux 会话失败: %v", killErr))
			}
			continue
		}

		runningFor := time.Since(status.CreatedAt)
		if timeout > 0 && runningFor >= timeout {
			tail, _ := manager.CaptureOutput(status.Name, 80)
			if killErr := manager.Kill(status.Name); killErr != nil {
				return fmt.Errorf("终止超时 tmux 会话失败: %w", killErr)
			}
			reason := fmt.Sprintf("tmux 会话 %s 已运行 %s，超过超时阈值 %s", status.Name, runningFor.Round(time.Second), timeout)
			if tail != "" {
				reason += "\n最后输出:\n" + tail
			}
			if err := rt.markTaskDead(task, reason, artifacts.LogFile); err != nil {
				return err
			}
			timedOut++
			continue
		}
		if timeout > 0 && runningFor >= timeout*8/10 {
			_ = rt.store.AppendEvent(task.TaskID, core.EventWarning, fmt.Sprintf("tmux 会话 %s 已运行 %s，接近超时阈值 %s", status.Name, runningFor.Round(time.Second), timeout))
			warnings++
		}
	}

	for _, session := range sessions {
		if _, exists := taskSessions[session.Name]; exists {
			continue
		}
		if err := manager.Kill(session.Name); err != nil {
			return fmt.Errorf("清理孤儿 tmux 会话失败: %w", err)
		}
		orphaned++
	}

	_, _ = fmt.Fprintf(out, "巡查完成: completed=%d timeout=%d warning=%d orphaned=%d running=%d\n", completed, timedOut, warnings, orphaned, len(runningTasks))
	_ = ctx
	return nil
}

func (rt *Runtime) Stats(out io.Writer) error {
	return rt.StatsWithOptions(out, StatsOptions{})
}

func (rt *Runtime) StatsWithOptions(out io.Writer, options StatsOptions) error {
	defer rt.store.Close()
	limit := options.Limit
	if limit <= 0 {
		limit = defaultStatsLimit
	}
	summary, err := rt.store.TokenStatsBetween(options.Start, options.End)
	if err != nil {
		return err
	}
	if summary.Runs == 0 {
		if hasStatsRange(options.Start, options.End) {
			_, _ = fmt.Fprintln(out, "所选范围内暂无 token 使用记录")
		} else {
			_, _ = fmt.Fprintln(out, "暂无 token 使用记录")
		}
		return nil
	}
	if hasStatsRange(options.Start, options.End) {
		_, _ = fmt.Fprintln(out, "统计范围:")
		if !options.Start.IsZero() {
			_, _ = fmt.Fprintf(out, "  起始日期: %s\n", options.Start.Format("2006-01-02"))
		}
		if !options.End.IsZero() {
			_, _ = fmt.Fprintf(out, "  截止日期: %s (含当日)\n", options.End.Add(-24*time.Hour).Format("2006-01-02"))
		}
		_, _ = fmt.Fprintln(out)
	}
	_, _ = fmt.Fprintf(out, "执行次数: %d\n", summary.Runs)
	_, _ = fmt.Fprintf(out, "会话数: %d\n", summary.Sessions)
	_, _ = fmt.Fprintf(out, "输入 tokens: %d\n", summary.InputTokens)
	_, _ = fmt.Fprintf(out, "输出 tokens: %d\n", summary.OutputTokens)
	_, _ = fmt.Fprintf(out, "缓存创建 tokens: %d\n", summary.CacheCreate)
	_, _ = fmt.Fprintf(out, "缓存命中 tokens: %d\n", summary.CacheRead)
	_, _ = fmt.Fprintf(out, "累计成本(USD): %.4f\n", summary.CostUSD)
	if !summary.FirstUsedAt.IsZero() {
		_, _ = fmt.Fprintf(out, "首次记录: %s\n", summary.FirstUsedAt.Format(time.RFC3339))
	}
	if !summary.LastUsedAt.IsZero() {
		_, _ = fmt.Fprintf(out, "最近记录: %s\n", summary.LastUsedAt.Format(time.RFC3339))
	}
	if options.Daily {
		daily, err := rt.store.DailyTokenStatsBetween(options.Start, options.End)
		if err != nil {
			return err
		}
		daily, dailyTruncated := limitDailyTokenStats(daily, limit)
		if len(daily) > 0 {
			_, _ = fmt.Fprintln(out)
			if dailyTruncated {
				_, _ = fmt.Fprintf(out, "按天聚合(最近 %d 天):\n", limit)
			} else {
				_, _ = fmt.Fprintln(out, "按天聚合:")
			}
			w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
			_, _ = fmt.Fprintln(w, "DAY\tRUNS\tSESSIONS\tINPUT\tOUTPUT\tCACHE_CREATE\tCACHE_READ\tCOST_USD")
			for _, item := range daily {
				_, _ = fmt.Fprintf(w, "%s\t%d\t%d\t%d\t%d\t%d\t%d\t%.4f\n",
					item.Day.Format("2006-01-02"),
					item.Runs,
					item.Sessions,
					item.InputTokens,
					item.OutputTokens,
					item.CacheCreate,
					item.CacheRead,
					item.CostUSD,
				)
			}
			if err := w.Flush(); err != nil {
				return err
			}
		}
	}
	stats, err := rt.store.TaskTokenStatsBetween(options.Start, options.End, limit)
	if err != nil {
		return err
	}
	if len(stats) > 0 {
		_, _ = fmt.Fprintln(out)
		_, _ = fmt.Fprintf(out, "任务明细(最近 %d 项):\n", limit)
		w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		_, _ = fmt.Fprintln(w, "ISSUE\tRUNS\tINPUT\tOUTPUT\tCACHE_CREATE\tCACHE_READ\tCOST_USD\tSESSION\tTITLE")
		for _, item := range stats {
			title := item.IssueTitle
			if len(title) > 40 {
				title = title[:37] + "..."
			}
			sessionID := item.LastSessionID
			if sessionID == "" {
				sessionID = "-"
			}
			_, _ = fmt.Fprintf(
				w,
				"#%d\t%d\t%d\t%d\t%d\t%d\t%.4f\t%s\t%s\n",
				item.IssueNumber,
				item.Runs,
				item.InputTokens,
				item.OutputTokens,
				item.CacheCreate,
				item.CacheRead,
				item.CostUSD,
				sessionID,
				title,
			)
		}
		if err := w.Flush(); err != nil {
			return err
		}
	}
	if !options.ShowRTKComparison {
		return nil
	}
	comparison, err := rt.store.RTKComparisonBetween(options.Start, options.End)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintln(out, "RTK 对比:")
	_, _ = fmt.Fprintf(out, "  RTK runs: %d\n", comparison.RTKRuns)
	_, _ = fmt.Fprintf(out, "  Plain runs: %d\n", comparison.PlainRuns)
	_, _ = fmt.Fprintf(out, "  RTK avg cost(USD): %.4f\n", comparison.RTKAvgCostUSD)
	_, _ = fmt.Fprintf(out, "  Plain avg cost(USD): %.4f\n", comparison.PlainAvgCostUSD)
	_, _ = fmt.Fprintf(out, "  RTK avg tokens: %.1f\n", comparison.RTKAvgTokens)
	_, _ = fmt.Fprintf(out, "  Plain avg tokens: %.1f\n", comparison.PlainAvgTokens)
	_, _ = fmt.Fprintf(out, "  Estimated savings: %.2f%%\n", comparison.SavingsPercent)
	if options.Daily {
		dailyComparison, err := rt.store.DailyRTKComparisonBetween(options.Start, options.End)
		if err != nil {
			return err
		}
		dailyComparison, dailyComparisonTruncated := limitDailyRTKComparisons(dailyComparison, limit)
		if len(dailyComparison) > 0 {
			_, _ = fmt.Fprintln(out)
			if dailyComparisonTruncated {
				_, _ = fmt.Fprintf(out, "按天 RTK 对比(最近 %d 天):\n", limit)
			} else {
				_, _ = fmt.Fprintln(out, "按天 RTK 对比:")
			}
			w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
			_, _ = fmt.Fprintln(w, "DAY\tRTK_RUNS\tPLAIN_RUNS\tRTK_AVG_COST\tPLAIN_AVG_COST\tRTK_AVG_TOKENS\tPLAIN_AVG_TOKENS\tSAVINGS")
			for _, item := range dailyComparison {
				_, _ = fmt.Fprintf(w, "%s\t%d\t%d\t%.4f\t%.4f\t%.1f\t%.1f\t%.2f%%\n",
					item.Day.Format("2006-01-02"),
					item.RTKRuns,
					item.PlainRuns,
					item.RTKAvgCostUSD,
					item.PlainAvgCostUSD,
					item.RTKAvgTokens,
					item.PlainAvgTokens,
					item.SavingsPercent,
				)
			}
			if err := w.Flush(); err != nil {
				return err
			}
		}
	}
	return nil
}

func hasStatsRange(start, end time.Time) bool {
	return !start.IsZero() || !end.IsZero()
}

func limitDailyTokenStats(items []storage.DailyTokenStat, limit int) ([]storage.DailyTokenStat, bool) {
	if limit <= 0 || len(items) <= limit {
		return items, false
	}
	return items[len(items)-limit:], true
}

func limitDailyRTKComparisons(items []storage.DailyRTKComparison, limit int) ([]storage.DailyRTKComparison, bool) {
	if limit <= 0 || len(items) <= limit {
		return items, false
	}
	return items[len(items)-limit:], true
}

func (rt *Runtime) Status(out io.Writer) error {
	defer rt.store.Close()
	tasks, err := rt.store.ListTasks()
	if err != nil {
		return err
	}
	counts := map[core.State]int{}
	for _, task := range tasks {
		counts[task.State]++
	}
	probe := rt.inspectScheduler()
	diagnosis := diagnoseScheduler(probe, probe.Requested)
	sessionSnapshot := rt.collectStatusSessions(tasks)
	tokenSummary, err := rt.store.TokenStats()
	if err != nil {
		return err
	}
	rtkComparison, err := rt.store.RTKComparisonBetween(time.Time{}, time.Time{})
	if err != nil {
		return err
	}
	recentEvents, err := rt.store.ListTaskEvents(20)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintln(out, "当前快照")
	_, _ = fmt.Fprintln(out)

	_, _ = fmt.Fprintln(out, "调度快照:")
	_, _ = fmt.Fprintf(out, "  配置请求: %s\n", probe.Requested)
	_, _ = fmt.Fprintf(out, "  当前生效: %s\n", diagnosis.Effective)
	_, _ = fmt.Fprintf(out, "  当前说明: %s\n", diagnosis.Reason)
	if diagnosis.Repair != "" {
		_, _ = fmt.Fprintf(out, "  修复建议: %s\n", diagnosis.Repair)
	}
	_, _ = fmt.Fprintln(out)

	_, _ = fmt.Fprintln(out, "会话快照:")
	_, _ = fmt.Fprintf(out, "  运行中任务: %d\n", sessionSnapshot.RunningTasks)
	if sessionSnapshot.Error != "" {
		_, _ = fmt.Fprintf(out, "  tmux 状态: %s\n", sessionSnapshot.Error)
	} else {
		_, _ = fmt.Fprintf(out, "  tmux 会话: %d\n", sessionSnapshot.TotalSessions)
		_, _ = fmt.Fprintf(out, "  会话健康: 正常=%d 接近超时=%d 超时=%d 已退出=%d 丢失=%d 孤儿=%d\n",
			sessionSnapshot.HealthySessions,
			sessionSnapshot.WarningSessions,
			sessionSnapshot.TimeoutSessions,
			sessionSnapshot.DeadSessions,
			sessionSnapshot.MissingSessions,
			sessionSnapshot.OrphanedSessions,
		)
		if len(sessionSnapshot.Items) > 0 {
			w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
			_, _ = fmt.Fprintln(w, "ISSUE\tSESSION\tAGE\tHEALTH\tTITLE")
			for _, item := range sessionSnapshot.Items {
				_, _ = fmt.Fprintf(w, "#%d\t%s\t%s\t%s\t%s\n", item.IssueNumber, item.SessionName, item.Age, item.Health, item.Title)
			}
			if err := w.Flush(); err != nil {
				return err
			}
		}
	}
	_, _ = fmt.Fprintln(out)

	_, _ = fmt.Fprintln(out, "任务概览:")
	_, _ = fmt.Fprintf(out, "  任务总数: %d\n", len(tasks))
	for _, state := range []core.State{core.StateNew, core.StateRunning, core.StateBlocked, core.StateFailed, core.StateDone, core.StateDead} {
		_, _ = fmt.Fprintf(out, "  %-8s %d\n", state, counts[state])
	}
	if len(tasks) == 0 {
		_, _ = fmt.Fprintln(out, "  当前无任务")
	} else {
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
		if err := w.Flush(); err != nil {
			return err
		}
	}
	_, _ = fmt.Fprintln(out)

	_, _ = fmt.Fprintln(out, "Token 快照:")
	if tokenSummary.Runs == 0 {
		_, _ = fmt.Fprintln(out, "  未检测到 token 使用记录")
	} else {
		_, _ = fmt.Fprintf(out, "  累计执行: %d runs / %d sessions\n", tokenSummary.Runs, tokenSummary.Sessions)
		_, _ = fmt.Fprintf(out, "  累计 tokens: input=%d output=%d cache_create=%d cache_read=%d\n", tokenSummary.InputTokens, tokenSummary.OutputTokens, tokenSummary.CacheCreate, tokenSummary.CacheRead)
		_, _ = fmt.Fprintf(out, "  累计成本(USD): %.4f\n", tokenSummary.CostUSD)
		if !tokenSummary.LastUsedAt.IsZero() {
			_, _ = fmt.Fprintf(out, "  最近记录: %s\n", tokenSummary.LastUsedAt.Format(time.RFC3339))
		}
		if rtkComparison.RTKRuns > 0 || rtkComparison.PlainRuns > 0 {
			_, _ = fmt.Fprintf(out, "  RTK 对比样本: rtk=%d plain=%d\n", rtkComparison.RTKRuns, rtkComparison.PlainRuns)
			if rtkComparison.RTKRuns > 0 && rtkComparison.PlainRuns > 0 {
				_, _ = fmt.Fprintf(out, "  RTK 估算节省: %.2f%%\n", rtkComparison.SavingsPercent)
			}
		}
		_, _ = fmt.Fprintln(out, "  详情请执行: ccclaw stats --rtk-comparison")
	}
	_, _ = fmt.Fprintln(out)

	_, _ = fmt.Fprintln(out, "最近异常:")
	alerts := rt.filterStatusAlerts(recentEvents, tasks)
	if len(alerts) == 0 {
		_, _ = fmt.Fprintln(out, "  最近 7 天无失败/超时告警")
		return nil
	}
	for _, alert := range alerts {
		_, _ = fmt.Fprintf(out, "  %s %s %s %s\n", alert.CreatedAt.Format(time.RFC3339), alert.IssueRef, alert.EventType, alert.Detail)
	}
	return nil
}

type statusSessionItem struct {
	IssueNumber int
	SessionName string
	Age         string
	Health      string
	Title       string
}

type statusSessionSnapshot struct {
	RunningTasks     int
	TotalSessions    int
	HealthySessions  int
	WarningSessions  int
	TimeoutSessions  int
	DeadSessions     int
	MissingSessions  int
	OrphanedSessions int
	Error            string
	Items            []statusSessionItem
}

type statusAlert struct {
	IssueRef  string
	EventType core.EventType
	Detail    string
	CreatedAt time.Time
}

func (rt *Runtime) collectStatusSessions(tasks []*core.Task) statusSessionSnapshot {
	snapshot := statusSessionSnapshot{}
	runningTasks := make([]*core.Task, 0)
	for _, task := range tasks {
		if task.State == core.StateRunning {
			runningTasks = append(runningTasks, task)
		}
	}
	snapshot.RunningTasks = len(runningTasks)
	if !tmux.Available("tmux") {
		snapshot.Error = "未检测到 tmux CLI，当前无法汇报会话健康"
		return snapshot
	}
	manager, err := tmux.New("tmux")
	if err != nil {
		snapshot.Error = fmt.Sprintf("初始化 tmux 失败: %v", err)
		return snapshot
	}
	sessions, err := manager.List("ccclaw-")
	if err != nil {
		snapshot.Error = fmt.Sprintf("读取 tmux 会话失败: %v", err)
		return snapshot
	}
	snapshot.TotalSessions = len(sessions)
	timeout, _ := time.ParseDuration(rt.cfg.Executor.Timeout)
	now := time.Now()
	sessionMap := make(map[string]tmux.SessionStatus, len(sessions))
	for _, session := range sessions {
		sessionMap[session.Name] = session
	}
	taskSessions := make(map[string]struct{}, len(runningTasks))
	for _, task := range runningTasks {
		sessionName := executor.SessionName(task.TaskID)
		taskSessions[sessionName] = struct{}{}
		item := statusSessionItem{
			IssueNumber: task.IssueNumber,
			SessionName: sessionName,
			Age:         "-",
			Health:      "missing",
			Title:       trimStatusText(task.IssueTitle, 40),
		}
		status, exists := sessionMap[sessionName]
		switch {
		case !exists:
			snapshot.MissingSessions++
		case status.PaneDead:
			snapshot.DeadSessions++
			item.Age = now.Sub(status.CreatedAt).Round(time.Second).String()
			item.Health = fmt.Sprintf("dead(exit=%d)", status.ExitCode)
		default:
			age := now.Sub(status.CreatedAt)
			item.Age = age.Round(time.Second).String()
			item.Health = "ok"
			if timeout > 0 && age >= timeout {
				snapshot.TimeoutSessions++
				item.Health = "timeout"
			} else if timeout > 0 && age >= timeout*8/10 {
				snapshot.WarningSessions++
				item.Health = "warning"
			} else {
				snapshot.HealthySessions++
			}
		}
		snapshot.Items = append(snapshot.Items, item)
	}
	for _, session := range sessions {
		if _, exists := taskSessions[session.Name]; !exists {
			snapshot.OrphanedSessions++
		}
	}
	return snapshot
}

func (rt *Runtime) filterStatusAlerts(events []storage.EventRecord, tasks []*core.Task) []statusAlert {
	taskRefs := make(map[string]string, len(tasks))
	for _, task := range tasks {
		taskRefs[task.TaskID] = fmt.Sprintf("#%d", task.IssueNumber)
	}
	cutoff := time.Now().Add(-7 * 24 * time.Hour)
	alerts := make([]statusAlert, 0, 5)
	for _, item := range events {
		if item.CreatedAt.Before(cutoff) {
			continue
		}
		if item.EventType != core.EventFailed && item.EventType != core.EventDead && item.EventType != core.EventWarning {
			continue
		}
		issueRef := taskRefs[item.TaskID]
		if issueRef == "" {
			issueRef = item.TaskID
		}
		alerts = append(alerts, statusAlert{
			IssueRef:  issueRef,
			EventType: item.EventType,
			Detail:    trimStatusText(strings.ReplaceAll(item.Detail, "\n", " "), 88),
			CreatedAt: item.CreatedAt,
		})
		if len(alerts) == 5 {
			break
		}
	}
	return alerts
}

func trimStatusText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len(value) <= limit {
		return value
	}
	if limit <= 3 {
		return value[:limit]
	}
	return value[:limit-3] + "..."
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
		{name: "tmux CLI", run: func() (string, error) { return commandVersion("tmux", "-V") }},
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
	approval, err := rt.gh.FindApproval(
		issue.Number,
		rt.cfg.Approval.Words,
		rt.cfg.Approval.RejectWords,
		rt.cfg.Approval.MinimumPermission,
	)
	if err != nil {
		return err
	}
	approved := trusted
	switch {
	case approval.Rejected:
		approved = false
	case approval.Approved:
		approved = true
	}
	targetRepo, routeReasons := rt.resolveTargetRepo(issue.Body)
	blockReasons := make([]string, 0, 2)
	if approval.Rejected {
		blockReasons = append(blockReasons, fmt.Sprintf("最近审批被 `%s` 使用 `%s` 否决", approval.Actor, approval.Command))
	} else if !approved {
		blockReasons = append(blockReasons, fmt.Sprintf(
			"发起者 `%s` 权限为 `%s`，等待 `%s` 及以上成员评论 %s",
			issue.User.Login,
			permission,
			rt.cfg.Approval.MinimumPermission,
			approvalCommandHelp(rt.cfg.Approval.Words),
		))
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
	existing.ApprovalCommand = approval.Command
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
			rt.reportBlocked(existing, blockReasonText(blockReasons))
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
			rt.reportBlocked(existing, blockReasonText(blockReasons))
		}
	}
	return nil
}

func (rt *Runtime) buildPrompt(task *core.Task, target *config.TargetConfig) string {
	keywords := make([]string, 0, len(task.Labels)+3)
	keywords = append(keywords, string(task.Intent), string(task.RiskLevel), task.IssueTitle)
	keywords = append(keywords, task.Labels...)
	matches := rt.memoryIndex(target).Match(keywords, 6)
	reportFile := filepath.ToSlash(filepath.Join("docs", "reports", core.SafeReportFileName(time.Now(), task.IssueNumber, task.IssueTitle)))

	var sb strings.Builder
	sb.WriteString("你是 ccclaw 执行器，请在目标仓库中完成以下 Issue 任务。\n\n")
	sb.WriteString(fmt.Sprintf("## 控制仓库\n- %s\n\n", task.ControlRepo))
	sb.WriteString(fmt.Sprintf("## 目标仓库\n- repo: %s\n- local_path: %s\n- kb_path: %s\n\n", target.Repo, target.LocalPath, target.KBPath))
	sb.WriteString(fmt.Sprintf("## Issue #%d: %s\n\n%s\n\n", task.IssueNumber, task.IssueTitle, task.IssueBody))
	sb.WriteString("## 元数据\n")
	sb.WriteString(fmt.Sprintf("- 发起者: %s\n- 发起者权限: %s\n- 风险等级: %s\n- 意图: %s\n- 标签: %s\n", task.IssueAuthor, task.IssueAuthorPermission, task.RiskLevel, task.Intent, strings.Join(task.Labels, ", ")))
	if task.ApprovalActor != "" {
		sb.WriteString(fmt.Sprintf("- 审批人: %s\n", task.ApprovalActor))
	}
	if len(matches) > 0 {
		sb.WriteString("\n## 相关 kb 摘要\n")
		for _, doc := range matches {
			sb.WriteString(fmt.Sprintf("- 路径: %s\n  标题: %s\n  摘要: %s\n", doc.Path, doc.Title, doc.Summary))
			if len(doc.Keywords) > 0 {
				sb.WriteString(fmt.Sprintf("  关键词: %s\n", strings.Join(doc.Keywords, ", ")))
			}
		}
		sb.WriteString("- 使用方式: 以上仅为索引摘要；仅在确有必要时再按路径打开原文细节\n")
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

func (rt *Runtime) buildExecutionOptions(task *core.Task, target *config.TargetConfig) executor.RunOptions {
	basePrompt := rt.buildPrompt(task, target)
	opts := executor.RunOptions{Prompt: basePrompt}
	if !rt.shouldResumeTask(task) {
		return opts
	}
	opts.ResumeSessionID = task.LastSessionID
	opts.Prompt = rt.buildResumePrompt(task, basePrompt)
	return opts
}

func (rt *Runtime) shouldResumeTask(task *core.Task) bool {
	if task == nil {
		return false
	}
	if task.State != core.StateFailed {
		return false
	}
	if task.RetryCount <= 0 || task.RetryCount >= core.MaxRetry {
		return false
	}
	return strings.TrimSpace(task.LastSessionID) != ""
}

func (rt *Runtime) buildResumePrompt(task *core.Task, basePrompt string) string {
	var sb strings.Builder
	sb.WriteString("上次任务执行中断，请基于已有 session 上下文继续完成，不要从头重复分析。\n")
	sb.WriteString(fmt.Sprintf("恢复 session_id: %s\n", task.LastSessionID))
	if strings.TrimSpace(task.ErrorMsg) != "" {
		sb.WriteString(fmt.Sprintf("上次失败原因: %s\n", task.ErrorMsg))
	}
	sb.WriteString("\n以下是原始任务提示词，继续执行时请保持同一交付边界：\n\n")
	sb.WriteString(basePrompt)
	return sb.String()
}

func (rt *Runtime) memoryIndex(target *config.TargetConfig) *memory.Index {
	if rt == nil {
		return &memory.Index{}
	}
	kbPath := rt.memRoot
	if target != nil && strings.TrimSpace(target.KBPath) != "" {
		kbPath = target.KBPath
	}
	if strings.TrimSpace(kbPath) == "" {
		if rt.mem != nil {
			return rt.mem
		}
		return &memory.Index{}
	}
	if rt.memCache == nil {
		rt.memCache = map[string]*memory.Index{}
	}
	if idx, ok := rt.memCache[kbPath]; ok && idx != nil {
		return idx
	}
	idx, err := memory.Build(kbPath)
	if err != nil {
		if rt.mem != nil {
			return rt.mem
		}
		return &memory.Index{}
	}
	rt.memCache[kbPath] = idx
	if kbPath == rt.memRoot && rt.mem == nil {
		rt.mem = idx
	}
	return idx
}

func (rt *Runtime) newExecutor() (*executor.Executor, error) {
	timeout, err := time.ParseDuration(rt.cfg.Executor.Timeout)
	if err != nil {
		return nil, fmt.Errorf("解析执行超时失败: %w", err)
	}
	var manager tmux.Manager
	if tmux.Available("tmux") {
		tmuxManager, err := tmux.New("tmux")
		if err != nil {
			return nil, err
		}
		manager = tmuxManager
	}
	return executor.New(
		rt.cfg.Executor.Command,
		rt.cfg.Executor.Binary,
		timeout,
		rt.cfg.Paths.LogDir,
		filepath.Join(rt.cfg.Paths.AppDir, "var", "results"),
		rt.secrets.Values,
		manager,
	)
}

func (rt *Runtime) persistExecutionMetrics(task *core.Task, result *executor.Result) error {
	if task == nil || result == nil {
		return nil
	}
	if result.SessionID == "" && result.CostUSD == 0 && result.Usage == (core.TokenUsage{}) {
		return nil
	}
	if err := rt.store.RecordTokenUsage(storage.TokenUsageRecord{
		TaskID:     task.TaskID,
		SessionID:  result.SessionID,
		PromptFile: result.PromptFile,
		Usage:      result.Usage,
		CostUSD:    result.CostUSD,
		DurationMS: result.Duration.Milliseconds(),
		RTKEnabled: result.RTKEnabled,
	}); err != nil {
		return err
	}
	return nil
}

func (rt *Runtime) finishTaskExecution(task *core.Task, result *executor.Result, runErr error) error {
	if task == nil {
		return nil
	}
	resumeSessionID := ""
	if result != nil {
		if result.SessionID != "" {
			task.LastSessionID = result.SessionID
		}
		resumeSessionID = strings.TrimSpace(result.ResumeSessionID)
		if err := rt.persistExecutionMetrics(task, result); err != nil {
			return err
		}
	}
	if runErr != nil {
		if resumeSessionID != "" {
			task.LastSessionID = ""
			_ = rt.store.AppendEvent(task.TaskID, core.EventWarning, fmt.Sprintf("恢复 session %s 失败，后续重试将降级为新执行", resumeSessionID))
		}
		task.RetryCount++
		task.ErrorMsg = runErr.Error()
		task.State = core.StateFailed
		eventType := core.EventFailed
		if task.RetryCount >= core.MaxRetry {
			task.State = core.StateDead
			eventType = core.EventDead
		}
		if err := rt.store.UpsertTask(task); err != nil {
			return err
		}
		_ = rt.store.AppendEvent(task.TaskID, eventType, task.ErrorMsg)
		rt.reportFailure(task)
		return nil
	}
	task.State = core.StateDone
	task.ErrorMsg = ""
	task.ReportPath = filepath.ToSlash(filepath.Join("docs", "reports", core.SafeReportFileName(time.Now(), task.IssueNumber, task.IssueTitle)))
	if err := rt.store.UpsertTask(task); err != nil {
		return err
	}
	_ = rt.store.AppendEvent(task.TaskID, core.EventDone, "任务执行完成")
	if result != nil {
		rt.reportSuccess(task, result.Duration, result.LogFile)
	}
	return nil
}

func (rt *Runtime) finalizePatrolSession(task *core.Task, execEngine *executor.Executor, status tmux.SessionStatus) error {
	result, resultErr := execEngine.LoadResult(task.TaskID)
	if result != nil {
		result.ExitCode = status.ExitCode
	}
	if resultErr == nil && status.ExitCode != 0 {
		resultErr = fmt.Errorf("tmux 会话退出码=%d", status.ExitCode)
	}
	return rt.finishTaskExecution(task, result, resultErr)
}

func (rt *Runtime) finalizeMissingSession(task *core.Task, execEngine *executor.Executor) (bool, error) {
	result, resultErr := execEngine.LoadResult(task.TaskID)
	if result == nil && resultErr != nil {
		return false, nil
	}
	return true, rt.finishTaskExecution(task, result, resultErr)
}

func (rt *Runtime) markTaskDead(task *core.Task, reason, logFile string) error {
	task.RetryCount++
	task.ErrorMsg = reason
	task.State = core.StateDead
	if err := rt.store.UpsertTask(task); err != nil {
		return err
	}
	_ = rt.store.AppendEvent(task.TaskID, core.EventDead, reason)
	rt.reportFailure(task)
	_ = logFile
	return nil
}

func (rt *Runtime) reportBlocked(task *core.Task, reason string) {
	if rt.rep == nil {
		return
	}
	_ = rt.rep.ReportBlocked(task, reason)
}

func (rt *Runtime) reportFailure(task *core.Task) {
	if rt.rep == nil {
		return
	}
	_ = rt.rep.ReportFailure(task)
}

func (rt *Runtime) reportSuccess(task *core.Task, duration time.Duration, logFile string) {
	if rt.rep == nil {
		return
	}
	_ = rt.rep.ReportSuccess(task, duration, logFile)
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
			return "请执行 systemctl --user daemon-reload && systemctl --user enable --now ccclaw-ingest.timer ccclaw-run.timer ccclaw-patrol.timer ccclaw-journal.timer"
		}
		return "请检查 user systemd 可用性，并按需执行 systemctl --user daemon-reload && systemctl --user enable --now ccclaw-ingest.timer ccclaw-run.timer ccclaw-patrol.timer ccclaw-journal.timer"
	case strings.Contains(reason, "未找到 systemctl"):
		return "当前环境缺少 systemctl；请安装 systemd 组件，或改用 cron/none"
	case isUserBusUnavailable(reason):
		return "请在用户登录会话中执行 systemctl --user daemon-reload && systemctl --user enable --now ccclaw-ingest.timer ccclaw-run.timer ccclaw-patrol.timer ccclaw-journal.timer；若仍失败，请先确认 user bus 已建立"
	case strings.Contains(reason, "is-enabled"), strings.Contains(reason, "disabled"), strings.Contains(reason, "not-found"):
		return "请执行 systemctl --user daemon-reload && systemctl --user enable --now ccclaw-ingest.timer ccclaw-run.timer ccclaw-patrol.timer ccclaw-journal.timer"
	case strings.Contains(reason, "is-active"), strings.Contains(reason, "inactive"), strings.Contains(reason, "failed"):
		return "请执行 systemctl --user restart ccclaw-ingest.timer ccclaw-run.timer ccclaw-patrol.timer ccclaw-journal.timer，并检查 journalctl --user -u ccclaw-ingest.service -u ccclaw-run.service -u ccclaw-patrol.service -u ccclaw-journal.service"
	default:
		if installed {
			return "请检查 user systemd 状态后重新执行 daemon-reload / enable --now"
		}
		return "请检查 user systemd 可用性，并按需执行 systemctl --user daemon-reload && systemctl --user enable --now ccclaw-ingest.timer ccclaw-run.timer ccclaw-patrol.timer ccclaw-journal.timer"
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
		"ccclaw-patrol.service",
		"ccclaw-patrol.timer",
		"ccclaw-journal.service",
		"ccclaw-journal.timer",
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
	units := []string{"ccclaw-ingest.timer", "ccclaw-run.timer", "ccclaw-patrol.timer", "ccclaw-journal.timer"}
	for _, unit := range units {
		if _, err := systemctlUser("is-enabled", unit); err != nil {
			return false, err.Error()
		}
		if _, err := systemctlUser("is-active", unit); err != nil {
			return false, err.Error()
		}
	}
	return true, "ccclaw-ingest.timer, ccclaw-run.timer, ccclaw-patrol.timer, ccclaw-journal.timer 已启用且运行中"
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

func approvalCommandHelp(words []string) string {
	normalized := normalizeApprovalWords(words)
	if len(normalized) == 0 {
		return fmt.Sprintf("`%s approve`", github.ApprovalPrefix)
	}
	if len(normalized) == 1 {
		return fmt.Sprintf("`%s %s`", github.ApprovalPrefix, normalized[0])
	}
	return fmt.Sprintf(
		"`%s %s`（同义词: %s）",
		github.ApprovalPrefix,
		normalized[0],
		strings.Join(normalized[1:], " / "),
	)
}

func normalizeApprovalWords(words []string) []string {
	normalized := make([]string, 0, len(words))
	seen := make(map[string]struct{}, len(words))
	for _, word := range words {
		candidate := strings.ToLower(strings.TrimSpace(word))
		if candidate == "" {
			continue
		}
		if _, exists := seen[candidate]; exists {
			continue
		}
		seen[candidate] = struct{}{}
		normalized = append(normalized, candidate)
	}
	return normalized
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

func (rt *Runtime) Journal(day time.Time, out io.Writer) error {
	defer rt.store.Close()
	if day.IsZero() {
		day = time.Now()
	}
	summary, err := rt.store.JournalDaySummary(day)
	if err != nil {
		return err
	}
	comparison, err := rt.store.RTKComparisonBetween(dayStart(day), dayEnd(day))
	if err != nil {
		return err
	}
	tasks, err := rt.store.JournalTaskSummaries(day)
	if err != nil {
		return err
	}
	events, err := rt.store.ListTaskEventsBetween(dayStart(day), dayEnd(day), 100)
	if err != nil {
		return err
	}
	path := rt.journalFilePath(day)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("创建 journal 目录失败: %w", err)
	}
	content := rt.renderJournal(day, summary, comparison, tasks, events)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("写入 journal 失败: %w", err)
	}
	if err := rt.refreshJournalSummaries(day); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "journal 已生成: %s\n", path)
	return nil
}

func (rt *Runtime) journalFilePath(day time.Time) string {
	return filepath.Join(rt.journalMonthDir(day), fmt.Sprintf("%s.%s.ccclaw_log.md", day.Format("2006.01.02"), journalUserName()))
}

func (rt *Runtime) journalRootDir() string {
	return filepath.Join(rt.cfg.Paths.HomeRepo, "kb", "journal")
}

func (rt *Runtime) journalYearDir(day time.Time) string {
	return filepath.Join(rt.journalRootDir(), day.Format("2006"))
}

func (rt *Runtime) journalMonthDir(day time.Time) string {
	year := day.Format("2006")
	month := day.Format("01")
	return filepath.Join(rt.journalRootDir(), year, month)
}

func (rt *Runtime) refreshJournalSummaries(day time.Time) error {
	if err := rt.writeJournalMonthSummary(day); err != nil {
		return err
	}
	if err := rt.writeJournalYearSummary(day); err != nil {
		return err
	}
	if err := rt.writeJournalRootSummary(); err != nil {
		return err
	}
	return nil
}

func (rt *Runtime) writeJournalMonthSummary(day time.Time) error {
	monthStart := time.Date(day.Year(), day.Month(), 1, 0, 0, 0, 0, day.Location())
	monthEnd := monthStart.AddDate(0, 1, 0)
	summary, err := rt.store.JournalSummaryBetween(monthStart, monthEnd)
	if err != nil {
		return err
	}
	comparison, err := rt.store.RTKComparisonBetween(monthStart, monthEnd)
	if err != nil {
		return err
	}
	files, err := collectJournalFiles(rt.journalMonthDir(day), ".ccclaw_log.md")
	if err != nil {
		return err
	}
	var body strings.Builder
	body.WriteString(fmt.Sprintf("## 范围\n\n- 月份: %s\n- 日志文件数: %d\n", monthStart.Format("2006-01"), len(files)))
	body.WriteString(renderJournalPeriodMetrics(summary, comparison))
	body.WriteString("\n## 日志入口\n\n")
	if len(files) == 0 {
		body.WriteString("- 当月暂无日报\n")
	} else {
		for _, name := range files {
			dayLabel := strings.TrimSuffix(name, ".ccclaw_log.md")
			body.WriteString(fmt.Sprintf("- [%s](./%s)\n", dayLabel, name))
		}
	}
	return writeManagedJournalFile(
		filepath.Join(rt.journalMonthDir(day), "summary.md"),
		fmt.Sprintf("# ccclaw journal 月汇总 %s", monthStart.Format("2006-01")),
		body.String(),
	)
}

func (rt *Runtime) writeJournalYearSummary(day time.Time) error {
	yearStart := time.Date(day.Year(), 1, 1, 0, 0, 0, 0, day.Location())
	yearEnd := yearStart.AddDate(1, 0, 0)
	summary, err := rt.store.JournalSummaryBetween(yearStart, yearEnd)
	if err != nil {
		return err
	}
	comparison, err := rt.store.RTKComparisonBetween(yearStart, yearEnd)
	if err != nil {
		return err
	}
	monthDirs, err := collectJournalSubdirs(rt.journalYearDir(day))
	if err != nil {
		return err
	}
	var body strings.Builder
	body.WriteString(fmt.Sprintf("## 范围\n\n- 年度: %s\n- 月目录数: %d\n", yearStart.Format("2006"), len(monthDirs)))
	body.WriteString(renderJournalPeriodMetrics(summary, comparison))
	body.WriteString("\n## 月度入口\n\n")
	if len(monthDirs) == 0 {
		body.WriteString("- 当年暂无月度归档\n")
	} else {
		for _, month := range monthDirs {
			body.WriteString(fmt.Sprintf("- [%s](./%s/summary.md)\n", month, month))
		}
	}
	return writeManagedJournalFile(
		filepath.Join(rt.journalYearDir(day), "summary.md"),
		fmt.Sprintf("# ccclaw journal 年汇总 %s", yearStart.Format("2006")),
		body.String(),
	)
}

func (rt *Runtime) writeJournalRootSummary() error {
	summary, err := rt.store.JournalSummaryBetween(time.Time{}, time.Time{})
	if err != nil {
		return err
	}
	comparison, err := rt.store.RTKComparisonBetween(time.Time{}, time.Time{})
	if err != nil {
		return err
	}
	yearDirs, err := collectJournalSubdirs(rt.journalRootDir())
	if err != nil {
		return err
	}
	var body strings.Builder
	body.WriteString(fmt.Sprintf("## 范围\n\n- 年度目录数: %d\n", len(yearDirs)))
	body.WriteString(renderJournalPeriodMetrics(summary, comparison))
	body.WriteString("\n## 年度入口\n\n")
	if len(yearDirs) == 0 {
		body.WriteString("- 当前暂无年度归档\n")
	} else {
		for _, year := range yearDirs {
			body.WriteString(fmt.Sprintf("- [%s](./%s/summary.md)\n", year, year))
		}
	}
	return writeManagedJournalFile(
		filepath.Join(rt.journalRootDir(), "summary.md"),
		"# ccclaw journal 总览",
		body.String(),
	)
}

func renderJournalPeriodMetrics(summary *storage.JournalDaySummary, comparison *storage.RTKComparison) string {
	if summary == nil {
		summary = &storage.JournalDaySummary{}
	}
	if comparison == nil {
		comparison = &storage.RTKComparison{}
	}
	var sb strings.Builder
	sb.WriteString("\n## 聚合指标\n\n")
	sb.WriteString(fmt.Sprintf("- 任务触达数: %d\n", summary.TasksTouched))
	sb.WriteString(fmt.Sprintf("- 启动任务数: %d\n", summary.Started))
	sb.WriteString(fmt.Sprintf("- 完成任务数: %d\n", summary.Done))
	sb.WriteString(fmt.Sprintf("- 失败任务数: %d\n", summary.Failed))
	sb.WriteString(fmt.Sprintf("- 僵死任务数: %d\n", summary.Dead))
	sb.WriteString(fmt.Sprintf("- 输入 tokens: %d\n", summary.InputTokens))
	sb.WriteString(fmt.Sprintf("- 输出 tokens: %d\n", summary.OutputTokens))
	sb.WriteString(fmt.Sprintf("- 缓存创建 tokens: %d\n", summary.CacheCreate))
	sb.WriteString(fmt.Sprintf("- 缓存命中 tokens: %d\n", summary.CacheRead))
	sb.WriteString(fmt.Sprintf("- 累计成本(USD): %.4f\n", summary.CostUSD))
	sb.WriteString(fmt.Sprintf("- RTK runs: %d\n", comparison.RTKRuns))
	sb.WriteString(fmt.Sprintf("- Plain runs: %d\n", comparison.PlainRuns))
	sb.WriteString(fmt.Sprintf("- RTK 平均成本(USD): %.4f\n", comparison.RTKAvgCostUSD))
	sb.WriteString(fmt.Sprintf("- 非 RTK 平均成本(USD): %.4f\n", comparison.PlainAvgCostUSD))
	sb.WriteString(fmt.Sprintf("- 估算节省比例: %.2f%%\n", comparison.SavingsPercent))
	return sb.String()
}

func collectJournalFiles(dir, suffix string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("读取 journal 目录失败: %w", err)
	}
	items := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name == "summary.md" || !strings.HasSuffix(name, suffix) {
			continue
		}
		items = append(items, name)
	}
	return items, nil
}

func collectJournalSubdirs(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("读取 journal 子目录失败: %w", err)
	}
	items := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			items = append(items, entry.Name())
		}
	}
	return items, nil
}

func writeManagedJournalFile(path, title, managedBody string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("创建 journal summary 目录失败: %w", err)
	}
	userBody, err := readJournalUserBody(path)
	if err != nil {
		return err
	}
	content := buildManagedJournalFile(title, managedBody, userBody)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("写入 journal summary 失败: %w", err)
	}
	return nil
}

func readJournalUserBody(path string) (string, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return journalUserPlaceholder, nil
		}
		return "", fmt.Errorf("读取 journal summary 失败: %w", err)
	}
	text := string(payload)
	start := strings.Index(text, journalUserStartTag)
	end := strings.Index(text, journalUserEndTag)
	if start < 0 || end < 0 || end < start {
		return journalUserPlaceholder, nil
	}
	body := strings.TrimSpace(text[start+len(journalUserStartTag) : end])
	if body == "" {
		return journalUserPlaceholder, nil
	}
	return body, nil
}

func buildManagedJournalFile(title, managedBody, userBody string) string {
	var sb strings.Builder
	sb.WriteString(title)
	sb.WriteString("\n\n")
	sb.WriteString(journalManagedStartTag)
	sb.WriteString("\n")
	sb.WriteString(strings.TrimSpace(managedBody))
	sb.WriteString("\n")
	sb.WriteString(journalManagedEndTag)
	sb.WriteString("\n\n")
	sb.WriteString(journalUserStartTag)
	sb.WriteString("\n")
	sb.WriteString(strings.TrimSpace(userBody))
	sb.WriteString("\n")
	sb.WriteString(journalUserEndTag)
	sb.WriteString("\n")
	return sb.String()
}

func (rt *Runtime) renderJournal(day time.Time, summary *storage.JournalDaySummary, comparison *storage.RTKComparison, tasks []storage.JournalTaskSummary, events []storage.EventRecord) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# ccclaw journal %s\n\n", day.Format("2006-01-02")))
	sb.WriteString("## 日汇总\n\n")
	sb.WriteString(fmt.Sprintf("- 任务触达数: %d\n", summary.TasksTouched))
	sb.WriteString(fmt.Sprintf("- 启动任务数: %d\n", summary.Started))
	sb.WriteString(fmt.Sprintf("- 完成任务数: %d\n", summary.Done))
	sb.WriteString(fmt.Sprintf("- 失败任务数: %d\n", summary.Failed))
	sb.WriteString(fmt.Sprintf("- 僵死任务数: %d\n", summary.Dead))
	sb.WriteString(fmt.Sprintf("- 输入 tokens: %d\n", summary.InputTokens))
	sb.WriteString(fmt.Sprintf("- 输出 tokens: %d\n", summary.OutputTokens))
	sb.WriteString(fmt.Sprintf("- 缓存创建 tokens: %d\n", summary.CacheCreate))
	sb.WriteString(fmt.Sprintf("- 缓存命中 tokens: %d\n", summary.CacheRead))
	sb.WriteString(fmt.Sprintf("- 累计成本(USD): %.4f\n", summary.CostUSD))
	sb.WriteString(fmt.Sprintf("- RTK 平均成本(USD): %.4f\n", comparison.RTKAvgCostUSD))
	sb.WriteString(fmt.Sprintf("- 非 RTK 平均成本(USD): %.4f\n", comparison.PlainAvgCostUSD))
	sb.WriteString(fmt.Sprintf("- 估算节省比例: %.2f%%\n", comparison.SavingsPercent))

	sb.WriteString("\n## 任务摘要\n\n")
	if len(tasks) == 0 {
		sb.WriteString("- 当日无任务变更\n")
	} else {
		for _, item := range tasks {
			sb.WriteString(fmt.Sprintf("- #%d `%s` 状态=`%s` runs=%d cost=%.4f tokens=%d/%d cache=%d/%d updated_at=%s\n",
				item.IssueNumber,
				item.IssueTitle,
				item.State,
				item.Runs,
				item.CostUSD,
				item.InputTokens,
				item.OutputTokens,
				item.CacheCreate,
				item.CacheRead,
				formatJournalTime(item.UpdatedAt),
			))
		}
	}

	sb.WriteString("\n## 巡查事件\n\n")
	patrolEvents := filterPatrolEvents(events)
	if len(patrolEvents) == 0 {
		sb.WriteString("- 当日无巡查事件\n")
	} else {
		for _, event := range patrolEvents {
			sb.WriteString(fmt.Sprintf("- %s [%s] %s %s\n",
				formatJournalTime(event.CreatedAt),
				event.EventType,
				event.TaskID,
				strings.TrimSpace(event.Detail),
			))
		}
	}
	return sb.String()
}

func filterPatrolEvents(events []storage.EventRecord) []storage.EventRecord {
	filtered := make([]storage.EventRecord, 0, len(events))
	for _, event := range events {
		detail := strings.ToLower(strings.TrimSpace(event.Detail))
		if strings.Contains(detail, "tmux") || strings.Contains(detail, "巡查") {
			filtered = append(filtered, event)
		}
	}
	return filtered
}

func journalUserName() string {
	for _, key := range []string{"USER", "LOGNAME", "USERNAME"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return "unknown"
}

func formatJournalTime(ts time.Time) string {
	if ts.IsZero() {
		return "-"
	}
	return ts.Format(time.RFC3339)
}

func dayStart(day time.Time) time.Time {
	return time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location())
}

func dayEnd(day time.Time) time.Time {
	return dayStart(day).Add(24 * time.Hour)
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
