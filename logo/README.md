# logo/

Authoring source for the AgentX startup banner. Nothing under this directory
is imported by the application directly — it feeds a build-time codegen step
(`cmd/logogen`) that turns the authored banner into Go source compiled into
`internal/surfaces/banner`.

## Files

- `agentx.txt` — the raw ASCII-art glyphs, uncolored. The hand-drawn starting
  point.
- `coloriz.py` — a one-off script used to hand-author the gradient coloring:
  reads `agentx.txt` and prints each character wrapped in an xterm-256
  `ESC[38;5;Nm` foreground code, fading from `first_color` to `last_color`
  left to right per line. Kept for provenance and for re-deriving a gradient
  if the glyphs change — it is not part of the build.
- `agentx.logo` — the authored, ANSI-colored banner (`coloriz.py agentx.txt >
  agentx.logo`, by hand). This is the build's source of truth for the
  banner's full-size (pinned) form.

There is no separate "collapsed" source file. The banner's collapsed row
(`AgentX - <activity>`, e.g. `AgentX - Thinking`) is synthesized at runtime
from whatever the agent is currently doing — see
`docs/ux/06_OUTPUT_WIDGET.md` ("Logo banner") and
`internal/surfaces/banner/banner.go`'s `buildLabelGrid` — using the same
gradient formula `coloriz.py` uses, not a build-time asset.

## Build-time conversion

`agentx.logo` stores color as literal `ESC[38;5;Nm` escape sequences
interleaved with the glyphs. Shipping that as-is (as the binary's banner
string, which is what this repo did before) means any runtime feature that
wants to touch the banner's color — a color-cycle animation, in particular —
would have to re-parse ANSI escapes to know what it's recoloring.

Instead, `make build` runs it through `cmd/logogen`:

    GIVEN agentx.logo, using only "ESC[38;5;Nm" (256-color foreground) and
    "ESC[0m" (reset) SGR sequences,
    WHEN cmd/logogen parses it,
    THEN it emits internal/surfaces/banner/logo_generated.go, defining
    `var LogoGrid = [][]Cell{...}` — one row per line of agentx.logo, one
    Cell{Rune, Color} per visible glyph, where Color is the xterm-256 palette
    index that glyph was authored with (-1 if none was set).

`internal/surfaces/banner/banner.go` (hand-written, not generated) defines the
`Cell` type and all rendering/animation behavior over `LogoGrid` — the
pinning, content-based sticky collapse, and rainbow-wave animation while the
agent is working. It never sees `agentx.logo`'s raw escape sequences, so it's
agnostic to the banner's actual content.

See `Makefile` (the `LOGO_SRC`/`LOGO_DST`/`LOGOGEN_*` rules) for the build
wiring, and `docs/implementation/09_makefile_and_quality_gate_contract.md`
for the quality-gate contract this participates in.

## Editing the banner

Edit `agentx.logo` directly (or regenerate the gradient from `agentx.txt` via
`coloriz.py`), then run `make build` — it rebuilds `cmd/logogen` if needed,
regenerates `internal/surfaces/banner/logo_generated.go` from the new source,
and rebuilds `agentx`. Both `agentx.logo` and the generated
`logo_generated.go` are committed, so a checkout doesn't need this directory
(or Python) present to build.
