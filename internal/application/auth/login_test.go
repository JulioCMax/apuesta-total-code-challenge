package auth_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/JulioCMax/apuesta-total-code-challenge/internal/application/auth"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/account"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/money"
)

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
// distinguish "no such account" from "wrong password".
func TestLogin_UnknownEmailReturnsSameGenericError(t *testing.T) {
	users := &fakeUserRepository{findErr: errors.New("not found")}
	passwords := &fakePasswordVerifier{}
	tokens := &fakeTokenIssuer{}
	uc := auth.NewLogin(users, passwords, tokens)

	_, err := uc.Execute(context.Background(), auth.LoginCommand{
		Email:    "nobody@apuestatotal.com",
		Password: "irrelevant",
	})

	require.ErrorIs(t, err, account.ErrInvalidCredentials)
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
