# Docs screenshot: an interactive chat session, several turns deep
#
# Used by arena/how-to/interfaces/use-the-tui.md.
# Makes real provider calls — export the provider keys before running:
#   set -a; . .env; set +a
# Run from the repo root: vhs recordings/tapes/docs-tui-chat.tape
#
# Two things make this tape fussier than the others:
#
#  1. `chat` opens on a provider picker driven by number keys, so the provider
#     must be chosen before any message is typed. Typing prose at the picker is
#     not inert — any digit in the text is read as a selection, and the rest of
#     the line spills into the chat input.
#  2. While a reply is streaming the page blurs the input (chatpage.go, the
#     p.busy branch), and a blurred textinput silently discards keystrokes. Each
#     Sleep after Enter must outlast the whole response or the next message
#     arrives with its opening words missing.

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
Sleep 6s

# Pick provider 1 (claude-3-5-haiku) at the picker.
Type "1"
Sleep 4s

# Two exchanges, each given time to finish streaming before the next keystroke.
Type "Hi, can you check on my order?"
Sleep 1s
Enter
Sleep 25s
Type "The order number is ORD-98765"
Sleep 1s
Enter
Sleep 25s
Show
Sleep 2s
Screenshot "docs/public/screenshots/tui-chat.png"
