package aws

import (
	"context"
	"testing"

	awsbase "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	cloudwatchtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/aws/aws-sdk-go-v2/service/codebuild"
	codebuildtypes "github.com/aws/aws-sdk-go-v2/service/codebuild/types"
	"github.com/aws/aws-sdk-go-v2/service/codepipeline"
	codepipelinetypes "github.com/aws/aws-sdk-go-v2/service/codepipeline/types"
)

type fakeActionClient struct {
	output *codepipeline.ListActionExecutionsOutput
}

func (f fakeActionClient) ListActionExecutions(context.Context, *codepipeline.ListActionExecutionsInput, ...func(*codepipeline.Options)) (*codepipeline.ListActionExecutionsOutput, error) {
	return f.output, nil
}

type fakeBuildClient struct {
	output *codebuild.BatchGetBuildsOutput
}

func (f fakeBuildClient) BatchGetBuilds(context.Context, *codebuild.BatchGetBuildsInput, ...func(*codebuild.Options)) (*codebuild.BatchGetBuildsOutput, error) {
	return f.output, nil
}

type fakeLogsClient struct {
	outputs []*cloudwatchlogs.GetLogEventsOutput
	calls   int
}

func (f *fakeLogsClient) GetLogEvents(context.Context, *cloudwatchlogs.GetLogEventsInput, ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.GetLogEventsOutput, error) {
	output := f.outputs[f.calls]
	f.calls++
	return output, nil
}

func TestLogSourcesDiscoversOnlyCodeBuildActions(t *testing.T) {
	buildID := "project-a:build-123"
	c := &Client{
		pipelineName: "delivery",
		actions: fakeActionClient{output: &codepipeline.ListActionExecutionsOutput{
			ActionExecutionDetails: []codepipelinetypes.ActionExecutionDetail{
				{
					StageName:  awsbase.String("Build"),
					ActionName: awsbase.String("Compile"),
					Input: &codepipelinetypes.ActionExecutionInput{ActionTypeId: &codepipelinetypes.ActionTypeId{
						Provider: awsbase.String("CodeBuild"),
					}},
					Output: &codepipelinetypes.ActionExecutionOutput{ExecutionResult: &codepipelinetypes.ActionExecutionResult{
						ExternalExecutionId: awsbase.String(buildID),
					}},
				},
				{
					StageName: awsbase.String("Deploy"),
					Input: &codepipelinetypes.ActionExecutionInput{ActionTypeId: &codepipelinetypes.ActionTypeId{
						Provider: awsbase.String("CodeDeploy"),
					}},
				},
			},
		}},
		builds: fakeBuildClient{output: &codebuild.BatchGetBuildsOutput{Builds: []codebuildtypes.Build{
			{
				Id:          awsbase.String(buildID),
				ProjectName: awsbase.String("project-a"),
				Logs: &codebuildtypes.LogsLocation{
					GroupName:  awsbase.String("/aws/codebuild/project-a"),
					StreamName: awsbase.String("build-123"),
				},
			},
		}}},
		logs: &fakeLogsClient{},
	}

	sources, err := c.logSources(context.Background(), "execution-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 1 {
		t.Fatalf("log sources = %d, want 1", len(sources))
	}
	if sources[0].Stage != "Build" || sources[0].Action != "Compile" || sources[0].Project != "project-a" {
		t.Errorf("unexpected source: %#v", sources[0])
	}
}

func TestLoadCloudWatchLogsPaginatesUntilTokenRepeats(t *testing.T) {
	logs := &fakeLogsClient{outputs: []*cloudwatchlogs.GetLogEventsOutput{
		{
			Events:           []cloudwatchtypes.OutputLogEvent{{Message: awsbase.String("first\n")}},
			NextForwardToken: awsbase.String("next"),
		},
		{
			Events:           []cloudwatchtypes.OutputLogEvent{{Message: awsbase.String("second")}},
			NextForwardToken: awsbase.String("next"),
		},
	}}

	content, err := loadCloudWatchLogs(context.Background(), logs, "group", "stream")
	if err != nil {
		t.Fatal(err)
	}
	if content != "first\nsecond\n" {
		t.Errorf("content = %q", content)
	}
	if logs.calls != 2 {
		t.Errorf("GetLogEvents calls = %d, want 2", logs.calls)
	}
}
