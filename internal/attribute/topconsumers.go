package attribute

import (
	"sort"

	"github.com/SuperMarioYL/ctxprof/internal/parser"
)

// CutCandidate is one named consumer of the context window, surfaced so the user
// can decide what to trim. It is a read-only diagnosis — ctxprof never edits a
// session — so a CutCandidate carries no "action", only the evidence:
//
//   - Bucket / Name: which bucket the item lives in and its display name
//     (a skill name, an MCP server, a file path).
//   - Tokens: the item's reconciled token count.
//   - WindowShare: Tokens as a fraction (0..1) of the session's peak single-turn
//     window occupancy — i.e. how much of the window this single item is worth.
//     When WindowOccupancy is 0 (degenerate/empty session) WindowShare is 0.
type CutCandidate struct {
	Bucket      string  `json:"bucket"`
	Name        string  `json:"name"`
	Tokens      int     `json:"tokens"`
	WindowShare float64 `json:"window_share"`
}

// TopCutCandidates merges every named Item across all buckets of alloc and returns
// the n largest by reconciled tokens, each annotated with its share of the peak
// window occupancy. It is a pure post-pass over the existing Allocation — no new
// attribution logic, no model in the loop.
//
// Only NAMED items participate: anonymous bucket weight (e.g. the system-prompt
// remainder, which has no per-item name) is intentionally excluded because there
// is nothing concrete for the user to "cut". n <= 0 returns nil; if fewer than n
// named items exist, all of them are returned.
func TopCutCandidates(alloc parser.Allocation, n int) []CutCandidate {
	if n <= 0 {
		return nil
	}

	occ := alloc.WindowOccupancy
	out := make([]CutCandidate, 0)
	for bucket, bd := range alloc.Buckets {
		for _, it := range bd.Items {
			if it.Name == "" || it.Tokens <= 0 {
				continue
			}
			share := 0.0
			if occ > 0 {
				share = float64(it.Tokens) / float64(occ)
			}
			out = append(out, CutCandidate{
				Bucket:      string(bucket),
				Name:        it.Name,
				Tokens:      it.Tokens,
				WindowShare: share,
			})
		}
	}

	// Largest first; ties broken deterministically by bucket then name so the
	// output is stable across runs (map iteration order above is random).
	sort.Slice(out, func(i, j int) bool {
		if out[i].Tokens != out[j].Tokens {
			return out[i].Tokens > out[j].Tokens
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
