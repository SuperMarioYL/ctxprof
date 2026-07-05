package render

import (
	"bytes"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/SuperMarioYL/ctxprof/internal/parser"
	runewidth "github.com/mattn/go-runewidth"
)

// The v0.1 truncate sliced s[:width-1] on a byte index, cutting CJK/emoji item
// names mid-rune into invalid UTF-8. These tests pin the display-width-aware
// replacement.

func TestTruncate_NeverCutsMidRune(t *testing.T) {
	cases := []string{
		"上下文窗口分析工具一二三四五六七八九十",                // pure CJK, wider than the column
		"项目/路径/very/long/混合/path/名称/segment", // mixed ASCII + CJK
		"😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀",                 // emoji run
	}
	for _, in := range cases {
		got := truncate(in, 28)
		if !utf8.ValidString(got) {
			t.Errorf("truncate(%q, 28) = %q is not valid UTF-8", in, got)
		}
		if w := runewidth.StringWidth(got); w > 28 {
			t.Errorf("truncate(%q, 28) display width = %d, want <= 28", in, w)
		}
		if !strings.HasSuffix(got, "…") {
			t.Errorf("truncate(%q, 28) = %q, expected trailing ellipsis on a truncation", in, got)
		}
	}
}

func TestTruncate_ShortStringUnchanged(t *testing.T) {
	for _, in := range []string{"caveman", "上下文", "a", ""} {
		if got := truncate(in, 28); got != in {
			t.Errorf("truncate(%q, 28) = %q, want unchanged", in, got)
		}
	}
}

func TestPadRight_DisplayWidth(t *testing.T) {
	// A CJK name is 3 runes but 6 display cells; padding must target cells, so
	// "上下文" (width 6) padded to 10 gets 4 trailing spaces, not 7.
	got := padRight("上下文", 10)
	if w := runewidth.StringWidth(got); w != 10 {
		t.Errorf("padRight display width = %d, want 10 (got %q)", w, got)
	}
	if n := strings.Count(got, " "); n != 4 {
		t.Errorf("padRight added %d spaces, want 4 (display-width aware)", n)
	}
	// Already-wide string is returned as-is.
	if got := padRight("0123456789AB", 10); got != "0123456789AB" {
		t.Errorf("padRight over-width = %q, want unchanged", got)
	}
}

func TestRightAlign_DisplayWidth(t *testing.T) {
	// "窗口" is width 4; right-aligned to 8 -> 4 leading spaces.
	got := rightAlign("窗口", 8)
	if w := runewidth.StringWidth(got); w != 8 {
		t.Errorf("rightAlign display width = %d, want 8 (got %q)", w, got)
	}
	if !strings.HasPrefix(got, "    ") {
		t.Errorf("rightAlign = %q, want 4 leading spaces", got)
	}
}

// End-to-end: a CJK skill name must render as valid UTF-8 with the column
// structure intact (no mid-rune corruption in the full tree output).
func TestTree_CJKItemNameRendersCleanly(t *testing.T) {
	alloc := parser.Allocation{
		SessionID:        "cjk",
		CumulativeTokens: 1000,
		WindowOccupancy:  900,
		WindowMax:        200_000,
		Estimated:        true,
		Buckets: map[parser.Bucket]parser.BucketBreakdown{
			parser.BucketSkill: {
				Tokens: 1000,
				Items: []parser.Item{
					{Name: "上下文窗口分析工具一二三四五六七八九十超长名称", Tokens: 1000},
				},
			},
		},
	}
	var buf bytes.Buffer
	if err := Tree(&buf, alloc, TreeOptions{NoColor: true}); err != nil {
		t.Fatalf("Tree: %v", err)
	}
	if !utf8.Valid(buf.Bytes()) {
		t.Fatal("Tree output contains invalid UTF-8 (mid-rune truncation regression)")
	}
	if !strings.Contains(buf.String(), "上下文窗口") {
		t.Error("Tree output dropped the CJK skill name prefix")
	}
}
