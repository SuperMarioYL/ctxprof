// Package estimate sizes individual content blocks with a local heuristic.
//
// The Claude Code JSONL records token totals only once per assistant turn
// (message.usage). There is no per-block token field, so to attribute the
// turn total across the blocks that make it up we first assign each block a
// rough weight here, then reconcile those weights against the real per-turn
// total during attribution (m2).
package estimate

import "unicode/utf8"

// Tokens returns a chars/4 token estimate for s.
//
// We count runes rather than bytes so multi-byte text (CJK, emoji) gets a
// realistic weight: tiktoken-style tokenizers charge roughly one token per
// CJK character versus one token per four English characters, and chars/4 on
// the rune count is a reasonable v0.1 compromise across both regimes.
//
// An empty string maps to 0; a non-empty string maps to at least 1 so very
// short blocks (e.g. a one-word tool argument) still contribute to the
// reconciled total instead of disappearing.
func Tokens(s string) int {
	if s == "" {
		return 0
	}
	runes := utf8.RuneCountInString(s)
	if runes < 4 {
		return 1
	}
	return runes / 4
}
