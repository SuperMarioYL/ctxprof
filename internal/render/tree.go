package render

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/SuperMarioYL/ctxprof/internal/parser"
)

// barWidth is the visible bar width in cells. Wide enough to resolve a
// percent-or-so change, narrow enough to share a line with the label and
// numbers on an 80-column terminal.
const barWidth = 14

// maxItemsPerBucket caps the named rows ctxprof prints inside one bucket so
// a long file-heavy session does not bury the headline split under thirty
// Read-tool rows. The full list is still in the JSON.
const maxItemsPerBucket = 5

// bucketOrder is the fixed render order in the tree. It mirrors the §1
// example layout in mvp_plan.md and is independent of bucket size so the
// shape of the tree stays familiar across sessions.
var bucketOrder = []parser.Bucket{
	parser.BucketSystem,
	parser.BucketSkill,
	parser.BucketMCP,
	parser.BucketFile,
	parser.BucketReasoning,
	parser.BucketOutput,
}

var bucketLabel = map[parser.Bucket]string{
	parser.BucketSystem:    "system",
	parser.BucketSkill:     "skill",
	parser.BucketMCP:       "mcp",
	parser.BucketFile:      "file",
	parser.BucketReasoning: "reasoning",
	parser.BucketOutput:    "output",
}

// bucketColor is the ANSI-256 foreground used per bucket when color is on.
// Picked for legibility on both light and dark terminals; can be themed later.
var bucketColor = map[parser.Bucket]lipgloss.Color{
	parser.BucketSystem:    lipgloss.Color("245"),
	parser.BucketSkill:     lipgloss.Color("213"),
	parser.BucketMCP:       lipgloss.Color("87"),
	parser.BucketFile:      lipgloss.Color("221"),
	parser.BucketReasoning: lipgloss.Color("141"),
	parser.BucketOutput:    lipgloss.Color("114"),
}

// approxBuckets are the buckets whose numbers are flagged in the tree with a
// trailing "~" because their underlying signal is not in the JSONL at all
// (system prompt + MCP-descriptor portion are read from first-turn
// cache_creation_input_tokens, not from per-block fields).
var approxBuckets = map[parser.Bucket]bool{
	parser.BucketSystem: true,
	parser.BucketMCP:    true,
}

// TreeOptions controls the look of Tree without affecting its data.
type TreeOptions struct {
	// NoColor disables ANSI styling. Use on dumb terminals or when piping.
	NoColor bool
}

// Tree renders alloc as a flame-graph-style ASCII tree to w.
//
// The headline shows the window utilization (TotalTokens / WindowMax). Each
// bucket gets a row with a unit-bar, the reconciled token count, and the
// percent of the session total. System and MCP rows carry a trailing "~"
// because their values are approximate per the honesty contract.
//
// If alloc.Estimated is true a one-line note is appended explaining what the
// numbers mean.
func Tree(w io.Writer, alloc parser.Allocation, opts TreeOptions) error {
	st := styler{useColor: !opts.NoColor}

	// TotalTokens is the cumulative sum of every per-turn message.usage total.
	// On a short session that stayed inside one window it reads naturally as
	// "% of the 200k window". On a long session it is genuine cumulative
	// throughput that legitimately exceeds one window (the cached prefix is
	// re-counted each turn by the harness), so the window-% would mislead —
	// present it as cumulative instead. Per-bucket rows are always % of total,
	// which is meaningful either way.
	if alloc.WindowMax > 0 && alloc.TotalTokens <= alloc.WindowMax {
		pct := float64(alloc.TotalTokens) / float64(alloc.WindowMax) * 100
		fmt.Fprintf(w, "session %s — %s / %s tokens (%.0f%% of window)\n",
			alloc.SessionID,
			humanInt(alloc.TotalTokens),
			humanInt(alloc.WindowMax),
			pct,
		)
	} else {
		fmt.Fprintf(w, "session %s — %s tokens cumulative (window: %s; spend re-counts the cached prefix each turn)\n",
			alloc.SessionID,
			humanInt(alloc.TotalTokens),
			humanInt(alloc.WindowMax),
		)
	}

	type row struct {
		bucket parser.Bucket
		bd     parser.BucketBreakdown
	}
	var rows []row
	for _, b := range bucketOrder {
		bd, ok := alloc.Buckets[b]
		if !ok || bd.Tokens == 0 {
			continue
		}
		rows = append(rows, row{b, bd})
	}

	for i, r := range rows {
		last := i == len(rows)-1
		branch := "├──"
		if last {
			branch = "└──"
		}

		bar := makeBar(r.bd.Tokens, alloc.TotalTokens, barWidth)
		rowPct := 0.0
		if alloc.TotalTokens > 0 {
			rowPct = float64(r.bd.Tokens) / float64(alloc.TotalTokens) * 100
		}
		approx := ""
		if approxBuckets[r.bucket] {
			approx = " ~"
		}
		label := st.render(fmt.Sprintf("%-10s", bucketLabel[r.bucket]), bucketColor[r.bucket])
		coloredBar := st.render(bar, bucketColor[r.bucket])
		fmt.Fprintf(w, "%s %s %s  %s  (%.1f%%)%s\n",
			branch, label, coloredBar,
			rightAlign(humanInt(r.bd.Tokens), 8),
			rowPct, approx,
		)

		items := append([]parser.Item(nil), r.bd.Items...)
		sort.Slice(items, func(i, j int) bool {
			if items[i].Tokens != items[j].Tokens {
				return items[i].Tokens > items[j].Tokens
			}
			return items[i].Name < items[j].Name
		})
		if len(items) > maxItemsPerBucket {
			items = items[:maxItemsPerBucket]
		}

		prefix := "│   "
		if last {
			prefix = "    "
		}
		for j, it := range items {
			ibranch := "├──"
			if j == len(items)-1 {
				ibranch = "└──"
			}
			fmt.Fprintf(w, "%s%s %-28s %s\n",
				prefix, ibranch, truncate(it.Name, 28),
				rightAlign(humanInt(it.Tokens), 8),
			)
		}
	}

	if alloc.Estimated {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "note: bucket numbers are calibrated estimates reconciled to real per-turn message.usage totals.")
		fmt.Fprintln(w, "      rows marked ~ (system, mcp) are approximated from the first turn's cache_creation_input_tokens.")
	}
	return nil
}

type styler struct {
	useColor bool
}

func (s styler) render(text string, c lipgloss.Color) string {
	if !s.useColor {
		return text
	}
	return lipgloss.NewStyle().Foreground(c).Render(text)
}

// makeBar returns a fixed-width unit bar showing n / total filled.
func makeBar(n, total, width int) string {
	if total <= 0 || width <= 0 {
		return strings.Repeat("░", width)
	}
	filled := int(float64(n) / float64(total) * float64(width))
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}

// humanInt formats n with comma thousand-separators ("184512" -> "184,512").
func humanInt(n int) string {
	neg := n < 0
	if neg {
		n = -n
	}
	s := strconv.Itoa(n)
	if len(s) <= 3 {
		if neg {
			return "-" + s
		}
		return s
	}
	var b strings.Builder
	first := len(s) % 3
	if first == 0 {
		first = 3
	}
	b.WriteString(s[:first])
	for i := first; i < len(s); i += 3 {
		b.WriteByte(',')
		b.WriteString(s[i : i+3])
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

func rightAlign(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return strings.Repeat(" ", width-len(s)) + s
}

func truncate(s string, width int) string {
	if len(s) <= width {
		return s
	}
	if width <= 1 {
		return s[:width]
	}
	return s[:width-1] + "…"
}
