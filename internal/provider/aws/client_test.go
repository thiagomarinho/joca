package aws

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/codepipeline/types"
)

func makeStageState(name string, stageStatus types.StageExecutionStatus, actions []types.ActionState) types.StageState {
	return types.StageState{
		StageName: aws.String(name),
		LatestExecution: &types.StageExecution{
			Status: stageStatus,
		},
		ActionStates: actions,
	}
}

func makeApprovalAction(token string) types.ActionState {
	t := token
	return types.ActionState{
		LatestExecution: &types.ActionExecution{
			Status: types.ActionExecutionStatusInProgress,
			Token:  &t,
		},
	}
}

func TestStageAfterApproval(t *testing.T) {
	stages := []types.StageState{
		makeStageState("Source", types.StageExecutionStatusSucceeded, nil),
		makeStageState("ManualApproval", types.StageExecutionStatusInProgress, []types.ActionState{
			makeApprovalAction("approval-token-123"),
		}),
		makeStageState("Deploy", types.StageExecutionStatusInProgress, nil),
	}
	got := stageAfterApproval(stages)
	if got != "Deploy" {
		t.Errorf("stageAfterApproval() = %q, want %q", got, "Deploy")
	}
}

func TestStageAfterApproval_lastStage(t *testing.T) {
	stages := []types.StageState{
		makeStageState("Source", types.StageExecutionStatusSucceeded, nil),
		makeStageState("ManualApproval", types.StageExecutionStatusInProgress, []types.ActionState{
			makeApprovalAction("approval-token-123"),
		}),
	}
	got := stageAfterApproval(stages)
	if got != "ManualApproval" {
		t.Errorf("stageAfterApproval() = %q, want %q", got, "ManualApproval")
	}
}

func TestShortSHA(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"a1b2c3d4e5f6789", "a1b2c3d"},
		{"abc1234", "abc1234"},
		{"abc", "abc"},
		{"", ""},
	}
	for _, tt := range tests {
		got := shortSHA(tt.in)
		if got != tt.want {
			t.Errorf("shortSHA(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
