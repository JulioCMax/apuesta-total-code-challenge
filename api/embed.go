// Package api embeds the hand-written OpenAPI 3 document
// (api/openapi.yaml) and a single, fully self-contained Swagger UI docs
// page built from vendored assets, so both `GET /openapi.yaml` and
// `GET /docs` (design.md's HTTP Layer route table; spec: api-platform/
// OpenAPI 3 Documentation Surface) work from a single Lambda zip or Docker
// image with no runtime dependency on the filesystem or the network.
//
// Everything under api/swagger-ui/ is vendored from swagger-ui-dist@5.32.14
// (https://www.npmjs.com/package/swagger-ui-dist), Apache License 2.0 — see
// api/swagger-ui/LICENSE, api/swagger-ui/NOTICE and
// api/swagger-ui/swagger-ui-bundle.js.LICENSE.txt (bundled third-party
// notices, e.g. classnames and xtend, both MIT). Only swagger-ui-bundle.js,
// swagger-ui.css and the 16x16 favicon are embedded into the binary:
// source maps, the ES-module build, and the "standalone preset" (only
// needed to render the CDN-style top URL bar that lets a caller point
// Swagger UI at an arbitrary external spec — deliberately omitted here)
// are vendored on disk for provenance but never go:embed'd, keeping the
// embedded footprint small and the docs page unable to load anything but
// this service's own /openapi.yaml.
package api

import (
	"bytes"
	_ "embed"
	"encoding/base64"
)

// OpenAPISpec is the exact bytes of api/openapi.yaml, served verbatim by
// GET /openapi.yaml.
//
//go:embed openapi.yaml
var OpenAPISpec []byte

//go:embed swagger-ui/swagger-ui-bundle.js
var swaggerUIBundleJS []byte

//go:embed swagger-ui/swagger-ui.css
var swaggerUICSS []byte

//go:embed swagger-ui/favicon-16x16.png
var swaggerUIFavicon []byte

// DocsHTML is a single, self-contained HTML document that renders
// OpenAPISpec through the vendored Swagger UI assets above. It carries no
// external <script src="..."> or <link href="..."> reference of any kind —
// the CSS, the JS bundle, and even the favicon are inlined directly into
// this one response — so it renders correctly for a fully offline client or
// behind a Content-Security-Policy that blocks every outbound request.
// Built once at package init from the embedded assets.
var DocsHTML = buildDocsHTML()

func buildDocsHTML() []byte {
	favicon := base64.StdEncoding.EncodeToString(swaggerUIFavicon)

	var buf bytes.Buffer
	buf.WriteString(docsHTMLHead)
	buf.WriteString(favicon)
	buf.WriteString(docsHTMLAfterFavicon)
	buf.Write(swaggerUICSS)
	buf.WriteString(docsHTMLAfterCSS)
	buf.Write(swaggerUIBundleJS)
	buf.WriteString(docsHTMLAfterJS)
	return buf.Bytes()
}

const docsHTMLHead = `<!doctype html>
<html lang="es">
<head>
<meta charset="UTF-8">
<title>Apuesta Total API - Documentación</title>
<meta name="description" content="Documentación interactiva de la API de apuestas del Mundial 2026 (Apuesta Total).">
<link rel="icon" type="image/png" href="data:image/png;base64,`

const docsHTMLAfterFavicon = `">
<style>
`

const docsHTMLAfterCSS = `
html { box-sizing: border-box; overflow-y: scroll; }
*, *:before, *:after { box-sizing: inherit; }
body { margin: 0; background: #fafafa; }
</style>
</head>
<body>
<div id="swagger-ui"></div>
<script>
`

const docsHTMLAfterJS = `
window.onload = function () {
  window.ui = SwaggerUIBundle({
    url: "/openapi.yaml",
    dom_id: "#swagger-ui",
    presets: [SwaggerUIBundle.presets.apis],
    layout: "BaseLayout",
    deepLinking: true
  });
};
</script>
</body>
</html>
`
