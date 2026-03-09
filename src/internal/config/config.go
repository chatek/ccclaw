package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	GitHub   GitHubConfig   `mapstructure:"github"`
	Paths    PathsConfig    `mapstructure:"paths"`
	Executor ExecutorConfig `mapstructure:"executor"`
	Approval ApprovalConfig `mapstructure:"approval"`
	Targets  []TargetConfig `mapstructure:"targets"`
}

type GitHubConfig struct {
	ControlRepo string `mapstructure:"control_repo"`
	IssueLabel  string `mapstructure:"issue_label"`
	Limit       int    `mapstructure:"limit"`
}

type PathsConfig struct {
	AppDir   string `mapstructure:"app_dir"`
	HomeRepo string `mapstructure:"home_repo"`
	StateDB  string `mapstructure:"state_db"`
	LogDir   string `mapstructure:"log_dir"`
	KBDir    string `mapstructure:"kb_dir"`
	EnvFile  string `mapstructure:"env_file"`
}

type ExecutorConfig struct {
	Provider string   `mapstructure:"provider"`
	Binary   string   `mapstructure:"binary"`
	Command  []string `mapstructure:"command"`
	Timeout  string   `mapstructure:"timeout"`
}

type ApprovalConfig struct {
	Command           string `mapstructure:"command"`
	MinimumPermission string `mapstructure:"minimum_permission"`
}

type TargetConfig struct {
	Repo      string `mapstructure:"repo"`
	LocalPath string `mapstructure:"local_path"`
	KBPath    string `mapstructure:"kb_path"`
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
	v.SetDefault("approval.minimum_permission", "write")

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
	if len(cfg.Targets) == 0 {
		return errors.New("至少需要一个 targets 项")
	}
	for _, target := range cfg.Targets {
		if target.Repo == "" || target.LocalPath == "" {
			return errors.New("targets.repo 与 targets.local_path 均不能为空")
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
