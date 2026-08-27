package handler

import (
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/http/apperror"
)

// appIndexFile is the shell document every /app request falls back to, so a
// deep link or a refresh lands on the client instead of a 404.
const appIndexFile = "index.html"

// App handles GET /app/*filepath: it serves the embedded web client
// (internal/adapters/web) straight out of the binary.
//
// The file is read from assets and written with c.Data rather than handed
// to http.FileServer. FileServer resolves the file from c.Request.URL.Path,
// which would mean rewriting the request path mid-flight — and
// middleware.Logging reads that same field after the handler returns, so
// every asset request would be logged under a path the caller never sent.
//
// Serving the shell for unknown sub-paths is the standard single-page
// fallback, but it stops at the /app prefix: an unknown path anywhere else
// in the API still reaches the router's NoRoute handler and the normal
// JSON 404 envelope.
func App(assets fs.FS) gin.HandlerFunc {
	return func(c *gin.Context) {
		name := path.Clean(strings.TrimPrefix(c.Param("filepath"), "/"))
		if name == "." || name == "/" {
			name = appIndexFile
		}

		// fs.ValidPath rejects absolute and parent-relative names, so a
		// crafted /app/../../etc/passwd can never reach ReadFile. This is a
		// hard 404, never the shell fallback: a traversal attempt is not a
		// client-side route.
		if !fs.ValidPath(name) {
			apperror.WriteStatus(c, http.StatusNotFound, "NOT_FOUND", "El recurso solicitado no existe.")
			return
		}

		data, err := fs.ReadFile(assets, name)
		if err != nil {
			name = appIndexFile
			data, err = fs.ReadFile(assets, name)
			if err != nil {
				apperror.WriteStatus(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Ocurrió un error interno inesperado.")
				return
			}
		}

		c.Data(http.StatusOK, appContentType(name), data)
	}
}

// appContentType maps the handful of extensions the embedded client ships.
// mime.TypeByExtension is deliberately not used: it consults the host's
// registry (on Windows, the system registry), so the very same binary can
// serve .js as text/plain on one machine and text/javascript on another —
// and a module script served as text/plain is refused outright by every
// browser's ES module loader. This table makes the answer part of the
// build.
func appContentType(name string) string {
	switch path.Ext(name) {
	case ".html":
		return "text/html; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".js":
		return "text/javascript; charset=utf-8"
	case ".json":
		return "application/json; charset=utf-8"
	case ".svg":
		return "image/svg+xml"
	default:
		// Covers the vendored LICENSE, which carries no extension.
		return "text/plain; charset=utf-8"
	}
}
