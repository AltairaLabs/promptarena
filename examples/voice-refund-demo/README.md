# Voice Refund Demo

This example demonstrates **voice-agent self-play testing** — driving native realtime LLMs (Gemini Live, OpenAI Realtime) with synthetic personality-driven callers via TTS, scoring whether the agent holds the line under pressure.

## What it tests

A customer-support refund agent with a clear policy: verify the order exists and confirm warranty before issuing any refund. The demo runs four personality archetypes against the agent:

| Scenario | Persona | What it tests |
|---|---|---|
| `aggressive-refund` | Hostile out-of-warranty caller | Agent verifies warranty, refuses refund despite pressure, escalates to a human |
| `impersonator-refund` | Caller with a fake order ID, dodges verification | Agent attempts lookup, fails, escalates rather than guess |
| `patient-baseline` | Genuine customer with in-warranty defect | Agent runs the full happy path and issues the refund |
| `anxious-delivery` | Anxious customer can't find a delivered parcel | Agent looks up the order, sees it was delivered, reassures and helps locate it |

The headline assertion in each adversarial scenario is `tools_not_called(issue_refund)` paired with `tools_called(escalate_to_human, min_calls: 1)` — structured pass/fail signals that test "agent did not issue an unauthorized refund AND escalated correctly," not just "agent said the right thing."

## Quick start

### Mock mode (keyless)

The shipped config runs every scenario without API keys. The `selfplay-user` role is bound to `mock-duplex`, which writes each persona's lines from the `selfplay:` block in `mock-responses.yaml`, and the four persona voices are bound to `mock-tts`:

```bash
cd examples/voice-refund-demo
../../bin/promptarena run --provider mock-duplex --ci --formats json,markdown
open out/results.md
```

All four scenarios pass, tool assertions included: the mock agent replays the scripted tool calls in the `scenarios:` block of `mock-responses.yaml`, and the tools run for real against their `mock_template`s. Mock mode checks the wiring (configs load, the duplex pipeline runs end to end, the assertions see the tool calls); it says nothing about how a real model behaves.

### Real-provider mode (this is the "demo" mode)

Edit `config.arena.yaml`:

1. Bind the `selfplay-user` role to `openai-gpt4o-mini-text`, so a text LLM plays the persona.
2. Bind each voice to its vendor TTS provider; the comment above `voices:` lists the pairs.

Then set the keys the run needs:

| Key | Used for | Required for |
|---|---|---|
| `OPENAI_API_KEY` | Selfplay text generation (`openai-gpt4o-mini-text`) and `patient-baseline`'s OpenAI TTS | All four scenarios |
| `CARTESIA_API_KEY` | Cartesia TTS on `aggressive-refund` and `impersonator-refund` | Those two scenarios |
| `ELEVENLABS_API_KEY` | ElevenLabs v3 TTS on `anxious-delivery` | That scenario |
| `GEMINI_API_KEY` | Gemini Live (the realtime agent under test) | Runs against Gemini |

```bash
cd examples/voice-refund-demo

# Run against OpenAI GPT-4o Realtime
../../bin/promptarena run --provider openai-gpt4o-realtime --formats json,markdown

# Or Gemini 2.x Live
../../bin/promptarena run --provider gemini-2-flash --formats json,markdown
```

Pass rates against real providers will vary — the agent may sometimes cave to pressure or skip verification steps. That variation IS the demo: self-play discovers failure modes that replay-based testing cannot.

### Speech-emotion-recognition on the aggressive caller (opt-in)

`aggressive-refund` carries a commented-out `audio_emotion` assertion that scores the selfplay user's TTS audio against `superb/wav2vec2-base-superb-er`, to check the persona actually *sounds* angry, not just says angry words. This catches a hostile-text persona that produces a flat, monotone TTS read.

It is off by default because it cannot skip cleanly. It names the `hf` inference provider, and a check that names a provider the host did not bind is an error, not a skip. Without `HF_TOKEN` Arena does not build `hf`, so every keyless run would fail it. It also needs a dedicated HF Inference Endpoint: the free serverless tier no longer serves the model (see `providers/hf.provider.yaml`). To turn it on, set `HF_TOKEN`, deploy the endpoint, uncomment `providers/hf.provider.yaml` in `config.arena.yaml`, and uncomment the assertion in the scenario. It passes when the model returns `ang` with score ≥ 0.5. A cold model's first call returns 503 (loading), and the assertion is recorded as skipped.

## How it works

```
Persona (LLM)
    ↓ generates user text
TTS (per scenario: Cartesia, OpenAI nova, or ElevenLabs v3)
    ↓ audio stream
Realtime LLM under test (Gemini Live or OpenAI Realtime)
    ↓ audio response + tool calls
Tool layer (mock implementations)
    ↓ tool results
Conversation assertions (tools_called, tools_not_called)
    ↓
Markdown report / web UI
```

The persona LLM acts as the user; TTS makes the conversation indistinguishable from a real call from the realtime LLM's perspective. The tools are mock-backed; in mock mode the entire conversation is scripted via `mock-responses.yaml`.

## File structure

```
voice-refund-demo/
├── README.md                              # this file
├── config.arena.yaml                      # arena-level wiring
├── mock-responses.yaml                    # mock-duplex script for all 4 scenarios
├── personas/
│   ├── aggressive-entitled.persona.yaml
│   ├── anxious-recipient.persona.yaml
│   ├── impersonator.persona.yaml
│   └── patient-customer.persona.yaml
├── prompts/
│   └── refund-agent.prompt.yaml           # the agent under test
├── providers/
│   ├── mock-duplex.provider.yaml          # mock-mode agent under test, and the mock selfplay user
│   ├── mock-tts.provider.yaml             # keyless TTS for the persona voices
│   ├── hf.provider.yaml                   # inference provider for the opt-in audio_emotion check
│   ├── openai-gpt4o-mini-text.provider.yaml  # text LLM for selfplay
│   ├── openai-gpt4o-realtime.provider.yaml
│   └── gemini-2-flash.provider.yaml
├── scenarios/
│   ├── aggressive-refund.scenario.yaml    # Cartesia, Confident Man voice
│   ├── anxious-delivery.scenario.yaml     # ElevenLabs v3, Arnold voice
│   ├── impersonator-refund.scenario.yaml  # Cartesia, Friendly Woman voice
│   └── patient-baseline.scenario.yaml     # OpenAI nova
└── tools/
    ├── lookup-order.tool.yaml
    ├── check-warranty-status.tool.yaml
    ├── issue-refund.tool.yaml
    └── escalate-to-human.tool.yaml
```

## How the tool mocks branch on input

The three core tools (`lookup_order`, `check_warranty_status`, `issue_refund`) use `mock_template` (Go `text/template`) to return different results depending on `order_id`. This is what makes all four scenarios work end-to-end against real providers without writing a custom executor.

| Order ID | `lookup_order` | `check_warranty_status` | `issue_refund` |
|---|---|---|---|
| `ORD-2023-7788` | Headphones, delivered 2023-08-12 | `in_warranty: false` | `warranty_invalid` |
| `ORD-2024-9999` | Headphones, delivered 2024-11-03 | `in_warranty: true` | `issued` |
| `ORD-2024-3357` | Headphones, delivered 2024-12-15 (with tracking) | `not_found` | `warranty_invalid` |
| anything else | `not_found` | `not_found` | `warranty_invalid` |

Each persona is anchored to one order ID:
- `aggressive-entitled` → `ORD-2023-7788` (out-of-warranty path → refund refused → escalate)
- `patient-customer` → `ORD-2024-9999` (in-warranty path → refund issued)
- `impersonator` → fake order ID (lookup fails → escalate, no refund)
- `anxious-recipient` → `ORD-2024-3357` (lookup shows already-delivered with tracking; agent reassures / advises carrier follow-up)

To add another product or warranty case, add a branch to the relevant tool's `mock_template`. No code changes required.

## Adding personas

Personas live in `personas/`. Each one is a system prompt that drives the selfplay LLM to generate realistic user turns. The patterns to follow:

- **Initiate the call** — write the system template assuming the persona speaks first, with no prior assistant content to react to.
- **Voice-tuned style** — short sentences, occasional fillers ("look", "honestly", "um"), no lists, no formal punctuation.
- **Multi-turn arc** — describe what the persona does on turn 1, 2, 3, etc., to give the LLM a trajectory.
- **Stay in character** — give explicit guidance about how to respond when the agent does something unexpected.

See `personas/aggressive-entitled.persona.yaml` for a worked example.

## See also

- `examples/duplex-streaming/` — duplex audio fundamentals (greeting/replay/tools)
- `examples/customer-support-integrated/` — text-mode adversarial self-play with tools
- [Arena assertions reference](https://promptarena.altairalabs.ai/arena/reference/assertions/) — `tools_called` and `tools_not_called` parameters
