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

func ManagedSystemdTimers() []string {
	return []string{
		"ccclaw-ingest.timer",
		"ccclaw-run.timer",
		"ccclaw-patrol.timer",
		"ccclaw-journal.timer",
	}
}

func managedSystemdUnits() []string {
	return []string{
		"ccclaw-ingest.service",
		"ccclaw-ingest.timer",
		"ccclaw-run.service",
		"ccclaw-run.timer",
		"ccclaw-patrol.service",
		"ccclaw-patrol.timer",
		"ccclaw-journal.service",
		"ccclaw-journal.timer",
	}
}

func EnableSystemd(ctx context.Context, cfg *config.Config) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("配置不能为空")
	}
	if _, err := exec.LookPath("systemctl"); err != nil {
		return "", fmt.Errorf("未找到 systemctl: %w", err)
	}
	if err := installManagedSystemdUnits(cfg); err != nil {
		return "", err
	}
	if _, err := runSystemctlUser(ctx, "daemon-reload"); err != nil {
		return "", err
	}
	args := append([]string{"enable", "--now"}, ManagedSystemdTimers()...)
	if _, err := runSystemctlUser(ctx, args...); err != nil {
		return "", err
	}
	return fmt.Sprintf("已启用 user systemd 定时器: %s", strings.Join(ManagedSystemdTimers(), ", ")), nil
}

func RestartSystemd(ctx context.Context) (string, error) {
	args := append([]string{"restart"}, ManagedSystemdTimers()...)
	if _, err := runSystemctlUser(ctx, args...); err != nil {
		return "", err
	}
	return fmt.Sprintf("已重启 user systemd 定时器: %s", strings.Join(ManagedSystemdTimers(), ", ")), nil
}

func UserControlReady(ctx context.Context) bool {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return false
	}
	_, err := runSystemctlUser(ctx, "show-environment")
	return err == nil
}

func DisableSystemd(ctx context.Context, cfg *config.Config) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("配置不能为空")
	}
	if _, err := exec.LookPath("systemctl"); err != nil {
		if removeErr := removeManagedSystemdUnits(cfg.Scheduler.SystemdUserDir); removeErr != nil {
			return "", removeErr
		}
		return "当前环境缺少 systemctl；已仅清理本地 user systemd 单元文件", nil
	}
	args := append([]string{"disable", "--now"}, ManagedSystemdTimers()...)
	if _, err := runSystemctlUser(ctx, args...); err != nil {
		if !isIgnorableSystemdDisableError(err.Error()) && !isUserBusUnavailable(err.Error()) {
			return "", err
		}
		if err := removeManagedSystemdUnits(cfg.Scheduler.SystemdUserDir); err != nil {
			return "", err
		}
		return "当前会话未直连 user bus；已清理本地 user systemd 单元文件，如登录会话中仍有旧 timer，请在登录会话再次执行清理", nil
	}
	if err := removeManagedSystemdUnits(cfg.Scheduler.SystemdUserDir); err != nil {
		return "", err
	}
	if _, err := runSystemctlUser(ctx, "daemon-reload"); err != nil {
		return "", err
	}
	return "已停用并清理 ccclaw user systemd 定时器", nil
}

func installManagedSystemdUnits(cfg *config.Config) error {
	if err := os.MkdirAll(cfg.Scheduler.SystemdUserDir, 0o755); err != nil {
		return fmt.Errorf("创建 user systemd 目录失败: %w", err)
	}
	units, err := GenerateSystemdUnitContents(cfg)
	if err != nil {
		return err
	}
	for name, content := range units {
		if err := os.WriteFile(filepath.Join(cfg.Scheduler.SystemdUserDir, name), []byte(content), 0o644); err != nil {
			return fmt.Errorf("写入 user systemd 单元失败: %w", err)
		}
	}
	return nil
}

func removeManagedSystemdUnits(unitDir string) error {
	if strings.TrimSpace(unitDir) == "" {
		return nil
	}
	for _, name := range managedSystemdUnits() {
		path := filepath.Join(unitDir, name)
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("删除 user systemd 单元失败: %w", err)
		}
	}
	return nil
}

type TimerStatus struct {
	Key            string
	TimerUnit      string
	ServiceUnit    string
	ActiveState    string
	UnitFileState  string
	Result         string
	Calendar       string
	CalendarWithTZ string
	NextLocal      string
	NextConfigTZ   string
	LastLocal      string
	LastConfigTZ   string
}

func ListManagedTimers(ctx context.Context, cfg *config.Config) ([]TimerStatus, error) {
	defs, err := ManagedTimerDefinitions(cfg)
	if err != nil {
		return nil, err
	}
	location, err := schedulerLocation(cfg)
	if err != nil {
		return nil, err
	}
	items := make([]TimerStatus, 0, len(defs))
	for _, def := range defs {
		props, err := ShowTimerProperties(ctx, def.TimerUnit)
		if err != nil {
			return nil, err
		}
		nextTime, err := parseSystemdTimestamp(props["NextElapseUSecRealtime"])
		if err != nil {
			return nil, err
		}
		lastTime, err := parseSystemdTimestamp(props["LastTriggerUSec"])
		if err != nil {
			return nil, err
		}
		items = append(items, TimerStatus{
			Key:            def.Key,
			TimerUnit:      def.TimerUnit,
			ServiceUnit:    def.ServiceUnit,
			ActiveState:    fallbackValue(props["ActiveState"], "-"),
			UnitFileState:  fallbackValue(props["UnitFileState"], "-"),
			Result:         fallbackValue(props["Result"], "-"),
			Calendar:       def.Calendar,
			CalendarWithTZ: def.CalendarWithTZ,
			NextLocal:      formatTimerStamp(nextTime, time.Local),
			NextConfigTZ:   formatTimerStamp(nextTime, location),
			LastLocal:      formatTimerStamp(lastTime, time.Local),
			LastConfigTZ:   formatTimerStamp(lastTime, location),
		})
	}
	return items, nil
}

func ShowTimerProperties(ctx context.Context, timer string) (map[string]string, error) {
	args := []string{"show", timer, "-p", "Id", "-p", "ActiveState", "-p", "UnitFileState", "-p", "NextElapseUSecRealtime", "-p", "LastTriggerUSec", "-p", "Result", "-p", "Triggers"}
	output, err := runSystemctlUser(ctx, args...)
	if err != nil {
		return nil, err
	}
	properties := map[string]string{}
	for _, line := range strings.Split(output, "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), "=", 2)
		if len(parts) != 2 {
			continue
		}
		properties[parts[0]] = strings.TrimSpace(parts[1])
	}
	return properties, nil
}

func parseSystemdTimestamp(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" || value == "-" {
		return time.Time{}, nil
	}
	for _, layout := range []string{"Mon 2006-01-02 15:04:05 MST", "Mon 2006-01-02 15:04:05 -0700"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("解析 systemd 时间失败: %s", value)
}

func formatTimerStamp(value time.Time, location *time.Location) string {
	if value.IsZero() {
		return "-"
	}
	if location == nil {
		location = time.Local
	}
	return value.In(location).Format("2006-01-02 15:04:05 MST")
}

func runSystemctlUser(ctx context.Context, args ...string) (string, error) {
	runCtx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()
	fullArgs := append([]string{"--user"}, args...)
	cmd := exec.CommandContext(runCtx, "systemctl", fullArgs...)
	output, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(output))
	if runCtx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("执行 `systemctl --user %s` 超时", strings.Join(args, " "))
	}
	if err != nil {
		if text == "" {
			text = err.Error()
		}
		return "", fmt.Errorf("执行 `systemctl --user %s` 失败: %s", strings.Join(args, " "), text)
	}
	return text, nil
}

func isIgnorableSystemdDisableError(message string) bool {
	return strings.Contains(message, "not loaded") ||
		strings.Contains(message, "not found") ||
		strings.Contains(message, "No such file") ||
		strings.Contains(message, "does not exist")
}

func fallbackValue(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
