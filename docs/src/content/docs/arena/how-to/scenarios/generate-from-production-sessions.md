---
title: Generate regression scenarios from production sessions
description: Turn a session that went wrong in production into an arena scenario that asserts it does not go wrong again.
verified:
  commit: 60cd1a146e6224c0d66c972d18f329a0ce76c522
  sources:
    - arena/arenaconfig/types.go
    - arena/cmd/promptarena/deploy_import_interactive.go
    - arena/cmd/promptarena/generate.go
    - arena/cmd/promptarena/generate_integration.go
    - arena/deploy/flow/config.go
    - arena/deploy/flow/connect.go
    - arena/deploy/flow/login.go
    - arena/deploy/flow/session.go
    - arena/deploy/flow/sessions.go
    - arena/deploy/flow/sessions_test.go
    - arena/generate/adapter.go
    - arena/generate/converter.go
    - arena/generate/decision.go
    - arena/generate/expectation.go
    - arena/generate/generate.go
    - arena/generate/mapping.go
    - arena/generate/sources/recordings.go
    - deploy/sessions.go
---

A production session that scored badly on an eval, tripped a guardrail, or
did the wrong thing is a regression test if it becomes a scenario.
`promptarena generate --source <adapter>` pulls sessions from the platform your
deploy adapter targets and writes one scenario per session that survives
filtering. With `--dedup` (the default), sessions with the same failure
fingerprint collapse to the first one. Sessions that fail to load or convert,
or that violate no `--expect` range, are skipped.

## When this works well

It works best for one-shot and agent-loop sessions: one user input, possibly
with attachments, and everything after it is the agent's own doing. Every user
message becomes a scenario turn, so a one-shot session yields one turn. The
recorded evals become the assertions.

Long multi-turn conversations convert less well. Later user turns were written
against the recorded answers, and the model will not give the same answers
twice. `generate` keeps every turn and warns you, in the output and in the
scenario description.

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
select on a measurement instead, state the range you expect:

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

A recorded session carries each eval's score and, where the platform stored
one, its pass or fail verdict. A failed verdict becomes an assertion with the
recorded params; a score alone becomes an assertion when the pack declares a
threshold or you pass `--expect`. `generate` prints one line per recorded eval
saying what it became and why:

```text
session 01J9… eval faithfulness (faithfulness) scored 0.42: asserted-expectation min_score 0.8 — user expectation (recorded score 0.42)
session 01J9… eval answer_relevancy (answer_relevancy) scored 0.9: reported — scored 0.9 in session 01J9…; no threshold known — pass --expect answer_relevancy>=<value> (or <=) or declare threshold on the eval in the pack
```

See [CLI commands](/arena/reference/cli-commands/) for the table that maps each
situation to asserted, dropped or reported.

Edit the generated YAML if a bound is wrong; it is a plain scenario file.

## What about sensitive data?

The platform redacts session data before it serves it, and `generate` writes
what it receives. Treat the output directory like any other
file that came from production, and check what your platform redacts before
committing generated scenarios to a shared repository.

## Programmatic use

`generate.Generate` runs the same pipeline in-process. It re-checks
`ListOptions.Expectations` against every session it fetches, so a source that
does not filter gets the same scenarios as one that does.

See [API reference](/arena/reference/api-reference/) for `Generate` and
`flow.OpenSessionSource`, and [Adapter SDK](/arena/reference/deploy/adapter-sdk/)
for the helpers (`ViolatesAny`, `UserTurnIndex`, `EvalResultsFromRows`,
`HasFailedVerdict`) that map stored records onto a `SessionDetail`.
