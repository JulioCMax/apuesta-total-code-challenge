package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// DefaultMaxRequestBodyBytes bounds every request body this API will ever
// read into memory. Every JSON payload this service accepts (login,
// calculate, place) is a handful of selection IDs and a stake — a few KB at
// most — so 1 MiB leaves generous headroom while still making an unbounded
// body impossible (finding W-body: no request body size limit anywhere,
// public endpoints included).
const DefaultMaxRequestBodyBytes = 1 << 20 // 1 MiB

// BodyLimit caps every request body at max via http.MaxBytesReader. The
// moment a handler's read (directly, or through gin's JSON binding) would
// cross that limit, the underlying reader fails instead of continuing to
// buffer bytes into memory. Handlers already treat any bind/read error as a
// VALIDATION_ERROR 400 (Calculate, Place), so an oversized body renders the
// exact same standard envelope as any other malformed request — this
// middleware's job is only to make the read bounded in the first place.
func BodyLimit(max int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, max)
		c.Next()
	}
}
