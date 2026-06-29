package render

import (
	"bytes"
	"strings"
	"testing"

	"github.com/SuperMarioYL/ctxprof/internal/attribute"
	"github.com/SuperMarioYL/ctxprof/internal/parser"
)

func trendAlloc(id string, occ int, skill, file int) parser.Allocation {
	return parser.Allocation{
		SessionID:       id,
		WindowOccupancy: occ,
		WindowMax:       200_000,
		Buckets: map[parser.Bucket]parser.BucketBreakdown{
			parser.BucketSkill: {Tokens: skill},
			parser.BucketFile:  {Tokens: file},
		},
	}
}

func TestTrend_ShowsPerBucketDriftAndDelta(t *testing.T) {
	var buf bytes.Buffer
	points := []TrendPoint{
		{Label: "sessA", Alloc: trendAlloc("sessA", 10_000, 1000, 2000)},
		{Label: "sessB", Alloc: trendAlloc("sessB", 14_000, 4000, 2000)},
	}
	if err := Trend(&buf, points, TreeOptions{NoColor: true}); err != nil {
		t.Fatalf("Trend error: %v", err)
	}
	out := buf.String()

	// Both session labels appear as columns.
	if !strings.Contains(out, "sessA") || !strings.Contains(out, "sessB") {
		t.Errorf("trend missing session labels:\n%s", out)
	}
	// The skill bucket grew 1000 → 4000, Δ +3,000 must be shown.
	if !strings.Contains(out, "+3,000") {
		t.Errorf("trend missing skill delta +3,000:\n%s", out)
	}
	// The file bucket is flat (2000 → 2000); a bucket present in both sessions
	// must still render its row.
	if !strings.Contains(out, "file") {
		t.Errorf("trend missing file row:\n%s", out)
	}
	// Window occupancy grew 10k → 14k.
	if !strings.Contains(out, "+4,000") {
		t.Errorf("trend missing window-occupancy delta +4,000:\n%s", out)
	}
}

func TestTrend_OmitsAllZeroBucket(t *testing.T) {
	var buf bytes.Buffer
	points := []TrendPoint{
		{Label: "s1", Alloc: trendAlloc("s1", 5000, 100, 0)},
		{Label: "s2", Alloc: trendAlloc("s2", 6000, 200, 0)},
	}
	if err := Trend(&buf, points, TreeOptions{NoColor: true}); err != nil {
		t.Fatalf("Trend error: %v", err)
	}
	// file is zero in every session → its row must be omitted to keep the table tight.
	for _, line := range strings.Split(buf.String(), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "file") {
			t.Errorf("all-zero file bucket should be omitted, found row: %q", line)
		}
	}
}

func TestTrend_Empty(t *testing.T) {
	var buf bytes.Buffer
	if err := Trend(&buf, nil, TreeOptions{NoColor: true}); err != nil {
		t.Fatalf("Trend error on empty: %v", err)
	}
	if !strings.Contains(buf.String(), "no sessions") {
		t.Errorf("empty trend should note no sessions, got: %q", buf.String())
	}
}

func TestCutCandidates_RendersRankedItems(t *testing.T) {
	var buf bytes.Buffer
	alloc := parser.Allocation{WindowOccupancy: 20_000, WindowMax: 200_000}
	cuts := []attribute.CutCandidate{
		{Bucket: "skill", Name: "caveman", Tokens: 6000, WindowShare: 0.30},
		{Bucket: "mcp", Name: "grafana", Tokens: 5000, WindowShare: 0.25},
	}
	CutCandidates(&buf, cuts, alloc, TreeOptions{NoColor: true})
	out := buf.String()

	if !strings.Contains(out, "cut-candidates") {
		t.Errorf("missing cut-candidates header:\n%s", out)
	}
	if !strings.Contains(out, "caveman") || !strings.Contains(out, "6,000") {
		t.Errorf("missing top candidate row:\n%s", out)
	}
	if !strings.Contains(out, "30.0% of window") {
		t.Errorf("missing window share for caveman:\n%s", out)
	}
	// The header must make the read-only / no-auto-edit contract explicit.
	if !strings.Contains(out, "diagnosis only") {
		t.Errorf("cut-candidates must state it is diagnosis only:\n%s", out)
	}
}

func TestCutCandidates_EmptyIsSilent(t *testing.T) {
	var buf bytes.Buffer
	CutCandidates(&buf, nil, parser.Allocation{}, TreeOptions{NoColor: true})
	if buf.Len() != 0 {
		t.Errorf("empty cut list must print nothing, got: %q", buf.String())
	}
}
