package render

import (
	"fmt"
	"io"
	"sort"

	"github.com/SuperMarioYL/ctxprof/internal/parser"
)

// BucketDelta is one bucket's before/after reconciled tokens across two sessions, plus
// the signed change (New - Old). It is a pure derived view over two Allocations — no new
// attribution logic, no model in the loop.
type BucketDelta struct {
	Bucket string `json:"bucket"`
	Old    int    `json:"old"`
	New    int    `json:"new"`
	Delta  int    `json:"delta"`
}

// ItemDelta is one named item's before/after reconciled tokens across two sessions. An
// item present in only one session has 0 for the missing side, so a newly-appeared skill
// shows Old=0 and a dropped MCP server shows New=0. Bucket carries which bucket the item
// lives in for display + stable ordering.
type ItemDelta struct {
	Bucket string `json:"bucket"`
	Name   string `json:"name"`
	Old    int    `json:"old"`
	New    int    `json:"new"`
	Delta  int    `json:"delta"`
}

// BucketDeltas returns one BucketDelta per bucket in the fixed bucketOrder, so the compare
// table's rows stay in the same familiar shape as the tree and trend views. Buckets that
// are zero in BOTH sessions are omitted to keep the table tight (a bucket that is non-zero
// in either session is kept so an appear/disappear is visible).
func BucketDeltas(oldAlloc, newAlloc parser.Allocation) []BucketDelta {
	out := make([]BucketDelta, 0, len(bucketOrder))
	for _, b := range bucketOrder {
		o := oldAlloc.Buckets[b].Tokens
		n := newAlloc.Buckets[b].Tokens
		if o == 0 && n == 0 {
			continue
		}
		out = append(out, BucketDelta{
			Bucket: string(b),
			Old:    o,
			New:    n,
			Delta:  n - o,
		})
	}
	return out
}

// ItemDeltas merges every NAMED item across all buckets of both sessions and returns the n
// with the largest absolute change, ranked by |Delta| descending. Only named items
// participate — anonymous bucket weight (the system-prompt remainder) has nothing concrete
// to compare — matching the cut-candidates rule. n <= 0 returns nil. Ties break
// deterministically (larger new-side first, then bucket, then name) so output is stable
// across runs despite random map iteration.
func ItemDeltas(oldAlloc, newAlloc parser.Allocation, n int) []ItemDelta {
	if n <= 0 {
		return nil
	}

	// key = bucket + "\x00" + name, so the same-named item in two different buckets
	// stays distinct.
	type key struct {
		bucket string
		name   string
	}
	merged := map[key]*ItemDelta{}
	collect := func(alloc parser.Allocation, assign func(d *ItemDelta, tok int)) {
		for bucket, bd := range alloc.Buckets {
			for _, it := range bd.Items {
				if it.Name == "" {
					continue
				}
				k := key{string(bucket), it.Name}
				d := merged[k]
				if d == nil {
					d = &ItemDelta{Bucket: string(bucket), Name: it.Name}
					merged[k] = d
				}
				assign(d, it.Tokens)
			}
		}
	}
	collect(oldAlloc, func(d *ItemDelta, tok int) { d.Old += tok })
	collect(newAlloc, func(d *ItemDelta, tok int) { d.New += tok })

	out := make([]ItemDelta, 0, len(merged))
	for _, d := range merged {
		d.Delta = d.New - d.Old
		if d.Delta == 0 {
			// Flat items are not "changes" — omit them from the change list so the
			// section shows only what actually moved.
			continue
		}
		out = append(out, *d)
	}

	sort.Slice(out, func(i, j int) bool {
		ai, aj := absInt(out[i].Delta), absInt(out[j].Delta)
		if ai != aj {
			return ai > aj
		}
		if out[i].New != out[j].New {
			return out[i].New > out[j].New
		}
		if out[i].Bucket != out[j].Bucket {
			return out[i].Bucket < out[j].Bucket
		}
		return out[i].Name < out[j].Name
	})

	if len(out) > n {
		out = out[:n]
	}
	return out
}

// Compare prints a two-session A/B diff: a per-bucket old→new→Δ table, then the largest
// per-item changes. Read-only: it lists evidence and never proposes an automated edit.
// oldLabel/newLabel are short column headers (session id or file base) for the two sides.
func Compare(w io.Writer, oldLabel, newLabel string, buckets []BucketDelta, items []ItemDelta, opts TreeOptions) {
	st := styler{useColor: !opts.NoColor}

	const labelW = 10
	const cellW = 12

	fmt.Fprintf(w, "compare  %s → %s  (Δ = new − old)\n", oldLabel, newLabel)

	// Bucket table header.
	fmt.Fprintf(w, "%s", padRight("bucket", labelW))
	fmt.Fprintf(w, "%s", rightAlign(truncate(oldLabel, cellW-1), cellW))
	fmt.Fprintf(w, "%s", rightAlign(truncate(newLabel, cellW-1), cellW))
	fmt.Fprintf(w, "%s\n", rightAlign("Δ", cellW+3))

	if len(buckets) == 0 {
		fmt.Fprintln(w, "(both sessions have zero attributed tokens)")
		return
	}
	for _, b := range buckets {
		label := st.render(padRight(bucketLabelOrRaw(b.Bucket), labelW), bucketColor[parser.Bucket(b.Bucket)])
		fmt.Fprintf(w, "%s", label)
		fmt.Fprintf(w, "%s", rightAlign(humanInt(b.Old), cellW))
		fmt.Fprintf(w, "%s", rightAlign(humanInt(b.New), cellW))
		fmt.Fprintf(w, "%s\n", rightAlign(signedHuman(b.Delta), cellW+3))
	}

	if len(items) == 0 {
		return
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "largest per-item changes (top %d by absolute Δ — diagnosis only, ctxprof never edits a session):\n", len(items))
	for i, it := range items {
		bucket := bucketLabelOrRaw(it.Bucket)
		colored := padRight(bucket, 10)
		if col, ok := bucketColor[parser.Bucket(it.Bucket)]; ok {
			colored = st.render(padRight(bucket, 10), col)
		}
		fmt.Fprintf(w, "  %2d. %s %s  %s → %s  (%s)\n",
			i+1,
			colored,
			padRight(truncate(it.Name, 32), 32),
			rightAlign(humanInt(it.Old), 8),
			rightAlign(humanInt(it.New), 8),
			signedHuman(it.Delta),
		)
	}
}

// absInt returns the absolute value of n.
func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
