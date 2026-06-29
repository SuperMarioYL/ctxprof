package attribute

import (
	"testing"

	"github.com/SuperMarioYL/ctxprof/internal/parser"
)

func allocWithItems() parser.Allocation {
	return parser.Allocation{
		WindowOccupancy: 20_000,
		WindowMax:       200_000,
		Buckets: map[parser.Bucket]parser.BucketBreakdown{
			parser.BucketSkill: {
				Tokens: 9000,
				Items: []parser.Item{
					{Name: "caveman", Tokens: 6000},
					{Name: "code-review", Tokens: 3000},
				},
			},
			parser.BucketMCP: {
				Tokens: 5000,
				Items:  []parser.Item{{Name: "grafana", Tokens: 5000}},
			},
			parser.BucketFile: {
				Tokens: 4000,
				Items:  []parser.Item{{Name: "docs/x.md", Tokens: 4000}},
			},
			// System carries anonymous weight only (no named items) — must be
			// excluded from cut-candidates entirely.
			parser.BucketSystem: {Tokens: 2000},
		},
	}
}

func TestTopCutCandidates_RanksLargestFirst(t *testing.T) {
	cuts := TopCutCandidates(allocWithItems(), 3)
	if len(cuts) != 3 {
		t.Fatalf("want 3 candidates, got %d: %+v", len(cuts), cuts)
	}
	wantOrder := []struct {
		name   string
		tokens int
	}{
		{"caveman", 6000},
		{"grafana", 5000},
		{"docs/x.md", 4000},
	}
	for i, w := range wantOrder {
		if cuts[i].Name != w.name || cuts[i].Tokens != w.tokens {
			t.Errorf("rank %d: want %s/%d, got %s/%d", i, w.name, w.tokens, cuts[i].Name, cuts[i].Tokens)
		}
	}
}

func TestTopCutCandidates_WindowShare(t *testing.T) {
	cuts := TopCutCandidates(allocWithItems(), 1)
	if len(cuts) != 1 {
		t.Fatalf("want 1, got %d", len(cuts))
	}
	// caveman 6000 / window 20000 = 0.30
	if got := cuts[0].WindowShare; got < 0.299 || got > 0.301 {
		t.Errorf("window share: want ~0.30, got %f", got)
	}
}

func TestTopCutCandidates_ExcludesAnonymousSystem(t *testing.T) {
	cuts := TopCutCandidates(allocWithItems(), 10)
	for _, c := range cuts {
		if c.Bucket == string(parser.BucketSystem) {
			t.Errorf("anonymous system weight must not appear as a cut-candidate: %+v", c)
		}
	}
	// 4 named items exist total; n=10 must return exactly those 4.
	if len(cuts) != 4 {
		t.Errorf("want 4 named candidates, got %d: %+v", len(cuts), cuts)
	}
}

func TestTopCutCandidates_NonPositiveN(t *testing.T) {
	if got := TopCutCandidates(allocWithItems(), 0); got != nil {
		t.Errorf("n=0 must return nil, got %+v", got)
	}
	if got := TopCutCandidates(allocWithItems(), -1); got != nil {
		t.Errorf("n<0 must return nil, got %+v", got)
	}
}

func TestTopCutCandidates_ZeroWindowOccupancy(t *testing.T) {
	a := allocWithItems()
	a.WindowOccupancy = 0
	cuts := TopCutCandidates(a, 2)
	if len(cuts) == 0 {
		t.Fatal("expected candidates even with zero occupancy")
	}
	for _, c := range cuts {
		if c.WindowShare != 0 {
			t.Errorf("zero occupancy must yield window share 0, got %f for %s", c.WindowShare, c.Name)
		}
	}
}

func TestTopCutCandidates_DeterministicTieBreak(t *testing.T) {
	a := parser.Allocation{
		WindowOccupancy: 1000,
		Buckets: map[parser.Bucket]parser.BucketBreakdown{
			parser.BucketSkill: {Items: []parser.Item{{Name: "b-skill", Tokens: 100}}},
			parser.BucketMCP:   {Items: []parser.Item{{Name: "a-mcp", Tokens: 100}}},
		},
	}
	// Equal tokens → break by bucket name ("mcp" < "skill"), so a-mcp first.
	first := TopCutCandidates(a, 2)
	for i := 0; i < 5; i++ {
		got := TopCutCandidates(a, 2)
		if got[0] != first[0] || got[1] != first[1] {
			t.Fatalf("ordering not deterministic across runs: %+v vs %+v", first, got)
		}
	}
	if first[0].Bucket != string(parser.BucketMCP) {
		t.Errorf("tie should break to mcp bucket first, got %s", first[0].Bucket)
	}
}
