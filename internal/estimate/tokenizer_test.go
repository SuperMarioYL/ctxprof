package estimate

import "testing"

// As of v0.2 Tokens delegates to the vendored BPE tokenizer, so it no longer
// follows the chars/4 formula. These tests pin the contract that survives the
// swap: empty -> 0, non-empty -> >= 1, and agreement with CountBPE.

func TestTokens_EmptyIsZero(t *testing.T) {
	if got := Tokens(""); got != 0 {
		t.Errorf("Tokens(\"\") = %d, want 0", got)
	}
}

func TestTokens_NonEmptyAtLeastOne(t *testing.T) {
	for _, s := range []string{"a", "abc", "上", "ctxprof", "hello world"} {
		if got := Tokens(s); got < 1 {
			t.Errorf("Tokens(%q) = %d, want >= 1", s, got)
		}
	}
}

func TestTokens_MatchesBPE(t *testing.T) {
	for _, s := range []string{"hello world", "the quick brown fox", "input_tokens", "上下文窗口分析工具"} {
		want, err := CountBPE(s)
		if err != nil {
			t.Fatalf("CountBPE(%q): %v", s, err)
		}
		if want < 1 {
			want = 1
		}
		if got := Tokens(s); got != want {
			t.Errorf("Tokens(%q) = %d, want %d (== CountBPE)", s, got, want)
		}
	}
}

// charsOver4 is the degraded fallback; it must still behave like the v0.1
// rune-based heuristic.
func TestCharsOver4Fallback(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"a", 1},
		{"abc", 1},
		{"abcd", 1},
		{"abcdefgh", 2},
		{"abcdefghijklmnop", 4},
		{"上下文窗口", 1},
		{"上下文窗口分析工具", 2},
	}
	for _, c := range cases {
		if got := charsOver4(c.in); got != c.want {
			t.Errorf("charsOver4(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}
