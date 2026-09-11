---
title: Generate regression scenarios from production sessions
description: Turn a session that went wrong in production into an arena scenario that asserts it does not go wrong again.
---

A production session that scored badly on an eval, tripped a guardrail, or
simply did the wrong thing is the best regression test you will ever write, if
it becomes a scenario before anyone forgets it. `promptarena generate --source
<adapter>` pulls sessions from the platform your deploy adapter targets and
writes one scenario per session.

## When this works well

It works best for **one-shot and agent-loop sessions**: one user input, possibly
with attachments, and everything after it is the agent's own doing (tool calls,
workflow transitions, the final answer). The input becomes the scenario's turn;
the agent's behaviour becomes assertions.

It works less well for long multi-turn conversations. Later user turns were
written against the recorded answers, and the model will not give the same
answers twice. `generate` keeps every turn anyway and warns you, in the output
and in the scenario description.

## Prerequisites

- An installed deploy adapter with the `sessions` capability, e.g.
  `promptarena deploy adapter install omnia`. An older adapter is reported
  with that exact command to run.
- A `deploy:` section in your arena config, and a token from `promptarena
  deploy login`. The adapter is handed the same merged profile `deploy apply`
  uses: endpoint, workspace, token. `--workspace` overrides the workspace.

## Generate

```bash
promptarena generate \
  --source omnia \
  --config arena.yaml \
  --filter-passed=false \
  --output scenarios/production
```

`--filter-passed=false` selects sessions with a failed recorded verdict. To
select on a **measurement** instead, state the range you expect:

```bash
promptarena generate \
  --source omnia \
  --config arena.yaml \
  --expect faithfulness>=0.8 \
  --output scenarios/faithfulness
```

That selects sessions where `faithfulness` scored below 0.8 (or was never
measured), and asserts `min_score: 0.8` in the generated scenarios.

## Read the decisions

Real sessions record measurements, not verdicts. An eval such as
`faithfulness` produces a score; whether it is acceptable is a threshold the
pack author or you decide. `generate` prints one line per recorded eval saying
what it became and why:

```
session 01J9… eval faithfulness (faithfulness) scored 0.42: asserted-expectation min_score 0.8 — user expectation (recorded score 0.42)
session 01J9… eval answer_relevancy (answer_relevancy) scored 0.9: reported — scored 0.9 in session 01J9…; no threshold known — pass --expect answer_relevancy>=<value> (or <=) or declare threshold on the eval in the pack
```

| Situation | Result |
|-----------|--------|
| The eval carried a verdict and it failed | Asserted with its recorded params |
| The eval carried a verdict and it passed | Dropped |
| The pack (from `--config`) declares a `threshold` for the eval | Asserted with that bound |
| You passed `--expect` for the eval | Asserted with that bound |
| None of the above | Reported, never asserted against an invented bound |

Edit the generated YAML if a bound is wrong; it is a plain scenario file.

## What about sensitive data?

Redaction happens upstream, before the platform serves session data; the
generator performs none of its own. Treat the output directory like any other
file that came from production, and check what your platform redacts before
committing generated scenarios to a shared repository.

## Programmatic use

The same pipeline is a library: `generate.Generate` takes any
`generate.SessionSourceAdapter` and returns scenarios in memory with the
per-eval decisions, and `flow.OpenSessionSource` wraps an installed deploy
adapter as one. A platform running in-process can implement the adapter
against its own store and call `Generate` directly.
