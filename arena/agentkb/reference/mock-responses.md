# mock-responses.yaml

The mock provider simulates **only the LLM** — it decides what the agent says and
which tools it calls. Tools themselves still execute for real (registry, executor,
workflow state machine, memory). So a mock run exercises everything except the
model call.

A `type: mock` provider points at this file:

```yaml
# providers/mock.provider.yaml
spec:
  id: mock
  type: mock
  model: mock-model
  additional_config:
    mock_config: mock-responses.yaml   # resolved relative to the arena config dir
    auto_respond: true                 # duplex/voice kits only
```

## File shape

```yaml
defaultResponse: "Fallback used when nothing more specific matches."

scenarios:
  <scenario-metadata-name>:
    defaultResponse: "Optional per-scenario fallback."
    turns:
      1: "A plain string is a text-only turn."
      2:
        content: "Structured turn."
        tool_calls:
          - name: lookup_order
            arguments:
              order_id: "VW-10021"
    steps:                              # composition kits only, keyed by step ID
      gather: "Response for the 'gather' step."

selfplay:
  <persona-metadata-name>:              # drives the simulated *user*, not the agent
    turns:
      1: "I want a refund."
```

## The two keys that catch people out

**1. Scenario keys are `metadata.name`, not `spec.id`.** The manifest loader runs
`SetID(GetName())`, so `metadata.name` overwrites whatever `spec.id` says — the
value in `spec.id` is never used for lookup. The same applies to persona keys under
`selfplay:`. If your two fields differ and the mock never fires, this is why.

**2. Turn numbers count LLM calls, not user turns.** They are 1-indexed. A user turn
that triggers a tool call consumes **two** entries: one for the call, one for the
reply after the tool result comes back. Get this wrong and responses slide out of
alignment part-way through a scenario.

## Structured turn fields

| Field | Notes |
|---|---|
| `type` | `text`, `tool_calls`, or `multimodal`. Defaults to `text`; set to `tool_calls` automatically when `tool_calls` is present, so you rarely write it. |
| `content` | The text. `text:` and `response:` are also accepted (`response` is legacy); `content` wins if more than one is present. |
| `tool_calls[].name` | Tool to invoke — must match a tool the kit defines. |
| `tool_calls[].arguments` | Map passed to the tool. The tool then runs for real. |
| `parts[]` | Multimodal parts: `type` of `text`/`image`/`audio`/`video`/`document`, plus the matching `text`/`image_url`/`audio_url`/`video_url`/`document_url`. URLs may be `mock://`, `http(s)://`, `data:`, or a file path. |
| `audio_file` | Duplex auto-respond only. Raw PCM, signed 16-bit little-endian, mono. Path resolves relative to this file. |
| `audio_sample_rate` | Hz. Defaults to 24000. |
| `audio_mime_type` | Defaults to `audio/pcm`. |

## Lookup order

Most specific wins: scenario + step → scenario + turn number → scenario
`defaultResponse` → global `defaultResponse`. Selfplay persona responses are looked
up under `selfplay:` by persona name on the same rules.

## Don't hand-write it twice

Once a kit has run once, generate the file from the results instead of authoring
it by hand:

```bash
promptarena mocks generate -i out -o mock-responses.yaml
promptarena mocks generate --dry-run          # print instead of writing
promptarena mocks generate --merge            # keep existing entries
promptarena mocks generate --per-scenario -o mocks/
```

## Prove the assertions actually bite

A mock file that scripts the agent behaving *correctly* only proves the happy path
parses. Keep a second file that scripts it behaving *badly* — promising a refund it
should refuse, calling a tool it should not — and a second arena config pointing at
it. If the violation suite still passes, your assertions are not measuring anything.
