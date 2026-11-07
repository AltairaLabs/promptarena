# Codegen Sandbox Example

A coding agent fixes a bug inside a
[codegen-sandbox](https://github.com/AltairaLabs/CodeGen-Sandbox) container
that Arena starts and stops for each session.

## What this demonstrates

- A `source: docker` MCP entry: Arena runs the MCP server as a container and
  manages its lifecycle.
- `scope: session`: one fresh container per scenario session, stopped and
  removed when the session ends.
- The HTTP+SSE MCP transport.
- A preloaded skill (`skills/codegen/SKILL.md`) that gives the agent its
  working discipline.

The only provider is a mock (`providers/mock-provider.yaml`), so the LLM's
decisions come from `mock-responses.yaml`. The tool calls are real: they run
inside the sandbox container.

## Run it

You need Docker. No API key is needed.

```bash
make -C ../.. build-go
docker pull ghcr.io/altairalabs/codegen-sandbox:latest
../../bin/promptarena run --ci --format markdown
open out/results.md
```

`--mock-provider` gives the same run, because the configured provider is
already a mock. Both start the sandbox container.

What happens:

1. Arena reads `config.arena.yaml` and finds `source: docker` on the
   `sandbox` MCP entry.
2. For the session, the docker source runs the image, mounts
   `skills/codegen` read-only at `/skills/codegen`, publishes the
   container's port 8080 on a free host port, and polls `/sse` until the
   server answers.
3. Arena connects to the `/sse` endpoint over HTTP+SSE and discovers the sandbox's tools (32 with
   the current image).
4. The mock agent seeds `/workspace` with a buggy Go function and a failing
   test, reads the file with `Read`, fixes it with `Edit`, and runs
   `run_tests`.
5. The two `tools_called` assertions check that `Read` and `run_tests` were
   called. The container is stopped and removed when the session ends.

## Files

| File | Purpose |
|---|---|
| `config.arena.yaml` | Arena config, including the `source: docker` MCP entry and the skill sources |
| `prompts/codegen-agent.yaml` | Prompt and the sandbox tools the agent may call |
| `scenarios/simple-fix.scenario.yaml` | One bug-fix scenario |
| `skills/codegen/SKILL.md` | The codegen skill, preloaded into the agent |
| `providers/mock-provider.yaml` | Mock provider |
| `mock-responses.yaml` | Canned LLM turns, including the tool calls |
