package tmux

import (
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

var ErrSessionNotFound = errors.New("tmux 会话不存在")

type SessionSpec struct {
	Name    string
	WorkDir string
	Command string
	LogFile string
}

type SessionStatus struct {
	Name      string
	CreatedAt time.Time
	PaneDead  bool
	ExitCode  int
	PanePID   int
}

type Manager interface {
	Launch(spec SessionSpec) error
	Status(name string) (*SessionStatus, error)
	CaptureOutput(name string, lines int) (string, error)
	Kill(name string) error
	List(prefix string) ([]SessionStatus, error)
}

type ExecManager struct {
	binary string
}

func Available(binary string) bool {
	if binary == "" {
		binary = "tmux"
	}
	_, err := exec.LookPath(binary)
	return err == nil
}

func New(binary string) (*ExecManager, error) {
	if binary == "" {
		binary = "tmux"
	}
	if _, err := exec.LookPath(binary); err != nil {
		return nil, fmt.Errorf("未找到 tmux: %w", err)
	}
	return &ExecManager{binary: binary}, nil
}

func (m *ExecManager) Launch(spec SessionSpec) error {
	if spec.Name == "" {
		return errors.New("tmux session name 不能为空")
	}
	if spec.WorkDir == "" {
		return errors.New("tmux workdir 不能为空")
	}
	if spec.Command == "" {
		return errors.New("tmux command 不能为空")
	}
	if _, err := m.run("new-session", "-d", "-s", spec.Name, "-c", spec.WorkDir); err != nil {
		return err
	}
	if _, err := m.run("set-option", "-t", spec.Name, "remain-on-exit", "on"); err != nil {
		return err
	}
	if _, err := m.run("set-option", "-t", spec.Name, "history-limit", "50000"); err != nil {
		return err
	}
	if strings.TrimSpace(spec.LogFile) != "" {
		pipeCommand := fmt.Sprintf("cat >> %s", shellQuote(spec.LogFile))
		if _, err := m.run("pipe-pane", "-t", spec.Name, "-o", pipeCommand); err != nil {
			return err
		}
	}
	if _, err := m.run("send-keys", "-t", spec.Name, spec.Command, "Enter"); err != nil {
		return err
	}
	return nil
}

func (m *ExecManager) Status(name string) (*SessionStatus, error) {
	output, err := m.run("list-panes", "-t", name, "-F", "#{session_name}\t#{session_created}\t#{pane_dead}\t#{pane_dead_status}\t#{pane_pid}")
	if err != nil {
		if isMissingSession(err) {
			return nil, ErrSessionNotFound
		}
		return nil, err
	}
	items, err := parseStatuses(output)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, ErrSessionNotFound
	}
	return &items[0], nil
}

func (m *ExecManager) CaptureOutput(name string, lines int) (string, error) {
	if lines <= 0 {
		lines = 200
	}
	output, err := m.run("capture-pane", "-p", "-t", name, "-S", fmt.Sprintf("-%d", lines))
	if err != nil {
		if isMissingSession(err) {
			return "", ErrSessionNotFound
		}
		return "", err
	}
	return strings.TrimSpace(output), nil
}

func (m *ExecManager) Kill(name string) error {
	_, err := m.run("kill-session", "-t", name)
	if err != nil && isMissingSession(err) {
		return nil
	}
	return err
}

func (m *ExecManager) List(prefix string) ([]SessionStatus, error) {
	output, err := m.run("list-panes", "-a", "-F", "#{session_name}\t#{session_created}\t#{pane_dead}\t#{pane_dead_status}\t#{pane_pid}")
	if err != nil {
		if isNoServer(err) {
			return nil, nil
		}
		return nil, err
	}
	items, err := parseStatuses(output)
	if err != nil {
		return nil, err
	}
	if prefix == "" {
		return items, nil
	}
	filtered := make([]SessionStatus, 0, len(items))
	for _, item := range items {
		if strings.HasPrefix(item.Name, prefix) {
			filtered = append(filtered, item)
		}
	}
	return filtered, nil
}

func (m *ExecManager) run(args ...string) (string, error) {
	cmd := exec.Command(m.binary, args...)
	output, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(output))
	if err != nil {
		if text == "" {
			text = err.Error()
		}
		return "", fmt.Errorf("tmux %s 失败: %s", strings.Join(args, " "), text)
	}
	return text, nil
}

func parseStatuses(raw string) ([]SessionStatus, error) {
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	statuses := make([]SessionStatus, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) != 5 {
			return nil, fmt.Errorf("无法解析 tmux 状态行: %s", line)
		}
		createdUnix, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("解析 tmux created_at 失败: %w", err)
		}
		exitCode, err := strconv.Atoi(parts[3])
		if err != nil {
			return nil, fmt.Errorf("解析 tmux exit code 失败: %w", err)
		}
		panePID, err := strconv.Atoi(parts[4])
		if err != nil {
			return nil, fmt.Errorf("解析 tmux pane pid 失败: %w", err)
		}
		statuses = append(statuses, SessionStatus{
			Name:      parts[0],
			CreatedAt: time.Unix(createdUnix, 0),
			PaneDead:  parts[2] == "1",
			ExitCode:  exitCode,
			PanePID:   panePID,
		})
	}
	return statuses, nil
}

func isMissingSession(err error) bool {
	text := err.Error()
	return strings.Contains(text, "can't find session") || strings.Contains(text, "找不到 session")
}

func isNoServer(err error) bool {
	text := err.Error()
	return strings.Contains(text, "no server running") || strings.Contains(text, "没有正在运行的 server")
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}
