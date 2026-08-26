package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/JulioCMax/apuesta-total-code-challenge/api"
)

// openAPIContentType is served for GET /openapi.yaml. There is no
// registered IANA media type for YAML; "application/yaml" is the de-facto
// convention every OpenAPI tool (Swagger UI, Redoc, VS Code's OpenAPI
// extension) recognizes.
const openAPIContentType = "application/yaml; charset=utf-8"

// OpenAPISpec handles GET /openapi.yaml: serves the hand-written OpenAPI 3
// document embedded at build time (spec: api-platform/OpenAPI 3
// Documentation Surface; design.md's OpenAPI section).
func OpenAPISpec() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Data(http.StatusOK, openAPIContentType, api.OpenAPISpec)
	}
}

// Docs handles GET /docs: a single self-contained HTML page, embedded at
// build time, that renders the OpenAPI spec through a vendored (never
// CDN-loaded) copy of Swagger UI — it must render with zero external
// network requests, even for an offline or strict-CSP reviewer (design.md's
// OpenAPI section).
func Docs() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Data(http.StatusOK, "text/html; charset=utf-8", api.DocsHTML)
	}
}
