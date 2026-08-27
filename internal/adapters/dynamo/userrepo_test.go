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

// TestPutUser_OverwritesAnExistingProfile proves PutUser (unconditional)
// forces an overwrite of an already-seeded profile, the primitive
// cmd/seed's SEED_RESET=true flag needs (design.md's seeding contract:
// "SEED_RESET=true forces an overwrite" — PutUserIfAbsent alone can never
// satisfy that, since its whole point is refusing to clobber).
func TestPutUser_OverwritesAnExistingProfile(t *testing.T) {
	client, table := requireDynamoLocal(t)
	repo := dynamo.NewUserRepository(client, table)
	seeded := mustSeedUser(t, repo, "user-5", "reset@apuestatotal.com", 1000)

	reset := seeded
	resetBalance, err := money.NewMoneyFromFloat(1)
	require.NoError(t, err)
	reset.Balance = resetBalance

	require.NoError(t, repo.PutUser(context.Background(), reset))

	got, err := repo.Balance(context.Background(), "user-5")
	require.NoError(t, err)
	require.Equal(t, "1.00", got.String(), "PutUser must overwrite the existing balance")
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

// TestSeedUser_IsIdempotentAcrossRuns proves re-seeding the same demo
// account does not create a second profile for it.
//
// This is the regression guard for a real defect: seeding minted a fresh
// ULID on every run, so PutUserIfAbsent's attribute_not_exists(PK)
// condition was evaluated against a partition key that had never existed
// and therefore always passed. Every boot and every deployment quietly
// added another profile for the same email, and login — which resolves
// through the email index — then returned whichever duplicate the index
// happened to yield.
func TestSeedUser_IsIdempotentAcrossRuns(t *testing.T) {
	client, table := requireDynamoLocal(t)
	repo := dynamo.NewUserRepository(client, table)
	ctx := context.Background()
	email := "idempotent@apuestatotal.com"

	first := buildUser(t, "seed-id-first", email, 1000)
	require.NoError(t, repo.SeedUser(ctx, first, false))

	// A second run with a DIFFERENT generated id, exactly as a redeploy
	// would produce.
	second := buildUser(t, "seed-id-second", email, 250)
	err := repo.SeedUser(ctx, second, false)

	require.ErrorIs(t, err, dynamo.ErrUserAlreadyExists,
		"a re-run must report the account as already seeded, not write another one")

	got, err := repo.FindByEmail(ctx, email)
	require.NoError(t, err)
	require.Equal(t, "seed-id-first", got.ID, "the original profile must survive")
	require.Equal(t, "1000.00", got.Balance.String(), "a re-run must never clobber a balance")
}

// TestSeedUser_ResetOverwritesTheSameProfile proves SEED_RESET restores
// the initial balance by overwriting the existing profile, keeping its
// identity, instead of adding a fresh one beside it.
//
// The identity assertion is the point: a reset that wrote a new id would
// also report success and would also show the right balance on the next
// lookup, while silently leaving the spent profile behind.
func TestSeedUser_ResetOverwritesTheSameProfile(t *testing.T) {
	client, table := requireDynamoLocal(t)
	repo := dynamo.NewUserRepository(client, table)
	ctx := context.Background()
	email := "reset@apuestatotal.com"

	original := buildUser(t, "reset-id-original", email, 1000)
	require.NoError(t, repo.SeedUser(ctx, original, false))

	// Simulate a played-with account.
	spent := original
	spent.Balance = mustMoneyValue(t, 680)
	require.NoError(t, repo.PutUser(ctx, spent))

	// Reset arrives with a newly generated id, as the seeder produces.
	replacement := buildUser(t, "reset-id-brand-new", email, 1000)
	require.NoError(t, repo.SeedUser(ctx, replacement, true))

	got, err := repo.FindByEmail(ctx, email)
	require.NoError(t, err)
	require.Equal(t, "1000.00", got.Balance.String(), "reset must restore the initial balance")
	require.Equal(t, "reset-id-original", got.ID,
		"reset must overwrite the existing profile, never add a second one")
}

func buildUser(t *testing.T, id, email string, balance float64) account.User {
	t.Helper()
	return account.User{
		ID:           id,
		Email:        email,
		PasswordHash: "bcrypt-hash-placeholder",
		Balance:      mustMoneyValue(t, balance),
		Currency:     "PEN",
		CreatedAt:    time.Now().UTC(),
	}
}

func mustMoneyValue(t *testing.T, v float64) money.Money {
	t.Helper()
	m, err := money.NewMoneyFromFloat(v)
	require.NoError(t, err)
	return m
}
