package dynamo_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/dynamo"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/platform/config"
)

// TestNewClient_WithoutLocalEndpointUsesTheDefaultCredentialChain proves
// the client does NOT install static credentials when no local endpoint is
// configured, so AWS's own credential chain resolves.
//
// This is the Lambda case. The Lambda runtime injects the execution role's
// TEMPORARY credentials as AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY and
// AWS_SESSION_TOKEN, and SigV4 rejects the pair without that session
// token. Overriding the provider with a static pair built from an empty
// session token therefore breaks every DynamoDB call in production with
// UnrecognizedClientException.
func TestNewClient_WithoutLocalEndpointUsesTheDefaultCredentialChain(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "ASIAROLEKEY")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "role-secret")
	t.Setenv("AWS_SESSION_TOKEN", "role-session-token")

	client, err := dynamo.NewClient(context.Background(), config.Config{
		AWSRegion:          "us-east-1",
		AWSAccessKeyID:     "ASIAROLEKEY",
		AWSSecretAccessKey: "role-secret",
		DynamoEndpoint:     "",
	})
	require.NoError(t, err)

	require.Nil(t, client.Options().BaseEndpoint, "no endpoint override must be applied when targeting real AWS")

	creds, err := client.Options().Credentials.Retrieve(context.Background())
	require.NoError(t, err)
	require.Equal(t, "role-session-token", creds.SessionToken,
		"the execution role's session token must survive; static credentials would discard it")
}

// TestNewClient_WithLocalEndpointInstallsStaticCredentials is the
// triangulation case: dynamodb-local needs a non-empty static pair and the
// base endpoint override, and neither may depend on any ambient AWS
// configuration.
func TestNewClient_WithLocalEndpointInstallsStaticCredentials(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "should-be-ignored")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "should-be-ignored")
	t.Setenv("AWS_SESSION_TOKEN", "should-be-ignored")

	client, err := dynamo.NewClient(context.Background(), config.Config{
		AWSRegion:          "us-east-1",
		AWSAccessKeyID:     "local",
		AWSSecretAccessKey: "local",
		DynamoEndpoint:     "http://localhost:8000",
	})
	require.NoError(t, err)

	require.NotNil(t, client.Options().BaseEndpoint)
	require.Equal(t, "http://localhost:8000", *client.Options().BaseEndpoint)

	creds, err := client.Options().Credentials.Retrieve(context.Background())
	require.NoError(t, err)
	require.Equal(t, "local", creds.AccessKeyID)
	require.Equal(t, "local", creds.SecretAccessKey)
}
