# Docs screenshot: the deploy plan page showing a diff against live resources
#
# Used by arena/how-to/interfaces/use-the-tui.md.
# Run from the repo root: vhs recordings/tapes/docs-tui-deploy-plan.tape
#
# Needs a reachable Omnia workspace API and a pack already applied to it, or
# the plan shows creates rather than updates. To reproduce the drift this
# screenshot depends on:
#
#   kubectl port-forward -n omnia-system svc/omnia-dashboard 13000:3000 &
#   export OMNIA_API_TOKEN=local-dev
#   cd examples/assertions-test
#   ../../bin/promptarena deploy apply --config config.arena.yaml
#   kubectl patch agentruntime assertions-test -n omnia-demo --type=merge \
#     -p '{"spec":{"serviceGroup":"drifted-by-hand"}}'
#
# examples/assertions-test/config.arena.yaml must point deploy.config at the
# forwarded endpoint (see the deploy: block).

Output "recordings/gifs/docs-tui-deploy-plan.gif"

Set FontSize 16
Set Width 1200
Set Height 700
Set Theme "Dracula"
Set Padding 24

Hide
Type "cd examples/assertions-test && clear"
Enter
Sleep 500ms
Type "../../bin/promptarena"
Enter
Sleep 5s

# Hub menu: View / Run / Chat / Inspect / Deploy — four Downs reaches Deploy.
Down
Sleep 400ms
Down
Sleep 400ms
Down
Sleep 400ms
Down
Sleep 400ms
Enter
# Preflight runs adapter + auth checks before the plan is offered.
Sleep 6s

# 'p' runs the plan against the live workspace.
Type "p"
Sleep 12s
Show
Sleep 2s
Screenshot "docs/public/screenshots/tui-deploy-plan.png"
