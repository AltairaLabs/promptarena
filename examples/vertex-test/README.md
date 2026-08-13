# vertex-test

Capability tests for Gemini on **Google Agent Runtime** — text, streaming and
tool calling.

This is the Google sibling of `examples/bedrock-test` and
`examples/azure-foundry-test`, and mirrors their structure.

Agent Runtime was formerly Vertex AI Agent Engine. The API resource is still
`reasoningEngines` and the host is still `aiplatform.googleapis.com`, which is
why the provider platform is named `vertex`.

## What it covers

| Scenario | Exercises |
|---|---|
| `text-basic` | Plain completion, tools disabled |
| `streaming` | Streamed response |
| `tools` | Function calling via the mocked `get_weather` tool |

## Running the arena locally

`providers/gemini-vertex-25flash.provider.yaml` pins a project — change
`platform.project` to your own, then:

```bash
gcloud auth application-default login
promptarena --config config.arena.yaml
```

Authentication is Application Default Credentials — no API keys. The account
needs `roles/aiplatform.user`.

## Deploying to Agent Runtime

The `promptarena-deploy-vertex` adapter deploys this pack as one Agent Runtime
engine per agent:

```bash
promptarena deploy adapter install vertex
promptarena deploy plan
promptarena deploy apply
```

It needs a `deploy` section in `config.arena.yaml`:

```yaml
deploy:
  provider: vertex
  vertex:
    project: my-project
    location: us-central1
    image: us-central1-docker.pkg.dev/my-project/ghcr-remote/altairalabs/promptkit-vertex-runtime
    providers:
      - name: default
        role: llm
        arena_provider: gemini-vertex-25flash
```

### Prerequisites

**The runtime image must be in Artifact Registry.** Agent Runtime cannot pull
from `ghcr.io` directly — configure an AR remote repository as a pull-through
cache, or push the image to AR directly.

**The Reasoning Engine Service Agent must be able to read it:**

```bash
gcloud artifacts repositories add-iam-policy-binding <REPO> \
  --location=<LOCATION> --project=<PROJECT> \
  --member="serviceAccount:service-<PROJECT_NUMBER>@gcp-sa-aiplatform-re.iam.gserviceaccount.com" \
  --role=roles/artifactregistry.reader
```

Without it the engine is created and then fails to start — the create call
itself succeeds, so nothing catches it earlier.

## Notes

`promptarena config-inspect` reports `tool get-weather.tool.yaml is defined but
not allowed by any prompt`. This is a pre-existing quirk: the check compares the
tool's *filename* against `allowed_tools`, which holds the tool *name*
(`get_weather`). `examples/bedrock-test` reports the same warning. The tool is
wired correctly.
