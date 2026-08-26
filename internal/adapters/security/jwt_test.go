package security_test

import (
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"

	"github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/security"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/account"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/money"
)

func mustDemoUser(t *testing.T) account.User {
	t.Helper()
	balance, err := money.NewMoneyFromFloat(1000)
	require.NoError(t, err)
	return account.User{
		ID:        "user-1",
		Email:     "demo@apuestatotal.com",
		Balance:   balance,
		Currency:  "PEN",
		CreatedAt: time.Now().UTC(),
	}
}

// TestJWT_IssueThenVerifyRoundTrip proves a token issued by Issue verifies
// back to exactly the same user identity and email, and that ExpiresIn
// reflects the configured TTL (spec: auth-and-balance/Demo User Login
// Issuing JWT).
func TestJWT_IssueThenVerifyRoundTrip(t *testing.T) {
	j := security.NewJWT("test-secret", time.Hour)
	user := mustDemoUser(t)

	token, expiresIn, err := j.Issue(user)
	require.NoError(t, err)
	require.NotEmpty(t, token)
	require.Equal(t, time.Hour, expiresIn)

	userID, email, err := j.Verify(token)
	require.NoError(t, err)
	require.Equal(t, user.ID, userID)
	require.Equal(t, user.Email, email)
}

// TestJWT_VerifyRejectsAlgNone proves the classic "alg: none" forgery is
// rejected: a token signed with the unsafe none method must never verify,
// even though it carries otherwise well-formed claims.
func TestJWT_VerifyRejectsAlgNone(t *testing.T) {
	j := security.NewJWT("test-secret", time.Hour)

	forged := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.RegisteredClaims{
		Subject:   "attacker",
		Issuer:    "apuesta-total-api",
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	})
	tokenString, err := forged.SignedString(jwt.UnsafeAllowNoneSignatureType)
	require.NoError(t, err)

	_, _, err = j.Verify(tokenString)
	require.Error(t, err)
}

// TestJWT_VerifyRejectsRS256Substitution proves an RS256-signed token is
// rejected outright: WithValidMethods restricts verification to HS256, so
// algorithm confusion never even reaches signature validation.
func TestJWT_VerifyRejectsRS256Substitution(t *testing.T) {
	j := security.NewJWT("test-secret", time.Hour)

	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	forged := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.RegisteredClaims{
		Subject:   "attacker",
		Issuer:    "apuesta-total-api",
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	})
	tokenString, err := forged.SignedString(rsaKey)
	require.NoError(t, err)

	_, _, err = j.Verify(tokenString)
	require.Error(t, err)
}

// TestJWT_VerifyRejectsWrongIssuer proves the issuer claim is checked, not
// merely present: a token signed with the correct secret but a different
// "iss" value must be rejected (design.md: "jwt.WithIssuer(...)").
func TestJWT_VerifyRejectsWrongIssuer(t *testing.T) {
	j := security.NewJWT("test-secret", time.Hour)

	forged := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Subject:   "user-1",
		Issuer:    "someone-else",
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	})
	tokenString, err := forged.SignedString([]byte("test-secret"))
	require.NoError(t, err)

	_, _, err = j.Verify(tokenString)
	require.Error(t, err)
}

// TestJWT_VerifyRejectsWrongSecret proves a token signed with a different
// secret is rejected even though every claim (including issuer) is
// well-formed.
func TestJWT_VerifyRejectsWrongSecret(t *testing.T) {
	issuer := security.NewJWT("secret-a", time.Hour)
	verifier := security.NewJWT("secret-b", time.Hour)
	user := mustDemoUser(t)

	token, _, err := issuer.Issue(user)
	require.NoError(t, err)

	_, _, err = verifier.Verify(token)
	require.Error(t, err)
}

// TestJWT_VerifyRejectsExpiredToken proves a token whose TTL has already
// elapsed is rejected, not silently accepted.
func TestJWT_VerifyRejectsExpiredToken(t *testing.T) {
	j := security.NewJWT("test-secret", -time.Hour) // already expired at issuance
	user := mustDemoUser(t)

	token, _, err := j.Issue(user)
	require.NoError(t, err)

	_, _, err = j.Verify(token)
	require.Error(t, err)
}
