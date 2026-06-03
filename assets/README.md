# assets/

Visual assets referenced from the project README.

## `demo.gif` — 30-second flame-tree demo

The README embeds `assets/demo.gif`, but the gif itself is **not checked into
the v0.1.0 source tag** — it is recorded by hand from a real Claude Code
session so the bucket labels (skill names, MCP server names, file paths) are
real and credible, not synthetic.

To regenerate it:

```bash
# install vhs once
brew install vhs   # or: go install github.com/charmbracelet/vhs@latest

# from the repo root
vhs assets/demo.tape
```

`demo.tape` is the [vhs](https://github.com/charmbracelet/vhs) script — it
opens a 1100×720 terminal, runs `ctxprof` on the most recent session, then
shows `ctxprof --json | jq`, then targets a specific session via `--session`.
Total run time is tuned for ~30 seconds at default playback speed.

If you replace this gif for a release, please record from a session where:

- the window is at ≥80% utilization (so the percentages are interesting),
- at least three buckets have non-trivial size (so the tree isn't lopsided),
- and the skill / MCP names are not project-confidential.
