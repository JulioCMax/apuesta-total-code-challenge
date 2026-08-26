package api

import (
	"testing"

	"github.com/stretchr/testify/require"
	yaml "go.yaml.in/yaml/v3"
)

// TestOpenAPISpec_IsValidYAML proves the embedded api/openapi.yaml parses
// cleanly as YAML and declares the expected top-level shape, catching a
// malformed hand-written spec at test time rather than only when a human
// opens GET /docs (design.md's OpenAPI section; spec: api-platform/OpenAPI
// 3 Documentation Surface).
func TestOpenAPISpec_IsValidYAML(t *testing.T) {
	require.NotEmpty(t, OpenAPISpec)

	var doc map[string]any
	err := yaml.Unmarshal(OpenAPISpec, &doc)
	require.NoError(t, err)

	require.Equal(t, "3.0.3", doc["openapi"])

	paths, ok := doc["paths"].(map[string]any)
	require.True(t, ok, "spec must declare a 'paths' map")
	require.Contains(t, paths, "/api/v1/betslip/place")
	require.Contains(t, paths, "/api/v1/events")
	require.Contains(t, paths, "/health")

	components, ok := doc["components"].(map[string]any)
	require.True(t, ok, "spec must declare a 'components' map")
	require.Contains(t, components, "schemas")
	require.Contains(t, components, "securitySchemes")
}

// TestDocsHTML_HasNoExternalScriptOrLinkTags proves the built docs page
// never asks the browser to fetch a <script> or <link> resource from an
// external origin — the CSS and JS are inlined directly into the document,
// and the only well-known CDN hosts a Swagger UI page could otherwise
// reference are absent (design.md's OpenAPI section: "Vendored (not CDN) so
// docs render offline"). This does not blanket-forbid the substring
// "https://" anywhere in the page: the vendored Swagger UI bundle itself
// legitimately contains http(s) literals with no live network effect (XML
// namespace URIs, JSON Schema $id identifiers, RFC references in code
// comments, an error-decoder helper URL used only if a React invariant
// throws) — none of those are asset-loading tags.
func TestDocsHTML_HasNoExternalScriptOrLinkTags(t *testing.T) {
	require.NotEmpty(t, DocsHTML)

	html := string(DocsHTML)
	// The favicon <link> intentionally uses a "data:" URI (fully inlined),
	// so the check below is scoped to "http" asset references specifically,
	// not to the presence of a <link>/<script> tag at all.
	require.NotContains(t, html, `src="http`)
	require.NotContains(t, html, `href="http`)
	for _, cdnHost := range []string{"cdn.jsdelivr.net", "unpkg.com", "cdnjs.cloudflare.com", "petstore.swagger.io"} {
		require.NotContains(t, html, cdnHost)
	}
}
