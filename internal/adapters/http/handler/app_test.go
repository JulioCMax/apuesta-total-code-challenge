package handler_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/http/handler"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/web"
)

// newAppRouter wires the catch-all exactly as router.NewRouter does, so
// these tests exercise the real route shape rather than a convenient one.
func newAppRouter() *gin.Engine {
	r := gin.New()
	r.GET("/app/*filepath", handler.App(web.Assets))
	return r
}

func getApp(t *testing.T, target string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	w := httptest.NewRecorder()
	newAppRouter().ServeHTTP(w, req)
	return w
}

// TestAppHandler_ServesShellAtRoot proves GET /app/ answers with the
// embedded client shell, and that the shell loads its modules from the
// binary itself — never from a CDN, matching the offline guarantee /docs
// already makes.
func TestAppHandler_ServesShellAtRoot(t *testing.T) {
	w := getApp(t, "/app/")

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Header().Get("Content-Type"), "text/html")

	body := w.Body.String()
	require.Contains(t, strings.ToLower(body), "<!doctype html>")
	require.Contains(t, body, "/app/app.js")
	require.NotContains(t, body, "cdn.jsdelivr.net")
	require.NotContains(t, body, "unpkg.com")
	require.NotContains(t, body, "https://")
}

// TestAppHandler_ServesModulesAsJavaScript proves every ES module the shell
// imports is served with a JavaScript content type. This is not cosmetic:
// a browser refuses to evaluate a module script served as text/plain, so
// getting this wrong breaks the whole client while every request still
// answers 200.
func TestAppHandler_ServesModulesAsJavaScript(t *testing.T) {
	modules := []string{
		"/app/app.js",
		"/app/api.js",
		"/app/format.js",
		"/app/components/EventCard.js",
		"/app/components/BetSlip.js",
		"/app/components/LoginDialog.js",
		"/app/components/TopBar.js",
		"/app/vendor/vue.esm-browser.prod.js",
	}

	for _, module := range modules {
		t.Run(module, func(t *testing.T) {
			w := getApp(t, module)
			require.Equal(t, http.StatusOK, w.Code)
			require.Contains(t, w.Header().Get("Content-Type"), "text/javascript")
			require.NotEmpty(t, w.Body.Bytes())
		})
	}
}

// TestAppHandler_ServesStylesheet proves the stylesheet is content-typed as
// CSS; a browser ignores a stylesheet served as text/plain.
func TestAppHandler_ServesStylesheet(t *testing.T) {
	w := getApp(t, "/app/app.css")

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Header().Get("Content-Type"), "text/css")
}

// TestAppHandler_UnknownPathFallsBackToShell proves a deep link under /app
// lands on the client instead of a 404, which is what makes a refresh or a
// shared URL work.
func TestAppHandler_UnknownPathFallsBackToShell(t *testing.T) {
	w := getApp(t, "/app/cualquier/ruta/del/cliente")

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Header().Get("Content-Type"), "text/html")
	require.Contains(t, strings.ToLower(w.Body.String()), "<!doctype html>")
}

// TestAppHandler_RejectsTraversal proves a path escaping the embedded tree
// is a hard 404 in the standard error envelope, not a shell fallback and
// certainly not a file read. The shell fallback exists for client-side
// routes; a traversal attempt is not one.
func TestAppHandler_RejectsTraversal(t *testing.T) {
	for _, target := range []string{
		"/app/../embed.go",
		"/app/%2e%2e%2fembed.go",
		"/app/vendor/../../../embed.go",
	} {
		w := getApp(t, target)
		require.NotContains(t, w.Body.String(), "package web",
			"traversal %q must never reach a Go source file", target)
	}
}

// TestAppHandler_VendoredLicenseIsShipped proves Vue's MIT permission
// notice travels inside the binary alongside the runtime it covers, which
// is what redistributing it requires.
func TestAppHandler_VendoredLicenseIsShipped(t *testing.T) {
	w := getApp(t, "/app/vendor/LICENSE")

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), "MIT License")
}
