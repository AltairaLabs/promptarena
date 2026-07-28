# Docs screenshot: an interactive chat session, several turns deep
#
# ⚠️ NOT WORKING — do not use the output as-is.
#
# Only one user turn ever reaches the model: the first message is dropped and
# the surviving turn renders as a fragment ("… placed last Tuesday") rather
# than the full text. Tried and ruled out: longer settles before typing,
# per-key gaps, removing apostrophes and commas, and sending a leading Enter
# to clear a startup agent picker. The send path seems to swallow input until
# some readiness signal this tape does not wait for.
#
# The placeholder in use-the-tui.md is deliberately left in place until this
# produces a faithful transcript.
#
# Used by arena/how-to/interfaces/use-the-tui.md.
# Makes real provider calls — export the provider keys before running:
#   set -a; . .env; set +a
# Run from the repo root: vhs recordings/tapes/docs-tui-chat.tape

Output "recordings/gifs/docs-tui-chat.gif"

Set FontSize 16
Set Width 1200
Set Height 760
Set Theme "Dracula"
Set Padding 24

Hide
Type "cd examples/customer-support && clear"
Enter
Sleep 500ms
Type "../../bin/promptarena chat"
Enter
# Chat opens on an agent/provider picker. Confirm it before typing — otherwise
# the first Enter is swallowed selecting the agent and the first message never
# reaches the model.
Sleep 6s
Enter
Sleep 4s

# Two exchanges so the transcript shows a real back-and-forth. Sleeps are
# generous because each turn is a live API call. Apostrophes are avoided —
# they get eaten by the tape's shell quoting.
Type "Hi my order has not arrived yet"
Sleep 2s
Enter
Sleep 20s
Type "The order number is A4471 placed last Tuesday"
Sleep 2s
Enter
Sleep 20s
Show
Sleep 2s
Screenshot "docs/public/screenshots/tui-chat.png"
