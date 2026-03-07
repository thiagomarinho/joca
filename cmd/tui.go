package cmd

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/thiagomarinho/joca/internal/config"
	"github.com/thiagomarinho/joca/internal/ui"
)

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Launch the interactive TUI (default)",
	RunE:  runTUI,
}

func init() {
	RootCmd.AddCommand(tuiCmd)

	// Make running `joca` with no subcommand launch the TUI.
	RootCmd.RunE = runTUI
}

func runTUI(_ *cobra.Command, _ []string) error {
	if err := config.Setup(); err != nil {
		return fmt.Errorf("setting up joca dir: %w", err)
	}

	var resolvedCfg config.Config
	config.Resolve(&resolvedCfg)

	appCfg, err := config.Load(resolvedCfg.ConfigFile)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	model := ui.New(appCfg, resolvedCfg)
	p := tea.NewProgram(model, tea.WithAltScreen())
	_, err = p.Run()
	return err
}
