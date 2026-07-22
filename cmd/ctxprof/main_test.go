package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/SuperMarioYL/ctxprof/internal/parser"
)

// writeSession drops a minimal two-turn Claude Code JSONL file and returns its path.
func writeSession(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "session.jsonl")
	// One assistant turn with usage so attribution produces a non-empty allocation.
	lines := []string{
		`{"sessionId":"test-session","message":{"role":"user","content":"hello there"}}`,
		`{"sessionId":"test-session","message":{"role":"assistant","usage":{"input_tokens":100,"output_tokens":50,"cache_read_input_tokens":0,"cache_creation_input_tokens":900},"content":[{"type":"text","text":"hi back"}]}}`,
	}
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("write session: %v", err)
	}
	return p
}

// runCmd executes the root command with args and returns combined stdout.
func runCmd(t *testing.T, args ...string) (string, error) {
	t.Helper()
	// Reset globals between runs so flag state from a prior invocation never leaks.
	flagJSON, flagSession, flagNoColor, flagWindowMax, flagCutCandidates, flagTrendSince =
		false, "", false, 200_000, 0, ""
	flagCompareTopItems = 10
	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

// TestAttributeSubcommandAcceptsWindowMax is the regression test for the
// fix-attribute-subcmd-flag-gap milestone: before the fix the root registered
// --window-max as a LOCAL flag, so `attribute … --window-max N` errored with
// "unknown flag" and the subcommand silently used the 200000 default. After
// promoting it to a PersistentFlag the subcommand both accepts the flag AND uses
// it as the headline denominator.
func TestAttributeSubcommandAcceptsWindowMax(t *testing.T) {
	sess := writeSession(t)
	out, err := runCmd(t, "attribute", sess, "--window-max", "100000", "--no-color")
	if err != nil {
		t.Fatalf("attribute --window-max should be accepted, got error: %v\noutput:\n%s", err, out)
	}
	if strings.Contains(out, "unknown flag") {
		t.Fatalf("attribute rejected --window-max:\n%s", out)
	}
	// The denominator in the headline must be the passed window, not the default.
	if !strings.Contains(out, "100,000") {
		t.Errorf("headline should use --window-max 100,000 denominator:\n%s", out)
	}
	if strings.Contains(out, "200,000") {
		t.Errorf("headline still shows default 200,000 despite --window-max 100000:\n%s", out)
	}
}

// TestAttributeSubcommandAcceptsNoColor confirms the other previously-unavailable
// persistent flag is now honored on the subcommand.
func TestAttributeSubcommandAcceptsNoColor(t *testing.T) {
	sess := writeSession(t)
	out, err := runCmd(t, "attribute", sess, "--no-color")
	if err != nil {
		t.Fatalf("attribute --no-color should be accepted, got: %v\n%s", err, out)
	}
	if strings.Contains(out, "unknown flag") {
		t.Fatalf("attribute rejected --no-color:\n%s", out)
	}
}

// TestCutCandidatesFlagRenders confirms --cut-candidates surfaces the section on
// the root command for a session that has named items.
func TestCutCandidatesFlagRenders(t *testing.T) {
	sess := writeSession(t)
	out, err := runCmd(t, sess, "--no-color", "--cut-candidates", "3")
	if err != nil {
		t.Fatalf("--cut-candidates should be accepted, got: %v\n%s", err, out)
	}
	// The minimal fixture has no named items, so the cut section is correctly
	// silent; the run must still succeed and render the tree headline.
	if !strings.Contains(out, "test-session") {
		t.Errorf("expected tree headline for the session:\n%s", out)
	}
}

// TestAttributeSubcommandAcceptsCutCandidates confirms `attribute` still honors
// --cut-candidates after it moved from a root PersistentFlag to a local flag
// re-declared on the subcommand (mirroring --json). profile() reads
// flagCutCandidates on both the root and attribute paths.
func TestAttributeSubcommandAcceptsCutCandidates(t *testing.T) {
	sess := writeSession(t)
	out, err := runCmd(t, "attribute", sess, "--no-color", "--cut-candidates", "3")
	if err != nil {
		t.Fatalf("attribute --cut-candidates should be accepted, got: %v\n%s", err, out)
	}
	if strings.Contains(out, "unknown flag") {
		t.Fatalf("attribute rejected --cut-candidates:\n%s", out)
	}
}

// TestCutCandidatesRejectedOnTrendCompare is the regression test for
// fix-cut-candidates-silent-noop-on-subcommands: --cut-candidates used to be a
// root PersistentFlag, so `trend`/`compare` advertised it on --help yet never
// read it — accepting the flag and silently rendering nothing. Now it is local
// to root + attribute, so trend/compare must REJECT it with "unknown flag"
// instead of swallowing it.
func TestCutCandidatesRejectedOnTrendCompare(t *testing.T) {
	a := writeSession(t)
	b := writeSession(t)
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"trend", []string{"trend", a, b, "--cut-candidates", "5"}},
		{"compare", []string{"compare", a, b, "--cut-candidates", "5"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := runCmd(t, tc.args...)
			if err == nil {
				t.Fatalf("%s should REJECT --cut-candidates (was a silent no-op), but it was accepted:\n%s", tc.name, out)
			}
			if !strings.Contains(out, "unknown flag") && !strings.Contains(err.Error(), "unknown flag") {
				t.Fatalf("%s should error 'unknown flag' for --cut-candidates, got: %v\n%s", tc.name, err, out)
			}
		})
	}
}

// TestTrendNeedsTwoSessions confirms the trend command rejects a single session.
func TestTrendNeedsTwoSessions(t *testing.T) {
	sess := writeSession(t)
	_, err := runCmd(t, "trend", sess, "--no-color")
	if err == nil {
		t.Fatal("trend with a single session should error")
	}
	if !strings.Contains(err.Error(), "at least 2") {
		t.Errorf("expected 'at least 2 sessions' error, got: %v", err)
	}
}

// TestTrendTwoSessions runs trend over two real fixture files end-to-end.
func TestTrendTwoSessions(t *testing.T) {
	a := writeSession(t)
	b := writeSession(t)
	out, err := runCmd(t, "trend", a, b, "--no-color")
	if err != nil {
		t.Fatalf("trend over two sessions failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "bucket") || !strings.Contains(out, "Δ first→last") {
		t.Errorf("trend table missing header:\n%s", out)
	}
}

// touchMtime sets a file's modification time so ordering-by-mtime is testable.
func touchMtime(t *testing.T, path string, mod time.Time) {
	t.Helper()
	if err := os.Chtimes(path, mod, mod); err != nil {
		t.Fatalf("chtimes %s: %v", path, err)
	}
}

// TestSortPathsByMtime is the direct regression test for fix-trend-explicit-args-unordered:
// explicit trend path args (including a lexically-expanded shell glob) must be ordered
// oldest→newest by mtime, NOT taken in argv order. Passing paths newest-first must come
// back oldest-first.
func TestSortPathsByMtime(t *testing.T) {
	dir := t.TempDir()
	older := filepath.Join(dir, "z-older.jsonl") // lexically LAST, but oldest mtime
	newer := filepath.Join(dir, "a-newer.jsonl") // lexically FIRST, but newest mtime
	for _, p := range []string{older, newer} {
		if err := os.WriteFile(p, []byte("{}\n"), 0o644); err != nil {
			t.Fatalf("write %s: %v", p, err)
		}
	}
	base := time.Now().Add(-time.Hour)
	touchMtime(t, older, base)
	touchMtime(t, newer, base.Add(30*time.Minute))

	// Pass NEWEST first (the wrong order the bug shipped verbatim).
	got := sortPathsByMtime([]string{newer, older})
	if len(got) != 2 || got[0] != older || got[1] != newer {
		t.Fatalf("expected oldest→newest [%s %s], got %v", older, newer, got)
	}

	// A lexical glob order (a-newer before z-older) must ALSO come back mtime-ordered,
	// not lexical — this is the `ctxprof trend *.jsonl` case.
	glob := sortPathsByMtime([]string{newer, older}) // shell would hand these lexically
	if glob[0] != older {
		t.Errorf("lexical glob order must be re-sorted by mtime; got first=%s want %s", glob[0], older)
	}
}

// TestTrendOrdersExplicitArgsByMtime is the end-to-end guard: `ctxprof trend new old`
// (out of order) must render the Δ column with the sign the mtime order implies, not the
// argv order. The older session has a smaller skill bucket; passing newer-first must still
// produce a POSITIVE first→last skill delta because trend re-sorts to oldest→newest.
func TestTrendOrdersExplicitArgsByMtime(t *testing.T) {
	dir := t.TempDir()
	// Older session: small assistant turn (fewer tokens).
	oldP := filepath.Join(dir, "old.jsonl")
	if err := os.WriteFile(oldP, []byte(
		`{"sessionId":"old","message":{"role":"assistant","usage":{"input_tokens":10,"output_tokens":0,"cache_read_input_tokens":0,"cache_creation_input_tokens":0},"content":[{"type":"tool_use","name":"Skill","input":{"command":"caveman"}}]}}`+"\n"),
		0o644); err != nil {
		t.Fatal(err)
	}
	// Newer session: larger assistant turn (more tokens in the same skill).
	newP := filepath.Join(dir, "new.jsonl")
	if err := os.WriteFile(newP, []byte(
		`{"sessionId":"new","message":{"role":"assistant","usage":{"input_tokens":5000,"output_tokens":0,"cache_read_input_tokens":0,"cache_creation_input_tokens":0},"content":[{"type":"tool_use","name":"Skill","input":{"command":"caveman"}}]}}`+"\n"),
		0o644); err != nil {
		t.Fatal(err)
	}
	base := time.Now().Add(-time.Hour)
	touchMtime(t, oldP, base)
	touchMtime(t, newP, base.Add(30*time.Minute))

	// Pass NEWEST first — the exact input that used to render a backward axis.
	out, err := runCmd(t, "trend", newP, oldP, "--no-color")
	if err != nil {
		t.Fatalf("trend failed: %v\n%s", err, out)
	}
	// Oldest→newest ordering means the skill grew 10 → 5,000, so the Δ must be POSITIVE.
	if !strings.Contains(out, "+4,990") {
		t.Errorf("trend should order oldest→newest and show a positive skill Δ (+4,990), got:\n%s", out)
	}
	if strings.Contains(out, "-4,990") {
		t.Errorf("trend rendered a backward (negative) Δ despite newer-first args — ordering not applied:\n%s", out)
	}
}

// TestCompareTwoSessions runs the compare subcommand end-to-end over two fixtures and
// asserts the per-bucket table + read-only contract render.
func TestCompareTwoSessions(t *testing.T) {
	dir := t.TempDir()
	oldP := filepath.Join(dir, "old.jsonl")
	newP := filepath.Join(dir, "new.jsonl")
	if err := os.WriteFile(oldP, []byte(
		`{"sessionId":"old","message":{"role":"assistant","usage":{"input_tokens":1000,"output_tokens":0,"cache_read_input_tokens":0,"cache_creation_input_tokens":0},"content":[{"type":"tool_use","name":"Skill","input":{"command":"caveman"}}]}}`+"\n"),
		0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newP, []byte(
		`{"sessionId":"new","message":{"role":"assistant","usage":{"input_tokens":4000,"output_tokens":0,"cache_read_input_tokens":0,"cache_creation_input_tokens":0},"content":[{"type":"tool_use","name":"Skill","input":{"command":"caveman"}}]}}`+"\n"),
		0o644); err != nil {
		t.Fatal(err)
	}
	out, err := runCmd(t, "compare", oldP, newP, "--no-color")
	if err != nil {
		t.Fatalf("compare failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "compare") || !strings.Contains(out, "Δ = new − old") {
		t.Errorf("compare header missing:\n%s", out)
	}
	// skill grew 1000 → 4000, Δ +3,000.
	if !strings.Contains(out, "+3,000") {
		t.Errorf("compare missing skill Δ +3,000:\n%s", out)
	}
	if !strings.Contains(out, "diagnosis only") {
		t.Errorf("compare must state diagnosis only:\n%s", out)
	}
}

// TestCompareNeedsExactlyTwo confirms the compare subcommand rejects other arg counts.
func TestCompareNeedsExactlyTwo(t *testing.T) {
	sess := writeSession(t)
	if _, err := runCmd(t, "compare", sess, "--no-color"); err == nil {
		t.Error("compare with one session should error")
	}
}

// TestCompareJSON confirms --json emits both allocations plus bucket_deltas.
func TestCompareJSON(t *testing.T) {
	sess := writeSession(t)
	other := writeSession(t)
	out, err := runCmd(t, "compare", sess, other, "--json", "--no-color")
	if err != nil {
		t.Fatalf("compare --json failed: %v\n%s", err, out)
	}
	for _, want := range []string{`"schema_version": "compare/v1"`, `"bucket_deltas"`, `"old"`, `"new"`} {
			if !strings.Contains(out, want) {
				t.Errorf("compare --json missing %q:\n%s", want, out)
			}
		}
}

// TestTrendLabel_MultibyteSessionIDNoMidRune is the regression test for
// fix-trend-label-byte-slice-session-id: trendLabel (and compareLabel, which
// delegates to it) shortened the session id via `id = id[:8]`, a byte-index slice
// that cuts a multibyte (CJK/emoji) sessionId mid-rune and emits invalid UTF-8
// into the trend/compare column header — the same defect class the v0.2
// fix-truncate-byte-slice-multibyte removed for item names (tree_test.go pins
// utf8.ValidString against it). The fix truncates at 8 RUNES so the label never
// splits a multibyte rune. Claude Code sessionIds are ULIDs (ASCII) so this is
// latent today, but the schema is harness-agnostic: a non-ASCII sessionId (a
// future harness, or a hand-fed JSONL) would corrupt the rendered header. The
// tree headline (alloc.SessionID verbatim) and --json are unaffected.
//
// A direct trendLabel test is the reliable guard: render.Trend/Compare pass the
// label through the display-width-aware truncate() (internal/render/tree.go),
// which heals invalid bytes into U+FFFD — so an end-to-end utf8.Valid on the
// rendered output could PASS even with the byte-slice bug. Asserting rune
// count == 8 on the RAW label pins the fix at its source.
func TestTrendLabel_MultibyteSessionIDNoMidRune(t *testing.T) {
	cases := []struct {
		name string
		// session is the multibyte (or ASCII-control) sessionId fed in.
		session string
		// buggyValid reports whether the OLD byte slice `session[:8]` is itself
		// valid UTF-8. For 3-byte CJK runes (8 mod 3 != 0) it is NOT, so those
		// cases genuinely exercise the mid-rune cut. A 10-char ASCII id lands on a
		// rune boundary (8 mod 1 == 0) so the byte slice is valid — that case
		// confirms the fix is behavior-preserving for the common ULID path, not
		// that it alone catches the bug.
		buggyValid bool
	}{
		{"short CJK (3 runes, 9 bytes — bug corrupts even a 3-rune id)", "短会话", false},
		{"pure CJK (3-byte runes)", "上下文窗口分析工具一二三四五六七八九十", false},
		{"mixed CJK + ascii", "会话编号01上下文窗口分析工具测试数据", false},
		{"emoji + CJK mix", "😀上下文窗口分析工具测试😀数据一二三", false},
		{"ascii ULID-like (behavior preserved)", "abc123ulid", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			alloc := parser.Allocation{SessionID: tc.session}
			label := trendLabel("irrelevant.jsonl", alloc)

			// Core regression assertion (mirrors tree_test.go's utf8.ValidString):
			// the label must never split a multibyte rune.
			if !utf8.ValidString(label) {
				t.Fatalf("trendLabel(%q) = %q is not valid UTF-8 (mid-rune split)", tc.session, label)
			}

			// Truncation is at 8 RUNES, not 8 bytes: a >8-rune id yields exactly 8
			// runes; a <=8-rune id is returned whole.
			sessionRunes := []rune(tc.session)
			want := 8
			if len(sessionRunes) < 8 {
				want = len(sessionRunes)
			}
			if r := []rune(label); len(r) != want {
				t.Errorf("trendLabel(%q) = %q has %d runes, want %d (8 runes not 8 bytes)", tc.session, label, len(r), want)
			}

			// No data corruption beyond truncation: the label is the id's first
			// `want` runes, verbatim.
			wantLabel := string(sessionRunes[:want])
			if label != wantLabel {
				t.Errorf("trendLabel(%q) = %q, want first %d runes %q", tc.session, label, want, wantLabel)
			}

			// Precondition (revert-verified): the OLD byte slice must have been
			// invalid UTF-8 for the multibyte cases, so each such case genuinely
			// guards the bug. If it ever reads valid, the case no longer exercises a
			// mid-rune cut and must be replaced.
			buggy := tc.session
			if len(buggy) > 8 {
				buggy = buggy[:8]
			}
			if utf8.ValidString(buggy) != tc.buggyValid {
				t.Errorf("precondition for %q: byte-slice `[:8]` valid=%v, want %v (case no longer guards the bug)", tc.session, utf8.ValidString(buggy), tc.buggyValid)
			}

			// compareLabel delegates to trendLabel — it must stay valid and equal.
			if got := compareLabel("irrelevant.jsonl", alloc); got != label || !utf8.ValidString(got) {
				t.Errorf("compareLabel diverged from trendLabel or is invalid UTF-8: got %q", got)
			}
		})
	}
}
