# Changelog

All notable changes to ctxprof will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0] — 2026-06-04

First public release. Reads a finished Claude Code JSONL session and prints
where the 200k-token context window went, broken into six buckets:
`system / skill / mcp / file / reasoning / output`.

### Added — m1: parse_session
- `internal/parser`: streaming JSONL reader. Per turn it surfaces the real
  `message.usage` totals (`input_tokens`, `output_tokens`,
  `cache_read_input_tokens`, `cache_creation_input_tokens`) and a flattened
  list of typed content blocks (`thinking` / `text` / `tool_use` /
  `tool_result`). Per-block token counts do not exist in the source data —
  callers see the raw text and an estimated weight only.
- `internal/parser/types.go`: shared `Session`, `Turn`, `Block`, `Bucket`,
  and `Allocation` types used end-to-end.
- `cmd/ctxprof parse <file>.jsonl`: debug surface emitting one JSON record
  per turn.

### Added — m2: attribute_buckets
- `internal/estimate`: local tokenizer (`chars/4` heuristic in v0.1) that
  assigns each content block an estimated token weight.
- `internal/attribute/classifier.go`: deterministic, no-model-in-the-loop
  rules. `thinking` → `reasoning`; assistant `text` → `output`; `tool_use`
  named `Read` → `file`, `Skill` → `skill`, `mcp__*` → `mcp`, anything else
  (`Bash`, `Edit`, …) → `output`; `tool_result` → `file`.
- `internal/attribute/reconcile.go`: per-turn rescaling so the summed block
  weights match the real `message.usage` total. Session-level total is
  exact; per-bucket splits are calibrated estimates. The `system` bucket is
  approximated from the first turn's `cache_creation_input_tokens` and
  flagged via `Allocation.Estimated`.
- Table-driven unit tests for both the classifier and the reconciler.

### Added — m3: render_treemap
- `internal/render/tree.go`: lipgloss-rendered flame-graph-style ASCII tree
  with a fixed bucket order (`system → skill → mcp → file → reasoning →
  output`), bar widths normalized to bucket size, ANSI colors per bucket,
  and a per-bucket cap of five named items so long file-heavy sessions
  don't bury the headline split.
- `internal/render/json.go`: emits `allocation_v1.json` matching
  `internal/schema/allocation_v1.json` so other tools can consume the
  structure.
- `internal/schema/allocation_v1.json`: the published spec — the moat
  against vendor lock-in. Other harness parsers (Codex, Aider) that emit
  the same shape get ctxprof's rendering for free.
- `cmd/ctxprof` root command: auto-discovers the most recent session under
  `~/.claude/projects/`, with `--session`, `--json`, `--no-color`, and
  `--window-max` flags.
- `examples/sample-session.jsonl`: redacted fixture used by the test suite.

### Polish
- Bilingual README (`README.md` English + `README.zh-CN.md` 简体中文 full
  translation, cross-linked at the top of each).
- shields.io badges (license, Go version, CI, release, Claude Code, MCP),
  capsule-render header, ToC, configuration table, `vs caveman` positioning
  table, and a copy-paste tweet pitch.
- `assets/demo.tape` ([vhs](https://github.com/charmbracelet/vhs)) script
  for the 30-second demo gif (gif itself is recorded manually before
  release — see `assets/README.md`).
- `.github/workflows/ci.yml`: `go build`, `go vet`, `go test -race` on push
  and pull request.
- MIT license, `.gitignore` covering build output and local agent state.

### Out of scope for v0.1.0
Web UI / TUI dashboard; real-time tail mode; Codex / Cursor / Aider
parsers; cost-in-dollars conversion; auto-suggestions for token cuts;
multi-session diff or trend graphs; hosted SaaS. See `README.md`'s roadmap
for what is queued for v0.2 / v0.3.

[0.1.0]: https://github.com/SuperMarioYL/ctxprof/releases/tag/v0.1.0
