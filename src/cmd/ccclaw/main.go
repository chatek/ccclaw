package main

import (
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"
	"time"

	"github.com/41490/ccclaw/internal/app"
	"github.com/41490/ccclaw/internal/buildinfo"
	"github.com/41490/ccclaw/internal/config"
	"github.com/41490/ccclaw/internal/scheduler"
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
	var showRTKComparison bool
	var statsFrom string
	var statsTo string
	var statsDaily bool
	var statsLimit int
	var journalDate string

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
		Short: "查看当前运行态快照",
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := app.NewRuntime(configPath, envFile)
			if err != nil {
				return err
			}
			return rt.Status(os.Stdout)
		},
	})

	statsCmd := &cobra.Command{
		Use:   "stats",
		Short: "查看 token 使用统计",
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := app.NewRuntime(configPath, envFile)
			if err != nil {
				return err
			}
			options, err := parseStatsOptions(statsFrom, statsTo, statsDaily, showRTKComparison, statsLimit)
			if err != nil {
				return err
			}
			return rt.StatsWithOptions(os.Stdout, options)
		},
	}
	statsCmd.Flags().StringVar(&statsFrom, "from", "", "按 YYYY-MM-DD 指定统计起始日期(含当日)")
	statsCmd.Flags().StringVar(&statsTo, "to", "", "按 YYYY-MM-DD 指定统计截止日期(含当日)")
	statsCmd.Flags().BoolVar(&statsDaily, "daily", false, "按天输出聚合统计")
	statsCmd.Flags().IntVar(&statsLimit, "limit", 20, "限制任务明细与 daily 视图输出规模")
	statsCmd.Flags().BoolVar(&showRTKComparison, "rtk-comparison", false, "显示 rtk 与非 rtk 的对比统计")
	rootCmd.AddCommand(statsCmd)

	rootCmd.AddCommand(&cobra.Command{
		Use:   "patrol",
		Short: "巡查 tmux 会话与运行中任务",
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := app.NewRuntime(configPath, envFile)
			if err != nil {
				return err
			}
			return rt.Patrol(cmd.Context(), os.Stdout)
		},
	})

	journalCmd := &cobra.Command{
		Use:   "journal",
		Short: "生成指定日期的 journal 日报",
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := app.NewRuntime(configPath, envFile)
			if err != nil {
				return err
			}
			day := time.Now()
			if journalDate != "" {
				parsed, err := time.ParseInLocation("2006-01-02", journalDate, time.Local)
				if err != nil {
					return fmt.Errorf("解析 --date 失败: %w", err)
				}
				day = parsed
			}
			return rt.Journal(day, os.Stdout)
		},
	}
	journalCmd.Flags().StringVar(&journalDate, "date", "", "按 YYYY-MM-DD 生成指定日期 journal")
	rootCmd.AddCommand(journalCmd)

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

	configCmd := &cobra.Command{
		Use:   "config",
		Short: "校验并展示当前配置",
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := app.NewRuntime(configPath, envFile)
			if err != nil {
				return err
			}
			return rt.ShowConfig(cmd.OutOrStdout())
		},
	}
	configCmd.AddCommand(&cobra.Command{
		Use:   "migrate-approval",
		Short: "将旧 approval.command 配置迁移为 words/reject_words",
		RunE: func(cmd *cobra.Command, args []string) error {
			changed, err := config.MigrateLegacyApproval(configPath)
			if err != nil {
				return err
			}
			if !changed {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "当前配置无需迁移 approval 门禁")
				return nil
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "已迁移审批配置: %s\n", configPath)
			return nil
		},
	})
	var schedulerMode string
	var schedulerUserDir string
	configSetSchedulerCmd := &cobra.Command{
		Use:   "set-scheduler",
		Short: "更新调度器配置",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return err
			}
			cfg.Scheduler.Mode = schedulerMode
			if schedulerUserDir != "" {
				cfg.Scheduler.SystemdUserDir = schedulerUserDir
			}
			cfg.NormalizePaths()
			if err := config.Save(configPath, cfg); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "已更新调度配置: mode=%s systemd_user_dir=%s\n", cfg.Scheduler.Mode, cfg.Scheduler.SystemdUserDir)
			return nil
		},
	}
	configSetSchedulerCmd.Flags().StringVar(&schedulerMode, "mode", "", "调度器模式: auto|systemd|cron|none")
	configSetSchedulerCmd.Flags().StringVar(&schedulerUserDir, "systemd-user-dir", "", "user systemd 单元目录")
	_ = configSetSchedulerCmd.MarkFlagRequired("mode")
	configCmd.AddCommand(configSetSchedulerCmd)
	rootCmd.AddCommand(configCmd)

	schedulerCmd := &cobra.Command{
		Use:   "scheduler",
		Short: "管理调度器后端",
	}
	schedulerCmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "单独查看当前调度器状态",
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := app.NewRuntime(configPath, envFile)
			if err != nil {
				return err
			}
			detail, err := rt.SchedulerStatus()
			if detail != "" {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), detail)
			}
			return err
		},
	})
	schedulerCmd.AddCommand(&cobra.Command{
		Use:   "enable-cron",
		Short: "写入或更新当前用户的受控 crontab 规则",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return err
			}
			message, err := scheduler.EnableCron(cmd.Context(), cfg)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), message)
			return nil
		},
	})
	schedulerCmd.AddCommand(&cobra.Command{
		Use:   "disable-cron",
		Short: "只清理当前用户 crontab 中的 ccclaw 受控规则",
		RunE: func(cmd *cobra.Command, args []string) error {
			message, err := scheduler.DisableCron(cmd.Context())
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), message)
			return nil
		},
	})
	useSchedulerCmd := &cobra.Command{
		Use:   "use MODE",
		Short: "统一切换调度后端并同步配置",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return err
			}
			result, err := scheduler.Use(cmd.Context(), cfg, args[0])
			if err != nil {
				return err
			}
			if err := config.Save(configPath, cfg); err != nil {
				return err
			}
			for _, step := range result.Steps {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), step)
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "已切换调度后端: mode=%s\n", result.Mode)
			return nil
		},
	}
	schedulerCmd.AddCommand(useSchedulerCmd)
	rootCmd.AddCommand(schedulerCmd)

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

func parseStatsOptions(from, to string, daily, showRTKComparison bool, limit int) (app.StatsOptions, error) {
	options := app.StatsOptions{
		Daily:             daily,
		ShowRTKComparison: showRTKComparison,
		Limit:             limit,
	}
	if options.Limit <= 0 {
		return options, fmt.Errorf("--limit 必须大于 0")
	}
	if from != "" {
		parsed, err := time.ParseInLocation("2006-01-02", from, time.Local)
		if err != nil {
			return options, fmt.Errorf("解析 --from 失败: %w", err)
		}
		options.Start = parsed
	}
	if to != "" {
		parsed, err := time.ParseInLocation("2006-01-02", to, time.Local)
		if err != nil {
			return options, fmt.Errorf("解析 --to 失败: %w", err)
		}
		options.End = parsed.Add(24 * time.Hour)
	}
	if !options.Start.IsZero() && !options.End.IsZero() && !options.Start.Before(options.End) {
		return options, fmt.Errorf("--from 必须早于或等于 --to")
	}
	return options, nil
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
