package main

import (
	"github.com/41490/ccclaw/internal/archive"
	"github.com/41490/ccclaw/internal/config"
	"github.com/spf13/cobra"
)

func addArchiveCommand(rootCmd *cobra.Command, configPath *string) {
	rootCmd.AddCommand(&cobra.Command{
		Use:   "archive",
		Short: "导出历史周 JSONL 为 Parquet 归档",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(*configPath)
			if err != nil {
				return err
			}
			_, err = archive.Run(cmd.Context(), cfg.Paths.VarDir, cmd.OutOrStdout())
			return err
		},
	})
}
