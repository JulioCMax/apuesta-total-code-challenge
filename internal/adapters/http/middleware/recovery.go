package middleware

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/http/apperror"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/platform/logging"
)

// Recovery must be the outermost middleware (design.md's HTTP Layer
// middleware order): it converts any panic from a downstream handler into
// the standard error envelope at 500, so a defect never crashes the
// process and never leaks an internal detail to the caller (spec: api-
// platform/Consistent Error Envelope, "Unexpected server error uses the
// standard envelope").
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				logging.FromContext(c.Request.Context()).Error("panic recovered",
					"error", fmt.Sprintf("%v", r), "path", c.Request.URL.Path)
				apperror.WriteStatus(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Ocurrió un error interno inesperado.")
				c.Abort()
			}
		}()
		c.Next()
	}
}
