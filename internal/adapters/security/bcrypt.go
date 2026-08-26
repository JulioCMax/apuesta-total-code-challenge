package security

import "golang.org/x/crypto/bcrypt"

// BcryptCost is the work factor used to hash every password (design.md:
// cost 10 — cost 12 adds ~250ms per login on Lambda for no demo value).
const BcryptCost = 10

// Bcrypt hashes and verifies passwords via golang.org/x/crypto/bcrypt.
// Verify implements application/auth.PasswordVerifier; Hash is used by
// cmd/seed (Phase 13) to store the demo users' credentials.
type Bcrypt struct{}

// NewBcrypt builds a Bcrypt hasher/verifier.
func NewBcrypt() *Bcrypt {
	return &Bcrypt{}
}

// Hash returns the bcrypt hash of plain at BcryptCost.
func (*Bcrypt) Hash(plain string) (string, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(plain), BcryptCost)
	if err != nil {
		return "", err
	}
	return string(hashed), nil
}

// Verify reports a non-nil error when plain does not match hash.
func (*Bcrypt) Verify(hash, plain string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain))
}
