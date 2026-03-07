package cmd

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/thiagomarinho/joca/internal/config"
)

// cfg is the shared configuration populated from global flags before any subcommand runs.
var cfg config.Config

// RootCmd is the base command for the CLI.
var RootCmd = &cobra.Command{
	Use:          "joca",
	Short:        "Interactive terminal UI for visualizing CI/CD pipeline status",
	SilenceUsage: true,
}

// Execute is the entry point called from main.
func Execute() {
	if err := RootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	RootCmd.PersistentFlags().StringVar(
		&cfg.Dir, "dir", ".", "Target directory",
	)
	RootCmd.PersistentFlags().BoolVar(
		&cfg.Verbose, "verbose", false, "Enable verbose output",
	)
	RootCmd.PersistentFlags().BoolVar(
		&cfg.Debug, "debug", false, "Enable debug output",
	)
}

func initConfig() {
	config.Resolve(&cfg)
}
