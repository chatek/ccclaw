package scheduler

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/41490/ccclaw/internal/config"
	"github.com/41490/ccclaw/internal/logging"
)

type doctorCheck struct {
	Name   string
	Detail string
	Repair string
	Err    error
}

func Doctor(ctx context.Context, cfg *config.Config, out io.Writer) error {
	checks := runDoctorChecks(ctx, cfg)
	failed := 0
	for _, check := range checks {
		if check.Err != nil {
			failed++
			_, _ = fmt.Fprintf(out, "[FAIL] %s: %v", check.Name, check.Err)
			if rendered := renderDoctorCheckExtra(check); rendered != "" {
				_, _ = fmt.Fprintf(out, " (%s)", rendered)
			}
			_, _ = fmt.Fprintln(out)
			continue
		}
		_, _ = fmt.Fprintf(out, "[ OK ] %s", check.Name)
		if rendered := renderDoctorCheckExtra(check); rendered != "" {
			_, _ = fmt.Fprintf(out, ": %s", rendered)
		}
		_, _ = fmt.Fprintln(out)
	}
	if failed > 0 {
		return fmt.Errorf("scheduler doctor 失败，共 %d 项异常", failed)
	}
	return nil
}

func renderDoctorCheckExtra(check doctorCheck) string {
	switch {
	case check.Detail != "" && check.Repair != "":
		return fmt.Sprintf("%s; 修复=%s", check.Detail, check.Repair)
	case check.Detail != "":
		return check.Detail
	case check.Repair != "":
		return "修复=" + check.Repair
	default:
		return ""
	}
}

func runDoctorChecks(ctx context.Context, cfg *config.Config) []doctorCheck {
	probe := InspectStatus(cfg)
	statusSnapshot, statusErr := CollectStatus(cfg)
	detail := statusSnapshot.Detail()
	runtimeLevel, levelErr := logging.NormalizeRuntimeLevel(cfg.Scheduler.Logs.Level)
	logLevelDetail := fmt.Sprintf(
		"configured=%s runtime=%s scheduler_logs_default=%s cli/stderr 与 journald 共享同一阈值",
		cfg.Scheduler.Logs.Level,
		runtimeLevel,
		cfg.Scheduler.Logs.Level,
	)
	if levelErr != nil {
		logLevelDetail = fmt.Sprintf("configured=%s", cfg.Scheduler.Logs.Level)
	}
	checks := []doctorCheck{
		{
			Name: "调度配置",
			Detail: fmt.Sprintf(
				"mode=%s calendar_timezone=%s log_level=%s archive_dir=%s",
				cfg.Scheduler.Mode,
				cfg.Scheduler.CalendarTimezone,
				cfg.Scheduler.Logs.Level,
				cfg.Scheduler.Logs.ArchiveDir,
			),
			Err: validateDoctorConfig(cfg),
		},
		{
			Name:   "运行态日志级别",
			Detail: logLevelDetail,
			Err:    levelErr,
		},
		{
			Name:   "调度状态",
			Detail: detail,
			Err:    statusErr,
		},
		{
			Name:   "journalctl",
			Detail: lookPathDetail("journalctl"),
			Err:    lookPathErr("journalctl"),
		},
	}

	archiveDetail, archiveErr := inspectArchiveDir(cfg.Scheduler.Logs.ArchiveDir)
	checks = append(checks, doctorCheck{
		Name:   "日志归档目录",
		Detail: archiveDetail,
		Err:    archiveErr,
	})
	archivePolicyDetail, archivePolicyErr := inspectArchivePolicy(cfg.Scheduler.Logs)
	checks = append(checks, doctorCheck{
		Name:   "日志归档策略",
		Detail: archivePolicyDetail,
		Err:    archivePolicyErr,
	})

	if needsSystemdDoctor(probe) {
		checks = append(checks, doctorCheck{
			Name:   "systemctl",
			Detail: lookPathDetail("systemctl"),
			Err:    lookPathErr("systemctl"),
		})
		checks = append(checks, doctorCheck{
			Name:   "loginctl",
			Detail: lookPathDetail("loginctl"),
			Err:    lookPathErr("loginctl"),
		})
		checks = append(checks, inspectLinger(ctx))
		checks = append(checks, inspectUserBus(ctx, cfg))
		checks = append(checks, inspectUnitDrift(cfg))
		checks = append(checks, inspectManagedTimers(ctx, cfg))
		checks = append(checks, inspectManagedServices(ctx, cfg))
	}

	if needsCronDoctor(probe) {
		checks = append(checks, doctorCheck{
			Name:   "crontab",
			Detail: lookPathDetail("crontab"),
			Err:    lookPathErr("crontab"),
		})
	}

	return checks
}

func needsSystemdDoctor(probe Probe) bool {
	switch probe.Requested {
	case "auto", "systemd":
		return true
	default:
		return probe.SystemdInstalled || probe.SystemdActive
	}
}

func needsCronDoctor(probe Probe) bool {
	return probe.Requested == "cron" || probe.CronActive
}

func validateDoctorConfig(cfg *config.Config) error {
	if cfg == nil {
		return fmt.Errorf("配置不能为空")
	}
	return cfg.Validate()
}

func lookPathDetail(name string) string {
	path, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	return path
}

func lookPathErr(name string) error {
	if _, err := exec.LookPath(name); err != nil {
		return fmt.Errorf("未找到 %s: %w", name, err)
	}
	return nil
}

func inspectArchiveDir(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("未配置 archive_dir")
	}
	info, err := os.Stat(path)
	switch {
	case err == nil:
		if !info.IsDir() {
			return path, fmt.Errorf("archive_dir 不是目录")
		}
		if probeWriteDir(path) != nil {
			return path, fmt.Errorf("archive_dir 不可写")
		}
		return path + " (已存在且可写)", nil
	case os.IsNotExist(err):
		parent := nearestExistingParent(path)
		if parent == "" {
			return path, fmt.Errorf("archive_dir 父目录不存在")
		}
		if writeErr := probeWriteDir(parent); writeErr != nil {
			return path, fmt.Errorf("archive_dir 尚未创建，且父目录不可写: %w", writeErr)
		}
		return path + " (尚未创建，首次 --archive 时自动落盘)", nil
	default:
		return path, fmt.Errorf("读取 archive_dir 失败: %w", err)
	}
}

func inspectArchivePolicy(logs config.SchedulerLogsConfig) (string, error) {
	detail := fmt.Sprintf(
		"retention_days=%d max_files=%d compress=%t",
		logs.RetentionDays,
		logs.MaxFiles,
		logs.Compress,
	)
	files, err := listManagedArchiveFiles(logs.ArchiveDir)
	if err != nil {
		return detail, fmt.Errorf("扫描受管归档失败: %w", err)
	}
	compressed := 0
	for _, file := range files {
		if file.Compressed {
			compressed++
		}
	}
	return fmt.Sprintf("%s managed_files=%d compressed=%d", detail, len(files), compressed), nil
}

func inspectUserBus(ctx context.Context, cfg *config.Config) doctorCheck {
	probe := InspectStatus(cfg)
	if UserControlReady(ctx) {
		return doctorCheck{
			Name:   "user bus",
			Detail: "当前会话可直接执行 systemctl --user",
		}
	}
	detail := probe.SystemdReason
	if detail == "" {
		detail = "当前会话未直连 user bus"
	}
	return doctorCheck{
		Name:   "user bus",
		Detail: detail,
		Err:    fmt.Errorf("当前会话未直连 user bus"),
	}
}

func inspectManagedTimers(ctx context.Context, cfg *config.Config) doctorCheck {
	items, err := ListManagedTimers(ctx, cfg)
	if err != nil {
		return doctorCheck{
			Name:   "托管 timers",
			Detail: "无法枚举托管 timer 状态",
			Err:    err,
		}
	}
	unhealthy := make([]string, 0, len(items))
	pendingFirstRun := make([]string, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.ActiveState) != "active" || strings.TrimSpace(item.UnitFileState) != "enabled" {
			unhealthy = append(unhealthy, fmt.Sprintf("%s(active=%s enabled=%s)", item.TimerUnit, item.ActiveState, item.UnitFileState))
			continue
		}
		if !item.HasLast {
			pendingFirstRun = append(pendingFirstRun, item.Key)
		}
	}
	if len(unhealthy) > 0 {
		return doctorCheck{
			Name: "托管 timers",
			Detail: fmt.Sprintf(
				"主机时区=%s 配置时区=%s 异常=%s",
				time.Local.String(),
				effectiveSchedulerTimezone(cfg),
				strings.Join(unhealthy, ", "),
			),
			Repair: "请先执行 `ccclaw scheduler timers --wide` 核对日程，再运行 `systemctl --user daemon-reload && systemctl --user enable --now " + strings.Join(ManagedSystemdTimers(), " ") + "`",
			Err:    fmt.Errorf("存在未启用或未运行的 timer"),
		}
	}
	detail := fmt.Sprintf(
		"%d/%d 已启用且运行中；主机时区=%s 配置时区=%s",
		len(items),
		len(items),
		time.Local.String(),
		effectiveSchedulerTimezone(cfg),
	)
	if len(pendingFirstRun) > 0 {
		detail += "；待首次触发=" + strings.Join(pendingFirstRun, ", ")
	}
	return doctorCheck{
		Name:   "托管 timers",
		Detail: detail,
	}
}

func inspectLinger(ctx context.Context) doctorCheck {
	status, err := InspectLingerStatus(ctx)
	if err != nil {
		return doctorCheck{
			Name:   "linger",
			Detail: "无法读取 loginctl linger 状态",
			Repair: "请确认 `loginctl` 可用，并执行 `loginctl enable-linger " + status.User + "`",
			Err:    err,
		}
	}
	detail := fmt.Sprintf("user=%s state=%s linger=%t", status.User, status.State, status.Enabled)
	if status.Enabled {
		return doctorCheck{
			Name:   "linger",
			Detail: detail,
		}
	}
	return doctorCheck{
		Name:   "linger",
		Detail: detail,
		Repair: "请执行 `loginctl enable-linger " + status.User + "`，避免用户退出登录后 timer 停止",
		Err:    fmt.Errorf("linger 未启用"),
	}
}

func inspectUnitDrift(cfg *config.Config) doctorCheck {
	status, err := DetectManagedUnitDrift(cfg)
	if err != nil {
		return doctorCheck{
			Name:   "unit 漂移",
			Detail: fmt.Sprintf("dir=%s", strings.TrimSpace(status.UnitDir)),
			Repair: "请重新执行 `ccclaw scheduler use systemd`，随后执行 `systemctl --user daemon-reload`",
			Err:    err,
		}
	}
	detail := fmt.Sprintf("dir=%s matched=%d/%d", status.UnitDir, status.Matched, status.Expected)
	if len(status.Missing) == 0 && len(status.Drifted) == 0 {
		return doctorCheck{
			Name:   "unit 漂移",
			Detail: detail,
		}
	}
	parts := []string{detail}
	if len(status.Missing) > 0 {
		parts = append(parts, "missing="+strings.Join(status.Missing, ","))
	}
	if len(status.Drifted) > 0 {
		parts = append(parts, "drift="+strings.Join(status.Drifted, ","))
	}
	return doctorCheck{
		Name:   "unit 漂移",
		Detail: strings.Join(parts, " "),
		Repair: "请重新执行 `ccclaw scheduler use systemd` 让 unit 与当前配置重新对齐",
		Err:    fmt.Errorf("检测到 user systemd 单元与当前配置漂移"),
	}
}

func inspectManagedServices(ctx context.Context, cfg *config.Config) doctorCheck {
	items, err := ListManagedServices(ctx, cfg)
	if err != nil {
		return doctorCheck{
			Name:   "托管 services",
			Detail: "无法枚举托管 service 状态",
			Err:    err,
		}
	}
	abnormal := make([]string, 0, len(items))
	units := make([]string, 0, len(items))
	for _, item := range items {
		if !serviceLooksHealthy(item) {
			abnormal = append(abnormal, fmt.Sprintf("%s(active=%s sub=%s result=%s status=%s)", item.ServiceUnit, item.ActiveState, item.SubState, item.Result, item.ExecMainStatus))
			units = append(units, item.ServiceUnit)
		}
	}
	if len(abnormal) == 0 {
		return doctorCheck{
			Name:   "托管 services",
			Detail: fmt.Sprintf("%d/%d 最近执行状态正常", len(items), len(items)),
		}
	}
	return doctorCheck{
		Name:   "托管 services",
		Detail: "最近异常=" + strings.Join(abnormal, ", "),
		Repair: buildServiceRepairHint(units),
		Err:    fmt.Errorf("存在最近失败或异常退出的 service"),
	}
}

func serviceLooksHealthy(item ServiceStatus) bool {
	if strings.EqualFold(strings.TrimSpace(item.ActiveState), "failed") {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(item.SubState), "failed") {
		return false
	}
	result := strings.ToLower(strings.TrimSpace(item.Result))
	if result != "" && result != "-" && result != "success" {
		return false
	}
	status := strings.TrimSpace(item.ExecMainStatus)
	if status != "" && status != "-" && status != "0" && result != "success" {
		return false
	}
	return true
}

func buildServiceRepairHint(units []string) string {
	if len(units) == 0 {
		return "请先执行 `systemctl --user status` 与 `journalctl --user` 复核异常 service"
	}
	seen := map[string]struct{}{}
	ordered := make([]string, 0, len(units))
	for _, unit := range units {
		if _, ok := seen[unit]; ok {
			continue
		}
		seen[unit] = struct{}{}
		ordered = append(ordered, unit)
	}
	return "请先执行 `systemctl --user status " + strings.Join(ordered, " ") + "` 与 `journalctl --user -u " + strings.Join(ordered, " -u ") + " -n 50 --no-pager` 定位失败原因，再按需重启 timer/service"
}

func nearestExistingParent(path string) string {
	current := filepath.Clean(path)
	for current != "." && current != string(filepath.Separator) {
		current = filepath.Dir(current)
		if info, err := os.Stat(current); err == nil && info.IsDir() {
			return current
		}
	}
	if _, err := os.Stat(string(filepath.Separator)); err == nil {
		return string(filepath.Separator)
	}
	return ""
}

func probeWriteDir(path string) error {
	handle, err := os.CreateTemp(path, ".ccclaw-writecheck-*")
	if err != nil {
		return err
	}
	name := handle.Name()
	if closeErr := handle.Close(); closeErr != nil {
		_ = os.Remove(name)
		return closeErr
	}
	return os.Remove(name)
}
