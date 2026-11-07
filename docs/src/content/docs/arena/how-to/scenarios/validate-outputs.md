---
title: Validate Outputs
description: Add assertions to scenario turns and conversations to check content, structure, tool use and quality, then read the results.
verified:
  commit: 8f9eac27be158741f7a1cd3e34f53e72b896c6ab
  sources:
    - arena/arenaconfig/judge_param.go
    - arena/arenaconfig/types.go
    - arena/assertions/types.go
    - arena/cmd/promptarena/run.go
    - arena/engine/conversation_executor.go
---

Check that each response meets your requirements by adding assertions to a scenario's turns and to the whole conversation.

## Prerequisites

- A scenario that runs. See [Write Test Scenarios](/arena/how-to/scenarios/write-scenarios/).
- For `llm_judge` checks, a judge configured under `judges` in the arena config.

## Steps

1. Check content. `content_includes` needs every pattern (case-insensitive), `content_matches` takes a regular expression, and `content_excludes` fails when any pattern appears:

   ```yaml
   turns:
     - role: user
       content: "What's the support email?"
       assertions:
         - type: content_includes
           params:
             patterns: ["support"]
           message: "Should mention support"
         - type: content_matches
           params:
             pattern: '[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]{2,}'
           message: "Should contain an email address"
         - type: content_excludes
           params:
             patterns: ["competitor"]
           message: "Should not mention competitors"
   ```

   `message` sits beside `params` and appears in the report. Patterns use Go regular expressions, which have no lookahead, so check for absent text with `content_excludes`.

2. Check structure when the response should be JSON:

   ```yaml
       assertions:
         - type: is_valid_json
           message: "Should return valid JSON"
         - type: json_schema
           params:
             schema:
               type: object
               required: [name, email]
               properties:
                 name:
                   type: string
                 email:
                   type: string
           message: "Should match user schema"
   ```

3. Check tool use. Assertions on a turn run after that turn; `conversation_assertions` run once, after the whole conversation:

   ```yaml
   spec:
     turns:
       - role: user
         content: "What's the weather in Paris?"
         assertions:
           - type: tools_called
             params:
               tools: ["get_weather"]
             message: "Should call weather tool"

     conversation_assertions:
       - type: tool_calls_with_args
         params:
           tool_name: "get_weather"
           expected_args:
             location: "Paris"
         message: "Should pass Paris as location"
   ```

4. Check context retention with a later turn that depends on an earlier one:

   ```yaml
   turns:
     - role: user
       content: "My name is Alice"
     - role: user
       content: "What's my name?"
       assertions:
         - type: content_includes
           params:
             patterns: ["Alice"]
           message: "Should remember the user's name"
   ```

5. Check quality with an LLM judge. Arena grades with a judge from the arena config's `judges`: the one `judge` names, or the first by name when `judge` is unset. Put the score threshold in `params` as `min_score`:

   ```yaml
       assertions:
         - type: llm_judge
           params:
             judge: quality-judge
             criteria: "Response maintains a professional, positive tone"
             min_score: 0.7
           message: "Must be professional"
   ```

   A `judge` that names no entry in `judges` fails at load.

   To grade with an external service, use `rest_eval`. Header values expand `${VAR}` from the environment:

   ```yaml
         - type: rest_eval
           params:
             url: "https://eval-service.example.com/evaluate"
             headers:
               Authorization: "Bearer ${EVAL_API_KEY}"
             criteria: "Response meets compliance requirements"
             min_score: 0.9
           message: "External compliance check"
   ```

6. To run an assertion only when a condition holds, add `when`. An assertion whose condition is not met is skipped, not failed:

   ```yaml
         - type: content_includes
           params:
             patterns: ["shipped"]
           when:
             tool_called: lookup_order
           message: "Should report shipping status after the lookup"
   ```

   Every assertion on a turn must pass for the turn to pass.

7. Prefer several loose checks to one exact match. An exact sentence breaks on any rewording:

   ```yaml
   # Brittle
   - type: content_matches
     params:
       pattern: "^Thank you for contacting AcmeCorp support\\.$"

   # Sturdier
   - type: content_includes
     params:
       patterns: ["thank", "AcmeCorp", "support"]
     message: "Must acknowledge support contact"
   ```

## Verify

Run the scenario and write the reports you need:

```bash
promptarena run --scenario <name> --format json,markdown,junit
```

The Markdown report shows each assertion's pass or fail with its `message`. In the JSON result, each assistant message carries its turn's results under `meta.assertions`:

```json
"assertions": {
  "passed": true,
  "total": 1,
  "failed": 0,
  "results": [
    {
      "type": "min_length",
      "passed": true,
      "message": "Response should not be empty",
      "details": { "explanation": "length 67, min 1", "score": 1 }
    }
  ]
}
```

Conversation assertion results are under the result's `conversation_assertions`.

## Troubleshooting

- To see the provider calls behind a failing assertion, run with `--verbose`: `promptarena run --verbose --scenario <name>`.
- An `llm_judge` check that fails with `judge_provider not found` has no judge: add one under `judges` in the arena config.

## Related pages

- [Assertions Reference](/arena/reference/assertions/), for every assertion type and parameter
- [Guardrails Reference](/arena/reference/validators/)
- [Integrate CI/CD](/arena/how-to/interfaces/run-in-ci/)
- Examples: `examples/assertions-test/`, `examples/guardrails-test/`, `examples/customer-support/`
