package config

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/spf13/viper"
)

type Config struct {
	DefaultTarget string         `mapstructure:"default_target" toml:"default_target"`
	GitHub        GitHubConfig   `mapstructure:"github" toml:"github"`
	Paths         PathsConfig    `mapstructure:"paths" toml:"paths"`
	Executor      ExecutorConfig `mapstructure:"executor" toml:"executor"`
	Approval      ApprovalConfig `mapstructure:"approval" toml:"approval"`
	Targets       []TargetConfig `mapstructure:"targets" toml:"targets"`
}

type GitHubConfig struct {
	ControlRepo string `mapstructure:"control_repo" toml:"control_repo"`
	IssueLabel  string `mapstructure:"issue_label" toml:"issue_label"`
	Limit       int    `mapstructure:"limit" toml:"limit"`
}

type PathsConfig struct {
	AppDir   string `mapstructure:"app_dir" toml:"app_dir"`
	HomeRepo string `mapstructure:"home_repo" toml:"home_repo"`
	StateDB  string `mapstructure:"state_db" toml:"state_db"`
	LogDir   string `mapstructure:"log_dir" toml:"log_dir"`
	KBDir    string `mapstructure:"kb_dir" toml:"kb_dir"`
	EnvFile  string `mapstructure:"env_file" toml:"env_file"`
}

type ExecutorConfig struct {
	Provider string   `mapstructure:"provider" toml:"provider"`
	Binary   string   `mapstructure:"binary" toml:"binary"`
	Command  []string `mapstructure:"command" toml:"command"`
	Timeout  string   `mapstructure:"timeout" toml:"timeout"`
}

type ApprovalConfig struct {
	Command           string `mapstructure:"command" toml:"command"`
	MinimumPermission string `mapstructure:"minimum_permission" toml:"minimum_permission"`
}

type TargetConfig struct {
	Repo      string `mapstructure:"repo" toml:"repo"`
	LocalPath string `mapstructure:"local_path" toml:"local_path"`
	KBPath    string `mapstructure:"kb_path" toml:"kb_path"`
	Disabled  bool   `mapstructure:"disabled" toml:"disabled,omitempty"`
}

type Secrets struct {
	Path   string
	Values map[string]string
}

func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("toml")
	v.SetDefault("github.issue_label", "ccclaw")
	v.SetDefault("github.limit", 20)
	v.SetDefault("executor.provider", "claude-code")
	v.SetDefault("executor.command", []string{"claude"})
	v.SetDefault("executor.timeout", "30m")
	v.SetDefault("approval.command", "/ccclaw approve")
	v.SetDefault("approval.minimum_permission", "admin")
	v.SetDefault("default_target", "")

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	var cfg Config
	if err := v.UnmarshalExact(&cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}
	cfg.NormalizePaths()

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (cfg *Config) NormalizePaths() {
	cfg.Paths.AppDir = ExpandPath(cfg.Paths.AppDir)
	cfg.Paths.HomeRepo = ExpandPath(cfg.Paths.HomeRepo)
	cfg.Paths.StateDB = ExpandPath(cfg.Paths.StateDB)
	cfg.Paths.LogDir = ExpandPath(cfg.Paths.LogDir)
	cfg.Paths.KBDir = ExpandPath(cfg.Paths.KBDir)
	cfg.Paths.EnvFile = ExpandPath(cfg.Paths.EnvFile)
	for idx := range cfg.Targets {
		cfg.Targets[idx].LocalPath = ExpandPath(cfg.Targets[idx].LocalPath)
		cfg.Targets[idx].KBPath = ExpandPath(cfg.Targets[idx].KBPath)
	}
	for idx, arg := range cfg.Executor.Command {
		cfg.Executor.Command[idx] = ExpandPath(arg)
	}
	cfg.Executor.Binary = ExpandPath(cfg.Executor.Binary)
}

func (cfg *Config) Validate() error {
	if cfg.GitHub.ControlRepo == "" {
		return errors.New("github.control_repo 不能为空")
	}
	if cfg.Paths.AppDir == "" {
		return errors.New("paths.app_dir 不能为空")
	}
	if cfg.Paths.HomeRepo == "" {
		return errors.New("paths.home_repo 不能为空")
	}
	if cfg.Paths.StateDB == "" {
		return errors.New("paths.state_db 不能为空")
	}
	if cfg.Paths.LogDir == "" {
		return errors.New("paths.log_dir 不能为空")
	}
	if cfg.Paths.KBDir == "" {
		return errors.New("paths.kb_dir 不能为空")
	}
	if cfg.Paths.EnvFile == "" {
		return errors.New("paths.env_file 不能为空")
	}
	if len(cfg.Executor.Command) == 0 && cfg.Executor.Binary == "" {
		return errors.New("executor.command 或 executor.binary 至少需要一个")
	}
	seenTargets := map[string]struct{}{}
	for _, target := range cfg.Targets {
		if target.Repo == "" || target.LocalPath == "" {
			return errors.New("targets.repo 与 targets.local_path 均不能为空")
		}
		if _, exists := seenTargets[target.Repo]; exists {
			return fmt.Errorf("targets.repo 不允许重复: %s", target.Repo)
		}
		seenTargets[target.Repo] = struct{}{}
	}
	if cfg.DefaultTarget != "" {
		target, err := cfg.TargetByRepo(cfg.DefaultTarget)
		if err != nil {
			return fmt.Errorf("default_target 无效: %w", err)
		}
		if target.Disabled {
			return fmt.Errorf("default_target 指向的 target 已禁用: %s", cfg.DefaultTarget)
		}
	}
	return nil
}

func (cfg *Config) TargetByRepo(repo string) (*TargetConfig, error) {
	for _, target := range cfg.Targets {
		if target.Repo == repo {
			copy := target
			if copy.KBPath == "" {
				copy.KBPath = cfg.Paths.KBDir
			}
			return &copy, nil
		}
	}
	return nil, fmt.Errorf("未找到 repo=%s 的 target 配置", repo)
}

func (cfg *Config) EnabledTargets() []TargetConfig {
	targets := make([]TargetConfig, 0, len(cfg.Targets))
	for _, target := range cfg.Targets {
		if target.Disabled {
			continue
		}
		if target.KBPath == "" {
			target.KBPath = cfg.Paths.KBDir
		}
		targets = append(targets, target)
	}
	return targets
}

func (cfg *Config) EnabledTargetByRepo(repo string) (*TargetConfig, error) {
	target, err := cfg.TargetByRepo(repo)
	if err != nil {
		return nil, err
	}
	if target.Disabled {
		return nil, fmt.Errorf("repo=%s 的 target 已禁用", repo)
	}
	return target, nil
}

func (cfg *Config) UpsertTarget(target TargetConfig, makeDefault bool) error {
	target.Repo = strings.TrimSpace(target.Repo)
	target.LocalPath = ExpandPath(target.LocalPath)
	target.KBPath = ExpandPath(target.KBPath)
	if target.Repo == "" || target.LocalPath == "" {
		return errors.New("repo 与 local_path 均不能为空")
	}
	if target.KBPath == "" {
		target.KBPath = cfg.Paths.KBDir
	}
	for idx := range cfg.Targets {
		if cfg.Targets[idx].Repo != target.Repo {
			continue
		}
		cfg.Targets[idx].LocalPath = target.LocalPath
		cfg.Targets[idx].KBPath = target.KBPath
		cfg.Targets[idx].Disabled = target.Disabled
		if makeDefault {
			cfg.DefaultTarget = target.Repo
		}
		return cfg.Validate()
	}
	cfg.Targets = append(cfg.Targets, target)
	if makeDefault {
		cfg.DefaultTarget = target.Repo
	}
	return cfg.Validate()
}

func (cfg *Config) DisableTarget(repo string) error {
	for idx := range cfg.Targets {
		if cfg.Targets[idx].Repo != repo {
			continue
		}
		cfg.Targets[idx].Disabled = true
		if cfg.DefaultTarget == repo {
			cfg.DefaultTarget = ""
		}
		return cfg.Validate()
	}
	return fmt.Errorf("未找到 repo=%s 的 target 配置", repo)
}

func Save(path string, cfg *Config) error {
	if cfg == nil {
		return errors.New("配置不能为空")
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	var buf bytes.Buffer
	encoder := toml.NewEncoder(&buf)
	encoder.SetIndentTables(true)
	if err := encoder.Encode(cfg); err != nil {
		return fmt.Errorf("序列化配置文件失败: %w", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("写入配置文件失败: %w", err)
	}
	return nil
}

func LoadSecrets(path string) (*Secrets, error) {
	if path == "" {
		return &Secrets{Values: map[string]string{}}, nil
	}
	path = ExpandPath(path)
	if err := ValidateEnvFile(path); err != nil {
		return nil, err
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("打开 .env 文件失败: %w", err)
	}
	defer file.Close()

	values := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf(".env 存在非法行: %q", line)
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		value = strings.Trim(value, `"'`)
		values[key] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("读取 .env 文件失败: %w", err)
	}
	return &Secrets{Path: path, Values: values}, nil
}

func ValidateEnvFile(path string) error {
	abs, err := filepath.Abs(ExpandPath(path))
	if err != nil {
		return fmt.Errorf("解析 .env 路径失败: %w", err)
	}

	info, err := os.Lstat(abs)
	if err != nil {
		return fmt.Errorf("读取 .env 文件信息失败: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf(".env 不允许使用符号链接: %s", abs)
	}
	if info.IsDir() {
		return fmt.Errorf(".env 路径不能是目录: %s", abs)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf(".env 权限过宽，必须为 0600 或更严格: %s (%#o)", abs, info.Mode().Perm())
	}

	allowedKey := regexp.MustCompile(`^[A-Z0-9_]+$`)
	file, err := os.Open(abs)
	if err != nil {
		return fmt.Errorf("打开 .env 文件失败: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 || !allowedKey.MatchString(strings.TrimSpace(parts[0])) {
			return fmt.Errorf(".env 存在非法键格式: %q", line)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("读取 .env 文件失败: %w", err)
	}
	return nil
}

func ExpandPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return path
	}
	home, err := os.UserHomeDir()
	if err == nil {
		if path == "~" {
			return home
		}
		if strings.HasPrefix(path, "~/") {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}
