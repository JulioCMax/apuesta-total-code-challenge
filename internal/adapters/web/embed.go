// Package web embeds the single-page web client served at GET /app: the
// same delivery strategy the OpenAPI docs page already uses (api/embed.go),
// applied to a UI instead of a spec. Everything the browser needs — markup,
// stylesheet, application modules and the vendored Vue runtime — ships
// inside the Go binary, so /app works from a single Lambda zip or Docker
// image with no runtime dependency on the filesystem, a CDN, or a build
// step.
//
// This package lives under internal/adapters on purpose. The web client is
// a delivery adapter like adapters/http: it consumes the very same public
// HTTP contract an external client would, holds no business rule, and the
// domain and application layers know nothing about it. Placing it here also
// keeps it inside the tree the Dockerfile already copies, so shipping a UI
// costs the image build exactly nothing.
//
// assets/vendor/ holds Vue 3.5.41's browser ESM build, vendored from the
// npm registry — see VENDORED.md in this directory for provenance and the
// reasoning behind vendoring it instead of adding a bundler. Its MIT
// LICENSE is embedded alongside the runtime rather than kept on disk for
// provenance only: the license text travels with every copy of the binary
// that redistributes it.
package web

import (
	"embed"
	"io/fs"
)

// assetsFS is the raw embedded tree, rooted one level above the files the
// browser actually requests. Assets unwraps that extra "assets/" segment.
//
// The all: prefix is required, not cosmetic: a bare //go:embed skips every
// entry whose name begins with "." or "_", which would silently drop such
// a file from the shipped UI and leave a 404 that only ever reproduces in
// the built binary.
//
//go:embed all:assets
var assetsFS embed.FS

// Assets is the file system GET /app serves, rooted at the directory the
// browser sees: "index.html", "app.css", "vendor/vue.esm-browser.prod.js".
//
// It is resolved once at package initialisation. fs.Sub can only fail here
// on a malformed path constant, which is a compile-time-fixed literal that
// //go:embed above has already proven exists — so a failure would mean the
// binary was built wrong, and panicking at startup is strictly better than
// serving a silently empty UI.
var Assets = mustSub("assets")

func mustSub(dir string) fs.FS {
	sub, err := fs.Sub(assetsFS, dir)
	if err != nil {
		panic("web: embedded asset directory " + dir + " is unreachable: " + err.Error())
	}
	return sub
}
