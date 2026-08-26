package auth_test

import (
	"context"
	"time"

	"github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/account"
	domainbetslip "github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/betslip"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/money"
)

// fakeUserRepository is a test double for auth.UserRepository, shared by
// login_test.go and balance_test.go.
type fakeUserRepository struct {
	user       account.User
	findErr    error
	balance    money.Money
	balanceErr error

	lastEmail  string
	lastUserID string
}

func (f *fakeUserRepository) FindByEmail(_ context.Context, email string) (account.User, error) {
	f.lastEmail = email
	if f.findErr != nil {
		return account.User{}, f.findErr
	}
	return f.user, nil
}

func (f *fakeUserRepository) Balance(_ context.Context, userID string) (money.Money, error) {
	f.lastUserID = userID
	if f.balanceErr != nil {
		return money.Money{}, f.balanceErr
	}
	return f.balance, nil
}

// fakePasswordVerifier is a test double for auth.PasswordVerifier.
type fakePasswordVerifier struct {
	err error

	lastHash, lastPlain string
}

func (f *fakePasswordVerifier) Verify(hash, plain string) error {
	f.lastHash, f.lastPlain = hash, plain
	return f.err
}

// fakeTokenIssuer is a test double for auth.TokenIssuer.
type fakeTokenIssuer struct {
	token     string
	expiresIn time.Duration
	err       error

	lastUser account.User
}

func (f *fakeTokenIssuer) Issue(u account.User) (string, time.Duration, error) {
	f.lastUser = u
	if f.err != nil {
		return "", 0, f.err
	}
	return f.token, f.expiresIn, nil
}

func (f *fakeTokenIssuer) Verify(_ string) (string, string, error) {
	return "", "", nil
}

// fakeBetHistory is a test double for auth.BetHistory, keyed by userID so
// tests can prove one caller never sees another caller's bets.
type fakeBetHistory struct {
	betsByUser map[string][]domainbetslip.Bet
	err        error

	lastUserID string
	lastLimit  int
	lastCursor string
}

func (f *fakeBetHistory) ListByUser(_ context.Context, userID string, limit int, cursor string) ([]domainbetslip.Bet, string, error) {
	f.lastUserID, f.lastLimit, f.lastCursor = userID, limit, cursor
	if f.err != nil {
		return nil, "", f.err
	}
	return f.betsByUser[userID], "", nil
}
