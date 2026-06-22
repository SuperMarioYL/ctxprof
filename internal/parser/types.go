package parser

// Role is the speaker for a turn in a Claude Code session.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// BlockType is the kind of content block inside an assistant or user turn.
type BlockType string

const (
	BlockThinking   BlockType = "thinking"
	BlockText       BlockType = "text"
	BlockToolUse    BlockType = "tool_use"
	BlockToolResult BlockType = "tool_result"
)

// Bucket is one of the six attribution targets that ctxprof reports.
type Bucket string

const (
	BucketSystem    Bucket = "system"
	BucketSkill     Bucket = "skill"
	BucketMCP       Bucket = "mcp"
	BucketFile      Bucket = "file"
	BucketReasoning Bucket = "reasoning"
	BucketOutput    Bucket = "output"
)

// Usage mirrors message.usage from a Claude Code JSONL line. Present only on
// assistant turns; nil on user turns.
type Usage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
}

// Total is the sum of all token fields — the ground-truth per-turn count.
// It is cumulative throughput, not window occupancy: cache_read is the cached
// prefix re-counted every turn, so summing Total() across turns counts the same
// window many times (see WindowFootprint and Allocation.WindowOccupancy).
func (u Usage) Total() int {
	return u.InputTokens + u.OutputTokens + u.CacheReadInputTokens + u.CacheCreationInputTokens
}

// WindowFootprint is this turn's instantaneous context-window occupancy: the
// input the model actually saw on this turn (fresh input + cached-prefix
// re-read + newly-cached bytes). It excludes output_tokens, which are generated
// rather than occupying the input window. The peak WindowFootprint across a
// session is the high-water mark of the window — the honest basis for the
// "X / 200,000 (Y%)" headline, which must not be driven by a cross-turn sum of
// Total() (that double-counts the cached prefix N times).
func (u Usage) WindowFootprint() int {
	return u.InputTokens + u.CacheReadInputTokens + u.CacheCreationInputTokens
}

// Block is one flattened content block inside a turn. The JSONL has no
// per-block token field, so EstTokens is a local heuristic (chars/4 in v0.1)
// that gets reconciled against the turn's real Usage total during attribution.
type Block struct {
	Type       BlockType `json:"type"`
	EstTokens  int       `json:"est_tokens"`
	RawExcerpt string    `json:"raw_excerpt,omitempty"`

	// ToolName is set when Type == BlockToolUse. Used by the classifier to
	// route blocks into mcp / file / skill buckets.
	ToolName string `json:"tool_name,omitempty"`

	// ToolInput is the parsed input map for a tool_use block, when present.
	// The Skill-tool case stores the skill name in input.command, which the
	// classifier needs to surface individual skill rows.
	ToolInput map[string]any `json:"tool_input,omitempty"`
}

// Turn is one assistant or user turn in the session.
type Turn struct {
	Idx    int     `json:"idx"`
	Role   Role    `json:"role"`
	Usage  *Usage  `json:"usage,omitempty"`
	Blocks []Block `json:"blocks"`
}

// Session is the full parsed JSONL file.
type Session struct {
	ID       string `json:"id"`
	FilePath string `json:"file_path"`
	Turns    []Turn `json:"turns"`
}

// Item is a single named row inside a bucket (e.g. one skill, one MCP server).
type Item struct {
	Name      string `json:"name"`
	Tokens    int    `json:"tokens"`
	SourceRef string `json:"source_ref,omitempty"`
}

// BucketBreakdown is the reconciled estimate for one bucket.
type BucketBreakdown struct {
	Tokens int    `json:"tokens"`
	Items  []Item `json:"items,omitempty"`
}

// Allocation is the schema-published attribution map for a session.
// It corresponds to internal/schema/allocation_v1.json (added in m2).
//
// Honesty contract: CumulativeTokens, WindowOccupancy and WindowMax are exact
// (read from message.usage); every BucketBreakdown.Tokens is a calibrated
// estimate. Estimated is always true.
//
// Window-% semantics (fixed in v0.2): the headline "X / WindowMax (Y%)" is
// driven by WindowOccupancy — the peak single-turn WindowFootprint — NOT by
// CumulativeTokens. cache_read is the cached prefix re-counted every turn, so
// summing it across turns inflates the headline (the same window counted N
// times). CumulativeTokens remains available as genuine end-to-end throughput.
type Allocation struct {
	SessionID string `json:"session_id"`

	// CumulativeTokens is the sum of every assistant turn's message.usage
	// Total() — genuine throughput over the whole session. It legitimately
	// exceeds WindowMax on long sessions and must NOT drive the window-%.
	// (Renamed from total_tokens in v0.2; see CHANGELOG.)
	CumulativeTokens int `json:"cumulative_tokens"`

	// WindowOccupancy is the peak single-turn WindowFootprint — the high-water
	// mark of context-window usage and the basis for the headline window-%.
	WindowOccupancy int `json:"window_occupancy"`

	WindowMax int                        `json:"window_max"`
	Buckets   map[Bucket]BucketBreakdown `json:"buckets"`
	Estimated bool                       `json:"estimated"`
}
