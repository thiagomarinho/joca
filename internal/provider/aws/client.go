package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/codepipeline"
	"github.com/aws/aws-sdk-go-v2/service/codepipeline/types"

	"github.com/thiagomarinho/joca/internal/provider"
)

// Client fetches AWS CodePipeline execution status.
type Client struct {
	pipelineName string
	region       string
	cp           *codepipeline.Client
}

// New creates an AWS CodePipeline client using the SDK default credential chain.
// If profile is non-empty, that named profile is used.
func New(ctx context.Context, pipelineName, region, profile string) (*Client, error) {
	opts := []func(*awsconfig.LoadOptions) error{}
	if region != "" {
		opts = append(opts, awsconfig.WithRegion(region))
	}
	if profile != "" {
		opts = append(opts, awsconfig.WithSharedConfigProfile(profile))
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("loading AWS config: %w", err)
	}
	return &Client{
		pipelineName: pipelineName,
		region:       region,
		cp:           codepipeline.NewFromConfig(cfg),
	}, nil
}

// NewWithSDKClient creates a client with an explicit SDK client (useful in tests).
func NewWithSDKClient(pipelineName, region string, cp *codepipeline.Client) *Client {
	return &Client{pipelineName: pipelineName, region: region, cp: cp}
}

func (c *Client) URL() string {
	r := c.region
	if r == "" {
		r = "us-east-1"
	}
	return fmt.Sprintf("https://%s.console.aws.amazon.com/codesuite/codepipeline/pipelines/%s/view", r, c.pipelineName)
}

func (c *Client) executionURL(executionID string) string {
	r := c.region
	if r == "" {
		r = "us-east-1"
	}
	return fmt.Sprintf("https://%s.console.aws.amazon.com/codesuite/codepipeline/pipelines/%s/executions/%s/visualization", r, c.pipelineName, executionID)
}

func (c *Client) CurrentStatus(ctx context.Context) (provider.Run, error) {
	out, err := c.cp.GetPipelineState(ctx, &codepipeline.GetPipelineStateInput{
		Name: aws.String(c.pipelineName),
	})
	if err != nil {
		return provider.Run{}, fmt.Errorf("GetPipelineState: %w", err)
	}

	status, execID := derivePipelineStatus(out.StageStates)
	url := c.URL()
	if execID != "" {
		url = c.executionURL(execID)
	}
	return provider.Run{
		ID:        execID,
		Status:    status,
		Stage:     currentStage(out.StageStates),
		StartedAt: aws.ToTime(out.Updated),
		URL:       url,
	}, nil
}

func (c *Client) RecentRuns(ctx context.Context, n int) ([]provider.Run, error) {
	out, err := c.cp.ListPipelineExecutions(ctx, &codepipeline.ListPipelineExecutionsInput{
		PipelineName: aws.String(c.pipelineName),
		MaxResults:   aws.Int32(int32(n)),
	})
	if err != nil {
		return nil, fmt.Errorf("ListPipelineExecutions: %w", err)
	}

	runs := make([]provider.Run, 0, len(out.PipelineExecutionSummaries))
	for _, s := range out.PipelineExecutionSummaries {
		execID := aws.ToString(s.PipelineExecutionId)
		runs = append(runs, provider.Run{
			ID:        execID,
			Status:    mapAWSStatus(s.Status),
			StartedAt: aws.ToTime(s.StartTime),
			URL:       c.executionURL(execID),
		})
	}
	return runs, nil
}

func (c *Client) Trigger(ctx context.Context) error {
	_, err := c.cp.StartPipelineExecution(ctx, &codepipeline.StartPipelineExecutionInput{
		Name: aws.String(c.pipelineName),
	})
	if err != nil {
		return fmt.Errorf("StartPipelineExecution: %w", err)
	}
	return nil
}

func currentStage(stages []types.StageState) string {
	for _, s := range stages {
		if s.LatestExecution != nil &&
			s.LatestExecution.Status == types.StageExecutionStatusInProgress {
			return aws.ToString(s.StageName)
		}
	}
	return ""
}

// derivePipelineStatus returns the overall pipeline status and the execution ID
// of the execution being reported (used to correlate with RecentRuns history).
func derivePipelineStatus(stages []types.StageState) (provider.Status, string) {
	// First pass: scan all stages for a pending manual approval. Approval is
	// more actionable than "running" and must not be masked by an earlier stage
	// that belongs to a concurrent newer execution.
	// The Token field is only present on manual approval actions that are
	// actively waiting — it is the approval/rejection token.
	for _, s := range stages {
		if s.LatestExecution == nil || s.LatestExecution.Status != types.StageExecutionStatusInProgress {
			continue
		}
		for _, action := range s.ActionStates {
			if action.LatestExecution != nil &&
				action.LatestExecution.Status == types.ActionExecutionStatusInProgress &&
				action.LatestExecution.Token != nil {
				return provider.StatusApproval, aws.ToString(s.LatestExecution.PipelineExecutionId)
			}
		}
	}

	// Second pass: return the first non-approval active stage status.
	for _, s := range stages {
		if s.LatestExecution == nil {
			continue
		}
		execID := aws.ToString(s.LatestExecution.PipelineExecutionId)
		switch s.LatestExecution.Status {
		case types.StageExecutionStatusInProgress:
			return provider.StatusRunning, execID
		case types.StageExecutionStatusFailed:
			return provider.StatusFailed, execID
		case types.StageExecutionStatusStopped, types.StageExecutionStatusStopping:
			return provider.StatusUnknown, execID
		}
	}

	// All stages succeeded or pipeline has no executions yet.
	if len(stages) > 0 {
		return provider.StatusSuccess, ""
	}
	return provider.StatusIdle, ""
}

func mapAWSStatus(s types.PipelineExecutionStatus) provider.Status {
	switch s {
	case types.PipelineExecutionStatusInProgress:
		return provider.StatusRunning
	case types.PipelineExecutionStatusSucceeded:
		return provider.StatusSuccess
	case types.PipelineExecutionStatusFailed:
		return provider.StatusFailed
	case types.PipelineExecutionStatusStopped, types.PipelineExecutionStatusStopping:
		return provider.StatusUnknown
	case types.PipelineExecutionStatusSuperseded:
		return provider.StatusUnknown
	}
	return provider.StatusUnknown
}
