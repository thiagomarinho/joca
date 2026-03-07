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

func (c *Client) CurrentStatus(ctx context.Context) (provider.Run, error) {
	out, err := c.cp.GetPipelineState(ctx, &codepipeline.GetPipelineStateInput{
		Name: aws.String(c.pipelineName),
	})
	if err != nil {
		return provider.Run{}, fmt.Errorf("GetPipelineState: %w", err)
	}

	status := derivePipelineStatus(out.StageStates)
	return provider.Run{
		ID:        c.pipelineName,
		Status:    status,
		StartedAt: aws.ToTime(out.Updated),
		URL:       c.URL(),
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
		runs = append(runs, provider.Run{
			ID:        aws.ToString(s.PipelineExecutionId),
			Status:    mapAWSStatus(s.Status),
			StartedAt: aws.ToTime(s.StartTime),
			URL:       c.URL(),
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

func derivePipelineStatus(stages []types.StageState) provider.Status {
	for _, s := range stages {
		if s.LatestExecution == nil {
			continue
		}
		switch s.LatestExecution.Status {
		case types.StageExecutionStatusInProgress:
			return provider.StatusRunning
		case types.StageExecutionStatusFailed:
			return provider.StatusFailed
		case types.StageExecutionStatusStopped, types.StageExecutionStatusStopping:
			return provider.StatusUnknown
		}
		// Check for manual approval actions
		for _, action := range s.ActionStates {
			if action.LatestExecution != nil &&
				action.LatestExecution.Status == types.ActionExecutionStatusInProgress &&
				action.CurrentRevision == nil {
				return provider.StatusApproval
			}
		}
	}
	// All stages succeeded or none in progress
	if len(stages) > 0 {
		return provider.StatusSuccess
	}
	return provider.StatusIdle
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
