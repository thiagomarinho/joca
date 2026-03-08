package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/codepipeline"
)

// ListPipelines fetches all CodePipeline pipeline names in the given region.
// Uses the standard credential chain + optional profile. Paginates automatically.
func ListPipelines(ctx context.Context, region, profile string) ([]string, error) {
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
	cp := codepipeline.NewFromConfig(cfg)

	var names []string
	var nextToken *string
	for {
		out, err := cp.ListPipelines(ctx, &codepipeline.ListPipelinesInput{
			NextToken: nextToken,
		})
		if err != nil {
			return nil, fmt.Errorf("ListPipelines: %w", err)
		}
		for _, p := range out.Pipelines {
			names = append(names, aws.ToString(p.Name))
		}
		if out.NextToken == nil {
			break
		}
		nextToken = out.NextToken
	}
	return names, nil
}
