package auth

import (
	"context"
	"time"

	"github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/account"
)

// LoginCommand is the input to the Login use case.
type LoginCommand struct {
	Email    string
	Password string
}

// LoginResult is the output of the Login use case. It deliberately carries
// only the issued token and its lifetime — never the user's password hash
// or any other credential material — so any naive logging of this value
// can never leak a secret.
type LoginResult struct {
	Token     string
	ExpiresIn time.Duration
}

// Login is the "demo user login" use case (spec: auth-and-balance/Demo User
// Login Issuing JWT).
type Login struct {
	users     UserRepository
	passwords PasswordVerifier
	tokens    TokenIssuer
}

// dummyPasswordHash is a fixed, valid bcrypt hash (cost 10, matching
// security.BcryptCost) with no known corresponding plaintext. Execute
// compares against it on the unknown-email path so that path costs the
// same order of magnitude as a known email with a wrong password, instead
// of short-circuiting before any bcrypt work runs.
const dummyPasswordHash = "$2a$10$khYmEFuc0YSgycppMDosa.YSBQLrw4oxqClzvLwwFq4WFuySorCnW"

// NewLogin builds a Login use case backed by users, passwords and tokens.
func NewLogin(users UserRepository, passwords PasswordVerifier, tokens TokenIssuer) *Login {
	return &Login{users: users, passwords: passwords, tokens: tokens}
}

// Execute authenticates cmd's email/password and issues a JWT on success.
// An unknown email and a wrong password both return the exact same
// account.ErrInvalidCredentials, so a caller can never learn whether an
// email exists from the response alone — including from response timing:
// an unknown email still pays the cost of one bcrypt compare (against
// dummyPasswordHash) before returning, closing the enumeration timing
// oracle a bare early-return would otherwise create (finding W-timing).
func (l *Login) Execute(ctx context.Context, cmd LoginCommand) (LoginResult, error) {
	user, err := l.users.FindByEmail(ctx, cmd.Email)
	if err != nil {
		_ = l.passwords.Verify(dummyPasswordHash, cmd.Password)
		return LoginResult{}, account.ErrInvalidCredentials
	}

	if err := l.passwords.Verify(user.PasswordHash, cmd.Password); err != nil {
		return LoginResult{}, account.ErrInvalidCredentials
	}

	token, expiresIn, err := l.tokens.Issue(user)
	if err != nil {
		return LoginResult{}, err
	}

	return LoginResult{Token: token, ExpiresIn: expiresIn}, nil
}
