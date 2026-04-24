package config_test

import (
	"testing"

	"github.com/thiagomarinho/joca/internal/config"
)

func TestCheckAutomationCycle_selfReference(t *testing.T) {
	rule := config.AutomationRule{
		Name:            "self",
		WatchPipeline:   "build",
		OnStatus:        "success",
		TriggerPipeline: "build",
	}
	// Self-reference is always rejected, regardless of allowChains.
	if err := config.CheckAutomationCycle(nil, rule, true); err == nil {
		t.Error("expected error for self-reference with allowChains=true, got nil")
	}
	if err := config.CheckAutomationCycle(nil, rule, false); err == nil {
		t.Error("expected error for self-reference with allowChains=false, got nil")
	}
}

func TestCheckAutomationCycle_directCycle_notAllowed(t *testing.T) {
	existing := []config.AutomationRule{
		{WatchPipeline: "deploy", OnStatus: "success", TriggerPipeline: "build"},
	}
	rule := config.AutomationRule{
		Name:            "r",
		WatchPipeline:   "build",
		OnStatus:        "success",
		TriggerPipeline: "deploy",
	}
	if err := config.CheckAutomationCycle(existing, rule, false); err == nil {
		t.Error("expected cycle error with allowChains=false, got nil")
	}
}

func TestCheckAutomationCycle_directCycle_allowed(t *testing.T) {
	existing := []config.AutomationRule{
		{WatchPipeline: "deploy", OnStatus: "success", TriggerPipeline: "build"},
	}
	rule := config.AutomationRule{
		Name:            "r",
		WatchPipeline:   "build",
		OnStatus:        "success",
		TriggerPipeline: "deploy",
	}
	if err := config.CheckAutomationCycle(existing, rule, true); err != nil {
		t.Errorf("expected no error with allowChains=true, got: %v", err)
	}
}

func TestCheckAutomationCycle_longerChain_notAllowed(t *testing.T) {
	existing := []config.AutomationRule{
		{WatchPipeline: "b", OnStatus: "success", TriggerPipeline: "c"},
		{WatchPipeline: "c", OnStatus: "success", TriggerPipeline: "a"},
	}
	// Adding a→b would create a→b→c→a.
	rule := config.AutomationRule{
		Name:            "r",
		WatchPipeline:   "a",
		OnStatus:        "success",
		TriggerPipeline: "b",
	}
	if err := config.CheckAutomationCycle(existing, rule, false); err == nil {
		t.Error("expected cycle error for longer chain, got nil")
	}
}

func TestCheckAutomationCycle_noCycle(t *testing.T) {
	existing := []config.AutomationRule{
		{WatchPipeline: "build", OnStatus: "success", TriggerPipeline: "test"},
	}
	rule := config.AutomationRule{
		Name:            "r",
		WatchPipeline:   "test",
		OnStatus:        "success",
		TriggerPipeline: "deploy",
	}
	for _, allow := range []bool{true, false} {
		if err := config.CheckAutomationCycle(existing, rule, allow); err != nil {
			t.Errorf("allowChains=%v: expected no error, got: %v", allow, err)
		}
	}
}

func TestCheckAutomationCycle_emptyExisting(t *testing.T) {
	rule := config.AutomationRule{
		Name:            "r",
		WatchPipeline:   "build",
		OnStatus:        "success",
		TriggerPipeline: "deploy",
	}
	for _, allow := range []bool{true, false} {
		if err := config.CheckAutomationCycle(nil, rule, allow); err != nil {
			t.Errorf("allowChains=%v: expected no error with empty existing, got: %v", allow, err)
		}
	}
}
