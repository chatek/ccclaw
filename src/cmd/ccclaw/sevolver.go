package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/41490/ccclaw/internal/config"
	"github.com/41490/ccclaw/internal/sevolver"
	"github.com/spf13/cobra"
)

func addSevolverCommand(rootCmd *cobra.Command, configPath, envFile *string) {
	rootCmd.AddCommand(&cobra.Command{
		Use:   "sevolver",
		Short: "执行一次 skill 生命周期自维护与缺口扫描",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(*configPath)
			if err != nil {
				return err
			}
			journalDir := filepath.Join(cfg.Paths.HomeRepo, "kb", "journal")
			if _, err := os.Stat(journalDir); err != nil {
				journalDir = filepath.Join(cfg.Paths.KBDir, "journal")
			}
			secrets := map[string]string{}
			if envFile != nil && strings.TrimSpace(*envFile) != "" {
				envPath := config.ExpandPath(*envFile)
				if _, err := os.Stat(envPath); err == nil {
					loaded, err := config.LoadSecrets(envPath)
					if err != nil {
						return err
					}
					secrets = loaded.Values
				}
			}
			_, err = sevolver.Run(sevolver.Config{
				KBDir:       cfg.Paths.KBDir,
				JournalDir:  journalDir,
				ReportDir:   cfg.Paths.KBDir,
				VarDir:      cfg.Paths.VarDir,
				ControlRepo: cfg.GitHub.ControlRepo,
				TargetRepo:  cfg.GitHub.ControlRepo,
				IssueLabel:  cfg.GitHub.IssueLabel,
				Secrets:     secrets,
			}, cmd.OutOrStdout())
			return err
		},
	})
}
