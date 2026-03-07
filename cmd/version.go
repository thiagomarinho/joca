package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/thiagomarinho/joca/version"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	RunE:  runVersion,
}

func init() {
	RootCmd.AddCommand(versionCmd)
}

func runVersion(_ *cobra.Command, _ []string) error {
	fmt.Printf("joca %s (commit %s)\n", version.Version, version.Commit)
	return nil
}
