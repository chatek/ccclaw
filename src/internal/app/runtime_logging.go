package app

import (
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/41490/ccclaw/internal/adapters/github"
	"github.com/41490/ccclaw/internal/adapters/reporter"
	"github.com/41490/ccclaw/internal/adapters/storage"
	"github.com/41490/ccclaw/internal/config"
	"github.com/41490/ccclaw/internal/executor"
	"github.com/41490/ccclaw/internal/logging"
	"github.com/41490/ccclaw/internal/memory"
	"github.com/41490/ccclaw/internal/vcs"
)

type RuntimeOptions struct {
	LogWriter        io.Writer
	LogLevelOverride string
}

var runtimeLogFixedKeys = []string{
	"entry",
	"task_id",
	"issue",
	"target_repo",
	"session_id",
}

func NewRuntimeWithOptions(configPath, envFile string, options RuntimeOptions) (*Runtime, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, err
	}
	if envFile == "" {
		envFile = cfg.Paths.EnvFile
	}
	secrets, err := config.LoadSecrets(envFile)
	if err != nil {
		return nil, err
	}
	store, err := storage.Open(cfg.Paths.StateDB)
	if err != nil {
		return nil, err
	}
	mem, err := memory.Build(cfg.Paths.KBDir)
	if err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("构建 kb 索引失败: %w", err)
	}

	logger, level, err := logging.New(options.LogWriter, chooseRuntimeLogLevel(cfg, options.LogLevelOverride))
	if err != nil {
		_ = store.Close()
		return nil, err
	}

	rt := &Runtime{
		cfg:      cfg,
		secrets:  secrets,
		store:    store,
		mem:      mem,
		memRoot:  cfg.Paths.KBDir,
		memCache: map[string]*memory.Index{cfg.Paths.KBDir: mem},
		ghCache:  map[string]*github.Client{},
		syncRepo: vcs.SyncRepo,
		log:      logger,
		logLevel: level,
	}
	rt.rep = reporter.New(rt.clientForRepo)
	return rt, nil
}

func chooseRuntimeLogLevel(cfg *config.Config, override string) string {
	if trimmed := strings.TrimSpace(override); trimmed != "" {
		return trimmed
	}
	if cfg == nil {
		return logging.LevelInfo
	}
	return cfg.Scheduler.Logs.Level
}

func (rt *Runtime) runtimeLogLevel() string {
	if rt == nil {
		return ""
	}
	return rt.logLevel
}

func (rt *Runtime) logDebug(entry, msg string, args ...any) {
	rt.logWithLevel(logging.LevelDebug, entry, msg, args...)
}

func (rt *Runtime) logInfo(entry, msg string, args ...any) {
	rt.logWithLevel(logging.LevelInfo, entry, msg, args...)
}

func (rt *Runtime) logWarning(entry, msg string, args ...any) {
	rt.logWithLevel(logging.LevelWarning, entry, msg, args...)
}

func (rt *Runtime) logError(entry, msg string, args ...any) {
	rt.logWithLevel(logging.LevelError, entry, msg, args...)
}

func (rt *Runtime) logWithLevel(level, entry, msg string, args ...any) {
	if rt == nil || rt.log == nil {
		return
	}
	fields := normalizeRuntimeLogFields(entry, args...)
	switch level {
	case logging.LevelDebug:
		rt.log.Debug(msg, fields...)
	case logging.LevelWarning:
		rt.log.Warning(msg, fields...)
	case logging.LevelError:
		rt.log.Error(msg, fields...)
	default:
		rt.log.Info(msg, fields...)
	}
}

func normalizeRuntimeLogFields(entry string, args ...any) []any {
	fixed := map[string]any{}
	if trimmed := strings.TrimSpace(entry); trimmed != "" {
		fixed["entry"] = trimmed
	}

	extras := make([]any, 0, len(args))
	for idx := 0; idx < len(args); idx++ {
		key, ok := args[idx].(string)
		if !ok || idx+1 >= len(args) {
			extras = append(extras, args[idx])
			continue
		}

		normalizedKey := strings.TrimSpace(strings.ToLower(key))
		value := args[idx+1]
		if slices.Contains(runtimeLogFixedKeys, normalizedKey) {
			if text := strings.TrimSpace(fmt.Sprint(value)); text != "" {
				fixed[normalizedKey] = value
			}
		} else {
			extras = append(extras, key, value)
		}
		idx++
	}

	fields := make([]any, 0, len(fixed)*2+len(extras))
	for _, key := range runtimeLogFixedKeys {
		if value, ok := fixed[key]; ok {
			fields = append(fields, key, value)
		}
	}
	fields = append(fields, extras...)
	return fields
}

func (rt *Runtime) issueRef(repo string, number int) string {
	repo = strings.TrimSpace(repo)
	if repo == "" && rt != nil && rt.cfg != nil {
		repo = rt.cfg.GitHub.ControlRepo
	}
	return fmt.Sprintf("%s#%d", repo, number)
}

func resultDuration(result *executor.Result) string {
	if result == nil || result.Duration <= 0 {
		return "0s"
	}
	return result.Duration.Round(time.Second).String()
}
