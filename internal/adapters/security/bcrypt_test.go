package security_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/security"
)

// TestBcrypt_HashThenVerifyMatches proves a hashed password verifies
// successfully against the exact plaintext it was hashed from, and that
// the stored hash never equals the plaintext.
func TestBcrypt_HashThenVerifyMatches(t *testing.T) {
	b := security.NewBcrypt()

	hash, err := b.Hash("correct horse battery staple")
	require.NoError(t, err)
	require.NotEqual(t, "correct horse battery staple", hash)

	err = b.Verify(hash, "correct horse battery staple")
	require.NoError(t, err)
}

// TestBcrypt_VerifyRejectsWrongPassword proves a mismatched plaintext is
// rejected, not silently accepted.
func TestBcrypt_VerifyRejectsWrongPassword(t *testing.T) {
	b := security.NewBcrypt()

	hash, err := b.Hash("correct horse battery staple")
	require.NoError(t, err)

	err = b.Verify(hash, "wrong password")
	require.Error(t, err)
}

// TestBcrypt_HashUsesCostTen proves the configured work factor is exactly
// 10 (design.md: "bcrypt cost 10 — cost 12 adds ~250ms per login on Lambda
// for no demo value").
func TestBcrypt_HashUsesCostTen(t *testing.T) {
	b := security.NewBcrypt()

	hash, err := b.Hash("password")
	require.NoError(t, err)

	cost, err := bcrypt.Cost([]byte(hash))
	require.NoError(t, err)
	require.Equal(t, 10, cost)
}
