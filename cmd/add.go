package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/thiagomarinho/joca/internal/config"
)

var addCmd = &cobra.Command{
	Use:   "add <provider> <target>",
	Short: "Add a pipeline to joca",
	Long: `Add a pipeline to ~/.joca/config.yaml.

Examples:
  joca add github thiagomarinho/my-api
  joca add github thiagomarinho/my-api --name my-api
  joca add aws infra-deploy --region us-east-1
  joca add aws infra-deploy --profile production --region us-east-1`,
	Args:    cobra.ExactArgs(2),
	RunE:    runAdd,
	GroupID: "",
}

var (
	addName       string
	addAWSProfile string
	addAWSRegion  string
	addGHWorkflow string
)

func init() {
	RootCmd.AddCommand(addCmd)
	addCmd.Flags().StringVar(&addName, "name", "", "Display name (defaults to the target argument)")
	addCmd.Flags().StringVar(&addAWSProfile, "profile", "", "AWS profile name (AWS only)")
	addCmd.Flags().StringVar(&addAWSRegion, "region", "", "AWS region (AWS only)")
	addCmd.Flags().StringVar(&addGHWorkflow, "workflow", "", "Workflow filename to track, e.g. ci.yml (GitHub only)")
}

func runAdd(cmd *cobra.Command, args []string) error {
	providerArg := strings.ToLower(args[0])
	target := args[1]

	var entry config.PipelineEntry

	switch providerArg {
	case "github", "gh":
		parts := strings.SplitN(target, "/", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return fmt.Errorf("github target must be <owner>/<repo>, got %q", target)
		}
		name := addName
		if name == "" {
			name = parts[1]
		}
		entry = config.PipelineEntry{
			Name:     name,
			Provider: config.ProviderGitHub,
			Owner:    parts[0],
			Repo:     parts[1],
			Workflow: addGHWorkflow,
		}

	case "aws", "codepipeline":
		name := addName
		if name == "" {
			name = target
		}
		entry = config.PipelineEntry{
			Name:         name,
			Provider:     config.ProviderAWS,
			PipelineName: target,
			AWSProfile:   addAWSProfile,
			AWSRegion:    addAWSRegion,
		}

	default:
		return fmt.Errorf("unknown provider %q — supported: github, aws", providerArg)
	}

	if err := config.Setup(); err != nil {
		return fmt.Errorf("setting up joca dir: %w", err)
	}

	var resolvedCfg config.Config
	config.Resolve(&resolvedCfg)

	if err := config.AddPipeline(resolvedCfg.ConfigFile, entry); err != nil {
		return err
	}

	cmd.Printf("Added pipeline %q (%s) to %s\n", entry.Name, entry.Provider, resolvedCfg.ConfigFile)
	return nil
}
