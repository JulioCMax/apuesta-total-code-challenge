# Vendored Swagger UI assets

Source: [`swagger-ui-dist`](https://www.npmjs.com/package/swagger-ui-dist) `5.32.14`
(npm registry tarball `swagger-ui-dist-5.32.14.tgz`), fetched 2026-08-26.

License: Apache License 2.0 (`LICENSE`, `NOTICE` in this directory, copied
verbatim from the upstream package). `swagger-ui-bundle.js` additionally
bundles a small number of third-party MIT-licensed snippets (`classnames`,
`xtend`); their notices are preserved in
`swagger-ui-bundle.js.LICENSE.txt`.

## Files kept (embedded via `//go:embed` in `api/embed.go`)

- `swagger-ui-bundle.js` — the core Swagger UI React bundle.
- `swagger-ui.css` — the stylesheet.
- `favicon-16x16.png` — inlined as a base64 `data:` URI in the generated
  docs page.

## Files kept for provenance only (not embedded into the Go binary)

- `LICENSE`, `NOTICE`, `swagger-ui-bundle.js.LICENSE.txt` — attribution.

## Deliberately dropped from the upstream package

- `swagger-ui-standalone-preset.js` — only needed for the CDN-style
  top URL bar that lets a visitor point Swagger UI at an arbitrary
  external spec URL. `/docs` always renders exactly one spec
  (`/openapi.yaml`), so this preset (and the attack surface it implies)
  is never vendored.
- `favicon-32x32.png` — the 16x16 icon alone is enough for a docs page.
- Every `*.map` source map, the ES-module bundle build, `index.html`,
  `oauth2-redirect.html/js`, `swagger-initializer.js`,
  `absolute-path.js` — none of these are used by this service's own
  single-file `/docs` page (`api/embed.go` builds its own minimal HTML
  shell instead of using upstream's `index.html`, which defaults to
  loading `https://petstore.swagger.io/v2/swagger.json` from a CDN).

Total embedded footprint: ~1.7 MB (`swagger-ui-bundle.js` ≈1.5 MB,
`swagger-ui.css` ≈180 KB, favicon ≈1 KB), close to design.md's original
~1.4 MB estimate.
