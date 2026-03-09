package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/41490/ccclaw/internal/app"
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

	rootCmd := &cobra.Command{
		Use:           "ccclaw",
		Short:         "ccclaw 长期异步任务执行器",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	rootCmd.PersistentFlags().StringVar(&configPath, "config", defaultConfigPath(), "TOML 配置文件路径")
	rootCmd.PersistentFlags().StringVar(&envFile, "env-file", defaultEnvFilePath(), "固定 .env 隐私配置文件路径")

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

	return rootCmd
}

func defaultConfigPath() string {
	candidates := []string{
		filepath.Join("ops", "config", "config.toml"),
		filepath.Join("ops", "config", "config.example.toml"),
		filepath.Join("dist", "ops", "config", "config.example.toml"),
		"/opt/ccclaw/ops/config/config.toml",
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return filepath.Join("ops", "config", "config.toml")
}

func defaultEnvFilePath() string {
	candidates := []string{
		".env",
		filepath.Join("dist", ".env"),
		"/opt/ccclaw/.env",
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return "/opt/ccclaw/.env"
}
