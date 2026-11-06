---
title: Generate Mock Responses from Arena Results
description: Convert recorded Arena runs into mock provider YAML so you can replay conversations without LLM calls.
verified:
  commit: 0dbee6290db4e4de3bff3101f260fd614f81c924
  sources:
    - arena/cmd/promptarena/mocks.go
    - arena/cmd/promptarena/run.go
    - arena/mocks/generator.go
    - arena/mocks/parser.go
    - arena/results/json/json_repository.go
    - arena/templates/testdata/2025-11-30T19-49Z_openai-gpt4o_default_hardware-faults_18c25790.json
---
Convert recorded Arena JSON results into mock provider YAML, so you can replay conversations without calling an LLM.

## Prerequisites

- Arena run results in `out/*.json`, written by `promptarena run ... --format json`.
- A path for the generated mocks. The default is `providers/mock-generated.yaml`.

## Steps

1. Run Arena and write JSON results:

   ```bash
   promptarena run \
     --scenario hardware-faults \
     --provider openai-gpt4o \
     --format json \
     --out out
   ```

2. Generate mocks from the recorded runs:

   ```bash
   promptarena mocks generate \
     --input out \
     --scenario hardware-faults \
     --provider openai-gpt4o \
     --output providers/mock-generated.yaml \
     --merge
   ```

   | Flag | Effect |
   |---|---|
   | `--input` | A single run file or a directory of JSON results. Defaults to `out`. |
   | `--scenario`, `--provider` | Include only runs for these scenario or provider IDs. Repeatable. |
   | `--merge` | Merge into the existing mock file instead of overwriting it. |

3. Optional: write one file per scenario:

   ```bash
   promptarena mocks generate \
     --input out \
     --per-scenario \
     --output providers/responses \
     --merge
   ```

   This writes `providers/responses/<scenario>.yaml` for each scenario.

4. Optional: preview without writing:

   ```bash
   promptarena mocks generate --input out --dry-run
   ```

   The command prints each file's path and generated YAML to stdout.

## Example: IoT maintenance

The repository keeps recorded `hardware-faults` runs from an OpenAI model in `arena/templates/testdata/`. This command refreshes the mocks of a project created from the `iot-maintenance-demo` template:

```bash
promptarena mocks generate \
  --input arena/templates/testdata \
  --scenario hardware-faults \
  --output iot-maintenance-demo/providers/responses/mock-assistant.yaml \
  --merge
```

The generated turns carry the tool calls and responses from the recorded run.

## Tips

- Pass `--default-response "<text>"` to set a fallback reply for turns that have no recorded response.
- Keep the recorded JSON results under version control, so you can regenerate the same mocks.
- Run the scenarios with `promptarena run --mock-provider --mock-config <file>` before you commit the mocks, to check that they replay.

## Related

- [Use Mock Providers](/arena/how-to/providers/use-mock-providers/)
