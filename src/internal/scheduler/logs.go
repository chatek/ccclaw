package scheduler

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

type LogsOptions struct {
	Scope  string
	Follow bool
	Since  string
	Lines  int
}

func StreamLogs(ctx context.Context, options LogsOptions, stdout, stderr io.Writer) error {
	if _, err := exec.LookPath("journalctl"); err != nil {
		return fmt.Errorf("未找到 journalctl: %w", err)
	}
	units, err := managedLogUnits(options.Scope)
	if err != nil {
		return err
	}
	args := []string{"--user", "--no-pager"}
	if options.Lines <= 0 {
		options.Lines = 50
	}
	args = append(args, "-n", fmt.Sprintf("%d", options.Lines))
	if strings.TrimSpace(options.Since) != "" {
		args = append(args, "--since", options.Since)
	}
	if options.Follow {
		args = append(args, "-f")
	}
	for _, unit := range units {
		args = append(args, "-u", unit)
	}
	cmd := exec.CommandContext(ctx, "journalctl", args...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Stdin = strings.NewReader("")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("执行 journalctl 失败: %w", err)
	}
	return nil
}

func managedLogUnits(scope string) ([]string, error) {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case "", "all":
		return []string{
			"ccclaw-ingest.service",
			"ccclaw-run.service",
			"ccclaw-patrol.service",
			"ccclaw-journal.service",
		}, nil
	case "ingest":
		return []string{"ccclaw-ingest.service"}, nil
	case "run":
		return []string{"ccclaw-run.service"}, nil
	case "patrol":
		return []string{"ccclaw-patrol.service"}, nil
	case "journal":
		return []string{"ccclaw-journal.service"}, nil
	default:
		return nil, fmt.Errorf("未知日志范围: %s (允许值: all|ingest|run|patrol|journal)", scope)
	}
}
