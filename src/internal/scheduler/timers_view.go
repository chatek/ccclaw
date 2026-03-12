package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/41490/ccclaw/internal/config"
)

type TimersView string

const (
	TimersViewHuman TimersView = "human"
	TimersViewWide  TimersView = "wide"
	TimersViewRaw   TimersView = "raw"
	TimersViewJSON  TimersView = "json"
)

type TimersRenderOptions struct {
	View TimersView
}

type timersJSONPayload struct {
	View           string             `json:"view"`
	HostTimezone   string             `json:"host_timezone"`
	ConfigTimezone string             `json:"config_timezone"`
	Items          []timerJSONPayload `json:"items"`
}

type timerJSONPayload struct {
	Task              string `json:"task"`
	TimerUnit         string `json:"timer_unit"`
	ServiceUnit       string `json:"service_unit"`
	ActiveState       string `json:"active_state"`
	UnitFileState     string `json:"unit_file_state"`
	Result            string `json:"result"`
	CalendarRaw       string `json:"calendar_raw"`
	CalendarEffective string `json:"calendar_effective"`
	NextHost          string `json:"next_host"`
	NextConfig        string `json:"next_config"`
	LastHost          string `json:"last_host"`
	LastConfig        string `json:"last_config"`
}

func ResolveTimersView(wide, raw, asJSON bool) (TimersView, error) {
	selected := 0
	if wide {
		selected++
	}
	if raw {
		selected++
	}
	if asJSON {
		selected++
	}
	if selected > 1 {
		return "", fmt.Errorf("--wide/--raw/--json 只能三选一")
	}
	switch {
	case wide:
		return TimersViewWide, nil
	case raw:
		return TimersViewRaw, nil
	case asJSON:
		return TimersViewJSON, nil
	default:
		return TimersViewHuman, nil
	}
}

func RenderManagedTimers(ctx context.Context, cfg *config.Config, out io.Writer, options TimersRenderOptions) error {
	items, err := ListManagedTimers(ctx, cfg)
	if err != nil {
		return err
	}
	hostTimezone := time.Local.String()
	configTimezone := effectiveSchedulerTimezone(cfg)
	switch options.View {
	case "", TimersViewHuman:
		return renderTimersHuman(out, items, hostTimezone, configTimezone)
	case TimersViewWide:
		return renderTimersWide(out, items, hostTimezone, configTimezone)
	case TimersViewRaw:
		return renderTimersRaw(out, items, hostTimezone, configTimezone)
	case TimersViewJSON:
		return renderTimersJSON(out, items, hostTimezone, configTimezone)
	default:
		return fmt.Errorf("未知 timers 视图: %s", options.View)
	}
}

func effectiveSchedulerTimezone(cfg *config.Config) string {
	if cfg == nil || strings.TrimSpace(cfg.Scheduler.CalendarTimezone) == "" {
		return "Local"
	}
	return strings.TrimSpace(cfg.Scheduler.CalendarTimezone)
}

func writeTimersMeta(out io.Writer, view TimersView, hostTimezone, configTimezone, note string) {
	_, _ = fmt.Fprintf(out, "视图: %s\n", view)
	_, _ = fmt.Fprintf(out, "主机时区: %s\n", hostTimezone)
	_, _ = fmt.Fprintf(out, "配置时区: %s\n", configTimezone)
	if note != "" {
		_, _ = fmt.Fprintf(out, "说明: %s\n", note)
	}
	_, _ = fmt.Fprintln(out)
}

func renderTimersHuman(out io.Writer, items []TimerStatus, hostTimezone, configTimezone string) error {
	writeTimersMeta(out, TimersViewHuman, hostTimezone, configTimezone, "默认视图仅保留关键列；详情用 --wide，原始字段用 --raw，脚本消费用 --json")
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintf(w, "TASK\tACTIVE\tENABLED\tNEXT[%s]\tLAST[%s]\n", configTimezone, configTimezone)
	for _, item := range items {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			item.Key,
			item.ActiveState,
			item.UnitFileState,
			item.NextConfigTZ,
			item.LastConfigTZ,
		)
	}
	return w.Flush()
}

func renderTimersWide(out io.Writer, items []TimerStatus, hostTimezone, configTimezone string) error {
	writeTimersMeta(out, TimersViewWide, hostTimezone, configTimezone, "CAL_RAW 为配置原文，CAL_CFG 为追加配置时区后的生效表达式")
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintf(w, "TASK\tTIMER\tACTIVE\tENABLED\tCAL_RAW\tCAL_CFG\tNEXT[%s]\tNEXT[%s]\tLAST[%s]\tLAST[%s]\n",
		hostTimezone,
		configTimezone,
		hostTimezone,
		configTimezone,
	)
	for _, item := range items {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			item.Key,
			item.TimerUnit,
			item.ActiveState,
			item.UnitFileState,
			item.Calendar,
			item.CalendarWithTZ,
			item.NextLocal,
			item.NextConfigTZ,
			item.LastLocal,
			item.LastConfigTZ,
		)
	}
	return w.Flush()
}

func renderTimersRaw(out io.Writer, items []TimerStatus, hostTimezone, configTimezone string) error {
	writeTimersMeta(out, TimersViewRaw, hostTimezone, configTimezone, "raw 视图保留原始字段名；稳定脚本消费请改用 --json")
	for _, item := range items {
		_, _ = fmt.Fprintf(out, "[%s]\n", item.Key)
		_, _ = fmt.Fprintf(out, "timer_unit=%s\n", item.TimerUnit)
		_, _ = fmt.Fprintf(out, "service_unit=%s\n", item.ServiceUnit)
		_, _ = fmt.Fprintf(out, "active_state=%s\n", item.ActiveState)
		_, _ = fmt.Fprintf(out, "unit_file_state=%s\n", item.UnitFileState)
		_, _ = fmt.Fprintf(out, "result=%s\n", item.Result)
		_, _ = fmt.Fprintf(out, "calendar_raw=%s\n", item.Calendar)
		_, _ = fmt.Fprintf(out, "calendar_effective=%s\n", item.CalendarWithTZ)
		_, _ = fmt.Fprintf(out, "next_host=%s\n", item.NextLocal)
		_, _ = fmt.Fprintf(out, "next_config=%s\n", item.NextConfigTZ)
		_, _ = fmt.Fprintf(out, "last_host=%s\n", item.LastLocal)
		_, _ = fmt.Fprintf(out, "last_config=%s\n\n", item.LastConfigTZ)
	}
	return nil
}

func renderTimersJSON(out io.Writer, items []TimerStatus, hostTimezone, configTimezone string) error {
	payload := timersJSONPayload{
		View:           string(TimersViewJSON),
		HostTimezone:   hostTimezone,
		ConfigTimezone: configTimezone,
		Items:          make([]timerJSONPayload, 0, len(items)),
	}
	for _, item := range items {
		payload.Items = append(payload.Items, timerJSONPayload{
			Task:              item.Key,
			TimerUnit:         item.TimerUnit,
			ServiceUnit:       item.ServiceUnit,
			ActiveState:       item.ActiveState,
			UnitFileState:     item.UnitFileState,
			Result:            item.Result,
			CalendarRaw:       item.Calendar,
			CalendarEffective: item.CalendarWithTZ,
			NextHost:          item.NextLocal,
			NextConfig:        item.NextConfigTZ,
			LastHost:          item.LastLocal,
			LastConfig:        item.LastConfigTZ,
		})
	}
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(payload)
}
