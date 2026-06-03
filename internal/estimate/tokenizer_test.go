package estimate_test

import (
	"testing"

	"github.com/SuperMarioYL/ctxprof/internal/estimate"
)

func TestTokens(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want int
	}{
		{"empty", "", 0},
		{"single char rounds up to 1", "a", 1},
		{"three chars still 1", "abc", 1},
		{"four chars → 1", "abcd", 1},
		{"eight chars → 2", "abcdefgh", 2},
		{"sixteen chars → 4", "abcdefghijklmnop", 4},
		{"counts runes not bytes (CJK)", "上下文窗口", 1}, // 5 runes → 5/4 = 1
		{"longer CJK", "上下文窗口分析工具", 2},          // 9 runes → 9/4 = 2
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := estimate.Tokens(c.in)
			if got != c.want {
				t.Errorf("Tokens(%q) = %d, want %d", c.in, got, c.want)
			}
		})
	}
}
