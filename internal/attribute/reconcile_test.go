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

	if alloc.TotalTokens != realTotal {
		t.Errorf("alloc.TotalTokens = %d, want %d", alloc.TotalTokens, realTotal)
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
	if alloc.TotalTokens != 0 {
		t.Errorf("empty session TotalTokens = %d, want 0", alloc.TotalTokens)
	}
	if len(alloc.Buckets) != 0 {
		t.Errorf("empty session buckets = %v, want empty", alloc.Buckets)
	}
	if alloc.SessionID != "empty" {
		t.Errorf("SessionID = %q", alloc.SessionID)
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
