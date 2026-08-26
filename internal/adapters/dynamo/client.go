package dynamo

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"

	"github.com/JulioCMax/apuesta-total-code-challenge/internal/platform/config"
)

// NewClient builds a *dynamodb.Client from cfg. cfg.DynamoEndpoint is the
// single switch between the two environments:
//
//   - Set (docker-compose, integration tests): BaseEndpoint overrides SDK
//     discovery so requests hit dynamodb-local, and the configured static
//     credentials are installed — dynamodb-local accepts any non-empty
//     pair and there is no credential chain to resolve locally.
//   - Empty (Lambda, real AWS): endpoint discovery and the DEFAULT
//     credential chain are both left untouched.
//
// Never install static credentials on the real-AWS path. The Lambda
// runtime exposes the execution role's TEMPORARY credentials through
// AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY / AWS_SESSION_TOKEN, and SigV4
// rejects that pair without its session token. Building a static provider
// from the id/secret alone silently discards the token and makes every
// DynamoDB call in production fail with UnrecognizedClientException, which
// is exactly why the endpoint — not the presence of credentials — decides.
func NewClient(ctx context.Context, cfg config.Config) (*dynamodb.Client, error) {
	local := cfg.DynamoEndpoint != ""

	opts := []func(*awsconfig.LoadOptions) error{awsconfig.WithRegion(cfg.AWSRegion)}
	if local {
		opts = append(opts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AWSAccessKeyID, cfg.AWSSecretAccessKey, ""),
		))
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("dynamo: load aws config: %w", err)
	}

	return dynamodb.NewFromConfig(awsCfg, func(o *dynamodb.Options) {
		if local {
			o.BaseEndpoint = aws.String(cfg.DynamoEndpoint)
		}
	}), nil
}
