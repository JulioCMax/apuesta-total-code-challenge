package dynamo_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/dynamo"
)

// TestUserPK_FormatsThePartitionKey proves the shared user partition key
// format (design.md's single-table design: PK = USER#<userId>).
func TestUserPK_FormatsThePartitionKey(t *testing.T) {
	require.Equal(t, "USER#user-1", dynamo.UserPK("user-1"))
}

// TestBetSK_FormatsTheSortKey proves the bet sort key format
// (SK = BET#<ulid>), so a user's history queries with
// begins_with(SK, "BET#").
func TestBetSK_FormatsTheSortKey(t *testing.T) {
	require.Equal(t, "BET#01ARZ3NDEKTSV4RRFFQ69G5FAV", dynamo.BetSK("01ARZ3NDEKTSV4RRFFQ69G5FAV"))
}

// TestIdempotencySK_FormatsTheSortKey proves the idempotency sort key
// format (SK = IDEMP#<key>).
func TestIdempotencySK_FormatsTheSortKey(t *testing.T) {
	require.Equal(t, "IDEMP#client-key-1", dynamo.IdempotencySK("client-key-1"))
}

// TestEmailGSI1PK_LowercasesTheEmail proves the EmailIndex GSI partition
// key is always lowercased, so a case-varying login attempt still resolves
// to the same seeded record (GSI1PK = EMAIL#<lowercased email>).
func TestEmailGSI1PK_LowercasesTheEmail(t *testing.T) {
	require.Equal(t, "EMAIL#demo@apuestatotal.com", dynamo.EmailGSI1PK("Demo@ApuestaTotal.com"))
}
