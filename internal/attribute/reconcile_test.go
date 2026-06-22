package attribute_test

import (
	"path/filepath"
	"testing"

	"github.com/SuperMarioYL/ctxprof/internal/attribute"
	"github.com/SuperMarioYL/ctxprof/internal/parser"
)

const sampleSessionPath = "../../examples/sample-session.jsonl"

func TestAttribute_BalancesToRealPerTurnTotals(t *testing.T) {
	abs, err := filepath.Abs(sampleSessionPath)
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	sess, err := parser.ParseFile(abs)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	alloc := attribute.Attribute(sess, 200_000)

	// Ground truth: sum of every assistant turn's message.usage total.
	realTotal := 0
	for _, turn := range sess.Turns {
		if turn.Usage != nil {
			realTotal += turn.Usage.Total()
		}
	}

	if alloc.CumulativeTokens != realTotal {
		t.Errorf("alloc.CumulativeTokens = %d, want %d", alloc.CumulativeTokens, realTotal)
	}

	bucketSum := 0
	for _, bd := range alloc.Buckets {
		bucketSum += bd.Tokens
	}
	if bucketSum != realTotal {
		t.Errorf("sum of bucket tokens = %d, real per-turn total = %d (reconciliation off by %d)",
			bucketSum, realTotal, bucketSum-realTotal)
	}

	if !alloc.Estimated {
		t.Error("Allocation.Estimated must be true in v0.1")
	}
	if alloc.WindowMax != 200_000 {
		t.Errorf("WindowMax = %d, want 200000", alloc.WindowMax)
	}
	if alloc.SessionID != "01J0Z5K3X4SAMPLEPROFILE" {
		t.Errorf("SessionID = %q", alloc.SessionID)
	}
}

func TestAttribute_ClassifierFiresOnRealFixture(t *testing.T) {
	abs, _ := filepath.Abs(sampleSessionPath)
	sess, err := parser.ParseFile(abs)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	alloc := attribute.Attribute(sess, 200_000)

	// Each of these buckets must have fired at least once on the fixture, or
	// the classifier signals are silently broken on a real session — the m2
	// kill criterion in mvp_plan §8.
	want := []parser.Bucket{
		parser.BucketReasoning, // a1 thinking
		parser.BucketOutput,    // a1, a2 text
		parser.BucketMCP,       // a1 mcp__grafana__get_panel
		parser.BucketFile,      // a2 Read on docs/incidents/2026-05.md
		parser.BucketSkill,     // a2 Skill caveman
		parser.BucketSystem,    // first-turn cache_creation
	}
	for _, b := range want {
		bd, ok := alloc.Buckets[b]
		if !ok || bd.Tokens == 0 {
			t.Errorf("bucket %q empty — classifier did not fire on fixture", b)
		}
	}

	// Skill bucket must surface "caveman" as a named item, MCP must surface
	// "grafana", file must surface the incident notes path.
	mustHaveItem(t, alloc.Buckets[parser.BucketSkill], "caveman")
	mustHaveItem(t, alloc.Buckets[parser.BucketMCP], "grafana")
	mustHaveItem(t, alloc.Buckets[parser.BucketFile], "docs/incidents/2026-05.md")
}

func TestAttribute_EmptySession(t *testing.T) {
	alloc := attribute.Attribute(&parser.Session{ID: "empty"}, 200_000)
	if alloc.CumulativeTokens != 0 {
		t.Errorf("empty session CumulativeTokens = %d, want 0", alloc.CumulativeTokens)
	}
	if len(alloc.Buckets) != 0 {
		t.Errorf("empty session buckets = %v, want empty", alloc.Buckets)
	}
	if alloc.SessionID != "empty" {
		t.Errorf("SessionID = %q", alloc.SessionID)
	}
}

// --- fix: cache-read double-count -------------------------------------------
//
// Three assistant turns that each re-read a growing cached prefix. The naive
// v0.1 headline summed every turn's Total() (cache_read included) into the
// window number, counting the same prefix three times. The fix drives the
// headline from WindowOccupancy = peak single-turn footprint instead.
func TestAttribute_WindowOccupancyIsPeakNotCumulative(t *testing.T) {
	u := func(in, read, create, out int) *parser.Usage {
		return &parser.Usage{
			InputTokens:              in,
			CacheReadInputTokens:     read,
			CacheCreationInputTokens: create,
			OutputTokens:             out,
		}
	}
	sess := &parser.Session{
		ID: "windowcheck",
		Turns: []parser.Turn{
			{Idx: 0, Role: parser.RoleAssistant, Usage: u(100, 40_000, 10_000, 500),
				Blocks: []parser.Block{{Type: parser.BlockText, EstTokens: 50}}},
			{Idx: 1, Role: parser.RoleAssistant, Usage: u(200, 55_000, 0, 600),
				Blocks: []parser.Block{{Type: parser.BlockText, EstTokens: 60}}},
			{Idx: 2, Role: parser.RoleAssistant, Usage: u(150, 70_000, 0, 700),
				Blocks: []parser.Block{{Type: parser.BlockText, EstTokens: 70}}},
		},
	}

	alloc := attribute.Attribute(sess, 200_000)

	// Cumulative is the honest sum of all per-turn totals (the throughput).
	wantCumulative := 0
	for _, tr := range sess.Turns {
		wantCumulative += tr.Usage.Total()
	}
	if alloc.CumulativeTokens != wantCumulative {
		t.Errorf("CumulativeTokens = %d, want %d", alloc.CumulativeTokens, wantCumulative)
	}

	// WindowOccupancy is the PEAK single-turn footprint (input+read+create),
	// here turn 2: 150 + 70_000 + 0 = 70_150.
	const wantOccupancy = 150 + 70_000
	if alloc.WindowOccupancy != wantOccupancy {
		t.Errorf("WindowOccupancy = %d, want %d (peak single-turn footprint)", alloc.WindowOccupancy, wantOccupancy)
	}

	// The whole point of the fix: occupancy must be far below the cumulative
	// sum, so the headline window-% is not inflated by re-counting the prefix.
	if alloc.WindowOccupancy >= alloc.CumulativeTokens {
		t.Errorf("WindowOccupancy (%d) should be < CumulativeTokens (%d) on a cache-heavy session",
			alloc.WindowOccupancy, alloc.CumulativeTokens)
	}

	// Reconciliation invariant still holds against cumulative.
	bucketSum := 0
	for _, bd := range alloc.Buckets {
		bucketSum += bd.Tokens
	}
	if bucketSum != alloc.CumulativeTokens {
		t.Errorf("bucket sum %d != CumulativeTokens %d", bucketSum, alloc.CumulativeTokens)
	}
}

// --- fix: system bucket swallows mcp descriptors ----------------------------
//
// When the first turn's cached prefix coincides with MCP tool_use blocks, the
// seed must be split between system and mcp, not dumped 100% into system.
func TestAttribute_FirstTurnSeedSplitSystemMCP(t *testing.T) {
	mcpBlock := parser.Block{Type: parser.BlockToolUse, ToolName: "mcp__grafana__get_panel", EstTokens: 10}
	readBlock := parser.Block{Type: parser.BlockToolUse, ToolName: "Read", EstTokens: 10,
		ToolInput: map[string]any{"file_path": "a.md"}}
	sess := &parser.Session{
		ID: "splitcheck",
		Turns: []parser.Turn{{
			Idx:   0,
			Role:  parser.RoleAssistant,
			Usage: &parser.Usage{CacheCreationInputTokens: 10_000, InputTokens: 100, OutputTokens: 100},
			// Two tool_use blocks, one of which is MCP -> 50% of the seed
			// (under the 0.75 cap) should land in mcp.
			Blocks: []parser.Block{mcpBlock, readBlock},
		}},
	}

	alloc := attribute.Attribute(sess, 200_000)

	sys := alloc.Buckets[parser.BucketSystem].Tokens
	mcp := alloc.Buckets[parser.BucketMCP].Tokens
	if sys == 0 {
		t.Fatal("system bucket empty — seed not attributed")
	}
	if mcp == 0 {
		t.Fatal("mcp bucket empty — first-turn seed still swallowed entirely by system (the bug)")
	}
	// 1 of 2 tool_use blocks is MCP -> ~50% of the 10_000 seed to mcp, plus the
	// mcp block's own reconciled share. The seed portion alone must be ~5_000.
	if mcp < 4_000 {
		t.Errorf("mcp bucket = %d, expected ~half the 10k seed (the split heuristic)", mcp)
	}

	// No tool_use blocks at all -> the whole seed stays in system (degraded
	// signal, matches pre-v0.2 behavior for MCP-free sessions).
	plain := &parser.Session{
		ID: "nomcp",
		Turns: []parser.Turn{{
			Idx:    0,
			Role:   parser.RoleAssistant,
			Usage:  &parser.Usage{CacheCreationInputTokens: 8_000, InputTokens: 50},
			Blocks: []parser.Block{{Type: parser.BlockText, EstTokens: 20}},
		}},
	}
	pa := attribute.Attribute(plain, 200_000)
	if pa.Buckets[parser.BucketMCP].Tokens != 0 {
		t.Errorf("mcp bucket = %d on an MCP-free session, want 0", pa.Buckets[parser.BucketMCP].Tokens)
	}
}

// --- fix: window-max zero schema violation ----------------------------------
//
// A zero or negative window must be clamped before any Allocation is built, so
// the emitted allocation_v1.json never carries window_max < 1. This mirrors the
// guard the CLI relies on (profile() -> attribute.Attribute).
func TestAttribute_WindowMaxGuardKeepsSchemaValid(t *testing.T) {
	abs, _ := filepath.Abs(sampleSessionPath)
	sess, err := parser.ParseFile(abs)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	for _, bad := range []int{0, -1, -200_000} {
		alloc := attribute.Attribute(sess, bad)
		if alloc.WindowMax < 1 {
			t.Errorf("Attribute(_, %d) -> WindowMax = %d, violates allocation_v1 minimum:1", bad, alloc.WindowMax)
		}
		if alloc.WindowMax != attribute.DefaultWindowMax {
			t.Errorf("Attribute(_, %d) -> WindowMax = %d, want clamp to %d", bad, alloc.WindowMax, attribute.DefaultWindowMax)
		}
	}
	// A positive value passes through untouched.
	if alloc := attribute.Attribute(sess, 123_456); alloc.WindowMax != 123_456 {
		t.Errorf("positive window-max not preserved: got %d", alloc.WindowMax)
	}
}

func mustHaveItem(t *testing.T, bd parser.BucketBreakdown, name string) {
	t.Helper()
	for _, it := range bd.Items {
		if it.Name == name {
			if it.Tokens <= 0 {
				t.Errorf("item %q present but Tokens = %d", name, it.Tokens)
			}
			return
		}
	}
	t.Errorf("item %q missing from bucket items %+v", name, bd.Items)
}
