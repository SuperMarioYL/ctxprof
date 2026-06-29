package render

import (
	"fmt"
	"io"

	"github.com/SuperMarioYL/ctxprof/internal/attribute"
	"github.com/SuperMarioYL/ctxprof/internal/parser"
)

// TrendPoint is one session in a trend view: a short column Label plus its
// reconciled Allocation. Callers pass these oldest→newest so the table reads
// left-to-right as time advances.
type TrendPoint struct {
	Label string
	Alloc parser.Allocation
}

// Trend prints a compact per-bucket drift table across the given sessions, so a
// user can see whether system / mcp / file budget is creeping up over time. One
// row per bucket (fixed bucketOrder), one column per session, each cell the
// bucket's reconciled token total for that session. A trailing "Δ" column shows
// the change from the first session to the last. A header row shows each
// session's peak window occupancy and window-%.
//
// Terminal-only and read-only — no graphs, no TUI. Width is bounded by the number
// of sessions; very wide trends simply wrap at the terminal like any tree row.
func Trend(w io.Writer, points []TrendPoint, opts TreeOptions) error {
	st := styler{useColor: !opts.NoColor}
	if len(points) == 0 {
		fmt.Fprintln(w, "no sessions to trend")
		return nil
	}

	const labelW = 10
	const cellW = 12

	// Header: session labels.
	fmt.Fprintf(w, "%s", padRight("bucket", labelW))
	for _, p := range points {
		fmt.Fprintf(w, "%s", rightAlign(truncate(p.Label, cellW-1), cellW))
	}
	fmt.Fprintf(w, "%s\n", rightAlign("Δ first→last", cellW+3))

	// Sub-header: each session's window occupancy + %.
	fmt.Fprintf(w, "%s", padRight("window", labelW))
	for _, p := range points {
		cell := humanInt(p.Alloc.WindowOccupancy)
		if p.Alloc.WindowMax > 0 {
			pct := float64(p.Alloc.WindowOccupancy) / float64(p.Alloc.WindowMax) * 100
			cell = fmt.Sprintf("%s/%.0f%%", humanInt(p.Alloc.WindowOccupancy), pct)
		}
		fmt.Fprintf(w, "%s", rightAlign(truncate(cell, cellW-1), cellW))
	}
	occDelta := points[len(points)-1].Alloc.WindowOccupancy - points[0].Alloc.WindowOccupancy
	fmt.Fprintf(w, "%s\n", rightAlign(signedHuman(occDelta), cellW+3))

	// One row per bucket.
	for _, b := range bucketOrder {
		// Skip a bucket that is zero in every session — keeps the table tight.
		anyNonZero := false
		for _, p := range points {
			if bd, ok := p.Alloc.Buckets[b]; ok && bd.Tokens != 0 {
				anyNonZero = true
				break
			}
		}
		if !anyNonZero {
			continue
		}

		label := st.render(padRight(bucketLabel[b], labelW), bucketColor[b])
		fmt.Fprintf(w, "%s", label)
		var first, last int
		for i, p := range points {
			tok := 0
			if bd, ok := p.Alloc.Buckets[b]; ok {
				tok = bd.Tokens
			}
			if i == 0 {
				first = tok
			}
			last = tok
			fmt.Fprintf(w, "%s", rightAlign(humanInt(tok), cellW))
		}
		fmt.Fprintf(w, "%s\n", rightAlign(signedHuman(last-first), cellW+3))
	}
	return nil
}

// CutCandidates prints the ranked largest single consumers of the window so the
// user sees exactly what to trim. Read-only: it lists evidence (item, bucket,
// tokens, window share) and never proposes an automated edit. Rendered after the
// flame tree by the root command when --cut-candidates N is given.
func CutCandidates(w io.Writer, cuts []attribute.CutCandidate, alloc parser.Allocation, opts TreeOptions) {
	st := styler{useColor: !opts.NoColor}
	if len(cuts) == 0 {
		return
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "top %d cut-candidates (largest single consumers — diagnosis only, ctxprof never edits a session):\n", len(cuts))
	for i, c := range cuts {
		bucket := bucketLabelOrRaw(c.Bucket)
		colored := bucket
		if col, ok := bucketColor[parser.Bucket(c.Bucket)]; ok {
			colored = st.render(padRight(bucket, 10), col)
		} else {
			colored = padRight(bucket, 10)
		}
		fmt.Fprintf(w, "  %2d. %s %s  %s  (%.1f%% of window)\n",
			i+1,
			colored,
			padRight(truncate(c.Name, 32), 32),
			rightAlign(humanInt(c.Tokens), 8),
			c.WindowShare*100,
		)
	}
}

// signedHuman formats a delta with an explicit +/- sign and comma grouping.
func signedHuman(n int) string {
	if n > 0 {
		return "+" + humanInt(n)
	}
	return humanInt(n) // humanInt already prefixes '-' for negatives
}

// bucketLabelOrRaw returns the human bucket label, falling back to the raw string
// for any bucket name not in the fixed map (defensive — should not happen).
func bucketLabelOrRaw(b string) string {
	if lbl, ok := bucketLabel[parser.Bucket(b)]; ok {
		return lbl
	}
	return b
}
