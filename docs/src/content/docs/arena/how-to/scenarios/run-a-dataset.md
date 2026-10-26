---
title: Run a Scenario Over a Dataset
description: Load scenario inputs and expected answers from a JSONL or CSV file, and get one result per row.
---

Run one scenario against every row of a JSONL or CSV file, with each row's values filled into the prompt and the assertions.

## Prerequisites

- A working arena config with at least one provider. See [Write Test Scenarios](/arena/how-to/scenarios/write-scenarios/).
- A dataset in JSONL (one JSON object per line) or CSV (a header row, then one record per row).

## Steps

1. Put the dataset next to the scenario. This example uses `data/capitals.jsonl`:

   ```json
   {"id": "fr", "country": "France", "answer": "Paris"}
   {"id": "jp", "country": "Japan", "answer": "Tokyo"}
   ```

   The `id` column names each row in reports. Without it, rows are named `1`, `2` and so on.

2. Add a `data` block to the scenario, and write `{column}` wherever a row value belongs:

   ```yaml
   apiVersion: promptkit.altairalabs.ai/v1alpha1
   kind: Scenario
   metadata:
     name: capitals
   spec:
     task_type: assistant
     description: Capital cities
     data:
       file: data/capitals.jsonl
     turns:
       - role: user
         content: "What is the capital of {country}?"
         assertions:
           - type: content_includes
             params:
               patterns: ["{answer}"]
   ```

   The path is relative to the scenario file. Placeholders also work in `conversation_assertions`, such as an `llm_judge` criterion, in `seed_memories` content and in `variables`. A value goes into a regex `pattern` as written, so prefer `content_includes` when values contain characters such as `+`, `$` or `(`.

3. If each JSONL line wraps the values in an object, as in `{"item": {"country": "France", ...}}`, name that key with `field`:

   ```yaml
   data:
     file: data/capitals.jsonl
     field: item
   ```

4. To measure how stable each row is, add `trials`. Each row then runs that many times, and its pass rate and `pass_threshold` are computed over that row's trials only:

   ```yaml
   spec:
     trials: 5
   ```

5. Run the scenario:

   ```bash
   promptarena run --scenario capitals --ci --formats json,markdown,junit
   ```

## Verify

The run summary and the Markdown report list each row as its own result, named `scenario[row-id]`:

```text
| mock | capitals[fr] | default | ✅ Pass | ...
| mock | capitals[jp] | default | ❌ Fail | ...
```

Each JSON result carries the label `data_row`, plus the row's values under `Params.data`, so you can slice the results by row with `jq`.

## Troubleshooting

- **`row 3 has no column for {answer}`**: the scenario uses a placeholder that the named row lacks. Check the column name for a typo, or add the column to that row.
- **`data and perturbations are mutually exclusive`**: a scenario with `data` cannot also use turn `perturbations`. Move the varying values into the dataset as columns.
- **`unsupported extension`**: name the file `.jsonl`, `.ndjson` or `.csv`.
- **A literal `{name}` that is not a column**: in a dataset scenario, every `{word}` is a placeholder and must name a column. Rephrase the text so it does not use single braces around a word.

## Related pages

- [`data` and `perturbations` in the configuration schema](/arena/reference/config-schema/#data)
- [Assertions Reference](/arena/reference/assertions/)
- [Validate Outputs](/arena/how-to/scenarios/validate-outputs/)
