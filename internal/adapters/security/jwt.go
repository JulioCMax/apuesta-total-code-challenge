// Package security holds the JWT issuer/verifier and the bcrypt password
// hasher/verifier. Both implement application/auth's consumer-owned ports
// (D2) and are the only place golang-jwt and golang.org/x/crypto/bcrypt are
// imported.
package security

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/account"
)

// issuer is the fixed "iss" claim value every issued token carries, and
// the only issuer Verify accepts (design.md: claims {sub, email, iss, iat,
// exp}).
const issuer = "apuesta-total-api"

// ErrInvalidToken is returned for any verification failure: bad signature,
// wrong issuer, expired token, or a disallowed signing algorithm. No
// further detail is exposed, matching the same "don't leak why" posture as
// account.ErrInvalidCredentials.
var ErrInvalidToken = errors.New("security: invalid token")

// claims is the JWT payload shape: the registered claims (sub, iss, iat,
// exp) plus the one custom claim the design requires (email).
type claims struct {
	Email string `json:"email"`
	jwt.RegisteredClaims
}

// JWT issues and verifies HS256 tokens. It implements
// application/auth.TokenIssuer.
type JWT struct {
	secret []byte
	ttl    time.Duration
}

// NewJWT builds a JWT issuer/verifier signing with secret and setting
// every issued token's lifetime to ttl (JWT_TTL from configuration).
func NewJWT(secret string, ttl time.Duration) *JWT {
	return &JWT{secret: []byte(secret), ttl: ttl}
}

// Issue mints an HS256 token for u, valid for j's configured TTL.
func (j *JWT) Issue(u account.User) (string, time.Duration, error) {
	now := time.Now().UTC()
	c := claims{
		Email: u.Email,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   u.ID,
			Issuer:    issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(j.ttl)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, c)
	signed, err := token.SignedString(j.secret)
	if err != nil {
		return "", 0, err
	}
	return signed, j.ttl, nil
}

// Verify parses tokenString, accepting only HS256-signed tokens issued by
// issuer with a currently-valid signature and expiry (jwt.WithValidMethods
// closes the algorithm-confusion class of attack — "alg: none" and RS256
// substitution both fail here before the signature is even checked).
func (j *JWT) Verify(tokenString string) (userID, email string, err error) {
	var c claims
	token, err := jwt.ParseWithClaims(tokenString, &c, func(*jwt.Token) (interface{}, error) {
		return j.secret, nil
	}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithIssuer(issuer))
	if err != nil || !token.Valid {
		return "", "", ErrInvalidToken
	}
	return c.Subject, c.Email, nil
}
