package cmd

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/thiagomarinho/joca/internal/config"
)

var automationCmd = &cobra.Command{
	Use:   "automation",
	Short: "Manage automation rules (pipeline chaining)",
}

// ── list ─────────────────────────────────────────────────────────────────────

var automationListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all automation rules",
	Args:  cobra.NoArgs,
	RunE:  runAutomationList,
}

func runAutomationList(cmd *cobra.Command, _ []string) error {
	var resolvedCfg config.Config
	config.Resolve(&resolvedCfg)

	appCfg, err := config.Load(resolvedCfg.ConfigFile)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	if len(appCfg.Automations) == 0 {
		cmd.Println("No automation rules configured.")
		cmd.Printf("Add one with: joca automation add --watch <pipeline> --on <status> --trigger <pipeline>\n")
		return nil
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(w, "NAME\tWATCH\tON STATUS\tTRIGGER\tFIRES\tMAX\tSTATUS"); err != nil {
		return err
	}
	for _, r := range appCfg.Automations {
		maxStr := "∞"
		if r.MaxFires > 0 {
			maxStr = fmt.Sprintf("%d", r.MaxFires)
		}
		status := "active"
		if r.Disabled {
			if r.MaxFires > 0 && r.FireCount >= r.MaxFires {
				status = "exhausted"
			} else {
				status = "disabled"
			}
		}
		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%s\t%s\n",
			r.Name, r.WatchPipeline, r.OnStatus, r.TriggerPipeline,
			r.FireCount, maxStr, status); err != nil {
			return err
		}
	}
	return w.Flush()
}

// ── add ───────────────────────────────────────────────────────────────────────

var (
	autoAddWatch    string
	autoAddOn       string
	autoAddTriggers []string
	autoAddName     string
	autoAddOnce     bool
	autoAddAlways   bool
	autoAddTimes    int
)

var automationAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add an automation rule",
	Long: `Add an automation rule that triggers a pipeline when another completes.

Examples:
  joca automation add --watch build --on success --trigger deploy --once
  joca automation add --watch tests --on failed --trigger notify
  joca automation add --watch ci --on success --trigger staging --times 3
  joca automation add --watch build --on success --trigger deploy --trigger notify`,
	Args: cobra.NoArgs,
	RunE: runAutomationAdd,
}

func init() {
	automationAddCmd.Flags().StringVar(&autoAddWatch, "watch", "", "Pipeline name to watch (required)")
	automationAddCmd.Flags().StringVar(&autoAddOn, "on", "", "Status transition to react to: success, failed, cancelled (required)")
	automationAddCmd.Flags().StringArrayVar(&autoAddTriggers, "trigger", nil, "Pipeline name to trigger; may be specified multiple times (required)")
	automationAddCmd.Flags().StringVar(&autoAddName, "name", "", "Rule name (auto-generated if omitted; ignored when multiple triggers are specified)")
	automationAddCmd.Flags().BoolVar(&autoAddOnce, "once", false, "Fire at most once then disable")
	automationAddCmd.Flags().BoolVar(&autoAddAlways, "always", false, "Fire every time the condition is met (default)")
	automationAddCmd.Flags().IntVar(&autoAddTimes, "times", 0, "Fire at most N times then disable")
	_ = automationAddCmd.MarkFlagRequired("watch")
	_ = automationAddCmd.MarkFlagRequired("on")
	_ = automationAddCmd.MarkFlagRequired("trigger")
}

func runAutomationAdd(cmd *cobra.Command, _ []string) error {
	onStatus := strings.ToLower(autoAddOn)
	switch onStatus {
	case "success", "failed", "cancelled":
		// valid
	default:
		return fmt.Errorf("--on must be one of: success, failed, cancelled (got %q)", autoAddOn)
	}

	// Mutually exclusive repeat flags.
	setCount := 0
	if autoAddOnce {
		setCount++
	}
	if autoAddAlways {
		setCount++
	}
	if autoAddTimes > 0 {
		setCount++
	}
	if setCount > 1 {
		return fmt.Errorf("--once, --always, and --times are mutually exclusive")
	}

	maxFires := 0 // unlimited by default
	if autoAddOnce {
		maxFires = 1
	} else if autoAddTimes > 0 {
		maxFires = autoAddTimes
	}

	if err := config.Setup(); err != nil {
		return fmt.Errorf("setting up joca dir: %w", err)
	}

	var resolvedCfg config.Config
	config.Resolve(&resolvedCfg)

	repeatDesc := "every time"
	if maxFires == 1 {
		repeatDesc = "once"
	} else if maxFires > 1 {
		repeatDesc = fmt.Sprintf("%d times", maxFires)
	}

	for _, trigger := range autoAddTriggers {
		name := autoAddName
		if name == "" || len(autoAddTriggers) > 1 {
			name = fmt.Sprintf("%s→%s→%s", autoAddWatch, onStatus, trigger)
		}
		rule := config.AutomationRule{
			Name:            name,
			WatchPipeline:   autoAddWatch,
			OnStatus:        onStatus,
			TriggerPipeline: trigger,
			MaxFires:        maxFires,
		}
		if err := config.AddAutomation(resolvedCfg.ConfigFile, rule); err != nil {
			return err
		}
		cmd.Printf("Added automation rule %q: when %q → %s, trigger %q (%s)\n",
			rule.Name, rule.WatchPipeline, rule.OnStatus, rule.TriggerPipeline, repeatDesc)
	}
	return nil
}

// ── delete ────────────────────────────────────────────────────────────────────

var automationDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete an automation rule",
	Args:  cobra.ExactArgs(1),
	RunE:  runAutomationDelete,
}

func runAutomationDelete(cmd *cobra.Command, args []string) error {
	var resolvedCfg config.Config
	config.Resolve(&resolvedCfg)

	if err := config.DeleteAutomation(resolvedCfg.ConfigFile, args[0]); err != nil {
		return err
	}
	cmd.Printf("Deleted automation rule %q\n", args[0])
	return nil
}

// ── enable / disable ──────────────────────────────────────────────────────────

var automationEnableCmd = &cobra.Command{
	Use:   "enable <name>",
	Short: "Enable a disabled automation rule",
	Args:  cobra.ExactArgs(1),
	RunE:  runAutomationEnable,
}

func runAutomationEnable(cmd *cobra.Command, args []string) error {
	return setAutomationDisabled(cmd, args[0], false)
}

var automationDisableCmd = &cobra.Command{
	Use:   "disable <name>",
	Short: "Disable an automation rule without deleting it",
	Args:  cobra.ExactArgs(1),
	RunE:  runAutomationDisable,
}

func runAutomationDisable(cmd *cobra.Command, args []string) error {
	return setAutomationDisabled(cmd, args[0], true)
}

func setAutomationDisabled(cmd *cobra.Command, name string, disabled bool) error {
	var resolvedCfg config.Config
	config.Resolve(&resolvedCfg)

	appCfg, err := config.Load(resolvedCfg.ConfigFile)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	found := false
	for i, r := range appCfg.Automations {
		if r.Name == name {
			appCfg.Automations[i].Disabled = disabled
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("automation rule %q not found", name)
	}
	if err := config.Save(resolvedCfg.ConfigFile, appCfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	action := "enabled"
	if disabled {
		action = "disabled"
	}
	cmd.Printf("Automation rule %q %s\n", name, action)
	return nil
}

// ── reset ─────────────────────────────────────────────────────────────────────

var automationResetCmd = &cobra.Command{
	Use:   "reset <name>",
	Short: "Reset fire count and re-enable an exhausted rule",
	Args:  cobra.ExactArgs(1),
	RunE:  runAutomationReset,
}

func runAutomationReset(cmd *cobra.Command, args []string) error {
	var resolvedCfg config.Config
	config.Resolve(&resolvedCfg)

	appCfg, err := config.Load(resolvedCfg.ConfigFile)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	found := false
	for i, r := range appCfg.Automations {
		if r.Name == args[0] {
			appCfg.Automations[i].FireCount = 0
			appCfg.Automations[i].Disabled = false
			found = true
			_ = r
			break
		}
	}
	if !found {
		return fmt.Errorf("automation rule %q not found", args[0])
	}
	if err := config.Save(resolvedCfg.ConfigFile, appCfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}
	cmd.Printf("Automation rule %q reset (fire_count=0, disabled=false)\n", args[0])
	return nil
}

func init() {
	RootCmd.AddCommand(automationCmd)
	automationCmd.AddCommand(automationListCmd)
	automationCmd.AddCommand(automationAddCmd)
	automationCmd.AddCommand(automationDeleteCmd)
	automationCmd.AddCommand(automationEnableCmd)
	automationCmd.AddCommand(automationDisableCmd)
	automationCmd.AddCommand(automationResetCmd)
}
