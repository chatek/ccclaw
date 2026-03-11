package scheduler

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/41490/ccclaw/internal/config"
)

const (
	ManagedCronBegin = "# >>> ccclaw managed cron >>>"
	ManagedCronEnd   = "# <<< ccclaw managed cron <<<"
	commandTimeout   = 5 * time.Second
)

func ManagedCronCommands(cfg *config.Config) []string {
	appBin := filepath.ToSlash(filepath.Join(cfg.Paths.AppDir, "bin", "ccclaw"))
	configPath := filepath.ToSlash(filepath.Join(cfg.Paths.AppDir, "ops", "config", "config.toml"))
	envFile := filepath.ToSlash(cfg.Paths.EnvFile)
	return []string{
		fmt.Sprintf("*/5 * * * * %s ingest --config %s --env-file %s", appBin, configPath, envFile),
		fmt.Sprintf("*/10 * * * * %s run --config %s --env-file %s", appBin, configPath, envFile),
		fmt.Sprintf("*/2 * * * * %s patrol --config %s --env-file %s", appBin, configPath, envFile),
		fmt.Sprintf("50 23 * * * %s journal --config %s --env-file %s", appBin, configPath, envFile),
	}
}

func ManagedCronBlock(cfg *config.Config) string {
	lines := make([]string, 0, 6)
	lines = append(lines, ManagedCronBegin)
	lines = append(lines, ManagedCronCommands(cfg)...)
	lines = append(lines, ManagedCronEnd)
	return strings.Join(lines, "\n") + "\n"
}

func ContainsManagedCron(content string, cfg *config.Config) bool {
	if !strings.Contains(content, ManagedCronBegin) || !strings.Contains(content, ManagedCronEnd) {
		return false
	}
	for _, command := range ManagedCronCommands(cfg) {
		if !strings.Contains(content, command) {
			return false
		}
	}
	return true
}

func StripManagedCron(content string) (string, bool) {
	if strings.TrimSpace(content) == "" {
		return "", false
	}
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	kept := make([]string, 0, len(lines))
	skip := false
	found := false
	for _, line := range lines {
		switch line {
		case ManagedCronBegin:
			skip = true
			found = true
			continue
		case ManagedCronEnd:
			skip = false
			continue
		}
		if skip {
			continue
		}
		kept = append(kept, line)
	}
	trimmed := strings.TrimSpace(strings.Join(kept, "\n"))
	if trimmed == "" {
		return "", found
	}
	return trimmed + "\n", found
}

func EnableCron(ctx context.Context, cfg *config.Config) (string, error) {
	current, hadCrontab, err := readCurrentCrontab(ctx)
	if err != nil {
		return "", err
	}
	base, hadManaged := StripManagedCron(current)
	desired := mergeManagedCron(base, ManagedCronBlock(cfg))
	if normalizeCronContent(current) == normalizeCronContent(desired) {
		return "当前用户 crontab 已包含最新 ccclaw 受控规则", nil
	}
	if err := writeCrontab(ctx, desired); err != nil {
		return "", err
	}
	switch {
	case hadManaged:
		return "已更新当前用户 crontab 中的 ccclaw 受控规则", nil
	case hadCrontab && strings.TrimSpace(base) != "":
		return "已向当前用户 crontab 追加 ccclaw 受控规则，并保留原有非受控规则", nil
	default:
		return "已写入当前用户首份 ccclaw 受控 crontab 规则", nil
	}
}

func DisableCron(ctx context.Context) (string, error) {
	current, hadCrontab, err := readCurrentCrontab(ctx)
	if err != nil {
		if strings.Contains(err.Error(), "未找到 crontab") {
			return "当前环境缺少 crontab，无需清理 ccclaw 受控规则", nil
		}
		return "", err
	}
	if !hadCrontab {
		return "当前用户未配置 crontab，无需清理", nil
	}
	trimmed, hadManaged := StripManagedCron(current)
	if !hadManaged {
		return "当前用户 crontab 中未发现 ccclaw 受控规则", nil
	}
	if normalizeCronContent(current) == normalizeCronContent(trimmed) {
		return "当前用户 crontab 中未发现 ccclaw 受控规则", nil
	}
	if err := writeCrontab(ctx, trimmed); err != nil {
		return "", err
	}
	if strings.TrimSpace(trimmed) == "" {
		return "已清理 ccclaw 受控 crontab 规则；当前用户 crontab 已为空", nil
	}
	return "已清理 ccclaw 受控 crontab 规则，并保留其他用户规则", nil
}

func mergeManagedCron(base, managed string) string {
	base = normalizeCronContent(base)
	managed = normalizeCronContent(managed)
	switch {
	case base == "":
		return managed
	case managed == "":
		return base
	default:
		return base + "\n" + managed
	}
}

func normalizeCronContent(content string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	return content + "\n"
}

func readCurrentCrontab(ctx context.Context) (string, bool, error) {
	if _, err := exec.LookPath("crontab"); err != nil {
		return "", false, fmt.Errorf("未找到 crontab: %w", err)
	}
	runCtx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, "crontab", "-l")
	output, err := cmd.CombinedOutput()
	if runCtx.Err() == context.DeadlineExceeded {
		return "", false, fmt.Errorf("执行 `crontab -l` 超时")
	}
	if err != nil {
		text := strings.TrimSpace(string(output))
		if strings.Contains(text, "no crontab for") || strings.Contains(text, "没有 crontab") {
			return "", false, nil
		}
		if text == "" {
			text = err.Error()
		}
		return "", false, fmt.Errorf("读取当前 crontab 失败: %s", text)
	}
	return normalizeCronContent(string(output)), true, nil
}

func writeCrontab(ctx context.Context, content string) error {
	if _, err := exec.LookPath("crontab"); err != nil {
		return fmt.Errorf("未找到 crontab: %w", err)
	}
	runCtx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()
	payload := normalizeCronContent(content)
	if payload == "" {
		payload = "\n"
	}
	cmd := exec.CommandContext(runCtx, "crontab", "-")
	cmd.Stdin = strings.NewReader(payload)
	output, err := cmd.CombinedOutput()
	if runCtx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("执行 `crontab -` 超时")
	}
	if err != nil {
		text := strings.TrimSpace(string(output))
		if text == "" {
			text = err.Error()
		}
		return fmt.Errorf("写入 crontab 失败: %s", text)
	}
	return nil
}
