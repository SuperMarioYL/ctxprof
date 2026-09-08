[简体中文](./README.zh-CN.md) · [Website](https://ctxprof.lei6393.com) · [GitHub](https://github.com/SuperMarioYL/ctxprof)

<picture>
  <source media="(max-width: 600px) and (prefers-color-scheme: dark)" srcset="./assets/presentation/hero-mobile-dark.svg">
  <source media="(max-width: 600px)" srcset="./assets/presentation/hero-mobile-light.svg">
  <source media="(prefers-color-scheme: dark)" srcset="./assets/presentation/hero-dark.svg">
  <img src="./assets/presentation/hero-light.svg" width="960" alt="Hero diagram">
</picture>

# ctxprof

**See what occupies your context window.**

ctxprof reads Claude Code session logs and attributes recorded usage to system, skill, MCP, file, reasoning and output buckets.

## Why use it

An aggregate usage number cannot tell you which loaded content deserves attention. A ranked allocation helps you inspect the largest consumers and compare sessions before changing your workflow.

- **Six useful buckets** — Inspect system, skills, MCP, files, reasoning and output.
- **Separate peak from throughput** — Cached prefixes are not treated as fresh window capacity on every turn.
- **Compare before trimming** — Trend and compare operate on explicit session files.

## Architecture

<picture>
  <source media="(max-width: 600px) and (prefers-color-scheme: dark)" srcset="./assets/presentation/architecture-mobile-dark.svg">
  <source media="(max-width: 600px)" srcset="./assets/presentation/architecture-mobile-light.svg">
  <source media="(prefers-color-scheme: dark)" srcset="./assets/presentation/architecture-dark.svg">
  <img src="./assets/presentation/architecture-light.svg" width="960" alt="Architecture diagram">
</picture>

The parser reads JSONL turns, a vendored byte-level BPE weights individual blocks, and reconciliation scales those weights to recorded usage. Attribution groups the result into six buckets. The renderer separates peak single-turn window occupancy from cumulative throughput.

| Component | Responsibility |
| --- | --- |
| `Session parser` | internal/parser |
| `Local BPE` | internal/estimate |
| `Reconciliation` | internal/attribute |
| `Tree / JSON` | internal/render |

## Install and quickstart

Build with the version declared in the repository manifest. Run the example from the repository root.

```bash
git clone https://github.com/SuperMarioYL/ctxprof.git
cd ctxprof
go build ./cmd/ctxprof
```

Read the complete synthetic session in examples/sample-session.jsonl and render its allocation tree.

```bash
go run ./cmd/ctxprof --session examples/sample-session.jsonl
```

## Recorded demo

<picture>
  <source media="(max-width: 600px) and (prefers-color-scheme: dark)" srcset="./assets/presentation/process-mobile-dark.svg">
  <source media="(max-width: 600px)" srcset="./assets/presentation/process-mobile-light.svg">
  <source media="(prefers-color-scheme: dark)" srcset="./assets/presentation/process-dark.svg">
  <img src="./assets/presentation/process-light.svg" width="960" alt="Process diagram">
</picture>

The recorded output separates window occupancy from cumulative tokens and displays estimated bucket shares.

```text
session 01J0Z5K3X4SAMPLEPROFILE — 23,180 / 200,000 tokens (12% of window, peak single-turn footprint)
  46,200 tokens cumulative throughput (re-counts the cached prefix each turn; not window occupancy)
├── system     ░░░░░░░░░░░░░░     1,050  (2.3%) ~
├── skill      █░░░░░░░░░░░░░     3,768  (8.2%)
│   └── caveman                         3,768
├── mcp        █████░░░░░░░░░    18,508  (40.1%) ~
│   └── grafana                        15,358
├── file       █░░░░░░░░░░░░░     4,337  (9.4%)
│   └── docs/incidents/2026-05.md       4,337
├── reasoning  ███░░░░░░░░░░░     9,945  (21.5%)
└── output     ██░░░░░░░░░░░░     8,592  (18.6%)

note: bucket numbers are calibrated estimates reconciled to real per-turn message.usage totals.
      rows marked ~ (system, mcp) are approximated from the first turn's cache_creation_input_tokens.
```

The complete command and output are recorded in [docs/demo-results.json](./docs/demo-results.json). Inputs and reproduction code are included in the repository.

![Existing terminal recording](./assets/demo.gif)

The existing recording is retained for context; the text example above documents the reproducible scenario.

## Usage

The CLI exposes the following operations. Commands after the example use your own paths or identifiers.

```bash
go run ./cmd/ctxprof --session examples/sample-session.jsonl --json
go run ./cmd/ctxprof --session examples/sample-session.jsonl --cut-candidates 10
# Compare your own sessions, old first:
ctxprof compare old.jsonl new.jsonl --json
ctxprof trend session-a.jsonl session-b.jsonl
```

## Configuration

Use --session to select a file explicitly; without it the CLI discovers sessions under ~/.claude/projects/. --window-max sets the window denominator. --json selects structured output; --cut-candidates N adds the largest individual consumers.

## Integrations and responsibilities

<picture>
  <source media="(max-width: 600px) and (prefers-color-scheme: dark)" srcset="./assets/presentation/integrations-mobile-dark.svg">
  <source media="(max-width: 600px)" srcset="./assets/presentation/integrations-mobile-light.svg">
  <source media="(prefers-color-scheme: dark)" srcset="./assets/presentation/integrations-dark.svg">
  <img src="./assets/presentation/integrations-light.svg" width="960" alt="Integrations diagram">
</picture>

The following routes are implemented in the source. Choose the input that matches your task and keep the resulting artifact with your project.

| Route | Implemented role |
| --- | --- |
| Claude JSONL | Recorded turns and usage |
| allocation/v1 | Structured allocation output |
| Trend / compare | Read-only session comparison |
| Terminal tree | Largest context consumers |

## Limits and next steps

- Per-block and per-bucket values are calibrated estimates. The vendored tokenizer is not Anthropic’s proprietary tokenizer.
- The example is a synthetic session; its usage numbers are fixture values, not measured savings.
- ctxprof diagnoses recorded data and does not unload skills or edit the session.

Future refinements should improve attribution quality against representative logs while preserving the explicit estimated flag and read-only workflow.

## License and contributions

See [LICENSE](./LICENSE). When reporting an issue, include a minimal input, the command, and the observed output.
