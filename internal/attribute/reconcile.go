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
// The first assistant turn's cache_creation_input_tokens is split off before
// reconciliation, on the theory that those bytes represent the harness-prepended
// system prompt + CLAUDE.md + MCP/tool descriptors — content that is never
// written into message.content but still consumes the window. As of v0.2 that
// seed is apportioned across the system and mcp buckets via splitFirstTurnSeed
// (so the MCP descriptor catalog no longer disappears entirely into system);
// both halves are approximate, the rest is calibrated.
//
// User turns carry no message.usage in the JSONL, so they do not add to the
// session total. The bytes a user turn contributes to context end up counted
// inside the *next* assistant turn's input_tokens, which is where they show
// up in the reconciled buckets.
// mcpSplitFloor / mcpSplitCap bound the documented heuristic that splits the
// first turn's cache_creation between the system prompt and the MCP/tool
// descriptors bundled into the same cached prefix (see splitFirstTurnSeed).
const (
	mcpSplitFloor = 0.0
	mcpSplitCap   = 0.75
)

// DefaultWindowMax is the clamp target for an invalid (<=0) window size, equal
// to Claude Code's current 200k context. Exported so the CLI and tests agree.
const DefaultWindowMax = 200_000

func Attribute(sess *parser.Session, windowMax int) parser.Allocation {
	// Guard the schema's window_max minimum:1. A zero/negative window is
	// meaningless for the headline percentage and would emit an
	// allocation_v1-invalid document, so clamp it here — the last gate before
	// any Allocation is produced, regardless of caller.
	if windowMax <= 0 {
		windowMax = DefaultWindowMax
	}
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
		alloc.CumulativeTokens += turnTotal

		// Track the peak single-turn window footprint. This — not the
		// cross-turn cumulative sum — drives the headline window-%, because
		// cache_read re-counts the cached prefix every turn (fix:
		// cache-read-double-count).
		if fp := turn.Usage.WindowFootprint(); fp > alloc.WindowOccupancy {
			alloc.WindowOccupancy = fp
		}

		available := turnTotal
		if !systemSeeded {
			seed := turn.Usage.CacheCreationInputTokens
			if seed > available {
				seed = available
			}
			sysSeed, mcpSeed := splitFirstTurnSeed(seed, turn.Blocks)
			add(parser.BucketSystem, "", sysSeed)
			add(parser.BucketMCP, "", mcpSeed)
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

// splitFirstTurnSeed apportions the first turn's cache_creation_input_tokens
// between the system bucket and the mcp bucket.
//
// The first cached prefix bundles the harness system prompt + CLAUDE.md + the
// MCP/tool descriptor catalog + the first user message — none of which are
// serialized into per-block content, so we cannot read the split. We
// approximate it (both halves are flagged approximate via render.approxBuckets)
// with a documented, deterministic heuristic:
//
//	fraction-to-mcp = clamp( mcpToolUseBlocks / totalToolUseBlocks , 0 , 0.75 )
//
// i.e. the more of the turn's tool_use blocks are MCP calls, the larger the
// share of the cached descriptor prefix we attribute to MCP, capped at 0.75 so
// the system prompt always keeps a plurality. When the turn has no tool_use
// blocks we cannot tell, so the whole seed stays in system (mcp = 0), matching
// pre-v0.2 behavior for MCP-free sessions.
func splitFirstTurnSeed(seed int, blocks []parser.Block) (sys, mcp int) {
	if seed <= 0 {
		return 0, 0
	}
	totalTools, mcpTools := 0, 0
	for _, b := range blocks {
		if b.Type != parser.BlockToolUse {
			continue
		}
		totalTools++
		if ClassifyBlock(b) == parser.BucketMCP {
			mcpTools++
		}
	}
	if totalTools == 0 || mcpTools == 0 {
		return seed, 0
	}
	frac := float64(mcpTools) / float64(totalTools)
	if frac < mcpSplitFloor {
		frac = mcpSplitFloor
	}
	if frac > mcpSplitCap {
		frac = mcpSplitCap
	}
	mcp = int(float64(seed) * frac)
	if mcp > seed {
		mcp = seed
	}
	return seed - mcp, mcp
}
