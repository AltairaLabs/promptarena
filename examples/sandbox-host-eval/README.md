# Sandbox host eval

A host-side eval program scores each session's sandbox container. Arena
hands it the container ID, the host port for each published container port
and the run's identity as JSON on stdin. The program uses the Docker CLI and
HTTP against the container and returns `{score, detail, data}`.

This is the pattern for harnesses like BaxBench, whose tests drive the app
over HTTP and also reach into its container (`docker exec`, `get_archive`,
`put_archive`, `stats`).

## What it shows

- `runtime.evals.sandbox_probe` binds an eval type to `evals/sandbox_probe.py`,
  a program on the host. The agent never sees it.
- `source_args.ports: [5000]` publishes container port 5000 on a free host
  port. The eval reads the mapping from `context.metadata.arena.sandboxes`.
- The probe checks three things and scores the fraction that pass:
  1. the container is still running when the eval starts
  2. `docker exec <id> cat /workspace/hello.txt` returns the file the agent
     wrote
  3. after killing anything on port 5000 and starting a fresh stand-in app,
     an HTTP GET to the mapped host port answers
- Both scenarios are scored. `finishes` ends normally. `hits-max-rounds` is cut
  off by `tool_policy.max_rounds: 1`, so its result has
  `StopReason: max_rounds`, and the eval still runs.

## Run it

Needs Docker and `python3` on the host. The model is mocked; the tools run for
real in the sandbox.

```bash
docker pull ghcr.io/altairalabs/codegen-sandbox:latest
make -C ../.. build-arena
../../bin/promptarena run -c config.arena.yaml --ci --formats json,csv
```

The command exits 1: `hits-max-rounds` is a failed run by design (it hit the
cap), even though it's scored. Each run's JSON has the probe under
`eval_results[]`:

```bash
jq '.eval_results[] | {score, details}' out/*_finishes_*.json
```

`results.csv` flattens the probe's numeric and boolean data into columns:
`eval.sandbox_probe.score`, `eval.sandbox_probe.container_alive`,
`eval.sandbox_probe.file_ok`, `eval.sandbox_probe.http_ok` and so on.

## Adapting it

Replace `evals/sandbox_probe.py` with your harness. Read
`context.metadata.arena` from stdin, and print one JSON object with
`score` and `data` to stdout. Raise `timeout_ms` to cover app start-up plus the
test run. See `runtime.evals` in the config reference for the full contract.
