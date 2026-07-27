# fontglyph test fixtures

These files are small redistributable fixtures used to exercise the reusable color glyph parsers and font backend without depending solely on host-installed emoji fonts. Most `.bin` files are synthetic OpenType color-table subsets, not copied from any font.

- `svg-gradient-table.bin`: OpenType `SVG ` table with one gradient-backed SVG document for glyph 10.
- `svg-text-table.bin`: OpenType `SVG ` table with centered `text`/`tspan` layout for glyph 20.
- `colr-var-scale-table.bin`: COLRv1 table with a `PaintVarScale` glyph and ItemVariationStore deltas.
- `colr-composite-multiply-table.bin`: COLRv1 table with a `PaintComposite` multiply node for source/backdrop paint graphs.
- `cpal-red-green.bin`: Minimal CPAL palette used by the COLR fixture.
- `noto-color-emoji-smoke.ttf`: small pyftsubset-generated subset of Google Noto Color Emoji for representative real color-font rasterization.
- `NotoEmoji-LICENSE.txt` and `noto-color-emoji-smoke.provenance.txt`: license/provenance for the Noto subset fixture.

Additional real-font fixture coverage also uses redistributable Go fonts from `golang.org/x/image/font/gofont` directly in tests, avoiding extra vendored binary font files while still exercising complete TTF parsing, metrics, rasterization, and simple shaping. Future licensed/OFL color-font subsets can be generated with `scripts/generate-font-fixture-subset.go` and added here once their license permits redistribution.
