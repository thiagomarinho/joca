package aws

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/codebuild"
	"github.com/aws/aws-sdk-go-v2/service/codepipeline"
	"github.com/aws/aws-sdk-go-v2/service/codepipeline/types"

	"github.com/thiagomarinho/joca/internal/provider"
)

type actionExecutionsAPI interface {
	ListActionExecutions(context.Context, *codepipeline.ListActionExecutionsInput, ...func(*codepipeline.Options)) (*codepipeline.ListActionExecutionsOutput, error)
}

type codeBuildAPI interface {
	BatchGetBuilds(context.Context, *codebuild.BatchGetBuildsInput, ...func(*codebuild.Options)) (*codebuild.BatchGetBuildsOutput, error)
}

type cloudWatchLogsAPI interface {
	GetLogEvents(context.Context, *cloudwatchlogs.GetLogEventsInput, ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.GetLogEventsOutput, error)
}

func (c *Client) withLogSources(run provider.Run) provider.Run {
	if run.ID == "" || c.actions == nil || c.builds == nil || c.logs == nil {
		return run
	}
	executionID := run.ID
	run.LogSources = func(ctx context.Context) ([]provider.LogSource, error) {
		return c.logSources(ctx, executionID)
	}
	return run
}

func (c *Client) logSources(ctx context.Context, executionID string) ([]provider.LogSource, error) {
	var details []types.ActionExecutionDetail
	var nextToken *string
	for {
		out, err := c.actions.ListActionExecutions(ctx, &codepipeline.ListActionExecutionsInput{
			PipelineName: aws.String(c.pipelineName),
			Filter: &types.ActionExecutionFilter{
				PipelineExecutionId: aws.String(executionID),
			},
			NextToken: nextToken,
		})
		if err != nil {
			return nil, fmt.Errorf("listing CodePipeline actions: %w", err)
		}
		details = append(details, out.ActionExecutionDetails...)
		if out.NextToken == nil {
			break
		}
		nextToken = out.NextToken
	}

	type actionBuild struct {
		detail  types.ActionExecutionDetail
		buildID string
	}
	var actions []actionBuild
	var buildIDs []string
	for _, detail := range details {
		if detail.Input == nil || detail.Input.ActionTypeId == nil ||
			aws.ToString(detail.Input.ActionTypeId.Provider) != "CodeBuild" ||
			detail.Output == nil || detail.Output.ExecutionResult == nil {
			continue
		}
		buildID := aws.ToString(detail.Output.ExecutionResult.ExternalExecutionId)
		if buildID == "" {
			continue
		}
		actions = append(actions, actionBuild{detail: detail, buildID: buildID})
		buildIDs = append(buildIDs, buildID)
	}
	if len(buildIDs) == 0 {
		return nil, nil
	}

	buildOut, err := c.builds.BatchGetBuilds(ctx, &codebuild.BatchGetBuildsInput{Ids: buildIDs})
	if err != nil {
		return nil, fmt.Errorf("getting CodeBuild builds: %w", err)
	}
	buildsByID := make(map[string]int, len(buildOut.Builds))
	for i := range buildOut.Builds {
		buildsByID[aws.ToString(buildOut.Builds[i].Id)] = i
	}

	sources := make([]provider.LogSource, 0, len(actions))
	for _, action := range actions {
		buildIndex, ok := buildsByID[action.buildID]
		if !ok {
			continue
		}
		build := buildOut.Builds[buildIndex]
		if build.Logs == nil {
			continue
		}
		group := aws.ToString(build.Logs.GroupName)
		stream := aws.ToString(build.Logs.StreamName)
		if group == "" || stream == "" {
			continue
		}
		groupName, streamName := group, stream
		sources = append(sources, provider.LogSource{
			Stage:   aws.ToString(action.detail.StageName),
			Action:  aws.ToString(action.detail.ActionName),
			Project: aws.ToString(build.ProjectName),
			Status:  mapActionStatus(action.detail.Status),
			Logs: func(ctx context.Context) (string, error) {
				return loadCloudWatchLogs(ctx, c.logs, groupName, streamName)
			},
		})
	}
	return sources, nil
}

func loadCloudWatchLogs(ctx context.Context, client cloudWatchLogsAPI, group, stream string) (string, error) {
	var sb strings.Builder
	var nextToken *string
	for {
		out, err := client.GetLogEvents(ctx, &cloudwatchlogs.GetLogEventsInput{
			LogGroupName:  aws.String(group),
			LogStreamName: aws.String(stream),
			NextToken:     nextToken,
			StartFromHead: aws.Bool(true),
		})
		if err != nil {
			return "", fmt.Errorf("getting CloudWatch log events: %w", err)
		}
		for _, event := range out.Events {
			message := aws.ToString(event.Message)
			sb.WriteString(message)
			if message != "" && !strings.HasSuffix(message, "\n") {
				sb.WriteByte('\n')
			}
		}
		if out.NextForwardToken == nil || (nextToken != nil && *out.NextForwardToken == *nextToken) {
			break
		}
		nextToken = out.NextForwardToken
	}
	return sb.String(), nil
}

func mapActionStatus(status types.ActionExecutionStatus) provider.Status {
	switch status {
	case types.ActionExecutionStatusInProgress:
		return provider.StatusRunning
	case types.ActionExecutionStatusSucceeded:
		return provider.StatusSuccess
	case types.ActionExecutionStatusFailed:
		return provider.StatusFailed
	case types.ActionExecutionStatusAbandoned:
		return provider.StatusCancelled
	}
	return provider.StatusUnknown
}
