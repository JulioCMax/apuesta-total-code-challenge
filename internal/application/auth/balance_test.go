package auth_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/JulioCMax/apuesta-total-code-challenge/internal/application/auth"
)

// TestBalance_ReturnsCallerBalance proves the Balance use case returns
// exactly the caller's balance from UserRepository (spec: auth-and-
// balance/Balance Query).
func TestBalance_ReturnsCallerBalance(t *testing.T) {
	repo := &fakeUserRepository{balance: mustMoney(t, "1000.00")}
	uc := auth.NewBalance(repo)

	got, err := uc.Execute(context.Background(), "user-1")

	require.NoError(t, err)
	require.Equal(t, "1000.00", got.String())
	require.Equal(t, "user-1", repo.lastUserID)
}

// TestBalance_PropagatesRepositoryError proves a repository failure
// surfaces unchanged rather than being swallowed.
func TestBalance_PropagatesRepositoryError(t *testing.T) {
	wantErr := errors.New("user not found")
	repo := &fakeUserRepository{balanceErr: wantErr}
	uc := auth.NewBalance(repo)

	_, err := uc.Execute(context.Background(), "user-1")

	require.ErrorIs(t, err, wantErr)
}
