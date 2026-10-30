# Brand integration validation

Reviewed on 2026-10-30.

## Repository checks

- All 57 supplied files match the source ZIP byte-for-byte.
- `bun install --frozen-lockfile` passed with the existing dependency lockfile.
- `bun run docs` passed Markdown lint, parsed all 11 Mermaid diagrams, and
  verified SHA-256 hashes for all 57 brand files.
- `bun run budget` passed: 7,481/7,500 production Go lines and 52/80 tooling lines.
- `git diff --check` passed.
- All 19 SVGs parse as XML, all 27 PNGs decode, and application dimensions match
  the supplied sizes. Local links and CSS font references resolve.
- Isolated copies confirmed that the checksum check rejects changed, missing,
  and added assets, and accepts an intentional `brand:update` followed by a check.

The checksum command runs through `bun run docs`, which is already part of
`make validate`, contributor hooks, and CI.

## Rendered review

- Reviewed the supplied guide in Chromium at 1440 px and 390 px.
- All three logo buttons load the matching asset and update their pressed state.
- Color buttons copy the expected hex value; unavailable clipboard access shows
  the selectable-value fallback.
- Images and local Space Grotesk/JetBrains Mono fonts load. No horizontal page
  overflow, browser script errors, or failed resource requests were observed.
- Rendered README and documentation-index markup at both widths in light and
  dark modes. The README banner loads, and the docs picture selects the primary
  or reversed lockup correctly. These checks used a local Markdown renderer.
- Inspected the supplied board, banner, and all eight PDF guide pages.

## Verification limits

Full `make validate` and `make check` were not run: Flox provisioning stalled
while resolving the existing act dependency. The documentation, brand, and
budget commands above ran with locally available Bun. CodeRabbit review was
unavailable because its CLI was signed out.

The documentation-header composition remains a design concept. The supplied
social-preview image is ready for upload; GitHub's repository social-preview
setting was not changed by this integration.
