# Changelog

All notable changes to ctxprof will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.6.0] — 2026-07-13

Correctness pass. One fix over the shipped v0.5 source — no new features, no new deps,
still read-only and terminal-only. It sharpens the same "where do your tokens go?" answer
v0.5 restored, this time for MCP- and tool-heavy sessions.

### Fixed
- **A `tool_result`'s tokens now land in the bucket of the tool that produced it — an MCP
  call's response counts as `mcp`, not `file`.** v0.5 correctly stopped dropping
  `tool_result` content, but it classified *every* folded `tool_result` to the `file`
  bucket (the classifier's context-free `tool_result → file` rule). Because the fold also
  inherits each result's item name from the originating `tool_use`, a large MCP tool
  response landed in the `file` bucket *under the MCP server's own name*. On the bundled
  demo session, the `mcp__grafana__get_panel` panel response (~9,360 tokens) showed up as a
  `file` row literally named `grafana`, while the `mcp` bucket under-counted the real window
  consumption; Bash / WebFetch / any non-`Read` tool response was likewise dumped into
  `file`. Now each `tool_result` is attributed to the bucket of the `tool_use` that produced
  it (matched by `tool_use_id`): an MCP response → `mcp`, a `Skill` → `skill`, a `Bash` →
  `output`, a `Read` → `file`, and an unknown origin falls back to `file`. On the demo the
  `grafana` request and response now consolidate under one `mcp` row (~15,358) and `file`
  holds only the real read path. The per-turn reconciliation balance and `cumulative_tokens`
  are unchanged (only the target bucket moves, never the amount). `ClassifyBlock` stays
  context-free; the origin-aware routing lives in `internal/attribute/reconcile.go`
  (`indexToolUse` + `classifyForAttribution`). Covered by `reconcile_test.go`
  (`TestAttribute_ToolResultInheritsOriginBucket`).

## [0.5.0] — 2026-07-05

Correctness pass. Three fixes over the shipped v0.4 source — no new features, no new
deps, still read-only and terminal-only. The headline fix restores ctxprof's core
promise ("where do your tokens go?") for file-heavy sessions.

### Fixed
- **`tool_result` content now lands in the `file` bucket instead of being swallowed by
  `output`/`reasoning`.** `tool_result` blocks carry the actual retrieved file/tool
  content — the single biggest input in a typical session — but they live in *user*
  turns, which carry no `message.usage`. Attribution skipped user turns entirely
  (`if turn.Usage == nil { continue }`), so a large read's bytes were distributed across
  only the *next* assistant turn's own output/thinking blocks: a 40k-char file read
  landed ~99% in `output`/`reasoning` while the `file` bucket caught only the tiny Read
  request. The classifier's `tool_result → file` rule was effectively dead on real
  sessions. Now each user turn's `tool_result` blocks are folded into the following
  assistant turn's reconciliation pool (their bytes are part of that turn's
  `input_tokens`), so the retrieved content lands in `file` and inherits the originating
  Read's `file_path` (matched by `tool_use_id`) as its item name. The per-turn balance
  invariant is preserved. Touches `internal/parser` (new `Block.ToolUseID`) and
  `internal/attribute/reconcile.go`; covered by `reconcile_test.go` +
  `classifier_test.go`.
- **`--json --cut-candidates` now validates against the published schema.** The emitted
  document carried a top-level `cut_candidates` array, but `allocation_v1.json` sets
  `additionalProperties: false` and never declared it (the schema predates the v0.3
  cut-candidates feature), so a documented flag combination emitted a document that
  failed the project's own schema. Added `cut_candidates` + a `$defs/cutCandidate`
  object to `allocation_v1.json`; plain `--json` still validates unchanged. Covered by
  the new `internal/schema/allocation_v1_test.go`.
- **`--cut-candidates` no longer silently no-ops on `trend`/`compare`.** It was a root
  `PersistentFlag`, so cobra advertised it on every subcommand's `--help`, yet `trend`
  and `compare` never read it — `ctxprof trend … --cut-candidates 5` silently accepted
  the flag and rendered nothing. Moved it to a local flag on root, re-declared on
  `attribute` (mirroring `--json`), so `trend`/`compare` now reject it with `unknown
  flag` instead of swallowing it. Covered by `cmd/ctxprof/main_test.go`.

## [0.4.0] — 2026-07-02

Fix-plus-feature pass. v0.3 shipped the multi-session trend and cut-candidates views;
v0.4 fixes a real ordering bug in that trend command and adds the natural third
follow-on question — "what changed between exactly these two sessions?".

### Added — two-session compare
- `ctxprof compare <old.jsonl> <new.jsonl>` profiles exactly two sessions through the
  existing parse→attribute pipeline and prints, for each of the six buckets, both
  sessions' reconciled tokens plus the signed delta (new − old), then the largest
  per-item changes (a skill / MCP server / file path that grew, shrank, appeared, or
  disappeared) across the pair — so you can pinpoint a regression without eyeballing
  two trend columns. `--json` emits an object carrying both `allocation_v1` objects
  plus a `bucket_deltas` array (each per-session allocation still validates against
  `allocation_v1`). Read-only and terminal-only — ctxprof never edits a session. New
  `internal/render/compare.go` (+ `compare_test.go`), a pure derived view over two
  `Allocation`s that reuses the same cross-bucket item merge as cut-candidates.

### Fixed
- **`trend` now orders explicit path args oldest→newest by mtime.** `resolveTrendPaths`
  returned explicit positional args verbatim, but the trend table's `Δ first→last`
  column and its left-to-right time axis assume oldest→newest ordering (the `--since`
  path already sorted by mtime). So `ctxprof trend b.jsonl a.jsonl` — or the common
  shell glob `ctxprof trend *.jsonl`, which expands lexically, not chronologically —
  rendered a backward time axis and a sign-flipped drift, silently lying about whether
  the budget was creeping up. Explicit args are now mtime-sorted through the same
  helper the `--since` path uses, so both entry points share one ordering. Covered by
  new `cmd/ctxprof/main_test.go` cases (`TestSortPathsByMtime`,
  `TestTrendOrdersExplicitArgsByMtime`).

## [0.3.0] — 2026-06-30

Scope-expansion pass. v0.2 closed out the accuracy backlog, so v0.3 lifts two
features that were explicitly deferred in earlier `out_of_scope` lines and fixes a
CLI flag-inheritance gap. A single snapshot now also answers "is my budget creeping
up?" and "what specifically should I cut?".

### Added — multi-session trend
- `ctxprof trend <a.jsonl> <b.jsonl> …` (or `ctxprof trend --since 7d` over
  `~/.claude/projects/`) profiles several sessions through the existing
  parse→attribute pipeline and prints a compact terminal table of how each bucket's
  occupancy and share moves across them (oldest→newest), with a `Δ first→last`
  column. `--json` emits an ordered array of `allocation_v1` objects (schema
  unchanged). Read-only and terminal-only — no graphs, no TUI. New
  `internal/render/trend.go` (+ `trend_test.go`); lifts the v0.2 "Multi-session
  diff / trend graphs — single-session snapshots only" out_of_scope line.

### Added — cut-candidates
- `--cut-candidates N` appends a ranked list of the N largest single named
  consumers across every bucket (a skill, an MCP server, a file path), each with
  its share of the peak window, so you see exactly what to trim. It is **diagnosis
  only** — ctxprof still never edits or rewrites a session. Also surfaced as an
  additive, omit-empty `cut_candidates` array under `--json` (a plain `--json` run
  without the flag still validates against `allocation_v1`). New
  `internal/attribute/topconsumers.go` (+ `topconsumers_test.go`), a pure
  cross-bucket merge+rank over the existing per-bucket `Items`.

### Fixed
- **`attribute` subcommand now honors `--window-max` and `--no-color`.** The root
  registered those (and `--session`) as **local** flags, so the `attribute`
  subcommand — which calls the same `profile()` that reads them — rejected
  `ctxprof attribute s.jsonl --window-max 100000` with "unknown flag" and silently
  ignored `--no-color`, an inconsistent CLI surface vs the root path. They are now
  root **persistent** flags, inherited by every subcommand. Regression-covered by
  the new `cmd/ctxprof/main_test.go`.

## [0.2.0] — 2026-06-23

Accuracy pass. The v0.1 numbers reconciled correctly at the session level, but
two of them were quietly misleading and the renderer corrupted non-ASCII names.
This release fixes that and swaps the placeholder tokenizer for a real one.

### Added — real BPE tokenizer
- `internal/estimate/bpe.go` + `internal/estimate/vocab.json`: per-block weights
  now come from a real, vendored byte-level BPE (GPT-2/cl100k-style reversible
  byte→unicode mapping plus a genuine merge table trained on a representative
  English/code/CJK corpus) instead of `chars/4`. It is embedded via `go:embed`,
  so there are still zero network calls at runtime. `Allocation.Estimated` stays
  `true` and the per-turn reconciliation step is unchanged — better per-block
  weights just pull the reconciliation scaling closer to 1.0, so the bucket
  splits are more trustworthy. This is deliberately **not** Anthropic's
  proprietary tokenizer (not public, out of scope); it is a real tokenizer
  tested against its own known-string fixtures in `bpe_test.go`.

### Fixed
- **Window-% no longer double-counts the cached prefix.** The headline
  `X / 200,000 (Y%)` used to be driven by a cross-turn sum of every turn's
  `message.usage` total — and `cache_read_input_tokens` is the cached prefix
  re-read each turn, so a long session counted the same window many times and
  the percentage ballooned past 100%. The headline is now driven by
  `window_occupancy`: the peak single-turn footprint
  (`input + cache_read + cache_creation`). The old cumulative number is still
  reported separately as genuine throughput. JSON field `total_tokens` is
  renamed `cumulative_tokens` and a new exact `window_occupancy` field is added;
  `internal/schema/allocation_v1.json` updated to match.
- **First-turn `cache_creation` is split between `system` and `mcp`.** It used
  to land 100% in `system`, which swallowed the MCP/tool-descriptor catalog
  bundled into the same cached prefix. It is now apportioned via a documented
  heuristic (share of `mcp__` `tool_use` blocks, capped at 0.75 so the system
  prompt keeps a plurality). Both halves are still flagged approximate.
- **`--window-max 0` no longer emits schema-invalid JSON.** A zero or negative
  window is clamped to 200,000 (with a stderr warning from the CLI) before
  attribution, so the emitted `window_max` always satisfies the schema's
  `minimum: 1`.
- **Multibyte item names no longer corrupt the tree.** Truncation and padding
  were byte-indexed, which sliced CJK/emoji names mid-rune into invalid UTF-8
  and misaligned every column on wide runes. They are now display-width aware
  via `github.com/mattn/go-runewidth` — relevant for the primary zh dev
  audience with Chinese project paths and skill names.

### Tests
- `internal/estimate/bpe_test.go`: known-string token-count fixtures,
  byte-bound and determinism checks, embedded-model load check.
- `internal/attribute/reconcile_test.go`: peak-vs-cumulative window occupancy,
  system/mcp seed split, and a programmatic `--window-max 0/-1` clamp assertion.
- `internal/render/tree_test.go`: CJK/emoji truncation and padding stay valid
  UTF-8 and width-aligned.

### Dependencies
- Add `github.com/mattn/go-runewidth@v0.0.16` (promoted from indirect).

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

[0.2.0]: https://github.com/SuperMarioYL/ctxprof/releases/tag/v0.2.0
[0.1.0]: https://github.com/SuperMarioYL/ctxprof/releases/tag/v0.1.0
