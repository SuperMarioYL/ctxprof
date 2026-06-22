package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
	flagJSON      bool
	flagSession   string
	flagNoColor   bool
	flagWindowMax int
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

	cmd.Flags().BoolVar(&flagJSON, "json", false, "emit allocation_v1.json to stdout instead of the tree")
	cmd.Flags().StringVar(&flagSession, "session", "", "explicit path to a JSONL session file")
	cmd.Flags().BoolVar(&flagNoColor, "no-color", false, "disable ANSI colors in the tree output")
	cmd.Flags().IntVar(&flagWindowMax, "window-max", 200_000, "context window size for percentage math")

	cmd.AddCommand(newParseCmd())
	cmd.AddCommand(newAttributeCmd())
	cmd.AddCommand(newVersionCmd())
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
type allocationJSON struct {
	SchemaVersion string `json:"schema_version"`
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
	if flagJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(allocationJSON{SchemaVersion: "allocation/v1", Allocation: alloc})
	}
	return render.Tree(out, alloc, render.TreeOptions{NoColor: flagNoColor})
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

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print ctxprof version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintln(cmd.OutOrStdout(), "ctxprof "+version)
		},
	}
}
