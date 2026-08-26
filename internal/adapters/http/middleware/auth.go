package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/http/apperror"
)

// userIDContextKey and emailContextKey are unexported gin.Context keys so
// no other package can collide with them; UserID/Email are the only way to
// read them back.
const (
	userIDContextKey = "user_id"
	emailContextKey  = "user_email"
)

// TokenVerifier verifies a bearer token and extracts identity claims.
// application/auth.TokenIssuer (its Verify method) satisfies this port
// structurally; adapters/security.JWT is the production implementation.
type TokenVerifier interface {
	Verify(token string) (userID, email string, err error)
}

// JWTAuth guards a route group: it rejects a request with no valid
// Authorization: Bearer token before any handler logic runs (spec: auth-
// and-balance/Auth Guard Middleware, bet-slip-placement/JWT-Guarded
// Placement — "MUST NOT touch balance or bet state"), and stashes the
// verified user ID/email in gin.Context on success.
func JWTAuth(verifier TokenVerifier) gin.HandlerFunc {
	const prefix = "Bearer "
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, prefix) {
			apperror.WriteStatus(c, http.StatusUnauthorized, "UNAUTHORIZED", "Debe autenticarse para acceder a este recurso.")
			c.Abort()
			return
		}

		token := strings.TrimPrefix(header, prefix)
		userID, email, err := verifier.Verify(token)
		if err != nil {
			apperror.WriteStatus(c, http.StatusUnauthorized, "UNAUTHORIZED", "El token proporcionado no es válido o expiró.")
			c.Abort()
			return
		}

		c.Set(userIDContextKey, userID)
		c.Set(emailContextKey, email)
		c.Next()
	}
}

// UserID returns the verified caller's user ID stashed by JWTAuth, or "" if
// the request never passed through it (e.g. a public route).
func UserID(c *gin.Context) string {
	v, _ := c.Get(userIDContextKey)
	s, _ := v.(string)
	return s
}

// Email returns the verified caller's email stashed by JWTAuth, or "".
func Email(c *gin.Context) string {
	v, _ := c.Get(emailContextKey)
	s, _ := v.(string)
	return s
}
