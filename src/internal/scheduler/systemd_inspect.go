package scheduler

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/41490/ccclaw/internal/config"
)

type ServiceStatus struct {
	Key            string
	TimerUnit      string
	ServiceUnit    string
	ActiveState    string
	SubState       string
	UnitFileState  string
	Result         string
	ExecMainCode   string
	ExecMainStatus string
}

type LingerStatus struct {
	User    string
	State   string
	Enabled bool
}

type UnitDriftStatus struct {
	UnitDir  string
	Missing  []string
	Drifted  []string
	Matched  int
	Expected int
}

func ListManagedServices(ctx context.Context, cfg *config.Config) ([]ServiceStatus, error) {
	defs, err := ManagedTimerDefinitions(cfg)
	if err != nil {
		return nil, err
	}
	items := make([]ServiceStatus, 0, len(defs))
	for _, def := range defs {
		props, err := showUnitProperties(ctx, def.ServiceUnit,
			"Id",
			"ActiveState",
			"SubState",
			"UnitFileState",
			"Result",
			"ExecMainCode",
			"ExecMainStatus",
		)
		if err != nil {
			return nil, err
		}
		items = append(items, ServiceStatus{
			Key:            def.Key,
			TimerUnit:      def.TimerUnit,
			ServiceUnit:    def.ServiceUnit,
			ActiveState:    fallbackValue(props["ActiveState"], "-"),
			SubState:       fallbackValue(props["SubState"], "-"),
			UnitFileState:  fallbackValue(props["UnitFileState"], "-"),
			Result:         fallbackValue(props["Result"], "-"),
			ExecMainCode:   fallbackValue(props["ExecMainCode"], "-"),
			ExecMainStatus: fallbackValue(props["ExecMainStatus"], "-"),
		})
	}
	return items, nil
}

func InspectLingerStatus(ctx context.Context) (LingerStatus, error) {
	uid := strconv.Itoa(os.Getuid())
	name := currentUserName()
	output, err := runLoginctl(ctx, "show-user", uid, "-p", "Linger", "-p", "State")
	if err != nil {
		return LingerStatus{User: name}, err
	}
	props := map[string]string{}
	for _, line := range strings.Split(output, "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), "=", 2)
		if len(parts) != 2 {
			continue
		}
		props[parts[0]] = strings.TrimSpace(parts[1])
	}
	enabled := strings.EqualFold(props["Linger"], "yes")
	return LingerStatus{
		User:    name,
		State:   fallbackValue(props["State"], "-"),
		Enabled: enabled,
	}, nil
}

func DetectManagedUnitDrift(cfg *config.Config) (UnitDriftStatus, error) {
	if cfg == nil {
		return UnitDriftStatus{}, fmt.Errorf("配置不能为空")
	}
	expected, err := GenerateSystemdUnitContents(cfg)
	if err != nil {
		return UnitDriftStatus{}, err
	}
	status := UnitDriftStatus{
		UnitDir:  strings.TrimSpace(cfg.Scheduler.SystemdUserDir),
		Expected: len(expected),
	}
	if status.UnitDir == "" {
		return status, fmt.Errorf("systemd_user_dir 不能为空")
	}
	for name, want := range expected {
		path := filepath.Join(status.UnitDir, name)
		body, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				status.Missing = append(status.Missing, name)
				continue
			}
			return status, fmt.Errorf("读取 unit 失败 %s: %w", path, err)
		}
		if normalizeUnitContent(string(body)) != normalizeUnitContent(want) {
			status.Drifted = append(status.Drifted, name)
			continue
		}
		status.Matched++
	}
	return status, nil
}

func normalizeUnitContent(content string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	return strings.TrimSpace(content)
}

func currentUserName() string {
	if name := strings.TrimSpace(os.Getenv("USER")); name != "" {
		return name
	}
	info, err := user.Current()
	if err != nil {
		return strconv.Itoa(os.Getuid())
	}
	return info.Username
}

func runLoginctl(ctx context.Context, args ...string) (string, error) {
	runCtx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, "loginctl", args...)
	output, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(output))
	if runCtx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("执行 `loginctl %s` 超时", strings.Join(args, " "))
	}
	if err != nil {
		if text == "" {
			text = err.Error()
		}
		return "", fmt.Errorf("执行 `loginctl %s` 失败: %s", strings.Join(args, " "), text)
	}
	return text, nil
}
