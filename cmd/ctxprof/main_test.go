package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
