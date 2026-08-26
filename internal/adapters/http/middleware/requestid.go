// Package middleware holds the Gin middleware chain (design.md's HTTP
// Layer: recovery -> requestID -> slog logging -> rateLimit -> jwtAuth) and
// depends only on apperror for the shared error envelope — never on the
// router (package http) or handler packages, so both of those can import
// middleware without an import cycle.
package middleware

import (
	"crypto/rand"
	"encoding/hex"

	"github.com/gin-gonic/gin"

	"github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/http/apperror"
)

// RequestIDHeader is the response header every request echoes its
// generated request ID under (design.md's Observability section).
const RequestIDHeader = "X-Request-Id"

// RequestID generates 8 bytes of crypto/rand hex per request, stores it in
// gin.Context under apperror.RequestIDContextKey (so apperror.RequestID and
// every downstream middleware/handler read the exact same value), and
// echoes it in the X-Request-Id response header before any other
// middleware or handler runs. It MUST be the outermost middleware after
// Recovery so every error envelope — including a recovered panic — carries
// a request ID.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		buf := make([]byte, 8)
		_, _ = rand.Read(buf)
		id := hex.EncodeToString(buf)

		c.Set(apperror.RequestIDContextKey, id)
		c.Writer.Header().Set(RequestIDHeader, id)
		c.Next()
	}
}
