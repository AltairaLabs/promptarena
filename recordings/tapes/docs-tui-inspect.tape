# Docs screenshot: the Inspect page (resolved config and live state)
#
# Used by arena/how-to/interfaces/use-the-tui.md.
# Run from the repo root: vhs recordings/tapes/docs-tui-inspect.tape

Output "recordings/gifs/docs-tui-inspect.gif"

Set FontSize 16
Set Width 1200
Set Height 760
Set Theme "Dracula"
Set Padding 24

Hide
Type "cd examples/customer-support && clear"
Enter
Sleep 500ms
Type "../../bin/promptarena"
Enter
Sleep 5s
# Hub menu order is View / Run / Chat / Inspect / Deploy — three Downs lands
# on Inspect.
Down
Down
Down
Enter
Sleep 3s
Show
Sleep 2s
Screenshot "docs/public/screenshots/tui-inspect.png"
