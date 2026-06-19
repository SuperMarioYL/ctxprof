# assets/

Visual assets referenced from the project README.

## `atlas-{light,dark}.svg` — architecture diagram

The dark/light architecture pair embedded in the README via a `<picture>`
block. They depict the four-package pipeline (`parser → estimate → attribute →
render`) and the six-bucket allocation. Hand-authored SVG — edit the source
directly; no generator step.

## `demo.gif` — flame-tree demo

The README embeds `assets/demo.gif`. It is rendered automatically by
[`.github/workflows/demo.yml`](../.github/workflows/demo.yml) on every
`v*.*.*` tag (and on manual dispatch) from [`docs/demo.tape`](../docs/demo.tape),
using the committed `examples/sample-session.jsonl` so the output is
deterministic.

To regenerate it locally:

```bash
# install vhs once
brew install vhs   # or: go install github.com/charmbracelet/vhs@latest

# from the repo root (needs the `ctxprof` binary on PATH)
go build -o /usr/local/bin/ctxprof ./cmd/ctxprof
vhs docs/demo.tape
```

`docs/demo.tape` is the [vhs](https://github.com/charmbracelet/vhs) script — it
opens a 1100×720 terminal, runs `ctxprof` on the sample session, then shows
`ctxprof --json | jq`, then drills into a single bucket. Total run time is
tuned for ~25 seconds at default playback speed.

If you re-record from a real session for a release, prefer one where:

- the window is at ≥80% utilization (so the percentages are interesting),
- at least three buckets have non-trivial size (so the tree isn't lopsided),
- and the skill / MCP names are not project-confidential.
