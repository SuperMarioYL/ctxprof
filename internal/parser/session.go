package parser

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/SuperMarioYL/ctxprof/internal/estimate"
	"github.com/tidwall/gjson"
)

// excerptMax caps the RawExcerpt length stored per block so a 200k-token
// session does not pull megabytes of message text into memory just for
// debugging output.
const excerptMax = 120

// scannerBufferMax is the longest JSONL line the parser will accept. Claude
// Code lines occasionally cross a few MB when a tool_result carries a large
// file dump, so we lift the default 64 KiB scanner limit.
const scannerBufferMax = 64 << 20

// ParseFile opens path and parses its JSONL contents into a Session.
func ParseFile(path string) (*Session, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open session: %w", err)
	}
	defer f.Close()

	sess, err := ParseReader(f)
	if err != nil {
		return nil, err
	}
	sess.FilePath = path
	return sess, nil
}

// ParseReader streams a Claude Code JSONL stream from r and returns the
// parsed Session.
//
// One input line becomes at most one Turn. Lines without a message field
// (summary / meta records that Claude Code occasionally interleaves) are
// skipped but still scanned for the top-level sessionId. Lines whose
// message.role is neither "user" nor "assistant" are skipped silently.
//
// Per the §2 contract in mvp_plan.md, Usage is populated only for assistant
// turns and is read straight from message.usage.{input_tokens,
// output_tokens, cache_read_input_tokens, cache_creation_input_tokens}.
// User turns carry no usage in the source data so Turn.Usage stays nil.
func ParseReader(r io.Reader) (*Session, error) {
	sess := &Session{Turns: []Turn{}}

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 1<<16), scannerBufferMax)

	idx := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line[0] != '{' {
			continue
		}

		if sess.ID == "" {
			if sid := gjson.Get(line, "sessionId"); sid.Exists() {
				sess.ID = sid.String()
			}
		}

		msg := gjson.Get(line, "message")
		if !msg.Exists() {
			continue
		}

		var role Role
		switch msg.Get("role").String() {
		case "user":
			role = RoleUser
		case "assistant":
			role = RoleAssistant
		default:
			continue
		}

		turn := Turn{Idx: idx, Role: role}
		idx++

		if role == RoleAssistant {
			if u := msg.Get("usage"); u.Exists() {
				turn.Usage = &Usage{
					InputTokens:              int(u.Get("input_tokens").Int()),
					OutputTokens:             int(u.Get("output_tokens").Int()),
					CacheReadInputTokens:     int(u.Get("cache_read_input_tokens").Int()),
					CacheCreationInputTokens: int(u.Get("cache_creation_input_tokens").Int()),
				}
			}
		}

		turn.Blocks = blocksFromMessage(msg)
		sess.Turns = append(sess.Turns, turn)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan jsonl: %w", err)
	}
	return sess, nil
}

// blocksFromMessage flattens message.content into typed Blocks.
//
// content may be either an array of block objects (the usual shape for
// assistant turns and for user turns that carry tool results) or a bare
// string (the common shape for free-text user turns). Both forms are
// normalized into Blocks so downstream attribution can treat them uniformly.
func blocksFromMessage(msg gjson.Result) []Block {
	c := msg.Get("content")
	if !c.Exists() {
		return nil
	}

	if c.Type == gjson.String {
		text := c.String()
		return []Block{{
			Type:       BlockText,
			EstTokens:  estimate.Tokens(text),
			RawExcerpt: excerpt(text),
		}}
	}

	if !c.IsArray() {
		return nil
	}

	var out []Block
	c.ForEach(func(_, b gjson.Result) bool {
		if blk, ok := blockFromJSON(b); ok {
			out = append(out, blk)
		}
		return true
	})
	return out
}

// blockFromJSON converts one element of message.content into a Block.
// Returns ok=false for unrecognized block types so the parser keeps moving
// rather than half-populating a Turn.
func blockFromJSON(b gjson.Result) (Block, bool) {
	switch b.Get("type").String() {
	case "thinking":
		text := b.Get("thinking").String()
		return Block{
			Type:       BlockThinking,
			EstTokens:  estimate.Tokens(text),
			RawExcerpt: excerpt(text),
		}, true

	case "text":
		text := b.Get("text").String()
		return Block{
			Type:       BlockText,
			EstTokens:  estimate.Tokens(text),
			RawExcerpt: excerpt(text),
		}, true

	case "tool_use":
		name := b.Get("name").String()
		inputRaw := b.Get("input").Raw
		return Block{
			Type:       BlockToolUse,
			EstTokens:  estimate.Tokens(name) + estimate.Tokens(inputRaw),
			RawExcerpt: excerpt(name + " " + inputRaw),
			ToolName:   name,
			ToolInput:  resultToMap(b.Get("input")),
			ToolUseID:  b.Get("id").String(),
		}, true

	case "tool_result":
		text := toolResultText(b.Get("content"))
		return Block{
			Type:       BlockToolResult,
			EstTokens:  estimate.Tokens(text),
			RawExcerpt: excerpt(text),
			ToolUseID:  b.Get("tool_use_id").String(),
		}, true
	}
	return Block{}, false
}

// toolResultText flattens the content payload of a tool_result block down to
// a single string for sizing purposes. The payload may be a bare string or
// an array of {type:"text", text:"..."} entries.
func toolResultText(c gjson.Result) string {
	if !c.Exists() {
		return ""
	}
	if c.Type == gjson.String {
		return c.String()
	}
	if !c.IsArray() {
		return ""
	}
	var sb strings.Builder
	c.ForEach(func(_, sub gjson.Result) bool {
		if sub.Get("type").String() == "text" {
			sb.WriteString(sub.Get("text").String())
		}
		return true
	})
	return sb.String()
}

// resultToMap converts a gjson object into a plain map for the classifier
// (m2) to inspect. Returns nil if r is not an object.
func resultToMap(r gjson.Result) map[string]any {
	if !r.Exists() || !r.IsObject() {
		return nil
	}
	m := make(map[string]any)
	r.ForEach(func(k, v gjson.Result) bool {
		m[k.String()] = v.Value()
		return true
	})
	return m
}

// excerpt returns at most excerptMax runes of s, with a one-character
// ellipsis appended when truncated.
func excerpt(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= excerptMax {
		return s
	}
	runes := []rune(s)
	if len(runes) <= excerptMax {
		return s
	}
	return string(runes[:excerptMax]) + "…"
}
