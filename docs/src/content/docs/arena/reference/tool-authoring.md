---
title: Tool Authoring
sidebar:
  order: 8
verified:
  commit: d12d5b388a693c119b19bf05eabade5b717cb10d
  sources:
    - arena/arenaconfig/loader.go
    - arena/arenaconfig/validator.go
    - arena/arenaconfig/validator_tools.go
    - arena/consent/hook.go
    - arena/engine/builder_integration.go
    - arena/turnexecutors/pipeline_stages_integration.go
    - examples/customer-support/tools/example-tool.tool.yaml
    - examples/voice-refund-demo/tools/lookup-order.tool.yaml
    - schemas/v1alpha1/tool.json
---

Tool YAML modes for PromptArena and the fields each mode takes.

For the full `Tool` schema (every field), see [Configuration Schema → Tool](/arena/reference/config-schema/#tool) or `schemas/v1alpha1/tool.json` in the repo.

## Modes

Valid `mode` values under `promptarena`: `mock`, `live`, `mcp` and `client`. `exec` (a subprocess) and `local` (an in-process handler) are run by the PromptKit SDK only: `promptarena` cannot execute them. A run stops at startup on a tool with either mode, and `promptarena validate` reports a `mode: local` tool as an error. `packc compile` accepts `mode: local`, since the SDK runs it from the compiled pack.

| `mode` | Behaviour | Fields |
|---|---|---|
| `mock` (static) | Returns `mock_result` for every call, whatever the args | `mock_result` |
| `mock` (template) | Renders `mock_template` with the call args and returns the result | `mock_template` |
| `live` | Calls a real HTTP endpoint | `http` |
| `mcp` | Uses a tool exposed by an MCP server configured at the arena level | none (auto-discovered) |
| `client` | Answered by the mock executor: returns the `mock_template` or `mock_result` output, or `{"result": "mock response"}` if neither is set | `client` (optional) |

Set `mock_result` or `mock_template`, not both: a tool that sets both fails to load. In a `kind: Tool` file, `mock_result_file` and `mock_template_file` read the same content from a file, resolved against the arena config directory.

## Mock: static (`mock_result`)

Returns the same response for every call.

```yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Tool
metadata:
  name: get-weather
spec:
  name: get_weather
  description: Get the current weather for a city.
  mode: mock
  timeout_ms: 1000
  input_schema:
    type: object
    properties:
      city: { type: string }
    required: [city]
  output_schema:
    type: object
    properties:
      temperature_c: { type: number }
      conditions: { type: string }
  mock_result:
    temperature_c: 18
    conditions: cloudy
```

## Mock: input-branching (`mock_template`)

Returns a response that depends on the tool-call args. The args are parsed as a JSON map and become the template's data context. The rendered output is parsed back as JSON. If the rendered text is not valid JSON, the executor wraps it in an object under the `result` key.

```yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Tool
metadata:
  name: lookup-order
spec:
  name: lookup_order
  description: Look up an order by ID.
  mode: mock
  timeout_ms: 1000
  input_schema:
    type: object
    properties:
      order_id: { type: string }
    required: [order_id]
  output_schema:
    type: object
    properties:
      order_id: { type: string }
      in_warranty: { type: boolean }
  mock_template: |
    {{- if eq .order_id "ORD-2024-9999" -}}
    {"order_id":"ORD-2024-9999","in_warranty":true}
    {{- else if eq .order_id "ORD-2023-7788" -}}
    {"order_id":"ORD-2023-7788","in_warranty":false}
    {{- else -}}
    {"error":"not_found"}
    {{- end -}}
```

### Template language

`mock_template` is rendered with Go's [`text/template`](https://pkg.go.dev/text/template) (`Option("missingkey=zero")`). The args map is the data context, so `.order_id` accesses the `order_id` field of the call.

Supported control flow includes:

- `{{ if eq .field "value" }}…{{ else if … }}…{{ else }}…{{ end }}`
- `{{ range .items }}…{{ end }}`
- Comparison helpers: `eq`, `ne`, `lt`, `gt`, `le`, `ge`
- `printf`, `index`, and the rest of the built-in template functions

The `{{- … -}}` form trims surrounding whitespace so that the rendered output parses as JSON.

### Multimodal mocks: `mock_parts`

`mock_parts` returns image, audio, video or document content alongside the JSON result. It works with both `mock_result` and `mock_template`. A part's `file_path` resolves against the config directory. The structure is defined by `MockPartSpec` and `MockMediaSpec` in `schemas/v1alpha1/tool.json`.

## Live: HTTP (`mode: live`)

Calls a real HTTP endpoint. Args are sent as JSON. A JSON response body is returned as is, a non-JSON body is wrapped in an object under `result`, and a non-2xx status is an error.

```yaml
spec:
  mode: live
  http:
    url: https://api.example.com/orders/lookup
    method: POST
    headers:
      Content-Type: "application/json"
    headers_from_env:
      - Authorization=API_TOKEN
    timeout_ms: 5000
    redact:
      - api_key
```

| `http` field | Type | Description |
|---|---|---|
| `url` | string | Endpoint URL |
| `method` | string | HTTP method |
| `headers` | map of string | Static request headers |
| `headers_from_env` | list of string | Entries of the form `Header-Name=ENV_VAR_NAME`. The header is set to the raw value of the environment variable. Entries without `=` are ignored, and an unset or empty variable sets no header. |
| `timeout_ms` | integer | Request timeout in milliseconds |
| `redact` | list of string | Top-level keys of a JSON object response whose values are replaced with `"[REDACTED]"` before the result reaches the model. Nested keys are not matched. |

In the example, `Authorization=API_TOKEN` sets the `Authorization` header to the value of `API_TOKEN`. Put the full header value, such as `Bearer ...`, in the variable.

## MCP: discovered tools (`mode: mcp`)

The tool is provided by an MCP server configured at the arena level. The arena discovers tools from configured servers, and the Tool YAML declares the contract.

```yaml
spec:
  mode: mcp
```

## Client (`mode: client`)

A `client` tool is answered by the mock executor: it returns `mock_template` or `mock_result` output, or `{"result": "mock response"}` if neither is set. A turn's `consent_overrides` map a tool name to `grant`, `deny` or `timeout` to simulate the consent decision; any other value fails to load. The optional `client` block carries the following settings.

```yaml
spec:
  mode: client
  mock_result:
    content: "file contents"
  client:
    timeout_ms: 5000
    categories: [filesystem]
    consent:
      required: true
      message: "Allow the agent to read your filesystem?"
      decline_strategy: error
    validate_output: true
```

| `client` field | Type | Description |
|---|---|---|
| `timeout_ms` | integer | Timeout in milliseconds |
| `categories` | list of string | Tool categories |
| `validate_output` | boolean | Validate the output against `output_schema` |
| `consent.required` | boolean | Required when `consent` is set |
| `consent.message` | string | Consent prompt text |
| `consent.decline_strategy` | string | `fallback`, `error` or `retry` |

## See also

- [Configuration Schema → Tool](/arena/reference/config-schema/#tool): full field reference
- [`examples/voice-refund-demo`](https://github.com/AltairaLabs/promptarena/tree/main/examples/voice-refund-demo): `mock_template` branching across four scenarios
- [`examples/customer-support`](https://github.com/AltairaLabs/promptarena/tree/main/examples/customer-support): `mode: mock` with static results
