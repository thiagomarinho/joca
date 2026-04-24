package config

import "fmt"

// AutomationRule defines a condition+action pair evaluated after every refresh.
// When a watched pipeline transitions to the specified status, the rule fires
// TriggerNew on the target pipeline.
type AutomationRule struct {
	// Name is a unique human-readable identifier for this rule.
	Name string `yaml:"name"`

	// WatchPipeline is the Name of the pipeline to observe.
	WatchPipeline string `yaml:"watch_pipeline"`

	// OnStatus is the status transition that fires this rule.
	// Valid values: "success", "failed", "cancelled".
	OnStatus string `yaml:"on_status"`

	// TriggerPipeline is the Name of the pipeline to start via TriggerNew.
	TriggerPipeline string `yaml:"trigger_pipeline"`

	// MaxFires is the maximum number of times this rule may fire.
	// 0 means unlimited (fire every time the condition is met).
	MaxFires int `yaml:"max_fires,omitempty"`

	// FireCount is incremented each time the rule fires. Persisted to YAML.
	FireCount int `yaml:"fire_count,omitempty"`

	// Disabled prevents the rule from firing. Set automatically when a
	// finite rule (MaxFires > 0) is exhausted. Can also be set manually.
	Disabled bool `yaml:"disabled,omitempty"`
}

// CheckAutomationCycle returns an error if adding newRule to existing would
// create a self-reference or (when allowChains is false) a dependency cycle.
//
// Self-references are always rejected regardless of allowChains.
// A cycle exists when following the chain watch→trigger through all rules
// eventually returns to newRule.WatchPipeline.
func CheckAutomationCycle(existing []AutomationRule, newRule AutomationRule, allowChains bool) error {
	if newRule.WatchPipeline == newRule.TriggerPipeline {
		return fmt.Errorf("pipeline %q cannot trigger itself", newRule.WatchPipeline)
	}

	if allowChains {
		return nil
	}

	// Build an adjacency list: watch → set of triggers (from existing rules).
	edges := make(map[string][]string, len(existing)+1)
	for _, r := range existing {
		edges[r.WatchPipeline] = append(edges[r.WatchPipeline], r.TriggerPipeline)
	}
	// Add the new edge tentatively.
	edges[newRule.WatchPipeline] = append(edges[newRule.WatchPipeline], newRule.TriggerPipeline)

	// DFS from newRule.TriggerPipeline; if we reach newRule.WatchPipeline, it's a cycle.
	target := newRule.WatchPipeline
	visited := make(map[string]bool)
	var dfs func(node string) bool
	dfs = func(node string) bool {
		if node == target {
			return true
		}
		if visited[node] {
			return false
		}
		visited[node] = true
		for _, next := range edges[node] {
			if dfs(next) {
				return true
			}
		}
		return false
	}

	if dfs(newRule.TriggerPipeline) {
		return fmt.Errorf("adding this rule would create a circular dependency involving %q", target)
	}
	return nil
}
