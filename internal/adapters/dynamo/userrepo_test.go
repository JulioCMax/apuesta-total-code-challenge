package dynamo_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/dynamo"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/account"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/money"
)

func mustSeedUser(t *testing.T, repo *dynamo.UserRepository, id, email string, balance float64) account.User {
	t.Helper()
	bal, err := money.NewMoneyFromFloat(balance)
	require.NoError(t, err)
	user := account.User{
		ID:           id,
		Email:        email,
		PasswordHash: "bcrypt-hash-placeholder",
		Balance:      bal,
		Currency:     "PEN",
		CreatedAt:    time.Now().UTC(),
	}
	require.NoError(t, repo.PutUserIfAbsent(context.Background(), user))
	return user
}

// TestFindByEmail_ReturnsTheSeededUser proves a user seeded via
// PutUserIfAbsent is found by FindByEmail through the EmailIndex GSI
// (design.md's DynamoDB Single-Table Design).
func TestFindByEmail_ReturnsTheSeededUser(t *testing.T) {
	client, table := requireDynamoLocal(t)
	repo := dynamo.NewUserRepository(client, table)
	seeded := mustSeedUser(t, repo, "user-1", "demo@apuestatotal.com", 1000)

	got, err := repo.FindByEmail(context.Background(), "demo@apuestatotal.com")

	require.NoError(t, err)
	require.Equal(t, seeded.ID, got.ID)
	require.Equal(t, seeded.PasswordHash, got.PasswordHash)
	require.Equal(t, "1000.00", got.Balance.String())
}

// TestFindByEmail_IsCaseInsensitive proves a login attempt with a
// differently-cased email still resolves to the seeded record (GSI1PK is
// always built from the lowercased email).
func TestFindByEmail_IsCaseInsensitive(t *testing.T) {
	client, table := requireDynamoLocal(t)
	repo := dynamo.NewUserRepository(client, table)
	seeded := mustSeedUser(t, repo, "user-2", "mixedcase@apuestatotal.com", 500)

	got, err := repo.FindByEmail(context.Background(), "MixedCase@ApuestaTotal.com")

	require.NoError(t, err)
	require.Equal(t, seeded.ID, got.ID)
}

// TestFindByEmail_ReturnsInvalidCredentialsForUnknownEmail proves an
// unseeded email yields the same typed error a wrong password would (spec:
// auth-and-balance/Demo User Login Issuing JWT — "without revealing
// whether the email exists").
func TestFindByEmail_ReturnsInvalidCredentialsForUnknownEmail(t *testing.T) {
	client, table := requireDynamoLocal(t)
	repo := dynamo.NewUserRepository(client, table)

	_, err := repo.FindByEmail(context.Background(), "nobody@apuestatotal.com")

	require.ErrorIs(t, err, account.ErrInvalidCredentials)
}

// TestBalance_ReturnsTheStoredBalance proves Balance reads the exact
// decimal amount stored on the profile item.
func TestBalance_ReturnsTheStoredBalance(t *testing.T) {
	client, table := requireDynamoLocal(t)
	repo := dynamo.NewUserRepository(client, table)
	mustSeedUser(t, repo, "user-3", "balance@apuestatotal.com", 2500.75)

	got, err := repo.Balance(context.Background(), "user-3")

	require.NoError(t, err)
	require.Equal(t, "2500.75", got.String())
}

// TestPutUserIfAbsent_NeverClobbersAnExistingBalance proves re-running the
// seed for the same user id is a no-op on an existing balance (design.md's
// seeding contract: "PutItem each demo user with attribute_not_exists(PK)
// — re-runs never clobber a played-with balance").
func TestPutUserIfAbsent_NeverClobbersAnExistingBalance(t *testing.T) {
	client, table := requireDynamoLocal(t)
	repo := dynamo.NewUserRepository(client, table)
	seeded := mustSeedUser(t, repo, "user-4", "noclobber@apuestatotal.com", 1000)

	changed := seeded
	changedBalance, err := money.NewMoneyFromFloat(1)
	require.NoError(t, err)
	changed.Balance = changedBalance

	err = repo.PutUserIfAbsent(context.Background(), changed)
	require.ErrorIs(t, err, dynamo.ErrUserAlreadyExists)

	got, err := repo.Balance(context.Background(), "user-4")
	require.NoError(t, err)
	require.Equal(t, "1000.00", got.String(), "a re-run seed must never clobber a played-with balance")
}

// TestCreateTable_IsIdempotent proves calling EnsureTable a second time
// against an already-created table is a safe no-op (design.md: "swallow
// ResourceInUseException"), matching the contract docker-compose's one-shot
// seeder and scripts/deploy-aws.sh both rely on.
func TestCreateTable_IsIdempotent(t *testing.T) {
	client, table := requireDynamoLocal(t) // table already created once by the helper

	err := dynamo.EnsureTable(context.Background(), client, table)

	require.NoError(t, err)
}
