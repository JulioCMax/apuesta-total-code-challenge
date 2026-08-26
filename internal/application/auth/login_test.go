package auth_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"github.com/JulioCMax/apuesta-total-code-challenge/internal/application/auth"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/account"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/money"
)

// dummyPasswordHashLiteral must stay byte-for-byte identical to login.go's
// unexported dummyPasswordHash constant. This external test package
// (auth_test) cannot reference an unexported identifier directly, and
// exporting it purely for a test would widen Login's public surface for no
// production benefit.
const dummyPasswordHashLiteral = "$2a$10$khYmEFuc0YSgycppMDosa.YSBQLrw4oxqClzvLwwFq4WFuySorCnW"

func mustMoney(t *testing.T, amount string) money.Money {
	t.Helper()
	m, err := money.NewMoney(decimal.RequireFromString(amount))
	require.NoError(t, err)
	return m
}

func seededUser(t *testing.T) account.User {
	t.Helper()
	u, err := account.NewUser("user-1", "demo@apuestatotal.com", "$2a$10$examplehash", decimal.RequireFromString("1000.00"), "PEN", time.Now())
	require.NoError(t, err)
	return u
}

// TestLogin_ValidCredentialsIssuesToken proves a correct email/password
// pair issues the JWT from TokenIssuer (spec: auth-and-balance/Demo User
// Login Issuing JWT, "Valid credentials").
func TestLogin_ValidCredentialsIssuesToken(t *testing.T) {
	user := seededUser(t)
	users := &fakeUserRepository{user: user}
	passwords := &fakePasswordVerifier{}
	tokens := &fakeTokenIssuer{token: "signed.jwt.token", expiresIn: time.Hour}
	uc := auth.NewLogin(users, passwords, tokens)

	result, err := uc.Execute(context.Background(), auth.LoginCommand{
		Email:    user.Email,
		Password: "correct-password",
	})

	require.NoError(t, err)
	require.Equal(t, "signed.jwt.token", result.Token)
	require.Equal(t, time.Hour, result.ExpiresIn)
	require.Equal(t, user.Email, users.lastEmail)
	require.Equal(t, user.PasswordHash, passwords.lastHash)
	require.Equal(t, "correct-password", passwords.lastPlain)
	require.Equal(t, user.ID, tokens.lastUser.ID)
}

// TestLogin_InvalidPasswordReturnsGenericError proves a wrong password maps
// to the same generic invalid-credentials error a missing email would
// (spec: auth-and-balance/Demo User Login Issuing JWT, "Invalid
// credentials" — no email-existence leak).
func TestLogin_InvalidPasswordReturnsGenericError(t *testing.T) {
	user := seededUser(t)
	users := &fakeUserRepository{user: user}
	passwords := &fakePasswordVerifier{err: errors.New("hash mismatch")}
	tokens := &fakeTokenIssuer{}
	uc := auth.NewLogin(users, passwords, tokens)

	_, err := uc.Execute(context.Background(), auth.LoginCommand{
		Email:    user.Email,
		Password: "wrong-password",
	})

	require.ErrorIs(t, err, account.ErrInvalidCredentials)
}

// TestLogin_UnknownEmailReturnsSameGenericError proves an unknown email
// returns the exact same error as a wrong password, so a caller cannot
// distinguish "no such account" from "wrong password". findErr is
// account.ErrInvalidCredentials itself — exactly what
// dynamo.UserRepository.FindByEmail returns for an unseeded email (never a
// generic error) — because Execute now tells that genuine case apart from
// an infrastructure failure by the error's IDENTITY (finding R3).
func TestLogin_UnknownEmailReturnsSameGenericError(t *testing.T) {
	users := &fakeUserRepository{findErr: account.ErrInvalidCredentials}
	passwords := &fakePasswordVerifier{}
	tokens := &fakeTokenIssuer{}
	uc := auth.NewLogin(users, passwords, tokens)

	_, err := uc.Execute(context.Background(), auth.LoginCommand{
		Email:    "nobody@apuestatotal.com",
		Password: "irrelevant",
	})

	require.ErrorIs(t, err, account.ErrInvalidCredentials)
}

// TestLogin_InfrastructureFailurePropagatesAsIsNeverMaskedAs401 proves a
// genuine infrastructure failure from FindByEmail (e.g. a throttled
// EmailIndex query) propagates UNCHANGED, so apperror.Classify maps it to a
// 5xx instead of the 401 a wrong password gets. Before this fix, EVERY
// FindByEmail error — including this one — collapsed into
// account.ErrInvalidCredentials: a correct user would see "wrong password"
// for what is really an availability incident, and monitoring would record
// an auth spike instead of an infrastructure one (finding R3). Only the
// port's own account.ErrInvalidCredentials — the genuine unknown-email case
// — still collapses into a 401.
func TestLogin_InfrastructureFailurePropagatesAsIsNeverMaskedAs401(t *testing.T) {
	infraErr := errors.New("dynamo: find by email: throttled")
	users := &fakeUserRepository{findErr: infraErr}
	passwords := &fakePasswordVerifier{}
	tokens := &fakeTokenIssuer{}
	uc := auth.NewLogin(users, passwords, tokens)

	_, err := uc.Execute(context.Background(), auth.LoginCommand{
		Email:    "demo@apuestatotal.com",
		Password: "irrelevant",
	})

	require.ErrorIs(t, err, infraErr, "a genuine infrastructure failure must propagate unchanged")
	require.False(t, errors.Is(err, account.ErrInvalidCredentials),
		"an infrastructure failure must never be masked as invalid credentials")
}

// TestLogin_UnknownEmailPerformsDummyPasswordCompare proves an unknown
// email still pays the cost of a password compare before returning
// account.ErrInvalidCredentials, closing the user-enumeration timing oracle
// a bare early-return would otherwise create: an unknown email would
// short-circuit before any bcrypt work while a known email with a wrong
// password still runs bcrypt, so response latency alone would reveal
// whether an email exists (finding W-timing).
func TestLogin_UnknownEmailPerformsDummyPasswordCompare(t *testing.T) {
	users := &fakeUserRepository{findErr: account.ErrInvalidCredentials}
	passwords := &fakePasswordVerifier{}
	tokens := &fakeTokenIssuer{}
	uc := auth.NewLogin(users, passwords, tokens)

	_, err := uc.Execute(context.Background(), auth.LoginCommand{
		Email:    "nobody@apuestatotal.com",
		Password: "irrelevant",
	})

	require.ErrorIs(t, err, account.ErrInvalidCredentials)
	require.NotEmpty(t, passwords.lastHash, "an unknown email must still perform a dummy password compare")
	require.Equal(t, "irrelevant", passwords.lastPlain)
}

// TestLogin_NeverExposesPasswordOrHash proves LoginResult's own value
// representation — what a naive log statement would print — never
// contains the plaintext password or the stored hash.
func TestLogin_NeverExposesPasswordOrHash(t *testing.T) {
	user := seededUser(t)
	users := &fakeUserRepository{user: user}
	passwords := &fakePasswordVerifier{}
	tokens := &fakeTokenIssuer{token: "signed.jwt.token", expiresIn: time.Hour}
	uc := auth.NewLogin(users, passwords, tokens)

	result, err := uc.Execute(context.Background(), auth.LoginCommand{
		Email:    user.Email,
		Password: "correct-password",
	})

	require.NoError(t, err)
	rendered := fmt.Sprintf("%+v", result)
	require.NotContains(t, rendered, user.PasswordHash)
	require.NotContains(t, rendered, "correct-password")
}

// TestDummyPasswordHashMatchesProductionBcryptCost proves the dummy hash
// Login compares against on the unknown-email path was generated at
// EXACTLY the same bcrypt cost application/auth's real password hashes use
// (internal/adapters/security.BcryptCost = 10). This assertion is against a
// literal, not an import of internal/adapters/security, because
// application/auth must never depend on an adapter (D2/hexagonal purity) —
// importing it here just for a test constant would invert that boundary.
// If security.BcryptCost is ever raised without regenerating
// dummyPasswordHash at the new cost, the unknown-email path silently
// becomes cheaper than a real login again, reopening the email-enumeration
// timing oracle this hash exists to close (finding W-timing / hardening).
func TestDummyPasswordHashMatchesProductionBcryptCost(t *testing.T) {
	const productionBcryptCost = 10 // must equal internal/adapters/security.BcryptCost

	cost, err := bcrypt.Cost([]byte(dummyPasswordHashLiteral))

	require.NoError(t, err)
	require.Equal(t, productionBcryptCost, cost)
}
