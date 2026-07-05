// Package schema holds the published allocation_v1 JSON Schema for ctxprof's
// structured output. The schema is the interop contract: any harness that emits
// a document conforming to allocation_v1.json can be rendered by ctxprof, so the
// binary is a viewer over a shared spec rather than a Claude-Code-only tool.
//
// The schema file itself (allocation_v1.json) is embedded so callers and tests
// can load it without a filesystem path.
package schema

import _ "embed"

// AllocationV1 is the raw bytes of allocation_v1.json — the published schema for
// the document emitted by `ctxprof --json` (and `--json --cut-candidates`).
//
//go:embed allocation_v1.json
var AllocationV1 []byte
