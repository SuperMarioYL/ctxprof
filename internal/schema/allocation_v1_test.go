package schema_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// schemaPath is the published allocation_v1 schema this package owns.
const schemaPath = "allocation_v1.json"

// loadSchema parses allocation_v1.json into a generic map so the tests can
// inspect its structure without a third-party JSON-schema dependency (the plan
// bans new deps; a full validator is not needed to pin the cut_candidates
// contract this test exists for).
func loadSchema(t *testing.T) map[string]any {
	t.Helper()
	abs, err := filepath.Abs(schemaPath)
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	raw, err := os.ReadFile(abs)
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("allocation_v1.json is not valid JSON: %v", err)
	}
	return doc
}

// TestSchema_IsWellFormed guards the basic shape: draft-2020-12, object type,
// additionalProperties:false at the top level, and the required-key list intact.
func TestSchema_IsWellFormed(t *testing.T) {
	doc := loadSchema(t)

	if doc["type"] != "object" {
		t.Errorf("top-level type = %v, want object", doc["type"])
	}
	if doc["additionalProperties"] != false {
		t.Errorf("top-level additionalProperties = %v, want false", doc["additionalProperties"])
	}
	req, _ := doc["required"].([]any)
	wantReq := map[string]bool{
		"schema_version": true, "session_id": true, "cumulative_tokens": true,
		"window_occupancy": true, "window_max": true, "buckets": true, "estimated": true,
	}
	got := map[string]bool{}
	for _, r := range req {
		got[r.(string)] = true
	}
	for k := range wantReq {
		if !got[k] {
			t.Errorf("required list missing %q", k)
		}
	}
	// cut_candidates must NOT be required (it is emitted only with the flag).
	if got["cut_candidates"] {
		t.Error("cut_candidates must be OPTIONAL (not in required), so plain --json still validates")
	}
}

// TestSchema_CutCandidatesDeclared is the regression guard for
// fix-cut-candidates-schema-not-updated: with additionalProperties:false, a
// top-level cut_candidates array must be a DECLARED property (with a matching
// $def) or `ctxprof --json --cut-candidates` emits a schema-invalid document.
func TestSchema_CutCandidatesDeclared(t *testing.T) {
	doc := loadSchema(t)

	props, ok := doc["properties"].(map[string]any)
	if !ok {
		t.Fatal("schema has no properties object")
	}
	cc, ok := props["cut_candidates"].(map[string]any)
	if !ok {
		t.Fatal("cut_candidates is not a declared top-level property — " +
			"`--json --cut-candidates` output fails additionalProperties:false (the bug)")
	}
	if cc["type"] != "array" {
		t.Errorf("cut_candidates.type = %v, want array", cc["type"])
	}
	items, ok := cc["items"].(map[string]any)
	if !ok || items["$ref"] != "#/$defs/cutCandidate" {
		t.Errorf("cut_candidates.items should $ref #/$defs/cutCandidate, got %v", cc["items"])
	}

	defs, ok := doc["$defs"].(map[string]any)
	if !ok {
		t.Fatal("schema has no $defs")
	}
	cd, ok := defs["cutCandidate"].(map[string]any)
	if !ok {
		t.Fatal("$defs.cutCandidate missing — the cut_candidates items $ref dangles")
	}
	// The $def must require exactly the four fields the Go CutCandidate marshals
	// (bucket, name, tokens, window_share) so a real emitted item validates.
	req, _ := cd["required"].([]any)
	gotReq := map[string]bool{}
	for _, r := range req {
		gotReq[r.(string)] = true
	}
	for _, f := range []string{"bucket", "name", "tokens", "window_share"} {
		if !gotReq[f] {
			t.Errorf("$defs.cutCandidate.required missing %q", f)
		}
	}
	if cd["additionalProperties"] != false {
		t.Error("$defs.cutCandidate should set additionalProperties:false")
	}
}

// TestSchema_EmittedDocumentsValidate structurally checks the two documents the
// CLI actually emits against the schema's additionalProperties:false top level:
//   - a plain --json document (no cut_candidates), and
//   - a --json --cut-candidates document (cut_candidates present).
//
// Both must have only declared top-level keys; the second is the case the bug
// broke. This is a stdlib-only structural check (declared-keys + required),
// not a full JSON-schema engine.
func TestSchema_EmittedDocumentsValidate(t *testing.T) {
	doc := loadSchema(t)
	props := doc["properties"].(map[string]any)
	declared := map[string]bool{}
	for k := range props {
		declared[k] = true
	}
	requiredKeys := []string{
		"schema_version", "session_id", "cumulative_tokens",
		"window_occupancy", "window_max", "buckets", "estimated",
	}

	base := map[string]any{
		"schema_version":    "allocation/v1",
		"session_id":        "sess-1",
		"cumulative_tokens": 1000,
		"window_occupancy":  800,
		"window_max":        200000,
		"estimated":         true,
		"buckets": map[string]any{
			"file": map[string]any{"tokens": 600, "items": []any{
				map[string]any{"name": "internal/huge/file.go", "tokens": 600},
			}},
			"output": map[string]any{"tokens": 400},
		},
	}

	// (1) plain --json — no cut_candidates.
	assertTopLevelValid(t, "plain --json", base, declared, requiredKeys)

	// (2) --json --cut-candidates — cut_candidates present. Pre-fix this key was
	// undeclared, so additionalProperties:false rejected the document.
	withCuts := map[string]any{}
	for k, v := range base {
		withCuts[k] = v
	}
	withCuts["cut_candidates"] = []any{
		map[string]any{"bucket": "file", "name": "internal/huge/file.go", "tokens": 600, "window_share": 0.75},
	}
	assertTopLevelValid(t, "--json --cut-candidates", withCuts, declared, requiredKeys)
}

func assertTopLevelValid(t *testing.T, label string, docObj map[string]any, declared map[string]bool, required []string) {
	t.Helper()
	for k := range docObj {
		if !declared[k] {
			t.Errorf("%s: top-level key %q is not declared in the schema (additionalProperties:false rejects it)", label, k)
		}
	}
	for _, r := range required {
		if _, ok := docObj[r]; !ok {
			t.Errorf("%s: required key %q missing from emitted document", label, r)
		}
	}
}
