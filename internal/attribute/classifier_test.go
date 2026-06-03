package attribute_test

import (
	"testing"

	"github.com/SuperMarioYL/ctxprof/internal/attribute"
	"github.com/SuperMarioYL/ctxprof/internal/parser"
)

func TestClassifyBlock(t *testing.T) {
	cases := []struct {
		name  string
		block parser.Block
		want  parser.Bucket
	}{
		{"thinking -> reasoning", parser.Block{Type: parser.BlockThinking}, parser.BucketReasoning},
		{"text -> output", parser.Block{Type: parser.BlockText}, parser.BucketOutput},
		{"tool_result -> file", parser.Block{Type: parser.BlockToolResult}, parser.BucketFile},
		{"mcp__grafana__get_panel -> mcp", parser.Block{Type: parser.BlockToolUse, ToolName: "mcp__grafana__get_panel"}, parser.BucketMCP},
		{"Read -> file", parser.Block{Type: parser.BlockToolUse, ToolName: "Read"}, parser.BucketFile},
		{"Skill -> skill", parser.Block{Type: parser.BlockToolUse, ToolName: "Skill"}, parser.BucketSkill},
		{"Bash falls through to output", parser.Block{Type: parser.BlockToolUse, ToolName: "Bash"}, parser.BucketOutput},
		{"Edit falls through to output", parser.Block{Type: parser.BlockToolUse, ToolName: "Edit"}, parser.BucketOutput},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := attribute.ClassifyBlock(c.block); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestItemName(t *testing.T) {
	cases := []struct {
		name   string
		block  parser.Block
		bucket parser.Bucket
		want   string
	}{
		{
			"skill name pulled from input.command",
			parser.Block{Type: parser.BlockToolUse, ToolName: "Skill", ToolInput: map[string]any{"command": "caveman"}},
			parser.BucketSkill,
			"caveman",
		},
		{
			"skill with no command has no item name",
			parser.Block{Type: parser.BlockToolUse, ToolName: "Skill", ToolInput: map[string]any{}},
			parser.BucketSkill,
			"",
		},
		{
			"mcp server segment is the name",
			parser.Block{Type: parser.BlockToolUse, ToolName: "mcp__grafana__get_panel"},
			parser.BucketMCP,
			"grafana",
		},
		{
			"mcp with no tool suffix still names the server",
			parser.Block{Type: parser.BlockToolUse, ToolName: "mcp__pencil"},
			parser.BucketMCP,
			"pencil",
		},
		{
			"file name pulled from input.file_path",
			parser.Block{Type: parser.BlockToolUse, ToolName: "Read", ToolInput: map[string]any{"file_path": "docs/incidents/2026-05.md"}},
			parser.BucketFile,
			"docs/incidents/2026-05.md",
		},
		{
			"tool_result has no item name",
			parser.Block{Type: parser.BlockToolResult},
			parser.BucketFile,
			"",
		},
		{
			"reasoning blocks are anonymous",
			parser.Block{Type: parser.BlockThinking},
			parser.BucketReasoning,
			"",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := attribute.ItemName(c.block, c.bucket); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}
