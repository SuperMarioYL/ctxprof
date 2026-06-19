# vhs tape — ctxprof happy path.
# Render in CI via .github/workflows/demo.yml, or locally:
#   vhs docs/demo.tape   (needs the `ctxprof` binary on PATH)
# Uses the committed examples/sample-session.jsonl so it renders deterministically.
Output assets/demo.gif

Set Shell "bash"
Set FontSize 16
Set Width 1100
Set Height 720
Set Theme "Catppuccin Mocha"
Set Padding 24
Set TypingSpeed 40ms
Set PlaybackSpeed 1.0

# Intro: the question this tool answers
Type "# Where did your Claude Code tokens actually go?"
Sleep 800ms
Enter

# 1) Flame tree of the six-bucket allocation
Type "ctxprof examples/sample-session.jsonl"
Sleep 400ms
Enter
Sleep 5s

# 2) Same allocation, structured — pipe allocation_v1.json to jq
Type "clear"
Enter
Sleep 300ms
Type "ctxprof examples/sample-session.jsonl --json | jq '{total: .total_tokens, buckets: (.buckets | map_values(.tokens))}'"
Sleep 400ms
Enter
Sleep 4s

# 3) Drill into a single bucket's items
Type "clear"
Enter
Sleep 300ms
Type "ctxprof examples/sample-session.jsonl --json | jq '.buckets.skill'"
Sleep 400ms
Enter
Sleep 4s

# Closing frame
Type "# six buckets, one binary, MIT — github.com/SuperMarioYL/ctxprof"
Sleep 1500ms
