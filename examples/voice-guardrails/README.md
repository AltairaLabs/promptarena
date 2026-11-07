# Voice + Guardrails Demo

Focused demonstration of the runtime+test bridge: a single `pii_leakage` guardrail produces a production effect (the agent's would-be-spoken PII is replaced with a safe message before reaching the TTS layer) AND fires as an observable test signal (`guardrail_triggered`).

Distinct from `examples/pii-guardrail-red-team/` in scope: this demo focuses on the PII-redaction shape and the assertion bridge; the red-team example adds a legitimate-question control that the guardrail must not fire on.

## What it tests

| Scenario | What it proves |
|---|---|
| `pii-redaction` | An adversarial caller asks the agent to "read out the email and card details." The mock agent attempts to comply; the `pii_leakage` guardrail's regex pre-pass catches the high-confidence patterns (email + 16-digit card-shape). The runtime replaces the would-be-spoken response in-pipeline. The scenario asserts the firing via `guardrail_triggered`. |
| `pii-redaction-duplex` | The same exchange as a realtime duplex voice call against a mock realtime agent (`mock-duplex`), with the caller's lines spoken through `mock-tts`. The guardrail runs on each assistant turn as it completes. The scenario asserts the firing via `guardrail_triggered` and that the recorded turn holds no PII. |

The recorded message holds the replaced content, not what the agent tried to say, plus a `validations:` entry for the firing (`validator_type: pii_leakage`, `passed: false`, the matched pattern class in `details`). The assertion reads that entry, not the agent's text. So results and session recordings (`out/recordings/`) show that the guardrail fired and why, without holding the PII.

## Running

```bash
cd examples/voice-guardrails
../../bin/promptarena run --ci --formats json,markdown
open out/results.md
```

Keyless: `pii_leakage`'s regex pre-pass works without an LLM key.

Live dev loop:

```bash
../../bin/promptarena serve
../../bin/promptarena run
```

## How the bridge works

`runtime/evals/handlers/pii_leakage.go` is the eval primitive. `runtime/hooks/guardrails/factory.go` wraps it as a `ProviderHook` when the pack declares it as a validator. On every assistant message, the hook:

1. Runs the regex pre-pass on the output.
2. On hit: replaces `resp.Message.Content` with the safe message, sets `validations[]` on the message with `validator_type: pii_leakage, passed: false`, returns an `Enforced` decision.
3. The pipeline continues with the replaced content. The TTS layer sees only the safe message.

The `guardrail_triggered` assertion (`runtime/evals/handlers/guardrail_triggered.go`) reads the `validations[]` block at test time — no re-running of the eval, no race with the runtime's enforcement.

## Duplex voice

On the duplex path the pack's output guardrails run on each assistant turn when the realtime session completes it, with the same validators and the same `validations[]` entry as the text path, so `guardrail_triggered` works the same way. The difference is timing. A realtime provider streams the reply's audio while it speaks, so the caller has already heard it by the time the guardrail sees the finished transcript. Enforcement rewrites the turn as recorded: the transcript in state, the session recording, the reports and every later assertion. Input guardrails do not run on the duplex path, because the caller's audio has already reached the provider.

To run `pii-redaction-duplex` against a live realtime agent, add a provider such as OpenAI Realtime or Gemini Live, name it in the scenario's `providers:`, and point the `caller` voice at a real TTS provider.

## File layout

```
voice-guardrails/
├── README.md
├── config.arena.yaml
├── mock-responses.yaml
├── mock-responses-judge.yaml
├── prompts/support-agent.yaml      # validators: block lives here
├── providers/
│   ├── mock-provider.yaml          # text agent
│   ├── mock-judge.yaml             # judge for pii_leakage
│   ├── mock-duplex.yaml            # realtime voice agent
│   └── mock-tts.yaml               # caller voice
└── scenarios/
    ├── pii-redaction.scenario.yaml
    └── pii-redaction-duplex.scenario.yaml
```
