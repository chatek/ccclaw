package scheduler

import (
	"context"
	"fmt"
	"strings"

	"github.com/41490/ccclaw/internal/config"
)

type UseResult struct {
	Mode  string
	Steps []string
}

func Use(ctx context.Context, cfg *config.Config, mode string) (UseResult, error) {
	result := UseResult{Mode: strings.TrimSpace(mode)}
	if cfg == nil {
		return result, fmt.Errorf("配置不能为空")
	}
	switch result.Mode {
	case "systemd":
		step, err := DisableCron(ctx)
		if err != nil {
			return result, err
		}
		result.Steps = append(result.Steps, step)
		step, err = EnableSystemd(ctx, cfg)
		if err != nil {
			return result, err
		}
		result.Steps = append(result.Steps, step)
	case "cron":
		step, err := DisableSystemd(ctx, cfg)
		if err != nil {
			return result, err
		}
		result.Steps = append(result.Steps, step)
		step, err = EnableCron(ctx, cfg)
		if err != nil {
			return result, err
		}
		result.Steps = append(result.Steps, step)
	case "none":
		step, err := DisableSystemd(ctx, cfg)
		if err != nil {
			return result, err
		}
		result.Steps = append(result.Steps, step)
		step, err = DisableCron(ctx)
		if err != nil {
			return result, err
		}
		result.Steps = append(result.Steps, step)
	default:
		return result, fmt.Errorf("不支持的调度模式: %s", mode)
	}
	cfg.Scheduler.Mode = result.Mode
	return result, nil
}
