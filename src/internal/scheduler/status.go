package scheduler

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/41490/ccclaw/internal/config"
)

type Probe struct {
	Requested        string
	SystemdUserDir   string
	SystemdInstalled bool
	SystemdActive    bool
	SystemdReason    string
	CronActive       bool
	CronReason       string
}

type Diagnosis struct {
	Effective string
	Reason    string
	Repair    string
	Context   []string
}

func DescribeStatus(cfg *config.Config) (string, error) {
	probe := InspectStatus(cfg)
	return summarize(probe)
}

func InspectStatus(cfg *config.Config) Probe {
	probe := Probe{}
	if cfg == nil {
		probe.Requested = "none"
		return probe
	}
	probe.Requested = strings.TrimSpace(cfg.Scheduler.Mode)
	probe.SystemdUserDir = cfg.Scheduler.SystemdUserDir
	if probe.Requested == "" {
		probe.Requested = "none"
	}
	probe.SystemdInstalled = hasSystemdUnitFiles(probe.SystemdUserDir)
	probe.SystemdActive, probe.SystemdReason = detectSystemdTimers()
	probe.CronActive, probe.CronReason = detectCronEntries(cfg)
	return probe
}

func summarize(probe Probe) (string, error) {
	requested := probe.Requested
	if requested == "" {
		requested = "none"
	}
	diagnosis := diagnose(probe, requested)

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

func diagnose(probe Probe, requested string) Diagnosis {
	diagnosis := Diagnosis{
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
		diagnosis.Reason = fallbackReason(probe.CronReason, "已检测到受控 crontab ingest/run/patrol/journal 规则")
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
		return "请补充受控 crontab 规则，确保 ingest/run/patrol/journal 四条任务都已写入"
	case strings.Contains(reason, "未找到 crontab"):
		return "当前环境缺少 crontab；请安装 cron/cronie，或执行 `ccclaw scheduler disable-cron` 后改用 systemd/none"
	case strings.Contains(reason, "未配置 crontab"), strings.Contains(reason, "未检测到受控 crontab 规则"):
		return "请执行 `ccclaw scheduler enable-cron` 补齐 ingest/run/patrol/journal 四条受控规则"
	default:
		return "请检查当前用户 crontab，或重新执行 `ccclaw scheduler enable-cron` 修复受控规则"
	}
}

func shouldAppendSchedulerContext(probe Probe, requested, effective string) bool {
	if effective == "systemd+cron" {
		return true
	}
	if probe.SystemdInstalled && !probe.SystemdActive {
		return true
	}
	return requested != effective
}

func schedulerContext(probe Probe) []string {
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

func hasSystemdUnitFiles(unitDir string) bool {
	if unitDir == "" {
		return false
	}
	for _, name := range managedSystemdUnits() {
		if _, err := os.Stat(filepath.Join(unitDir, name)); err != nil {
			return false
		}
	}
	return true
}

func detectSystemdTimers() (bool, string) {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return false, fmt.Sprintf("未找到 systemctl: %v", err)
	}
	for _, unit := range ManagedSystemdTimers() {
		if _, err := runSystemctlUser(context.Background(), "is-enabled", unit); err != nil {
			return false, err.Error()
		}
		if _, err := runSystemctlUser(context.Background(), "is-active", unit); err != nil {
			return false, err.Error()
		}
	}
	return true, strings.Join(ManagedSystemdTimers(), ", ") + " 已启用且运行中"
}

func detectCronEntries(cfg *config.Config) (bool, string) {
	if _, err := exec.LookPath("crontab"); err != nil {
		return false, fmt.Sprintf("未找到 crontab: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "crontab", "-l")
	output, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return false, "执行 `crontab -l` 超时"
	}
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
	if ContainsManagedCron(string(output), cfg) {
		return true, "已检测到受控 crontab ingest/run/patrol/journal 规则"
	}
	return false, "未检测到受控 crontab 规则"
}
