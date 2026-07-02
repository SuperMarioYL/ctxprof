package render

import (
	"bytes"
	"strings"
	"testing"

	"github.com/SuperMarioYL/ctxprof/internal/parser"
)

// cmpAlloc builds an Allocation with the given bucket totals and optional named items,
// so the compare helpers can be exercised without a real session.
func cmpAlloc(id string, buckets map[parser.Bucket]parser.BucketBreakdown) parser.Allocation {
	return parser.Allocation{
		SessionID:       id,
		WindowOccupancy: 20_000,
		WindowMax:       200_000,
		Buckets:         buckets,
	}
}

func TestBucketDeltas_ComputesSignedChangeAndOmitsAllZero(t *testing.T) {
	oldA := cmpAlloc("old", map[parser.Bucket]parser.BucketBreakdown{
		parser.BucketSkill: {Tokens: 1000},
		parser.BucketFile:  {Tokens: 2000},
		parser.BucketMCP:   {Tokens: 0},
	})
	newA := cmpAlloc("new", map[parser.Bucket]parser.BucketBreakdown{
		parser.BucketSkill: {Tokens: 4000},
		parser.BucketFile:  {Tokens: 2000},
		parser.BucketMCP:   {Tokens: 0},
	})
	deltas := BucketDeltas(oldA, newA)

	got := map[string]BucketDelta{}
	for _, d := range deltas {
		got[d.Bucket] = d
	}
	// skill grew 1000 → 4000, Δ +3000.
	if d := got["skill"]; d.Old != 1000 || d.New != 4000 || d.Delta != 3000 {
		t.Errorf("skill delta wrong: %+v", d)
	}
	// file is flat 2000 → 2000 but non-zero, so it must be present with Δ 0.
	if d, ok := got["file"]; !ok || d.Delta != 0 {
		t.Errorf("file bucket should be present with Δ 0, got: %+v (present=%v)", d, ok)
	}
	// mcp is zero in BOTH sessions → omitted to keep the table tight.
	if _, ok := got["mcp"]; ok {
		t.Errorf("all-zero mcp bucket should be omitted, got: %+v", got["mcp"])
	}
}

func TestBucketDeltas_KeepsBucketNonZeroInOnlyOneSession(t *testing.T) {
	oldA := cmpAlloc("old", map[parser.Bucket]parser.BucketBreakdown{
		parser.BucketSkill: {Tokens: 0},
	})
	newA := cmpAlloc("new", map[parser.Bucket]parser.BucketBreakdown{
		parser.BucketSkill: {Tokens: 500},
	})
	deltas := BucketDeltas(oldA, newA)
	if len(deltas) != 1 || deltas[0].Bucket != "skill" || deltas[0].Delta != 500 {
		t.Errorf("a bucket that appears only in the new session must be kept with Δ +500, got: %+v", deltas)
	}
}

func TestItemDeltas_RanksByAbsoluteChangeAndHandlesAppearDisappear(t *testing.T) {
	oldA := cmpAlloc("old", map[parser.Bucket]parser.BucketBreakdown{
		parser.BucketSkill: {Tokens: 9000, Items: []parser.Item{
			{Name: "caveman", Tokens: 6000},  // grows to 6500 → Δ +500
			{Name: "frontend", Tokens: 3000}, // disappears → Δ -3000
		}},
		parser.BucketMCP: {Tokens: 0},
	})
	newA := cmpAlloc("new", map[parser.Bucket]parser.BucketBreakdown{
		parser.BucketSkill: {Tokens: 6500, Items: []parser.Item{
			{Name: "caveman", Tokens: 6500},
		}},
		parser.BucketMCP: {Tokens: 4000, Items: []parser.Item{
			{Name: "grafana", Tokens: 4000}, // appears → Δ +4000
		}},
	})

	items := ItemDeltas(oldA, newA, 10)
	if len(items) != 3 {
		t.Fatalf("expected 3 changed items, got %d: %+v", len(items), items)
	}
	// Largest absolute change first: grafana +4000, then frontend -3000, then caveman +500.
	if items[0].Name != "grafana" || items[0].Old != 0 || items[0].New != 4000 || items[0].Delta != 4000 {
		t.Errorf("top change should be grafana appearing (+4000): %+v", items[0])
	}
	if items[1].Name != "frontend" || items[1].New != 0 || items[1].Delta != -3000 {
		t.Errorf("second change should be frontend disappearing (-3000): %+v", items[1])
	}
	if items[2].Name != "caveman" || items[2].Delta != 500 {
		t.Errorf("third change should be caveman +500: %+v", items[2])
	}
}

func TestItemDeltas_OmitsFlatItemsAndRespectsN(t *testing.T) {
	oldA := cmpAlloc("old", map[parser.Bucket]parser.BucketBreakdown{
		parser.BucketFile: {Tokens: 3000, Items: []parser.Item{
			{Name: "a.go", Tokens: 1000}, // flat
			{Name: "b.go", Tokens: 2000}, // grows
		}},
	})
	newA := cmpAlloc("new", map[parser.Bucket]parser.BucketBreakdown{
		parser.BucketFile: {Tokens: 3500, Items: []parser.Item{
			{Name: "a.go", Tokens: 1000}, // flat → omitted
			{Name: "b.go", Tokens: 2500}, // Δ +500
		}},
	})
	items := ItemDeltas(oldA, newA, 10)
	if len(items) != 1 || items[0].Name != "b.go" || items[0].Delta != 500 {
		t.Errorf("flat item a.go must be omitted; only b.go (+500) remains, got: %+v", items)
	}
	// n=0 returns nil.
	if got := ItemDeltas(oldA, newA, 0); got != nil {
		t.Errorf("n=0 must return nil, got: %+v", got)
	}
}

func TestCompare_RendersBucketTableAndItemChanges(t *testing.T) {
	oldA := cmpAlloc("sessOld", map[parser.Bucket]parser.BucketBreakdown{
		parser.BucketSkill: {Tokens: 1000, Items: []parser.Item{{Name: "caveman", Tokens: 1000}}},
	})
	newA := cmpAlloc("sessNew", map[parser.Bucket]parser.BucketBreakdown{
		parser.BucketSkill: {Tokens: 4000, Items: []parser.Item{{Name: "caveman", Tokens: 4000}}},
	})
	var buf bytes.Buffer
	Compare(&buf, "sessOld", "sessNew",
		BucketDeltas(oldA, newA), ItemDeltas(oldA, newA, 10), TreeOptions{NoColor: true})
	out := buf.String()

	if !strings.Contains(out, "sessOld") || !strings.Contains(out, "sessNew") {
		t.Errorf("compare header missing session labels:\n%s", out)
	}
	if !strings.Contains(out, "+3,000") {
		t.Errorf("compare missing skill Δ +3,000:\n%s", out)
	}
	if !strings.Contains(out, "caveman") {
		t.Errorf("compare missing per-item change for caveman:\n%s", out)
	}
	// The read-only / no-auto-edit contract must be explicit, like cut-candidates.
	if !strings.Contains(out, "diagnosis only") {
		t.Errorf("compare must state it is diagnosis only:\n%s", out)
	}
}

func TestCompare_EmptyBucketsIsExplicit(t *testing.T) {
	var buf bytes.Buffer
	Compare(&buf, "a", "b", nil, nil, TreeOptions{NoColor: true})
	if !strings.Contains(buf.String(), "zero attributed tokens") {
		t.Errorf("empty compare should note zero attributed tokens, got: %q", buf.String())
	}
}
