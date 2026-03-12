package logging

import (
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"
)

const (
	LevelDebug   = "debug"
	LevelInfo    = "info"
	LevelWarning = "warning"
	LevelError   = "error"
)

type Logger struct {
	inner *slog.Logger
	level string
}

func New(out io.Writer, rawLevel string) (*Logger, string, error) {
	level, err := NormalizeRuntimeLevel(rawLevel)
	if err != nil {
		return nil, "", err
	}
	if out == nil {
		out = io.Discard
	}
	handler := slog.NewTextHandler(out, &slog.HandlerOptions{
		Level: slogLevel(level),
		ReplaceAttr: func(_ []string, attr slog.Attr) slog.Attr {
			switch attr.Key {
			case slog.TimeKey:
				if value, ok := attr.Value.Any().(time.Time); ok {
					attr.Value = slog.StringValue(value.Format(time.RFC3339))
				}
			case slog.LevelKey:
				attr.Value = slog.StringValue(renderLevel(attr.Value.String()))
			}
			return attr
		},
	})
	return &Logger{
		inner: slog.New(handler),
		level: level,
	}, level, nil
}

func NormalizeRuntimeLevel(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", LevelInfo, "notice":
		return LevelInfo, nil
	case LevelDebug:
		return LevelDebug, nil
	case LevelWarning, "warn":
		return LevelWarning, nil
	case LevelError, "err", "crit", "alert", "emerg":
		return LevelError, nil
	default:
		return "", fmt.Errorf("未知运行态日志级别: %s (允许值: debug|info|warning|error)", strings.TrimSpace(raw))
	}
}

func JournalPriority(raw string) (string, error) {
	trimmed := strings.ToLower(strings.TrimSpace(raw))
	switch trimmed {
	case "":
		return "", nil
	case "error":
		return "err", nil
	case "warn":
		return "warning", nil
	case "emerg", "alert", "crit", "err", "warning", "notice", "info", "debug":
		return trimmed, nil
	default:
		return "", fmt.Errorf("未知日志级别: %s (允许值: emerg|alert|crit|err|warning|notice|info|debug|error)", strings.TrimSpace(raw))
	}
}

func (l *Logger) Level() string {
	if l == nil {
		return ""
	}
	return l.level
}

func (l *Logger) Debug(msg string, args ...any) {
	if l == nil || l.inner == nil {
		return
	}
	l.inner.Debug(msg, args...)
}

func (l *Logger) Info(msg string, args ...any) {
	if l == nil || l.inner == nil {
		return
	}
	l.inner.Info(msg, args...)
}

func (l *Logger) Warning(msg string, args ...any) {
	if l == nil || l.inner == nil {
		return
	}
	l.inner.Warn(msg, args...)
}

func (l *Logger) Error(msg string, args ...any) {
	if l == nil || l.inner == nil {
		return
	}
	l.inner.Error(msg, args...)
}

func slogLevel(level string) slog.Level {
	switch level {
	case LevelDebug:
		return slog.LevelDebug
	case LevelWarning:
		return slog.LevelWarn
	case LevelError:
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func renderLevel(raw string) string {
	level := strings.ToLower(strings.TrimSpace(raw))
	switch level {
	case "warn":
		return LevelWarning
	case "info", "debug", "error":
		return level
	default:
		return level
	}
}
