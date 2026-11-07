---
title: 'Tutorial 2: Multi-Prompt Packs'
description: Compile three customer-service prompts and a tool definition into one pack, then add a fourth prompt and recompile.
sidebar:
  order: 2
verified:
  commit: 8f9eac27be158741f7a1cd3e34f53e72b896c6ab
  sources:
    - arena/arenaconfig/loader.go
    - arena/arenaconfig/types.go
    - packc/compiler/helpers_integration.go
    - packc/main.go
    - packc/main_interactive.go
    - schemas/v1alpha1/arena.json
---
In this tutorial we build one pack for a customer-service application. It holds three prompts (greeting, support and escalation) and the definition of a tool the support prompt can call.

## Prerequisites

- [Tutorial 1: Your First Pack](/packc/tutorials/01-first-pack/) completed, with `packc` on your `PATH`

## Step 1: Create the project

```bash
mkdir customer-service-pack
cd customer-service-pack
mkdir -p prompts tools config packs
```

## Step 2: Write three prompts

The greeting prompt:

```bash
cat > prompts/greeting.yaml <<'EOF'
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: PromptConfig
metadata:
  name: Customer Greeting
spec:
  task_type: greeting
  description: Welcome customers and identify their needs
  version: v1.0.0
  system_template: |
    You are a friendly customer service representative.
    Greet the customer warmly and ask how you can help them today.
    Customer: {{customer_name}}
  template_engine:
    version: v1
    syntax: "{{variable}}"
  variables:
    - name: customer_name
      type: string
      required: true
EOF
```

The support prompt. Its `allowed_tools` names a tool we define in step 3:

```bash
cat > prompts/support.yaml <<'EOF'
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: PromptConfig
metadata:
  name: Technical Support
spec:
  task_type: support
  description: Provide technical support and solutions
  version: v1.0.0
  system_template: |
    You are a knowledgeable technical support specialist.
    Help customers solve their technical issues with clear, step-by-step guidance.
    Customer: {{customer_name}}
    Issue: {{issue}}
  template_engine:
    version: v1
    syntax: "{{variable}}"
  variables:
    - name: customer_name
      type: string
      required: true
    - name: issue
      type: string
      required: true
  allowed_tools:
    - search_knowledge_base
EOF
```

The escalation prompt:

```bash
cat > prompts/escalation.yaml <<'EOF'
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: PromptConfig
metadata:
  name: Escalation Handler
spec:
  task_type: escalation
  description: Handle complex issues requiring escalation
  version: v1.0.0
  system_template: |
    You are a senior support specialist handling escalated issues.
    Document the issue thoroughly and assure the customer it has high priority.
    Customer: {{customer_name}}
    Issue: {{issue}}
    Urgency: {{urgency}}
  template_engine:
    version: v1
    syntax: "{{variable}}"
  variables:
    - name: customer_name
      type: string
      required: true
    - name: issue
      type: string
      required: true
    - name: urgency
      type: string
      required: false
      default: normal
EOF
```

## Step 3: Define the tool

```bash
cat > tools/search_knowledge_base.yaml <<'EOF'
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Tool
metadata:
  name: search_knowledge_base
spec:
  name: search_knowledge_base
  description: Search the support knowledge base
  input_schema:
    type: object
    properties:
      query:
        type: string
    required: [query]
  output_schema:
    type: object
  mode: mock
EOF
```

The tool's `input_schema` becomes the tool's `parameters` in the pack.

## Step 4: List everything in the Arena config

```bash
cat > config/arena.yaml <<'EOF'
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: customer-service
spec:
  prompt_configs:
    - id: greeting
      file: ../prompts/greeting.yaml
    - id: support
      file: ../prompts/support.yaml
    - id: escalation
      file: ../prompts/escalation.yaml
  tools:
    - file: ../tools/search_knowledge_base.yaml
  providers: []
  defaults: {}
EOF
```

## Step 5: Compile the pack

```bash
packc compile \
  --config config/arena.yaml \
  --output packs/customer-service.pack.json \
  --id customer-service
```

You see the three prompts, sorted by task type, and the tool:

```text
✓ Pack compiled successfully: packs/customer-service.pack.json
  Contains 3 prompts: [escalation greeting support]
  Contains 1 tools: [search_knowledge_base]
```

## Step 6: Inspect the pack

```bash
packc inspect packs/customer-service.pack.json
```

The prompts section lists each prompt by task type, the support prompt shows its tool, and a tools section lists the pack's tools:

```text
=== Prompts (3) ===

[escalation]
  Name: Escalation Handler
  Description: Handle complex issues requiring escalation
  Version: v1.0.0
  Variables: 2 required, 1 optional
    Required: [customer_name issue]

[greeting]
  Name: Customer Greeting
  Description: Welcome customers and identify their needs
  Version: v1.0.0
  Variables: 1 required, 0 optional
    Required: [customer_name]

[support]
  Name: Technical Support
  Description: Provide technical support and solutions
  Version: v1.0.0
  Variables: 2 required, 0 optional
    Required: [customer_name issue]
  Tools (1): [search_knowledge_base]

=== Tools (1) ===
  search_knowledge_base: Search the support knowledge base
```

An application loads this one file and selects a prompt by its task type: `greeting`, `support` or `escalation`.

## Step 7: Validate the pack

```bash
packc validate packs/customer-service.pack.json
```

You see `✓ Schema validation passed` and `✓ Pack structure is valid`.

## Step 8: Add a fourth prompt

```bash
cat > prompts/followup.yaml <<'EOF'
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: PromptConfig
metadata:
  name: Follow-up Check
spec:
  task_type: followup
  description: Check on customer satisfaction after support
  version: v1.0.0
  system_template: |
    You are conducting a satisfaction check after customer support.
    Ask if the issue was resolved and if they need any additional help.
    Customer: {{customer_name}}
  template_engine:
    version: v1
    syntax: "{{variable}}"
  variables:
    - name: customer_name
      type: string
      required: true
EOF
```

Add it to `prompt_configs` in `config/arena.yaml`, after the escalation entry:

```yaml
    - id: followup
      file: ../prompts/followup.yaml
```

Recompile:

```bash
packc compile \
  --config config/arena.yaml \
  --output packs/customer-service.pack.json \
  --id customer-service
```

The summary now reads `Contains 4 prompts`, and `packc inspect` shows a `[followup]` block.

## Next steps

- [Tutorial 3: Validation Workflows](/packc/tutorials/03-validation-workflow/): catch broken prompts before they ship
- [Organize packs](/packc/how-to/organize-packs/): lay out larger projects
- [Pack format](/packc/explanation/pack-format/): how prompts, tools and fragments sit in a pack
