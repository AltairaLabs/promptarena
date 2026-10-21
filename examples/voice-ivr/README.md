# Voice IVR Workflow Demo

A workflow-driven IVR for a fictional bank. The pack defines a three-state machine — `verifying` → `{resolution, handoff}` — and the scenarios assert every state transition. Each state's prompt offers the model only the tool that state uses: `lookup_account` in `verifying`, `check_balance` in `resolution`, `transfer_to_agent` in `handoff`.

## What it tests

| Scenario | What the agent must do |
|---|---|
| `balance-check` | Hear the balance request, verify identity (`lookup_account`), transition to resolution, run `check_balance` there, end the call. Never call `transfer_to_agent`. |
| `handoff-to-agent` | Hear a suspected-fraud report, verify identity, transition to handoff, run `transfer_to_agent` there. Never call `check_balance`. |

Each turn carries `state_is` or `transitioned_to` assertions; the final turn checks `workflow_complete`, `workflow_transition_order`, and the expected `tools_called` / `tools_not_called` pattern.

## Running

The default config runs deterministically against a text mock provider — `mock-responses.yaml` drives the LLM through scripted `workflow__transition` calls so the assertions pass without provider keys:

```bash
cd examples/voice-ivr
../../bin/promptarena run --ci --formats json,markdown
open out/results.md
```

For the live dev loop or to play back the report:

```bash
../../bin/promptarena serve   # web UI
../../bin/promptarena run              # in-terminal (TUI is the default)
```

## Switching to live voice

The scenarios are voice-agnostic — to drive the same workflow through a duplex voice provider:

1. Add a duplex provider (e.g., `providers/openai-realtime.provider.yaml`) and register it in `config.arena.yaml`.
2. Add a `duplex:` block to each scenario (see `examples/voice-refund-demo/scenarios/*.yaml` for the shape).
3. Set the scenario's user turns to `role: selfplay-user` with a persona instead of scripted text turns, or keep the scripted text and let the duplex stack TTS it.
4. Run with provider keys in your environment.

## File layout

```
voice-ivr/
├── README.md
├── config.arena.yaml          # pack + workflow definition
├── mock-responses.yaml        # scripted LLM turns including workflow__transition calls
├── prompts/
│   ├── verifying.yaml
│   ├── resolution.yaml
│   └── handoff.yaml
├── providers/
│   └── mock-provider.yaml
├── scenarios/
│   ├── balance-check.scenario.yaml
│   └── handoff-to-agent.scenario.yaml
└── tools/
    ├── lookup-account.tool.yaml
    ├── check-balance.tool.yaml
    └── transfer-to-agent.tool.yaml
```

## Extending

- **Add a self-service path**: define a new terminal state in `config.arena.yaml`, add a new `on_event:` mapping in `verifying`, add a prompt (with its own `allowed_tools`) and a scenario.
- **Check tool use per state**: the `workflow_tool_access` assertion can enforce that the agent only uses certain tools in each state.
