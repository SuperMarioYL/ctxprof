// Package estimate sizes individual content blocks with a local heuristic.
//
// The Claude Code JSONL records token totals only once per assistant turn
// (message.usage). There is no per-block token field, so to attribute the
// turn total across the blocks that make it up we first assign each block a
// rough weight here, then reconcile those weights against the real per-turn
// total during attribution (m2).
package estimate

import "unicode/utf8"

// Tokens returns a token estimate for s used to weight a content block before
// the per-turn reconciliation step.
//
// As of v0.2 this runs the vendored byte-level BPE tokenizer (internal/
// estimate/bpe.go + vocab.json) — a real, deterministic tokenizer rather than
// the chars/4 heuristic shipped in v0.1. Accurate per-block weights tighten the
// reconciliation scaling toward 1.0 and make the per-bucket splits more
// trustworthy. The reconciliation step itself is unchanged: bucket numbers are
// still calibrated estimates, and Allocation.Estimated stays true.
//
// An empty string maps to 0. A non-empty string maps to at least 1 so very
// short blocks (e.g. a one-word tool argument) still contribute to the
// reconciled total instead of disappearing. If the embedded vocab somehow
// fails to load, we fall back to the v0.1 chars/4 heuristic rather than panic.
func Tokens(s string) int {
	if s == "" {
		return 0
	}
	if n, err := CountBPE(s); err == nil {
		if n < 1 {
			return 1
		}
		return n
	}
	return charsOver4(s)
}

// charsOver4 is the v0.1 fallback heuristic, retained only for the degraded
// path where the embedded BPE model fails to load. It counts runes so CJK text
// gets a realistic weight.
func charsOver4(s string) int {
	runes := utf8.RuneCountInString(s)
	if runes < 4 {
		return 1
	}
	return runes / 4
}
