// Package attribute classifies parsed content blocks into the six buckets and
// reconciles per-turn estimated weights against the real message.usage totals
// so bucket numbers are calibrated estimates, not exact reads.
//
// Bucket rules (deterministic, no model in the loop):
//
//	type:thinking                                  -> reasoning
//	type:text                                      -> output
//	type:tool_use, name == "Read"                  -> file
//	type:tool_use, name == "Skill"                 -> skill   (input.command = skill name)
//	type:tool_use, name has prefix "mcp__"         -> mcp     (mcp__<server>__<tool> -> server)
//	type:tool_use, anything else (Bash, Edit, ...) -> output  (the model's action surface)
//	type:tool_result                               -> file    (context-free fallback)
//
// The tool_result -> file rule is a context-free FALLBACK: ClassifyBlock looks
// only at the block itself, and a tool_result alone does not know which tool
// produced it. During reconciliation (reconcile.go) each tool_result is instead
// attributed to the bucket of its originating tool_use (matched by tool_use_id):
// an MCP call's response -> mcp, a Skill's -> skill, a Read's -> file, a Bash's
// -> output. file is used only when the origin is unknown. So a large MCP query
// response is counted as mcp window consumption, not misfiled as a file.
//
// The system bucket is not derivable from any per-block signal because the
// initial system prompt is never serialized into message.content — it lives
// only in the prefix the harness sends to the model. Reconcile approximates it
// from the first assistant turn's cache_creation_input_tokens (the initial
// cached context) and flags it as approximate via Allocation.Estimated.
package attribute

import (
	"strings"

	"github.com/SuperMarioYL/ctxprof/internal/parser"
)

// ClassifyBlock returns the bucket a block contributes to. It looks only at
// the block's own fields — no turn-level context — so it is safe to call on
// any Block in any order.
func ClassifyBlock(b parser.Block) parser.Bucket {
	switch b.Type {
	case parser.BlockThinking:
		return parser.BucketReasoning
	case parser.BlockText:
		return parser.BucketOutput
	case parser.BlockToolResult:
		return parser.BucketFile
	case parser.BlockToolUse:
		switch {
		case strings.HasPrefix(b.ToolName, "mcp__"):
			return parser.BucketMCP
		case b.ToolName == "Read":
			return parser.BucketFile
		case b.ToolName == "Skill":
			return parser.BucketSkill
		default:
			return parser.BucketOutput
		}
	}
	return parser.BucketOutput
}

// ItemName returns a human-readable row name for a block, scoped to the bucket
// it was classified into. Empty string means "do not surface as a named item"
// (the block's tokens still roll up into the bucket total).
//
// For skill blocks the name is input.command (caveman / code-review / ...).
// For mcp blocks the server segment of mcp__<server>__<tool> is the name.
// For file blocks the file_path (when present) is the name; tool_result blocks
// land in the file bucket without a name because the JSONL does not carry the
// path of the read they correspond to.
func ItemName(b parser.Block, bucket parser.Bucket) string {
	switch bucket {
	case parser.BucketSkill:
		if cmd := stringField(b.ToolInput, "command"); cmd != "" {
			return cmd
		}
	case parser.BucketMCP:
		if rest, ok := strings.CutPrefix(b.ToolName, "mcp__"); ok {
			if idx := strings.Index(rest, "__"); idx >= 0 {
				return rest[:idx]
			}
			return rest
		}
	case parser.BucketFile:
		if fp := stringField(b.ToolInput, "file_path"); fp != "" {
			return fp
		}
	}
	return ""
}

func stringField(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key].(string)
	if !ok {
		return ""
	}
	return v
}
