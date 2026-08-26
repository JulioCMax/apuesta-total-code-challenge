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

// NewClient builds a *dynamodb.Client from cfg. When cfg.DynamoEndpoint is
// set (the local/docker-compose default), BaseEndpoint overrides SDK
// discovery so requests hit dynamodb-local instead of real AWS; an empty
// DynamoEndpoint (the Lambda default) leaves discovery untouched, so the
// same code path targets real AWS in production. This constructor has no
// dedicated unit test: it is exercised end to end by every integration
// test in this package, which is the only way to prove a *dynamodb.Client
// actually reaches an endpoint.
func NewClient(ctx context.Context, cfg config.Config) (*dynamodb.Client, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(cfg.AWSRegion),
		awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AWSAccessKeyID, cfg.AWSSecretAccessKey, ""),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("dynamo: load aws config: %w", err)
	}

	return dynamodb.NewFromConfig(awsCfg, func(o *dynamodb.Options) {
		if cfg.DynamoEndpoint != "" {
			o.BaseEndpoint = aws.String(cfg.DynamoEndpoint)
		}
	}), nil
}
