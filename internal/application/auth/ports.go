// Package auth holds the Login, Balance and History use cases: demo-user
// authentication, balance lookup, and a caller's own bet history. All three
// consume caller-supplied ports (D2 — consumer-owned interfaces, no ports/
// folder) so the HTTP adapter never talks to DynamoDB or the JWT/bcrypt
// adapters directly.
package auth

import (
	"context"
	"time"

	"github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/account"
	domainbetslip "github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/betslip"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/money"
)

// UserRepository looks up a demo user by email and reads a user's current
// balance. internal/adapters/dynamo.UserRepository is the production
// implementation.
type UserRepository interface {
	FindByEmail(ctx context.Context, email string) (account.User, error)
	Balance(ctx context.Context, userID string) (money.Money, error)
}

// TokenIssuer issues and verifies the HS256 JWT used by every protected
// route. internal/adapters/security.JWT is the production implementation.
type TokenIssuer interface {
	Issue(u account.User) (token string, expiresIn time.Duration, err error)
	Verify(token string) (userID, email string, err error)
}

// PasswordVerifier compares a bcrypt hash against a plaintext password.
// internal/adapters/security.Bcrypt is the production implementation.
type PasswordVerifier interface {
	Verify(hash, plain string) error
}

// BetHistory lists a caller's own persisted bets (spec: bet-history/List
// Caller's Own Bets). internal/adapters/dynamo.BetRepository satisfies it
// structurally alongside application/betslip.BetRepository.
type BetHistory interface {
	ListByUser(ctx context.Context, userID string, limit int, cursor string) ([]domainbetslip.Bet, string, error)
}
