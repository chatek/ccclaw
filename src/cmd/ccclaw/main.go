package main

import (
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/41490/ccclaw/internal/app"
	"github.com/41490/ccclaw/internal/buildinfo"
	"github.com/41490/ccclaw/internal/config"
	"github.com/spf13/cobra"
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	var configPath string
	var envFile string
	var runLimit int
	var showVersion bool

	rootCmd := &cobra.Command{
		Use:           "ccclaw",
		Short:         "ccclaw 长期异步任务执行器",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if showVersion {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), buildinfo.Short())
				return nil
			}
			return cmd.Help()
		},
	}

	rootCmd.PersistentFlags().StringVar(&configPath, "config", defaultConfigPath(), "TOML 配置文件路径")
	rootCmd.PersistentFlags().StringVar(&envFile, "env-file", defaultEnvFilePath(), "固定 .env 隐私配置文件路径")
	rootCmd.Flags().BoolVarP(&showVersion, "version", "V", false, "显示版本")

	rootCmd.AddCommand(&cobra.Command{
		Use:   "ingest",
		Short: "拉取并入队 Issue 任务",
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := app.NewRuntime(configPath, envFile)
			if err != nil {
				return err
			}
			return rt.Ingest(cmd.Context())
		},
	})

	runCmd := &cobra.Command{
		Use:   "run",
		Short: "执行待处理任务",
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := app.NewRuntime(configPath, envFile)
			if err != nil {
				return err
			}
			return rt.Run(cmd.Context(), runLimit)
		},
	}
	runCmd.Flags().IntVar(&runLimit, "limit", 10, "本轮最多执行任务数")
	rootCmd.AddCommand(runCmd)

	rootCmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "查看任务状态",
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := app.NewRuntime(configPath, envFile)
			if err != nil {
				return err
			}
			return rt.Status(os.Stdout)
		},
	})

	rootCmd.AddCommand(&cobra.Command{
		Use:   "doctor",
		Short: "执行环境与部署健康检查",
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := app.NewRuntime(configPath, envFile)
			if err != nil {
				return err
			}
			return rt.Doctor(cmd.Context(), os.Stdout)
		},
	})

	rootCmd.AddCommand(&cobra.Command{
		Use:   "config",
		Short: "校验并展示当前配置",
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := app.NewRuntime(configPath, envFile)
			if err != nil {
				return err
			}
			return rt.ShowConfig(os.Stdout)
		},
	})

	targetCmd := &cobra.Command{
		Use:   "target",
		Short: "管理目标仓库绑定",
	}

	targetCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "列出当前 target 配置",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return err
			}
			if len(cfg.Targets) == 0 {
				_, _ = fmt.Fprintln(os.Stdout, "当前未绑定任何 target")
				return nil
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			_, _ = fmt.Fprintln(w, "REPO\tSTATUS\tDEFAULT\tLOCAL_PATH\tKB_PATH")
			for _, target := range cfg.Targets {
				status := "enabled"
				if target.Disabled {
					status = "disabled"
				}
				isDefault := ""
				if cfg.DefaultTarget == target.Repo {
					isDefault = "*"
				}
				_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", target.Repo, status, isDefault, target.LocalPath, target.KBPath)
			}
			return w.Flush()
		},
	})

	var addRepo string
	var addPath string
	var addKBPath string
	var makeDefault bool
	targetAddCmd := &cobra.Command{
		Use:   "add",
		Short: "追加或更新一个 target",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return err
			}
			if err := cfg.UpsertTarget(config.TargetConfig{
				Repo:      addRepo,
				LocalPath: addPath,
				KBPath:    addKBPath,
			}, makeDefault); err != nil {
				return err
			}
			return config.Save(configPath, cfg)
		},
	}
	targetAddCmd.Flags().StringVar(&addRepo, "repo", "", "target 仓库 owner/repo")
	targetAddCmd.Flags().StringVar(&addPath, "path", "", "target 本地路径")
	targetAddCmd.Flags().StringVar(&addKBPath, "kb-path", "", "target 对应 kb 路径，默认继承全局 kb_dir")
	targetAddCmd.Flags().BoolVar(&makeDefault, "default", false, "设为默认 target")
	_ = targetAddCmd.MarkFlagRequired("repo")
	_ = targetAddCmd.MarkFlagRequired("path")
	targetCmd.AddCommand(targetAddCmd)

	var disableRepo string
	targetDisableCmd := &cobra.Command{
		Use:   "disable",
		Short: "禁用一个 target",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return err
			}
			if err := cfg.DisableTarget(disableRepo); err != nil {
				return err
			}
			return config.Save(configPath, cfg)
		},
	}
	targetDisableCmd.Flags().StringVar(&disableRepo, "repo", "", "要禁用的 target 仓库 owner/repo")
	_ = targetDisableCmd.MarkFlagRequired("repo")
	targetCmd.AddCommand(targetDisableCmd)
	rootCmd.AddCommand(targetCmd)

	rootCmd.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "显示版本",
		Run: func(cmd *cobra.Command, args []string) {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), buildinfo.Short())
		},
	})

	return rootCmd
}

func defaultConfigPath() string {
	home, _ := os.UserHomeDir()
	candidates := []string{
		filepath.Join(home, ".ccclaw", "ops", "config", "config.toml"),
		filepath.Join("ops", "config", "config.toml"),
		filepath.Join("ops", "config", "config.example.toml"),
		filepath.Join("dist", "ops", "config", "config.example.toml"),
	}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	if home != "" {
		return filepath.Join(home, ".ccclaw", "ops", "config", "config.toml")
	}
	return filepath.Join("ops", "config", "config.toml")
}

func defaultEnvFilePath() string {
	home, _ := os.UserHomeDir()
	candidates := []string{
		filepath.Join(home, ".ccclaw", ".env"),
		".env",
		filepath.Join("dist", ".env"),
	}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	if home != "" {
		return filepath.Join(home, ".ccclaw", ".env")
	}
	return ".env"
}
