package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/SuperMarioYL/ctxprof/internal/attribute"
	"github.com/SuperMarioYL/ctxprof/internal/parser"
	"github.com/SuperMarioYL/ctxprof/internal/render"
	"github.com/spf13/cobra"
)

// version is the build version, injected at release time by GoReleaser via
// -ldflags "-X main.version=...". It defaults to a dev marker for local builds.
var version = "v0.1.0-dev"

var (
	flagJSON          bool
	flagSession       string
	flagNoColor       bool
	flagWindowMax     int
	flagCutCandidates int
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ctxprof [session.jsonl]",
		Short: "Profile where Claude Code tokens go inside a session",
		Long: `ctxprof reads a finished Claude Code JSONL session file and prints a
flame-graph-style allocation of its 200k-token context window, broken into
six buckets: system / skill / mcp / file / reasoning / output.

With no argument it auto-discovers the most recent session under
~/.claude/projects/. Pass a path positionally or via --session to target a
specific file. Use --json to emit allocation_v1.json instead of the tree.`,
		Args: cobra.MaximumNArgs(1),
		RunE: runRoot,
	}

	// --json is local to the root (and re-declared on `attribute`, which also emits
	// an allocation). --session / --no-color / --window-max are PERSISTENT so every
	// subcommand that renders an allocation (notably `attribute`, which calls
	// profile()) inherits them. Before this they were registered with cmd.Flags()
	// (local), so `ctxprof attribute s.jsonl --window-max 100000` errored with
	// "unknown flag" and --no-color was silently unavailable on the subcommand even
	// though profile() reads flagWindowMax/flagNoColor.
	cmd.Flags().BoolVar(&flagJSON, "json", false, "emit allocation_v1.json to stdout instead of the tree")
	cmd.PersistentFlags().StringVar(&flagSession, "session", "", "explicit path to a JSONL session file")
	cmd.PersistentFlags().BoolVar(&flagNoColor, "no-color", false, "disable ANSI colors in the tree output")
	cmd.PersistentFlags().IntVar(&flagWindowMax, "window-max", 200_000, "context window size for percentage math")
	cmd.PersistentFlags().IntVar(&flagCutCandidates, "cut-candidates", 0,
		"after the tree, list the N largest single consumers across all buckets (0 = off; a sensible default is 10)")

	cmd.AddCommand(newParseCmd())
	cmd.AddCommand(newAttributeCmd())
	cmd.AddCommand(newVersionCmd())
	cmd.AddCommand(newTrendCmd())
	return cmd
}

func runRoot(cmd *cobra.Command, args []string) error {
	path, err := resolveSessionPath(args)
	if err != nil {
		return err
	}
	return profile(cmd, path)
}

// allocationJSON wraps a parser.Allocation with the schema_version constant so
// the emitted document conforms to internal/schema/allocation_v1.json. The
// embedded Allocation fields are promoted to the top level on marshal.
//
// CutCandidates is an OPTIONAL, additive field: it is only populated (and only
// marshalled) when --cut-candidates N is passed. allocation_v1.json sets
// additionalProperties:false, so the field is omitempty — a plain `--json` run
// without --cut-candidates emits a document that still validates against v1.
type allocationJSON struct {
	SchemaVersion string                   `json:"schema_version"`
	CutCandidates []attribute.CutCandidate `json:"cut_candidates,omitempty"`
	parser.Allocation
}

// resolveWindowMax warns when --window-max is invalid so the user sees why the
// number changed. attribute.Attribute performs the actual clamp (the last gate
// before any Allocation is built), keeping the emitted allocation_v1.json
// schema-valid even if some other caller skips this warning.
func resolveWindowMax(cmd *cobra.Command, w int) int {
	if w > 0 {
		return w
	}
	fmt.Fprintf(cmd.ErrOrStderr(),
		"warning: --window-max %d is invalid (must be > 0); using %d\n", w, attribute.DefaultWindowMax)
	return attribute.DefaultWindowMax
}

// profile runs the full pipeline: parse the session, attribute + reconcile its
// blocks into buckets, then render either the flame tree or allocation_v1.json.
func profile(cmd *cobra.Command, path string) error {
	sess, err := parser.ParseFile(path)
	if err != nil {
		return err
	}
	windowMax := resolveWindowMax(cmd, flagWindowMax)
	alloc := attribute.Attribute(sess, windowMax)
	out := cmd.OutOrStdout()

	var cuts []attribute.CutCandidate
	if flagCutCandidates > 0 {
		cuts = attribute.TopCutCandidates(alloc, flagCutCandidates)
	}

	if flagJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(allocationJSON{
			SchemaVersion: "allocation/v1",
			CutCandidates: cuts,
			Allocation:    alloc,
		})
	}
	if err := render.Tree(out, alloc, render.TreeOptions{NoColor: flagNoColor}); err != nil {
		return err
	}
	if flagCutCandidates > 0 {
		render.CutCandidates(out, cuts, alloc, render.TreeOptions{NoColor: flagNoColor})
	}
	return nil
}

// resolveSessionPath picks the JSONL file to profile, in priority order:
//  1. positional arg
//  2. --session flag
//  3. most-recently-modified .jsonl under ~/.claude/projects/
func resolveSessionPath(args []string) (string, error) {
	if len(args) == 1 {
		return args[0], nil
	}
	if flagSession != "" {
		return flagSession, nil
	}
	return findLatestSession()
}

func findLatestSession() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate home dir: %w", err)
	}
	root := filepath.Join(home, ".claude", "projects")
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("no Claude Code session dir at %s (pass --session)", root)
	}

	type entry struct {
		path string
		mod  time.Time
	}
	var found []entry
	walkErr := filepath.WalkDir(root, func(p string, d os.DirEntry, werr error) error {
		if werr != nil {
			return nil
		}
		if d.IsDir() || filepath.Ext(p) != ".jsonl" {
			return nil
		}
		fi, ferr := d.Info()
		if ferr != nil {
			return nil
		}
		found = append(found, entry{p, fi.ModTime()})
		return nil
	})
	if walkErr != nil {
		return "", walkErr
	}
	if len(found) == 0 {
		return "", fmt.Errorf("no .jsonl files found under %s (pass --session)", root)
	}
	sort.Slice(found, func(i, j int) bool { return found[i].mod.After(found[j].mod) })
	return found[0].path, nil
}

func newParseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "parse <file.jsonl>",
		Short: "Emit one JSON record per turn (debug helper, m1 surface)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sess, err := parser.ParseFile(args[0])
			if err != nil {
				return err
			}
			return render.PerTurnJSON(cmd.OutOrStdout(), sess)
		},
	}
}

func newAttributeCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "attribute <file.jsonl>",
		Short: "Classify and reconcile blocks into the six buckets (m2 surface)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return profile(cmd, args[0])
		},
	}
	c.Flags().BoolVar(&flagJSON, "json", false, "emit allocation_v1.json instead of the tree")
	return c
}

// flagTrendSince, when set, selects sessions modified within the given duration
// under ~/.claude/projects/ for the trend command (e.g. "7d", "48h").
var flagTrendSince string

func newTrendCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "trend [session1.jsonl session2.jsonl ...]",
		Short: "Show per-bucket budget drift across multiple sessions (m5 surface)",
		Long: `trend profiles several sessions and prints how each bucket's window
occupancy and share moves across them, so you can see whether system / mcp / file
budget is creeping up over time. Sessions are ordered oldest→newest by file mtime.

Pass explicit paths, or use --since to pick recent sessions under ~/.claude/projects/
(e.g. --since 7d). Read-only and terminal-only — no graphs, no TUI. With --json it
emits an ordered array of allocation_v1 objects.`,
		Args: cobra.ArbitraryArgs,
		RunE: runTrend,
	}
	c.Flags().BoolVar(&flagJSON, "json", false, "emit an ordered JSON array of allocation_v1 objects instead of the trend table")
	c.Flags().StringVar(&flagTrendSince, "since", "", "select sessions modified within this duration under ~/.claude/projects/ (e.g. 7d, 48h)")
	return c
}

func runTrend(cmd *cobra.Command, args []string) error {
	paths, err := resolveTrendPaths(args)
	if err != nil {
		return err
	}
	if len(paths) < 2 {
		return fmt.Errorf("trend needs at least 2 sessions; got %d (pass more paths or widen --since)", len(paths))
	}

	windowMax := resolveWindowMax(cmd, flagWindowMax)
	points := make([]render.TrendPoint, 0, len(paths))
	jsonRows := make([]allocationJSON, 0, len(paths))
	for _, p := range paths {
		sess, perr := parser.ParseFile(p)
		if perr != nil {
			return fmt.Errorf("parse %s: %w", p, perr)
		}
		alloc := attribute.Attribute(sess, windowMax)
		points = append(points, render.TrendPoint{Label: trendLabel(p, alloc), Alloc: alloc})
		jsonRows = append(jsonRows, allocationJSON{SchemaVersion: "allocation/v1", Allocation: alloc})
	}

	out := cmd.OutOrStdout()
	if flagJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(jsonRows)
	}
	return render.Trend(out, points, render.TreeOptions{NoColor: flagNoColor})
}

// trendLabel picks a short, stable column label for a session: the session id if
// present, else the file's base name without extension.
func trendLabel(path string, alloc parser.Allocation) string {
	if alloc.SessionID != "" {
		id := alloc.SessionID
		if len(id) > 8 {
			id = id[:8]
		}
		return id
	}
	base := filepath.Base(path)
	return base[:len(base)-len(filepath.Ext(base))]
}

// resolveTrendPaths returns the ordered (oldest→newest) session paths for trend:
// explicit args verbatim if given, else the sessions under ~/.claude/projects/
// modified within --since.
func resolveTrendPaths(args []string) ([]string, error) {
	if len(args) > 0 {
		return args, nil
	}
	if flagTrendSince == "" {
		return nil, fmt.Errorf("trend needs session paths or --since <duration> (e.g. --since 7d)")
	}
	dur, err := parseSinceDuration(flagTrendSince)
	if err != nil {
		return nil, err
	}
	return sessionsSince(dur)
}

// parseSinceDuration accepts Go durations plus a "<n>d" day shorthand.
func parseSinceDuration(s string) (time.Duration, error) {
	if strings.HasSuffix(s, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err != nil {
			return 0, fmt.Errorf("invalid --since %q: %w", s, err)
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid --since %q (try 7d, 48h, 30m): %w", s, err)
	}
	return d, nil
}

// sessionsSince returns .jsonl sessions under ~/.claude/projects/ modified within
// dur, ordered oldest→newest so the trend reads left-to-right as time advances.
func sessionsSince(dur time.Duration) ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("locate home dir: %w", err)
	}
	root := filepath.Join(home, ".claude", "projects")
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("no Claude Code session dir at %s (pass session paths)", root)
	}
	cutoff := time.Now().Add(-dur)
	type entry struct {
		path string
		mod  time.Time
	}
	var found []entry
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, werr error) error {
		if werr != nil || d.IsDir() || filepath.Ext(p) != ".jsonl" {
			return nil
		}
		fi, ferr := d.Info()
		if ferr != nil || fi.ModTime().Before(cutoff) {
			return nil
		}
		found = append(found, entry{p, fi.ModTime()})
		return nil
	})
	sort.Slice(found, func(i, j int) bool { return found[i].mod.Before(found[j].mod) })
	out := make([]string, len(found))
	for i, e := range found {
		out[i] = e.path
	}
	return out, nil
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print ctxprof version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintln(cmd.OutOrStdout(), "ctxprof "+version)
		},
	}
}
