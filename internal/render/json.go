// Package render emits ctxprof output in machine- and human-readable forms.
//
// The m1 surface here is PerTurnJSON: one JSON object per assistant or user
// turn, written line-delimited so callers can pipe it through jq. The
// flame-graph tree (lipgloss) lands in m3.
package render

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/SuperMarioYL/ctxprof/internal/parser"
)

// PerTurnRecord is the JSON shape printed by `ctxprof parse <file>`. It
// mirrors parser.Turn but is declared separately so the on-disk debug
// schema does not silently follow internal struct churn.
type PerTurnRecord struct {
	Idx    int             `json:"idx"`
	Role   parser.Role     `json:"role"`
	Usage  *parser.Usage   `json:"usage"`
	Blocks []parser.Block  `json:"blocks"`
}

// PerTurnJSON writes one PerTurnRecord per line to w.
func PerTurnJSON(w io.Writer, sess *parser.Session) error {
	if sess == nil {
		return fmt.Errorf("nil session")
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	for _, t := range sess.Turns {
		rec := PerTurnRecord{
			Idx:    t.Idx,
			Role:   t.Role,
			Usage:  t.Usage,
			Blocks: t.Blocks,
		}
		if err := enc.Encode(rec); err != nil {
			return fmt.Errorf("encode turn %d: %w", t.Idx, err)
		}
	}
	return nil
}
