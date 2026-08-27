# Vendored web client dependencies

Source: [`vue`](https://www.npmjs.com/package/vue) `3.5.41` (npm registry
tarball `vue-3.5.41.tgz`, obtained with `npm pack vue@3.5.41`), fetched
2026-08-26.

License: MIT (`assets/vendor/LICENSE`, copied verbatim from the upstream
package).

## Files kept (embedded via `//go:embed` in `embed.go`)

- `assets/vendor/vue.esm-browser.prod.js` — the production ES module build
  of Vue 3, **including the template compiler**. That inclusion is the
  whole point of choosing this artifact: it is what lets components declare
  their markup as ordinary `template` strings with no compilation step.
- `assets/vendor/LICENSE` — MIT permission notice. Embedded rather than
  kept on disk for provenance only, so the notice travels with every binary
  that redistributes the runtime.

## Deliberately dropped from the upstream package

- `vue.runtime.esm-browser.prod.js` — the same runtime **without** the
  template compiler. Smaller, but it only renders components whose
  templates were compiled ahead of time, which is precisely the build step
  this approach exists to avoid.
- The development builds (`vue.esm-browser.js`,
  `vue.runtime.esm-browser.js`) — warnings and dev tooling that a shipped
  binary should not carry.
- Every CommonJS, UMD and bundler-targeted build, the TypeScript
  declarations, and the source maps — none are reachable from a browser
  loading an ES module directly.

## Why vendored instead of a bundler

A build step would have to run before `go build`, which means a Node stage
in the Dockerfile, a `node_modules` tree, and a Go binary whose contents
depend on an npm install succeeding. The existing OpenAPI docs page already
answers this question the other way (see `api/swagger-ui/VENDORED.md`): one
reviewed file, committed, with its provenance and license recorded here.

The cost of that choice is real and worth stating: no single-file
components, no tree-shaking, and upgrades are a manual `npm pack` and file
replacement rather than a version bump. For a UI of this size, none of
those outweigh keeping `docker compose up` a single command with no
JavaScript toolchain anywhere in the build.

## Upgrading

```sh
npm pack vue@<version>
tar -xzf vue-<version>.tgz package/dist/vue.esm-browser.prod.js package/LICENSE
cp package/dist/vue.esm-browser.prod.js assets/vendor/vue.esm-browser.prod.js
cp package/LICENSE assets/vendor/LICENSE
```

Then update the version recorded at the top of this file.
