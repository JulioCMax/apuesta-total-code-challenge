package handler_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/http/handler"
)

// TestOpenAPISpecHandler_ReturnsSpecBody proves GET /openapi.yaml (wired by
// handler.OpenAPISpec) answers 200 with the embedded OpenAPI document body,
// content-typed so a browser or tool treats it as YAML rather than plain
// text (spec: api-platform/OpenAPI 3 Documentation Surface).
func TestOpenAPISpecHandler_ReturnsSpecBody(t *testing.T) {
	r := gin.New()
	r.GET("/openapi.yaml", handler.OpenAPISpec())

	req := httptest.NewRequest(http.MethodGet, "/openapi.yaml", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Header().Get("Content-Type"), "yaml")

	body := w.Body.String()
	require.Contains(t, body, "openapi: 3.0")
	require.Contains(t, body, "/api/v1/betslip/place")
}

// TestDocsHandler_ReturnsHTML proves GET /docs (wired by handler.Docs)
// answers 200 with a single self-contained HTML page that embeds Swagger UI
// (no CDN) and points it at /openapi.yaml, so it renders fully offline with
// zero additional network requests — required for a strict-CSP or offline
// reviewer (design.md's OpenAPI section).
func TestDocsHandler_ReturnsHTML(t *testing.T) {
	r := gin.New()
	r.GET("/docs", handler.Docs())

	req := httptest.NewRequest(http.MethodGet, "/docs", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Header().Get("Content-Type"), "text/html")

	body := w.Body.String()
	require.Contains(t, strings.ToLower(body), "<!doctype html>")
	require.Contains(t, strings.ToLower(body), "swaggeruibundle")
	require.Contains(t, body, "/openapi.yaml")
	require.NotContains(t, body, "cdn.jsdelivr.net")
	require.NotContains(t, body, "unpkg.com")
	require.NotContains(t, body, "petstore.swagger.io")
}
