package attribute

import (
	"sort"

	"github.com/SuperMarioYL/ctxprof/internal/parser"
)

// Attribute builds an Allocation by classifying every block, then reconciling
// each assistant turn's locally-estimated block weights against that turn's
// real message.usage total. Bucket numbers are therefore calibrated estimates
// (the per-block tokenizer is a chars/4 heuristic), but the session-level
// invariant holds exactly: sum of all bucket tokens == sum of all per-turn
// message.usage totals == Allocation.TotalTokens.
//
// The first assistant turn's cache_creation_input_tokens is split off into
// the system bucket before reconciliation, on the theory that those bytes
// represent the harness-prepended system prompt and initial tool descriptors
// — content that is never written into message.content but still consumes
// the window. That portion is therefore approximate; the rest is calibrated.
//
// User turns carry no message.usage in the JSONL, so they do not add to the
// session total. The bytes a user turn contributes to context end up counted
// inside the *next* assistant turn's input_tokens, which is where they show
// up in the reconciled buckets.
func Attribute(sess *parser.Session, windowMax int) parser.Allocation {
	alloc := parser.Allocation{
		WindowMax: windowMax,
		Buckets:   map[parser.Bucket]parser.BucketBreakdown{},
		Estimated: true,
	}
	if sess != nil {
		alloc.SessionID = sess.ID
	}
	if sess == nil || len(sess.Turns) == 0 {
		return alloc
	}

	bucketTokens := map[parser.Bucket]int{}
	bucketItems := map[parser.Bucket]map[string]int{}

	add := func(bucket parser.Bucket, name string, n int) {
		if n == 0 && name == "" {
			return
		}
		bucketTokens[bucket] += n
		if name == "" {
			return
		}
		if bucketItems[bucket] == nil {
			bucketItems[bucket] = map[string]int{}
		}
		bucketItems[bucket][name] += n
	}

	systemSeeded := false
	for _, turn := range sess.Turns {
		if turn.Usage == nil {
			continue
		}
		turnTotal := turn.Usage.Total()
		alloc.TotalTokens += turnTotal

		available := turnTotal
		if !systemSeeded {
			seed := turn.Usage.CacheCreationInputTokens
			if seed > available {
				seed = available
			}
			add(parser.BucketSystem, "", seed)
			available -= seed
			systemSeeded = true
		}

		if available <= 0 || len(turn.Blocks) == 0 {
			// Nothing to attribute at the block level; park whatever is left
			// in system so the per-turn balance holds.
			if available > 0 {
				add(parser.BucketSystem, "", available)
			}
			continue
		}

		estSum := 0
		for _, b := range turn.Blocks {
			estSum += b.EstTokens
		}
		if estSum == 0 {
			// All blocks came in at zero estimated weight (e.g. empty
			// tool_result). Distribute available evenly to the first block's
			// bucket to keep the balance intact without inventing structure.
			add(ClassifyBlock(turn.Blocks[0]), ItemName(turn.Blocks[0], ClassifyBlock(turn.Blocks[0])), available)
			continue
		}

		// Proportionally scale each block's estimate to the available real
		// total, giving the last block the remainder so per-turn balance is
		// exact despite integer truncation.
		assigned := 0
		for i, b := range turn.Blocks {
			var scaled int
			if i == len(turn.Blocks)-1 {
				scaled = available - assigned
			} else {
				scaled = int(float64(b.EstTokens) / float64(estSum) * float64(available))
				if scaled < 0 {
					scaled = 0
				}
				if scaled > available-assigned {
					scaled = available - assigned
				}
			}
			assigned += scaled
			bucket := ClassifyBlock(b)
			add(bucket, ItemName(b, bucket), scaled)
		}
	}

	for bucket, total := range bucketTokens {
		bd := parser.BucketBreakdown{Tokens: total}
		if names := bucketItems[bucket]; len(names) > 0 {
			items := make([]parser.Item, 0, len(names))
			for name, tok := range names {
				items = append(items, parser.Item{Name: name, Tokens: tok})
			}
			sort.Slice(items, func(i, j int) bool {
				if items[i].Tokens != items[j].Tokens {
					return items[i].Tokens > items[j].Tokens
				}
				return items[i].Name < items[j].Name
			})
			bd.Items = items
		}
		alloc.Buckets[bucket] = bd
	}
	return alloc
}
