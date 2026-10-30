# Bellows brand

Confidence, before the push.

Bellows runs your existing GitHub Actions workflows locally using act and
Docker. Use “Run GitHub Actions locally before you push.” as the short
description, and “Keep the feedback close.” as the supporting line.

This directory is the canonical asset library, following
[Popcorn's brand-library structure](https://github.com/smeltery/popcorn/tree/main/assets/brand).
The supplied version 2 kit is preserved with its original filenames and relative
paths. Its “proposed” labels describe the original handoff; this README records
how the repository uses it.

## Preview and source material

Open [index.html](index.html) locally for the responsive guide, logo switcher,
color swatches, typography, and downloads. All artwork and fonts are local.
For clipboard support, serve the repository from localhost with any static HTTP
server. The swatches also show selectable hex values if copying is unavailable.

- [Brand guide](bellows-brand-guide-v2.pdf): eight-page visual reference.
- [Brand board](brand-board.png): compact overview of the identity.
- [Original handoff](START-HERE.txt): supplied usage notes and references.
- [Provenance](PROVENANCE.md): archive identity, font sources, and licensing notes.
- [Validation](VALIDATION.md): integration checks and rendered review.

## Artwork

| Asset | Use |
| --- | --- |
| [Primary lockup](logos/svg/bellows-primary.svg) | Ember symbol and Ink lettering on light surfaces |
| [Reversed lockup](logos/svg/bellows-reversed.svg) | Ember symbol and Paper lettering on dark surfaces |
| `logos/svg/bellows-{ink,paper}.svg` | One-color lockups |
| `logos/svg/bellows-on-{paper,ink,ember}.svg` | Lockups with intentional, opaque backgrounds |
| [Stacked lockup](logos/svg/bellows-stacked.svg) | Taller compositions |
| `logos/svg/wordmark-*.svg`, `logos/svg/symbol-*.svg` | Separate lettering and symbols |
| [App tile](logos/png/app-icon.png) | 512 px avatar or application icon |
| [Adaptive favicon](logos/svg/favicon.svg) | Browser icon with light/dark preference support |
| `logos/png/icon-{size}.png`, [favicon.ico](logos/png/favicon.ico) | 16, 32, 48, 64, 180, 192, 256, and 512 px tiles; multi-size ICO |
| [README banner](applications/readme-banner.png) | 1600 × 480 repository banner |
| [Social preview](applications/social-preview.png) | 1280 × 640 repository or social card |
| [Documentation concept](applications/docs-header.png) | 1600 × 900 design reference |

SVG masters have outlined lettering and do not need installed fonts. Matching
PNG exports live in `logos/png/`; application compositions have SVG and PNG
versions in `applications/`. Keep vector masters when placing or editing artwork.
Transparent exports retain alpha; background-specific variants are opaque.

Preserve the proportions, spacing, lowercase lettering, and orientation. Keep
one quarter of the symbol height clear around the visible artwork. Minimum
digital sizes are 160 px wide for a lockup and 16 px high for a symbol. Do not
add effects or decorations to the mark.

## Colors and typography

[tokens/brand-tokens.json](tokens/brand-tokens.json) records the palette,
type scale, spacing, logo constraints, and approved normal-text contrast pairs.
[tokens/brand.css](tokens/brand.css) provides matching CSS variables and local
font loading; preserve its sibling `fonts/` directory when reusing it.
[tokens/bellows.gpl](tokens/bellows.gpl) is the editor palette.

Use Paper (`#F5F3ED`) and Ink (`#191C22`) for the main surfaces, with Ember
(`#F06A43`) for emphasis. Use Ink text on Ember, Burnt Ember (`#A9361C`) for
accent text on Paper, and Paper or `#BBC2CA` text on Ink. Steel (`#626B75`)
is secondary text on Paper. Use the supplied semantic colors on light surfaces
and pair statuses with visible labels.

Use locally hosted Space Grotesk 400–700 for headings and body text, and
JetBrains Mono 400–500 for commands, metadata, and technical data. Both original
variable TTFs and their SIL Open Font License notices are in `fonts/`. The CSS
uses `font-display: swap` with system fallbacks. Logo lettering is supplied as
artwork; it does not depend on interface font rendering.

## Repository integration

The root README uses the supplied banner. The documentation index uses the
primary and reversed SVG lockups according to the reader's color preference.
Both reference this library directly, so there are no copied assets to sync.
The documentation concept remains reference artwork; the repository's user
documentation lives in Markdown under `docs/`.

Keep diagnostics direct: name the issue, explain the limitation, and give a
useful next step. Compatibility and performance claims must stay specific.
Hosted CI continues to provide platform coverage and merge enforcement.

To set GitHub's repository social preview, upload
`applications/social-preview.png` through the repository settings. Committing
the image does not update that setting.

## Updating assets

Edit assets here and retain the local font licenses and relative paths. Review
the guide on desktop and mobile, including all three logo variants, then run:

```sh
bun run brand:update
bun run docs
bun run budget
```

`checksums.json` records SHA-256 hashes of all non-Markdown brand files except
the manifest itself. `bun run brand:check` detects changed, added, or missing
files through the existing `make validate`, hooks, and CI checks. Regenerate the
manifest only after reviewing intentional changes. The hash check verifies file
integrity; it does not regenerate PNGs from SVGs or prove visual equivalence.
No export generator was supplied with this kit.
Git attributes preserve the upstream font licenses' original line endings and
whitespace.
