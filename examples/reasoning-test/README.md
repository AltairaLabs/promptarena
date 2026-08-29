# reasoning-test

Demonstrates unified model **reasoning ("thinking")** capture across providers
and across turns (epic #1527). Gemini 3 Flash, Claude Sonnet and GPT-5 each work
through the same workshop-throughput problem twice: once as a **four-turn**
chained problem where every turn depends on the one before, and once as a
**tool loop** where the facts must be fetched before they can be reasoned over.
Both produce substantial reasoning traces while the spoken answer stays a
single line.

The point isn't that one model thinks. It's that three providers configure,
gate and return thinking in completely different ways, and it all still lands on
`Message.Reasoning` in one comparable shape.

## Run it

Needs `GEMINI_API_KEY` (or `GOOGLE_API_KEY`), `ANTHROPIC_API_KEY` and
`OPENAI_API_KEY`. From this directory:

```bash
../../bin/promptarena run --ci --formats json,markdown
```

(Build the CLI first with `make build-arena` from the repo root.)

A full run is 2 scenarios across 3 providers — a four-turn chained problem and
a two-turn tool loop — costing a couple of pence.

Run one at a time with `--scenario reasoning` or `--scenario reasoning-tools`.

## What to look for

The prompt demands a **terse** answer on every turn, so the split is obvious:

- `content` is one line — `ANSWER: 208`.
- `reasoning` holds the multi-step thinking, captured **separately**.

Reasoning is a **sibling of content** (`Message.Reasoning`), never mixed into
the answer, exports, or future-turn context.

**The check** — see the two side by side. A populated `reasoning` proves
capture; a one-line `content` proves it didn't leak into the answer:

```bash
jq '.Messages[] | select(.role=="assistant") | {content, reasoning}' out/*.json
```

A real run looks like this:

```
=== claude-sonnet ===                === openai-gpt5 ===
  turn 1: 'ANSWER: 208'   382          turn 1: 'ANSWER: 208'    982
  turn 2: 'ANSWER: 345'   571          turn 2: 'ANSWER: 345'    900
  turn 3: 'ANSWER: 553'     0          turn 3: 'ANSWER: 553'      0
  turn 4: 'ANSWER: Machi' 970          turn 4: 'ANSWER: A, 176' 1762
  total 1923 chars                     total 3644 chars
```

Reasoning is also visible in:

- **TUI** (`../../bin/promptarena run` without `--ci`): a `💭 Reasoning` section
  in the turn detail; interactive/voice sessions stream it live.
- **Web UI** (`promptarena serve`): a collapsible reasoning disclosure on the
  message, for both live runs and historical results. Unlike the TUI it does not
  stream token-by-token — the trace appears once, when the message lands.

It is **not** in the **markdown report** (`out/results.md`), which carries the
summary, per-run and cost tables only. (It does render a `reasoning` field, but
that is the *judge's* reasoning on an assertion — a different thing from model
thinking.)

Reasoning **is** persisted: the save stage stores it on the message and results
are rebuilt from the conversation store, which is why `out/*.json` holds the
traces. Those files also carry **opaque** reasoning (provider signatures and
encrypted blocks — never displayed, and often larger than the text beside them),
which is stripped before the browser but not before disk. Treat a reasoning
run's `out/*.json` as sensitive.

Reasoning is not fed back into later turns — each turn re-derives from the
conversation content alone, which is exactly why turns 2–4 produce fresh
traces.

## The four turns

| Turn | Asks | Answer | Why it needs reasoning |
|---|---|---|---|
| 1 | Monday total | 208 | Chain four rates, then two different stop times |
| 2 | Tuesday total | 345 | Same rates, mid-shift breakdown, derived +40% replacement |
| 3 | Combined total | 553 | Carry both prior totals forward |
| 4 | Best single machine | A, 176 | Per-machine attribution across both days |

Turn 3 is deliberately a one-step addition, and it shows: Claude and GPT-5
usually return an **empty** trace for it while returning hundreds of characters
either side. That is not a capture failure — there is simply nothing to
summarise. It's the clearest demonstration here that trace length tracks problem
difficulty, so if you simplify this scenario expect the traces to shrink or
vanish.

## Three providers, three different contracts

Each provider file documents its own contract inline. The short version:

| Provider | Control | The trap |
|---|---|---|
| Gemini 3 | `thinking_level: high` | 2.5's `thinking_budget` silently yields no summaries |
| Claude | `thinking_budget: 2048` | wire shape auto-selected by model generation |
| OpenAI | `api_mode: responses` + `reasoning_effort` | no trace at all on Chat Completions |

**Gemini's control is generation-specific and fails silently.** Gemini 3 takes
`thinking_level`; Gemini 2.5 takes `thinking_budget` and rejects
`thinking_level` with a hard 400. Send `thinking_budget` to a Gemini 3 model and
you get no thought summaries at all — the run still passes, `reasoning` is just
empty. That silence is the trap. See PromptKit issue #1843. Gemini also omits
the summary on the occasional call even when configured correctly; it is not
guaranteed per request.

**Claude picks its own wire shape.** `thinking_budget` is what you set, but
PromptKit translates it per model generation: 4.5-and-older take
`enabled`+`budget_tokens`, 4.6 takes either, and the 5-series takes `adaptive`
and rejects `enabled`. Sending the wrong one is a 400, not a degraded response.
PromptKit also drops `temperature` (the API rejects a custom one while thinking
is on) and raises `max_tokens` if the budget would leave no headroom for an
answer — which is why the provider file sets a temperature that never reaches
the wire.

**OpenAI hides the trace unless you switch API.** This one cost the most time,
so it is worth stating plainly: reasoning summaries are a **Responses API**
feature, and PromptKit defaults every non-`-pro` model to Chat Completions. On
that path `reasoning_summary` does nothing and `reasoning` comes back empty
however you configure it. `api_mode: responses` is what makes GPT-5 reasoning
visible at all. Two follow-on quirks, both handled in the provider file:

- `gpt-5.2` rejects `top_p` outright, so it is declared in
  `unsupported_params`.
- The Responses API rejects `seed`, and **`unsupported_params` cannot suppress
  it** — PromptKit's Responses builder sends `seed` unguarded. Arena instead
  omits the seed entirely when none is configured. Before that fix, every
  Responses-API call 400'd with `Unknown parameter: 'seed'`.

Worth noting what the fix bought: on Chat Completions, GPT-5 returned *wrong*
totals (396, then 516 at higher effort) with no visible trace. On the Responses
API it returns 208 with the richest reasoning of the three providers. The
reasoning wasn't just invisible — it wasn't really happening.

## Scenario 2: reasoning across a tool loop

`scenarios/reasoning-tools.scenario.yaml` runs the same problem with **nothing
given inline** — the facts live behind two tools:

- `get_machine_specs` returns D's rate plus the *relationships* defining A, B
  and C, so the model must chain D → C → B → A itself.
- `get_shift_log` returns the shift length plus hours lost *at the end* of the
  shift, so run time is another derivation rather than a quotable value.

Both are `mode: mock`: the tool RESULT is canned, but the tool genuinely
executes and the model's decision to call it is real. That is the part being
demonstrated. A second turn then asks a what-if that must reuse the tool
results without re-fetching.

The result is reasoning on **both sides** of the loop — think, call, observe,
think again:

```
=== openai-gpt5 ===
  assistant reasoning 1130   CALLS ['get_machine_specs', 'get_shift_log']
  tool      (result)
  tool      (result)
  assistant reasoning  900   -> derives rates and run times, answers 208
  assistant reasoning  860   -> follow-up, reuses results, answers 296
```

All three providers issue both calls in parallel in a single assistant turn.
Totals for the two-turn scenario: GPT-5 2890 chars, Claude 930, Gemini 847.
Gemini typically returns nothing on the tool-call turn itself but 500+ after the
results land.

### Two traps that fail silently

**`allowed_tools` is an allowlist: no entry, no tool.** Listing tools under
`spec.tools` in the arena config registers them with the arena; the prompt
config's `allowed_tools` is what grants a given prompt access to them. A prompt
with no `allowed_tools` gets no tools. That is by design — a prompt should only
reach the tools you name.

The thing to watch for is a **mismatch between the prompt text and the
allowlist**, which is easy to author by accident. This example's system prompt
says "call the tools to fetch what you need". While the allowlist was still
empty, the models did the only thing they could with that instruction: they
improvised, emitting `<tool_call>` / `<function_calls>` blocks and fabricated
results as ordinary text — Claude invented machines "M1, M2, M3" with made-up
rates. Nothing malfunctioned; the prompt promised something the config hadn't
granted.

So when a transcript *looks* like it used tools, confirm there is a real `tool`
role in the messages rather than tool-shaped text sitting in `content`.

**The same applies to provider `capabilities`.** If you declare a
`capabilities` list, it is exclusive — omit `tools` and the provider won't be
offered any. The working examples in this repo omit the block entirely, which
permits everything.

## No mock/offline path

Every other example here runs against the mock provider with no API keys. This
one cannot: the mock provider's response schema (`type` of `text`, `tool_calls`
or `multimodal`) has **no reasoning field**, so a mock can't produce a trace.
Demonstrating reasoning requires real thinking models until PromptKit's mock
gains one.
