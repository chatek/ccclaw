package scheduler

import (
	"fmt"
	"strings"
	"time"

	"github.com/41490/ccclaw/internal/config"
)

type TimerDefinition struct {
	Key            string
	TimerUnit      string
	ServiceUnit    string
	ServiceCommand string
	ServiceDesc    string
	TimerDesc      string
	Calendar       string
	CalendarWithTZ string
}

func ManagedTimerDefinitions(cfg *config.Config) ([]TimerDefinition, error) {
	if cfg == nil {
		return nil, fmt.Errorf("配置不能为空")
	}
	location, err := schedulerLocation(cfg)
	if err != nil {
		return nil, err
	}
	timers := cfg.Scheduler.Timers
	defs := []TimerDefinition{
		{
			Key:            "ingest",
			TimerUnit:      "ccclaw-ingest.timer",
			ServiceUnit:    "ccclaw-ingest.service",
			ServiceCommand: "ingest",
			ServiceDesc:    "ccclaw ingest service",
			TimerDesc:      "Run ccclaw ingest on schedule",
			Calendar:       timers.Ingest,
		},
		{
			Key:            "run",
			TimerUnit:      "ccclaw-run.timer",
			ServiceUnit:    "ccclaw-run.service",
			ServiceCommand: "run",
			ServiceDesc:    "ccclaw run service",
			TimerDesc:      "Run ccclaw worker on schedule",
			Calendar:       timers.Run,
		},
		{
			Key:            "patrol",
			TimerUnit:      "ccclaw-patrol.timer",
			ServiceUnit:    "ccclaw-patrol.service",
			ServiceCommand: "patrol",
			ServiceDesc:    "ccclaw patrol service",
			TimerDesc:      "Run ccclaw patrol on schedule",
			Calendar:       timers.Patrol,
		},
		{
			Key:            "journal",
			TimerUnit:      "ccclaw-journal.timer",
			ServiceUnit:    "ccclaw-journal.service",
			ServiceCommand: "journal",
			ServiceDesc:    "ccclaw journal service",
			TimerDesc:      "Run ccclaw journal on schedule",
			Calendar:       timers.Journal,
		},
	}
	for idx := range defs {
		defs[idx].CalendarWithTZ = calendarWithTimezone(defs[idx].Calendar, location.String())
	}
	return defs, nil
}

func GenerateSystemdUnitContents(cfg *config.Config) (map[string]string, error) {
	defs, err := ManagedTimerDefinitions(cfg)
	if err != nil {
		return nil, err
	}
	configPath := config.ExpandPath(fmt.Sprintf("%s/ops/config/config.toml", cfg.Paths.AppDir))
	envFile := config.ExpandPath(cfg.Paths.EnvFile)
	workDir := config.ExpandPath(cfg.Paths.AppDir)
	units := make(map[string]string, len(defs)*2)
	for _, def := range defs {
		units[def.ServiceUnit] = strings.Join([]string{
			"[Unit]",
			fmt.Sprintf("Description=%s", def.ServiceDesc),
			"After=network-online.target",
			"",
			"[Service]",
			"Type=oneshot",
			fmt.Sprintf("WorkingDirectory=%s", workDir),
			fmt.Sprintf("ExecStart=%s/bin/ccclaw %s --config %s --env-file %s", workDir, def.ServiceCommand, configPath, envFile),
			"",
		}, "\n")
		units[def.TimerUnit] = strings.Join([]string{
			"[Unit]",
			fmt.Sprintf("Description=%s", def.TimerDesc),
			"",
			"[Timer]",
			fmt.Sprintf("OnCalendar=%s", def.CalendarWithTZ),
			"Persistent=true",
			fmt.Sprintf("Unit=%s", def.ServiceUnit),
			"",
			"[Install]",
			"WantedBy=timers.target",
			"",
		}, "\n")
	}
	return units, nil
}

func schedulerLocation(cfg *config.Config) (*time.Location, error) {
	if cfg == nil || strings.TrimSpace(cfg.Scheduler.CalendarTimezone) == "" {
		return time.Local, nil
	}
	location, err := time.LoadLocation(cfg.Scheduler.CalendarTimezone)
	if err != nil {
		return nil, fmt.Errorf("加载调度时区失败: %w", err)
	}
	return location, nil
}

func calendarWithTimezone(calendar, timezone string) string {
	calendar = strings.TrimSpace(calendar)
	timezone = strings.TrimSpace(timezone)
	if calendar == "" || timezone == "" || hasExplicitCalendarTimezone(calendar) {
		return calendar
	}
	return calendar + " " + timezone
}

func hasExplicitCalendarTimezone(calendar string) bool {
	fields := strings.Fields(strings.TrimSpace(calendar))
	if len(fields) == 0 {
		return false
	}
	last := fields[len(fields)-1]
	if last == "UTC" {
		return true
	}
	return strings.Contains(last, "/")
}
