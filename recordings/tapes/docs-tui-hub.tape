# Docs screenshot: the TUI hub menu (View / Run / Chat / Inspect / Deploy)
#
# Used by arena/index.md (hero) and arena/how-to/interfaces/use-the-tui.md.
# Run from the repo root: vhs recordings/tapes/docs-tui-hub.tape

Output "recordings/gifs/docs-tui-hub.gif"

Set FontSize 16
Set Width 1200
Set Height 440
Set Theme "Dracula"
Set Padding 24

# Set-up is hidden: only the settled TUI is captured.
Hide
Type "cd examples/customer-support && clear"
Enter
Sleep 500ms
Type "../../bin/promptarena"
Enter
Sleep 5s
Show
Sleep 2s
Screenshot "docs/public/screenshots/tui-hub.png"
