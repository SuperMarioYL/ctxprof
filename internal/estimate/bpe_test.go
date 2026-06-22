package estimate

import "testing"

// The vendored model is a real BPE trained on a representative corpus. These
// fixtures are token counts for known strings under THAT model (not Anthropic's
// proprietary tokenizer, which is out of scope). They lock the tokenizer's
// behavior so a future vocab.json change is a deliberate, reviewed event.

func TestCountBPE_KnownStrings(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want int
	}{
		{"empty", "", 0},
		{"single ascii letter", "a", 1},
		{"common word merges to one", "the", 1},
		{"hello world is two", "hello world", 2},
		{"short identifier", "ctxprof", 1},
		{"snake_case identifier", "input_tokens", 3},
		{"cjk phrase merges tightly", "上下文窗口", 1},
		{"longer cjk phrase", "上下文窗口分析工具", 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := CountBPE(c.in)
			if err != nil {
				t.Fatalf("CountBPE(%q): %v", c.in, err)
			}
			if got != c.want {
				t.Errorf("CountBPE(%q) = %d, want %d", c.in, got, c.want)
			}
		})
	}
}

// A real tokenizer is sub-linear in characters for common text (many chars
// collapse into one token) but never produces more tokens than bytes.
func TestCountBPE_BoundedByBytes(t *testing.T) {
	for _, s := range []string{
		"the quick brown fox jumps over the lazy dog",
		"func main() { fmt.Println(\"hi\") }",
		"上下文窗口分析工具 token 估算",
		"system skill mcp file reasoning output",
	} {
		got, err := CountBPE(s)
		if err != nil {
			t.Fatalf("CountBPE(%q): %v", s, err)
		}
		if got < 1 {
			t.Errorf("CountBPE(%q) = %d, want >= 1", s, got)
		}
		if got > len(s) {
			t.Errorf("CountBPE(%q) = %d exceeds byte length %d", s, got, len(s))
		}
	}
}

// Determinism: the same input always yields the same count.
func TestCountBPE_Deterministic(t *testing.T) {
	const s = "Claude Code context window profiler with MCP descriptors"
	first, err := CountBPE(s)
	if err != nil {
		t.Fatalf("CountBPE: %v", err)
	}
	for i := 0; i < 5; i++ {
		got, _ := CountBPE(s)
		if got != first {
			t.Fatalf("non-deterministic: run %d = %d, first = %d", i, got, first)
		}
	}
}

// The vendored model must actually load and carry the expected name; a broken
// embed should fail loudly here rather than silently degrading to chars/4.
func TestModelLoads(t *testing.T) {
	if _, err := loadModel(); err != nil {
		t.Fatalf("loadModel: %v", err)
	}
	if name := modelName(); name != "ctxprof-bpe-v1" {
		t.Errorf("modelName() = %q, want ctxprof-bpe-v1", name)
	}
}
