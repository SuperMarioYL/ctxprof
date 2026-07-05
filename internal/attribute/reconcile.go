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

	// tool_result blocks carry the retrieved file/tool content — the single
	// biggest input in most sessions — but they live in USER turns, which have
	// no message.usage. Their bytes are actually part of the NEXT assistant
	// turn's input_tokens / cache_read. So we fold each user turn's tool_result
	// blocks into the following assistant turn's reconciliation pool: their
	// EstTokens join estSum and get scaled alongside the assistant's own output,
	// landing (per the classifier's tool_result -> file rule) in the file bucket
	// instead of being skipped entirely. Their item name (the read file_path /
	// MCP server) is recovered by matching each result's ToolUseID to the
	// originating tool_use, indexed below.
	resultName := indexToolUseNames(sess)
	var pendingResults []parser.Block

	systemSeeded := false
	for _, turn := range sess.Turns {
		if turn.Usage == nil {
			// User turns carry no usage; stash any tool_result blocks so the
			// next assistant turn reconciles them (they belong to its input).
			for _, b := range turn.Blocks {
				if b.Type == parser.BlockToolResult {
					pendingResults = append(pendingResults, b)
				}
			}
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

		// The blocks reconciled for this assistant turn are the preceding user
		// turn(s)' tool_result content (their bytes are in THIS turn's input)
		// plus the assistant's own blocks. Consume the pending results here.
		blocks := turn.Blocks
		if len(pendingResults) > 0 {
			blocks = make([]parser.Block, 0, len(pendingResults)+len(turn.Blocks))
			blocks = append(blocks, pendingResults...)
			blocks = append(blocks, turn.Blocks...)
			pendingResults = nil
		}

		available := turnTotal
		if !systemSeeded {
			seed := turn.Usage.CacheCreationInputTokens
			if seed > available {
				seed = available
			}
			sysSeed, mcpSeed := splitFirstTurnSeed(seed, blocks)
			add(parser.BucketSystem, "", sysSeed)
			add(parser.BucketMCP, "", mcpSeed)
			available -= seed
			systemSeeded = true
		}

		if available <= 0 || len(blocks) == 0 {
			// Nothing to attribute at the block level; park whatever is left
			// in system so the per-turn balance holds.
			if available > 0 {
				add(parser.BucketSystem, "", available)
			}
			continue
		}

		estSum := 0
		for _, b := range blocks {
			estSum += b.EstTokens
		}
		if estSum == 0 {
			// All blocks came in at zero estimated weight (e.g. empty
			// tool_result). Distribute available evenly to the first block's
			// bucket to keep the balance intact without inventing structure.
			add(ClassifyBlock(blocks[0]), itemNameFor(blocks[0], ClassifyBlock(blocks[0]), resultName), available)
			continue
		}

		// Proportionally scale each block's estimate to the available real
		// total, giving the last block the remainder so per-turn balance is
		// exact despite integer truncation.
		assigned := 0
		for i, b := range blocks {
			var scaled int
			if i == len(blocks)-1 {
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
			add(bucket, itemNameFor(b, bucket, resultName), scaled)
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

// indexToolUseNames maps each tool_use block's ToolUseID to the display name it
// classified into (a Read's file_path, an MCP server, a skill name). It lets a
// tool_result block — which carries the retrieved content but not the originating
// path — inherit the name of the tool_use that produced it, so a 40k-token file
// read surfaces as a named row in the file bucket rather than an anonymous blob.
func indexToolUseNames(sess *parser.Session) map[string]string {
	names := map[string]string{}
	for _, turn := range sess.Turns {
		for _, b := range turn.Blocks {
			if b.Type != parser.BlockToolUse || b.ToolUseID == "" {
				continue
			}
			if name := ItemName(b, ClassifyBlock(b)); name != "" {
				names[b.ToolUseID] = name
			}
		}
	}
	return names
}

// itemNameFor is ItemName with one addition: a tool_result block (which the
// classifier routes to the file bucket but leaves unnamed, since the JSONL does
// not repeat the path on the result) inherits the name of the tool_use it
// answers, looked up by ToolUseID in resultName. Falls back to the plain
// ItemName for every other block, and for a result whose originating tool_use
// had no surfacable name (e.g. a Bash result) it stays unnamed — its tokens
// still roll into the bucket total.
func itemNameFor(b parser.Block, bucket parser.Bucket, resultName map[string]string) string {
	if b.Type == parser.BlockToolResult && b.ToolUseID != "" {
		if name := resultName[b.ToolUseID]; name != "" {
			return name
		}
	}
	return ItemName(b, bucket)
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
