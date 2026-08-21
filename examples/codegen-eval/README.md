# Codegen Eval — A/B Testing a Skill

Does adding a skill to a coding agent actually change what it does?

This example answers that question with a controlled comparison. Two arena
configs are identical in every respect — same model, same prompt, same tool
palette, same five tasks — except that one loads a `codegen-disciplined` skill
and the other does not. Run both, compare pass rate, cost and tool usage, and
you have evidence rather than an impression.

The tasks are real: each scenario hands the agent a Go package with a bug and a
hidden test suite, and the agent passes only if the tests do. Work happens inside
a container, so the agent genuinely edits files and runs tests rather than
describing what it would do.

## What's here

```
examples/codegen-eval/
├── sonnet-baseline.arena.yaml       Bundle A — no skill
├── sonnet-disciplined.arena.yaml    Bundle D — with the skill
├── scenarios/                       5 Go bugfix tasks
│   ├── go-add-bugfix                (easy, arithmetic)
│   ├── go-strings-reverse           (easy, encoding)
│   ├── go-fizzbuzz-order            (medium, control-flow)
│   ├── go-counter-race              (medium, concurrency)
│   └── go-binary-search             (hard, boundary)
├── packs/
│   └── codegen-agent.yaml           Shared prompt + tool palette
├── providers/
│   └── claude-sonnet.provider.yaml  claude-sonnet-4-6
└── skills/
    ├── codegen-disciplined/         Read-before-Edit, verify-before-done
    └── codegen-metrics/             Host-only; captures diff size per session
```

Each scenario carries `difficulty` and `category` labels, which is what makes the
stratified queries below possible — pass rate overall tells you less than pass
rate by difficulty.

The two arena configs sit at the example root rather than under `configs/`
because scenario and skill paths may not escape the config directory. Keep paths
forward-only.

## Prerequisites

This example calls a real model and runs code, so unlike most examples here it
needs credentials and Docker:

```bash
export ANTHROPIC_API_KEY=...     # or put it in a .env the binary can read
docker pull ghcr.io/altairalabs/codegen-sandbox:latest
make -C ../.. build-arena
```

## Running

```bash
# Bundle A — baseline
PROMPTKIT_SCHEMA_SOURCE=local ../../bin/promptarena run \
  --config sonnet-baseline.arena.yaml --ci --formats json,markdown

# Bundle D — with the skill
PROMPTKIT_SCHEMA_SOURCE=local ../../bin/promptarena run \
  --config sonnet-disciplined.arena.yaml --ci --formats json,markdown
```

Pass/fail comes from the hidden tests. The `tool_exec` gate calls `run_tests`,
and the sandbox returns an error on any failure, so one failing hidden test fails
the scenario.

A single run of each is enough to see the mechanism, but not enough to compare
two bundles — model output varies between runs. For that, set `spec.trials: 3` on
each scenario first. Per-run JSON lands in `out/<bundle>/`, and aggregate
`pass_rate` and `flakiness_score` in
`out/<bundle>/report-data.json[].trial_results`.

Budget roughly $0.10 per session: 5 scenarios × 2 bundles × 3 trials is about
$3 a sweep. Leaving trials at 1 cuts that by three.

## Reading the results

Pass rate by difficulty, for either bundle:

```bash
jq '[.results[] | {
       diff: .Labels.difficulty,
       cat:  .Labels.category,
       passed: .ConversationAssertions.passed
     }] |
     group_by(.diff) |
     map({diff: .[0].diff,
          n: length,
          pass: ([.[] | select(.passed)] | length)})' \
  out/sonnet-baseline/report-data.json
```

Cost against pass rate, both bundles side by side:

```bash
for b in sonnet-baseline sonnet-disciplined; do
  jq --arg b "$b" '
    [.results[] | {pass: .ConversationAssertions.passed, cost: .Cost.TotalCost}] |
    {bundle: $b,
     n: length,
     pass_rate: ([.[] | select(.pass)] | length) / length,
     total_cost: ([.[] | .cost] | add)}' \
    "out/$b/report-data.json"
done
```

Diff size per session, captured by the `codegen-metrics` skill:

```bash
jq -r '.eval_results[]
       | select(.eval_id == "diff_stats")
       | .details.result | fromjson
       | "loc=\(.total_loc) impl=\(.impl_loc) tests=\(.test_loc)"' \
   out/<run-id>.json
```

That last one reads a different channel from the others. Eval results land in
`eval_results[]` — production-shaped, no pass/fail — while test-time gates land
in `conversation_assertions.results[]`. The `pack_evals:` block that produces it
sits at arena-config level deliberately: these are runtime evals that would also
fire after compilation in production, and reading them from the config means they
can be exercised without compiling a pack first.

## What to expect

On these five tasks the two bundles produce **the same code**. `total_loc` and
`impl_loc` come out identical for every scenario, and both pass 5/5 — at this
difficulty the model converges on the same edit regardless of the skill.

The difference shows up in tool usage:

| Scenario | Baseline | With the skill |
|---|---|---|
| go-add-bugfix | Edit=1 Write=3 run_tests=2 | + run_lint=1 |
| go-binary-search | Edit=1 Write=3 run_tests=2 | + run_lint=1 |
| go-fizzbuzz-order | Edit=1 Write=3 run_tests=2 | + run_lint=1 |
| go-counter-race | Bash=2 Write=4 run_lint=1 run_tests=2 | (same) |
| go-strings-reverse | Edit=1 Write=3 run_tests=2 | (same) |

The "verify before done" instruction shows up as explicit lint calls on three of
five tasks, not as different code. That is a useful result in itself: a skill can
change how an agent works without changing what it produces, and only a
comparison like this one distinguishes the two.

Treat the table as an illustration of the shape rather than a benchmark — it is a
single trial. Harder tasks, where there is more room for the agent to make
different choices, are where a code-shape difference would be expected to appear.

## Adapting it

- **Different skill** — point `sonnet-disciplined.arena.yaml` at your own skill
  directory. Everything else stays.
- **Different model** — edit `providers/claude-sonnet.provider.yaml`, or add a
  second provider and let arena fan out across both.
- **Your own tasks** — add scenarios with `difficulty` and `category` labels so
  the stratified queries keep working.
