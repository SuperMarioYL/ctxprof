package parser_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/SuperMarioYL/ctxprof/internal/parser"
)

const sampleSessionPath = "../../examples/sample-session.jsonl"

func TestParseFile_SampleSession(t *testing.T) {
	abs, err := filepath.Abs(sampleSessionPath)
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	sess, err := parser.ParseFile(abs)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	if sess.ID != "01J0Z5K3X4SAMPLEPROFILE" {
		t.Errorf("sess.ID = %q, want %q", sess.ID, "01J0Z5K3X4SAMPLEPROFILE")
	}
	if len(sess.Turns) != 4 {
		t.Fatalf("len(Turns) = %d, want 4", len(sess.Turns))
	}

	roles := []parser.Role{
		parser.RoleUser,
		parser.RoleAssistant,
		parser.RoleUser,
		parser.RoleAssistant,
	}
	for i, want := range roles {
		if got := sess.Turns[i].Role; got != want {
			t.Errorf("turn %d role = %q, want %q", i, got, want)
		}
	}

	// User turn 0 has no usage; assistant turn 1 must carry the real totals
	// from message.usage exactly as written in the fixture.
	if sess.Turns[0].Usage != nil {
		t.Errorf("user turn 0 should have nil usage, got %+v", sess.Turns[0].Usage)
	}
	a1 := sess.Turns[1].Usage
	if a1 == nil {
		t.Fatal("assistant turn 1 missing usage")
	}
	wantA1 := parser.Usage{InputTokens: 120, OutputTokens: 480, CacheReadInputTokens: 18000, CacheCreationInputTokens: 4200}
	if *a1 != wantA1 {
		t.Errorf("assistant turn 1 usage = %+v, want %+v", *a1, wantA1)
	}
	if a1.Total() != 120+480+18000+4200 {
		t.Errorf("Total() = %d, want %d", a1.Total(), 120+480+18000+4200)
	}

	a2 := sess.Turns[3].Usage
	if a2 == nil {
		t.Fatal("assistant turn 3 missing usage")
	}
	wantA2 := parser.Usage{InputTokens: 80, OutputTokens: 220, CacheReadInputTokens: 22000, CacheCreationInputTokens: 1100}
	if *a2 != wantA2 {
		t.Errorf("assistant turn 3 usage = %+v, want %+v", *a2, wantA2)
	}

	// Assistant turn 1 blocks: thinking + text + mcp tool_use.
	a1Blocks := sess.Turns[1].Blocks
	if len(a1Blocks) != 3 {
		t.Fatalf("assistant turn 1 blocks = %d, want 3", len(a1Blocks))
	}
	if a1Blocks[0].Type != parser.BlockThinking {
		t.Errorf("a1 block 0 type = %q, want thinking", a1Blocks[0].Type)
	}
	if a1Blocks[1].Type != parser.BlockText {
		t.Errorf("a1 block 1 type = %q, want text", a1Blocks[1].Type)
	}
	if a1Blocks[2].Type != parser.BlockToolUse {
		t.Errorf("a1 block 2 type = %q, want tool_use", a1Blocks[2].Type)
	}
	if a1Blocks[2].ToolName != "mcp__grafana__get_panel" {
		t.Errorf("a1 block 2 tool name = %q, want mcp__grafana__get_panel", a1Blocks[2].ToolName)
	}
	if a1Blocks[2].ToolInput["dashboard"] != "api-latency" {
		t.Errorf("a1 block 2 tool input.dashboard = %v, want api-latency", a1Blocks[2].ToolInput["dashboard"])
	}
	for i, b := range a1Blocks {
		if b.EstTokens <= 0 {
			t.Errorf("a1 block %d est_tokens = %d, want >0", i, b.EstTokens)
		}
	}

	// Assistant turn 3 blocks: text + Read tool_use + Skill tool_use.
	a2Blocks := sess.Turns[3].Blocks
	if len(a2Blocks) != 3 {
		t.Fatalf("assistant turn 3 blocks = %d, want 3", len(a2Blocks))
	}
	if a2Blocks[1].ToolName != "Read" || a2Blocks[1].ToolInput["file_path"] != "docs/incidents/2026-05.md" {
		t.Errorf("a2 Read block = %+v", a2Blocks[1])
	}
	if a2Blocks[2].ToolName != "Skill" || a2Blocks[2].ToolInput["command"] != "caveman" {
		t.Errorf("a2 Skill block = %+v", a2Blocks[2])
	}

	// User turn 0 (string content) collapses into one text block.
	u0Blocks := sess.Turns[0].Blocks
	if len(u0Blocks) != 1 || u0Blocks[0].Type != parser.BlockText {
		t.Fatalf("user turn 0 blocks = %+v, want one text block", u0Blocks)
	}
	if !strings.Contains(u0Blocks[0].RawExcerpt, "Grafana") {
		t.Errorf("user turn 0 excerpt missing keyword: %q", u0Blocks[0].RawExcerpt)
	}

	// User turn 2 (tool_result array) yields one tool_result block.
	u2Blocks := sess.Turns[2].Blocks
	if len(u2Blocks) != 1 || u2Blocks[0].Type != parser.BlockToolResult {
		t.Fatalf("user turn 2 blocks = %+v, want one tool_result block", u2Blocks)
	}
}

func TestParseReader_SkipsUnknownAndBlankLines(t *testing.T) {
	const jsonl = `

{"sessionId":"s1","summary":"meta record, no message"}
{"type":"user","message":{"role":"user","content":"hi"}}
{"type":"system","message":{"role":"system","content":"ignored"}}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"hello back"}],"usage":{"input_tokens":1,"output_tokens":2,"cache_read_input_tokens":3,"cache_creation_input_tokens":4}}}
`
	sess, err := parser.ParseReader(strings.NewReader(jsonl))
	if err != nil {
		t.Fatalf("ParseReader: %v", err)
	}
	if sess.ID != "s1" {
		t.Errorf("ID = %q, want s1", sess.ID)
	}
	if len(sess.Turns) != 2 {
		t.Fatalf("Turns = %d, want 2 (system row skipped)", len(sess.Turns))
	}
	if sess.Turns[0].Role != parser.RoleUser {
		t.Errorf("turn 0 role = %q, want user", sess.Turns[0].Role)
	}
	if sess.Turns[1].Usage == nil || sess.Turns[1].Usage.Total() != 10 {
		t.Errorf("turn 1 usage total = %v, want 10", sess.Turns[1].Usage)
	}
	if sess.Turns[0].Idx != 0 || sess.Turns[1].Idx != 1 {
		t.Errorf("turn indices = %d,%d, want 0,1", sess.Turns[0].Idx, sess.Turns[1].Idx)
	}
}
