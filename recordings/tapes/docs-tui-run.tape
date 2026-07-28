# Docs screenshot: a live run in progress — run feed, log panel, results
#
# Used by arena/how-to/interfaces/use-the-tui.md.
# Makes real provider calls so the run stays on screen long enough to capture;
# export the provider keys before running:
#   set -a; . .env; set +a
# Run from the repo root: vhs recordings/tapes/docs-tui-run.tape

Output "recordings/gifs/docs-tui-run.gif"

Set FontSize 16
Set Width 1200
Set Height 760
Set Theme "Dracula"
Set Padding 24

Hide
Type "cd examples/customer-support && clear"
Enter
Sleep 500ms
# No --ci, so the live TUI renders instead of plain logs.
Type "../../bin/promptarena run --scenario customer-support-scenarios"
Enter
Sleep 6s
Show
Sleep 2s
# Captured mid-flight: some trials finished, others still streaming.
Screenshot "docs/public/screenshots/tui-run.png"
