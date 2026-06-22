package estimate

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
)

// vocabData is the vendored, self-contained BPE model. It carries a GPT-2 /
// cl100k-style reversible byte->unicode mapping plus a real merge table trained
// on a representative English/code/CJK corpus. It is embedded at build time so
// the tokenizer makes no network calls at runtime.
//
// This is deliberately NOT Anthropic's proprietary tokenizer (that is not
// public and is out of scope per the plan). It is a real, deterministic BPE —
// accurate enough to weight content blocks before the per-turn reconciliation
// step, and tested against its own known-string fixtures in bpe_test.go.
//
//go:embed vocab.json
var vocabData []byte

// bpeModel holds the decoded merge table and pretokenization regexp.
type bpeModel struct {
	// pattern splits raw text into pretokens before BPE runs, the way
	// GPT-2/cl100k tokenizers do. RE2-compatible (no lookaround).
	pattern *regexp.Regexp
	// ranks maps a merge pair "a b" (in byte-unicode space) to its priority;
	// lower rank merges first, exactly as in the trained merge order.
	ranks map[string]int
}

// vocabFile is the on-disk shape of vocab.json.
type vocabFile struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Pattern     string         `json:"pattern"`
	Merges      []string       `json:"merges"`
	Vocab       map[string]int `json:"vocab"`
}

var (
	model     *bpeModel
	modelErr  error
	modelOnce sync.Once
)

// byteToUnicode is the standard GPT-2 reversible byte->rune mapping. Bytes that
// are printable map to themselves; the rest map to a private high range so the
// pretokenization regexp never sees a control byte. The merge table in
// vocab.json was trained in this same space, so encoding must use it too.
var byteToUnicode = buildByteToUnicode()

func buildByteToUnicode() [256]rune {
	var table [256]rune
	var bs []int
	for b := '!'; b <= '~'; b++ {
		bs = append(bs, int(b))
	}
	for b := 0xA1; b <= 0xAC; b++ {
		bs = append(bs, b)
	}
	for b := 0xAE; b <= 0xFF; b++ {
		bs = append(bs, b)
	}
	inBs := map[int]bool{}
	for _, b := range bs {
		inBs[b] = true
	}
	cs := append([]int(nil), bs...)
	n := 0
	for b := 0; b < 256; b++ {
		if !inBs[b] {
			bs = append(bs, b)
			cs = append(cs, 256+n)
			n++
		}
	}
	for i, b := range bs {
		table[b] = rune(cs[i])
	}
	return table
}

// loadModel decodes the embedded vocab once. A malformed embed is a build-time
// programming error, so callers surface it rather than papering over it.
func loadModel() (*bpeModel, error) {
	modelOnce.Do(func() {
		var vf vocabFile
		if err := json.Unmarshal(vocabData, &vf); err != nil {
			modelErr = fmt.Errorf("decode vocab.json: %w", err)
			return
		}
		re, err := regexp.Compile(vf.Pattern)
		if err != nil {
			modelErr = fmt.Errorf("compile pretokenizer pattern: %w", err)
			return
		}
		ranks := make(map[string]int, len(vf.Merges))
		for i, m := range vf.Merges {
			ranks[m] = i
		}
		model = &bpeModel{pattern: re, ranks: ranks}
	})
	return model, modelErr
}

// CountBPE returns the BPE token count for s using the vendored model. It is
// the accurate replacement for the chars/4 heuristic: pretokenize, map each
// pretoken's bytes into byte-unicode space, then greedily apply the lowest-rank
// merge until none remain.
//
// An empty string maps to 0. The count is deterministic for a given vocab.json.
func CountBPE(s string) (int, error) {
	if s == "" {
		return 0, nil
	}
	m, err := loadModel()
	if err != nil {
		return 0, err
	}
	total := 0
	for _, piece := range m.pattern.FindAllString(s, -1) {
		if piece == "" {
			continue
		}
		total += len(m.encodePiece(piece))
	}
	return total, nil
}

// encodePiece BPE-encodes a single pretoken and returns the resulting symbols.
func (m *bpeModel) encodePiece(piece string) []string {
	// Map each raw byte of the pretoken to its byte-unicode rune; each becomes
	// a one-rune starting symbol, matching how the merges were trained.
	raw := []byte(piece)
	symbols := make([]string, 0, len(raw))
	for _, b := range raw {
		symbols = append(symbols, string(byteToUnicode[b]))
	}
	if len(symbols) < 2 {
		return symbols
	}

	for len(symbols) > 1 {
		// Find the adjacent pair with the lowest merge rank.
		bestRank := int(^uint(0) >> 1)
		bestIdx := -1
		for i := 0; i+1 < len(symbols); i++ {
			if r, ok := m.ranks[symbols[i]+" "+symbols[i+1]]; ok && r < bestRank {
				bestRank = r
				bestIdx = i
			}
		}
		if bestIdx < 0 {
			break // no remaining mergeable pair
		}
		merged := symbols[bestIdx] + symbols[bestIdx+1]
		next := make([]string, 0, len(symbols)-1)
		next = append(next, symbols[:bestIdx]...)
		next = append(next, merged)
		next = append(next, symbols[bestIdx+2:]...)
		symbols = next
	}
	return symbols
}

// modelName exposes the vendored model name for diagnostics/tests.
func modelName() string {
	var vf vocabFile
	_ = json.Unmarshal(vocabData, &vf)
	return strings.TrimSpace(vf.Name)
}
