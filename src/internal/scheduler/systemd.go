package scheduler

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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
	sourceDir := filepath.Join(cfg.Paths.AppDir, "ops", "systemd")
	if info, err := os.Stat(sourceDir); err != nil || !info.IsDir() {
		return fmt.Errorf("缺少 user systemd 模板目录: %s", sourceDir)
	}
	if err := os.MkdirAll(cfg.Scheduler.SystemdUserDir, 0o755); err != nil {
		return fmt.Errorf("创建 user systemd 目录失败: %w", err)
	}
	for _, name := range managedSystemdUnits() {
		if err := copyFile(filepath.Join(sourceDir, name), filepath.Join(cfg.Scheduler.SystemdUserDir, name)); err != nil {
			return err
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

func copyFile(src, dst string) error {
	input, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("打开模板失败: %w", err)
	}
	defer input.Close()

	output, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("写入 user systemd 单元失败: %w", err)
	}
	defer output.Close()

	if _, err := io.Copy(output, input); err != nil {
		return fmt.Errorf("复制 user systemd 单元失败: %w", err)
	}
	if err := output.Chmod(0o644); err != nil {
		return fmt.Errorf("设置 user systemd 单元权限失败: %w", err)
	}
	return nil
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
