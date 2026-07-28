# Docs screenshot: the results browser previewing a run and its transcript
#
# Used by arena/how-to/interfaces/use-the-tui.md.
# Needs results in examples/customer-support/out — generate them first with:
#   ../../bin/promptarena run --ci --mock-provider --mock-config mock-responses.yaml \
#     --scenario customer-support-scenarios --formats json
# Run from the repo root: vhs recordings/tapes/docs-tui-results.tape

Output "recordings/gifs/docs-tui-results.gif"

Set FontSize 16
Set Width 1200
Set Height 760
Set Theme "Dracula"
Set Padding 24

Hide
Type "cd examples/customer-support && clear"
Enter
Sleep 500ms
Type "../../bin/promptarena view out/"
Enter
Sleep 4s
# The listing opens on the artifacts/ directory, with media/ below it and the
# per-run JSON files after that. Two Downs reaches the first result; Enter opens
# it so the preview shows a transcript rather than a file listing.
Down
Sleep 800ms
Down
Sleep 800ms
Enter
Sleep 4s
Show
Sleep 2s
Screenshot "docs/public/screenshots/tui-results.png"
