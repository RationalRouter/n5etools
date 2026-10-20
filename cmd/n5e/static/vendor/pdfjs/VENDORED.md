# PDF.js (vendored)

Mozilla's PDF.js generic viewer, used by the in-app sourcebook reader
(`/sourcebooks/{book}`, see `cmd/n5e/sourcebooks.go`). Apache-2.0, see
`LICENSE`.

- Version: **6.4.299**, from `pdfjs-6.4.299-dist.zip` on
  https://github.com/mozilla/pdf.js/releases (the generic build, not legacy).
- Vendored files are unmodified, except `web/locale/locale.json`, which is cut
  down to the three locales kept.

## Removed from the release zip

- `*.map` source maps
- `web/compressed.tracemonkey-pldi-09.pdf` (sample document)
- `web/debugger.mjs`, `web/debugger.css` (only loaded with `#pdfbug`)
- `build/pdf.sandbox.mjs`, `web/wasm/quickjs-eval.*` (PDF scripting, which the
  reader turns off)
- every locale except `en-US`, `es-ES` and `pt-BR`

## Configuration

The viewer is configured from the embedding page, not by editing these files:
`static/js/sourcebook-view.js` handles the viewer's `webviewerloaded` event and
sets `disablePreferences`, `localeProperties`, `enableScripting`,
`viewerCssTheme` and `annotationEditorMode` on `PDFViewerApplicationOptions`.

## Upgrading

Unzip the new release, repeat the removals above, rewrite
`web/locale/locale.json` to the three kept locales, and check that the option
names above still exist in `web/viewer.mjs`.
