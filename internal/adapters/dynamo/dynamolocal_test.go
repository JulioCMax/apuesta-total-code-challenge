package dynamo_test

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"

	"github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/dynamo"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/platform/config"
)

// testEndpoint is the dynamodb-local endpoint every integration test in
// this package targets, overridable via DYNAMO_ENDPOINT.
func testEndpoint() string {
	if e := os.Getenv("DYNAMO_ENDPOINT"); e != "" {
		return e
	}
	return "http://localhost:8000"
}

// requireDynamoLocal skips t in -short mode or when the dynamodb-local
// endpoint is unreachable (design.md's Testing Strategy: "skipped when the
// endpoint is unreachable"), then returns a ready client plus a freshly
// created, uniquely-named table dropped in t.Cleanup. Every test gets its
// own table so concurrent/idempotency scenarios never interfere with each
// other.
func requireDynamoLocal(t *testing.T) (*dynamodb.Client, string) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping dynamodb-local integration test in -short mode")
	}

	cfg := config.Config{
		AWSRegion:          "us-east-1",
		AWSAccessKeyID:     "local",
		AWSSecretAccessKey: "local",
		DynamoEndpoint:     testEndpoint(),
	}
	client, err := dynamo.NewClient(context.Background(), cfg)
	if err != nil {
		t.Skipf("dynamodb-local endpoint unreachable: %v", err)
	}

	pingCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := client.ListTables(pingCtx, &dynamodb.ListTablesInput{}); err != nil {
		t.Skipf("dynamodb-local endpoint unreachable at %s: %v", testEndpoint(), err)
	}

	table := fmt.Sprintf("apuesta-total-test-%d-%d", time.Now().UnixNano(), rand.Int63())
	if err := dynamo.EnsureTable(context.Background(), client, table); err != nil {
		t.Fatalf("create test table %q: %v", table, err)
	}
	t.Cleanup(func() {
		_, _ = client.DeleteTable(context.Background(), &dynamodb.DeleteTableInput{TableName: &table})
	})

	return client, table
}
