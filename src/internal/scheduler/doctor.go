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
	Err    error
}

func Doctor(ctx context.Context, cfg *config.Config, out io.Writer) error {
	checks := runDoctorChecks(ctx, cfg)
	failed := 0
	for _, check := range checks {
		if check.Err != nil {
			failed++
			if check.Detail != "" {
				_, _ = fmt.Fprintf(out, "[FAIL] %s: %v (%s)\n", check.Name, check.Err, check.Detail)
			} else {
				_, _ = fmt.Fprintf(out, "[FAIL] %s: %v\n", check.Name, check.Err)
			}
			continue
		}
		if check.Detail != "" {
			_, _ = fmt.Fprintf(out, "[ OK ] %s: %s\n", check.Name, check.Detail)
		} else {
			_, _ = fmt.Fprintf(out, "[ OK ] %s\n", check.Name)
		}
	}
	if failed > 0 {
		return fmt.Errorf("scheduler doctor 失败，共 %d 项异常", failed)
	}
	return nil
}

func runDoctorChecks(ctx context.Context, cfg *config.Config) []doctorCheck {
	probe := InspectStatus(cfg)
	detail, statusErr := summarize(probe)
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
		checks = append(checks, inspectUserBus(ctx, cfg))
		checks = append(checks, inspectManagedTimers(ctx, cfg))
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
	for _, item := range items {
		if strings.TrimSpace(item.ActiveState) != "active" || strings.TrimSpace(item.UnitFileState) != "enabled" {
			unhealthy = append(unhealthy, fmt.Sprintf("%s(active=%s enabled=%s)", item.TimerUnit, item.ActiveState, item.UnitFileState))
		}
	}
	if len(unhealthy) > 0 {
		return doctorCheck{
			Name: "托管 timers",
			Detail: fmt.Sprintf(
				"主机时区=%s 配置时区=%s 异常=%s",
				time.Local.String(),
				cfg.Scheduler.CalendarTimezone,
				strings.Join(unhealthy, ", "),
			),
			Err: fmt.Errorf("存在未启用或未运行的 timer"),
		}
	}
	return doctorCheck{
		Name: "托管 timers",
		Detail: fmt.Sprintf(
			"%d/%d 已启用且运行中；主机时区=%s 配置时区=%s",
			len(items),
			len(items),
			time.Local.String(),
			cfg.Scheduler.CalendarTimezone,
		),
	}
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
