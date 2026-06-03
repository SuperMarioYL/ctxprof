# vhs script — record assets/demo.gif by running: `vhs assets/demo.tape`
# Targets a ~30s demo of `ctxprof` on a near-full Claude Code session,
# showing the flame tree first and then the structured --json output.
#
# Requires: https://github.com/charmbracelet/vhs (and `ctxprof` on PATH).

Output assets/demo.gif

Set Shell "bash"
Set FontSize 16
Set Width 1100
Set Height 720
Set Theme "Catppuccin Mocha"
Set Padding 24
Set TypingSpeed 40ms
Set PlaybackSpeed 1.0

# Intro: ask the question this tool answers
Type "# Where did your Claude Code tokens actually go?"
Sleep 800ms
Enter

# Show the flame tree on the last finished session
Type "ctxprof"
Sleep 400ms
Enter
Sleep 5s

# Now pipe the structured allocation to jq for buckets
Type "clear"
Enter
Sleep 300ms
Type "ctxprof --json | jq '{total: .total_tokens, buckets: (.buckets | map_values(.tokens))}'"
Sleep 400ms
Enter
Sleep 4s

# Point at a specific session via --session and emit JSON
Type "clear"
Enter
Sleep 300ms
Type "ctxprof --session ~/.claude/projects/demo/abc.jsonl --json | jq '.buckets.skill'"
Sleep 400ms
Enter
Sleep 4s

# Closing frame
Type "# six buckets, one binary, MIT — github.com/SuperMarioYL/ctxprof"
Sleep 1500ms
