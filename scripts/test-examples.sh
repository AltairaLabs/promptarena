#!/usr/bin/env bash
# Validate every example config, then RUN every example that needs no API key.
#
# The single list of examples that run: `make test-examples` and the CI
# "Test example packs" job both call this script. They used to keep a list
# each, and the two had drifted — each ran examples the other did not.
#
# Usage: scripts/test-examples.sh   (from the repo root, after `make build-go`)
set -euo pipefail

BIN="${PROMPTARENA_BIN:-$PWD/bin/promptarena}"
PACKC="${PACKC_BIN:-$PWD/bin/packc}"
# Validate and run against the in-repo schema snapshots (the runtime the packs
# ship against) rather than the published remote schemas.
export PROMPTKIT_SCHEMA_SOURCE=local

# ── Validation: every example config ────────────────────────────────────────
#
# Full validation, not --schema-only: the business-logic pass is what checks
# references between files. A scenario that pins a provider the config no
# longer declares passes the schema and fails every run (capability-matrix's
# combination scenarios kept naming openai-gpt41-mini after it was removed).

# Configs that only get the schema check. codegen-eval loads
# packs/codegen-agent.yaml, which the root .gitignore's packs/ rule kept from
# ever being committed.
SCHEMA_ONLY=(
  examples/codegen-eval/sonnet-baseline.arena.yaml
  examples/codegen-eval/sonnet-disciplined.arena.yaml
)

echo "Validating example configs against the in-repo schemas…"
failed=0
total=0
while IFS= read -r cfg; do
  total=$((total + 1))
  flags=()
  for s in "${SCHEMA_ONLY[@]}"; do
    [ "$cfg" = "$s" ] && flags=(--schema-only)
  done
  if ! "$BIN" validate "$cfg" "${flags[@]}" </dev/null >/dev/null 2>&1; then
    echo "  ✗ $cfg failed validation"
    "$BIN" validate "$cfg" "${flags[@]}" </dev/null 2>&1 | grep -iE 'error|❌|  - ' | head -5 || true
    failed=$((failed + 1))
  fi
done < <(find examples -maxdepth 2 \( -name 'config.arena.yaml' -o -name '*.arena.yaml' -o -name 'arena.yaml' \) -not -path '*/out/*' | sort)
echo "  Validated $total config(s); $failed failed."
[ "$failed" -eq 0 ]

# ── Mock runs ───────────────────────────────────────────────────────────────
#
# Schema validation covers every example; this is the subset that actually
# RUNS, and it should be every example that can run without a key. A pack that
# parses can still fail to build its pipeline or fail its own assertions:
# voice-guardrails and pii-guardrail-red-team were both broken by the PromptKit
# v2.5.0 bump (a judge-backed guardrail that names no provider is now refused
# at load) while build, test, lint and the schema sweep all stayed green, and
# resilience-testing and client-tools sat failing their own assertions on main
# because nothing ran them.
#
# Most runs pass no --mock-provider: each example declares mock_config on its
# own provider, and the flag replaces the assistant content wholesale, which
# would discard the scripted tool calls the workflow examples are driven by.
# The flag is only used where an example's config names real providers.
#
# Not run here, deliberately (each still gets the schema check above):
#   - real providers or local model servers: azure-foundry-test, bedrock-test,
#     codegen-anthropic, codegen-eval, context-management,
#     load-testing, memory-agent, ollama-local, reasoning-test, vertex-test,
#     vllm-local, arena-media-test (its
#     mock-media provider cannot produce the image/audio its scenarios assert)
#   - interactive voice consoles with no scenarios: voice-console-asm,
#     voice-console-vad
#   - npx-launched MCP servers: mcp-chatbot, mcp-everything-test,
#     mcp-filesystem-test, mcp-memory-test
#   - a Docker sandbox image: codegen-sandbox, sandbox-host-eval (which also
#     exits 1 by design), test-a-codegen-agent
#   - live public HTTP APIs: live-http-tools (mathjs, open-meteo) — mock model,
#     but a third-party outage would fail CI

run() {
  local ex="$1"
  shift
  echo "=== $ex ==="
  (cd "examples/$ex" && "$BIN" run "$@" </dev/null)
}

# Showcase packs whose configs name real providers: --mock-provider swaps them.
run customer-support  --config config.arena.yaml --ci --mock-provider --mock-config mock-responses.yaml --scenario customer-support-scenarios --formats json
run multimodal-basics --config config.arena.yaml --ci --mock-provider --mock-config mock-responses.yaml --formats json
run variables-demo    --config config.arena.yaml --ci --mock-provider --mock-config mock-config.yaml --formats json
run assertions-test   --config config.arena.yaml --ci --mock-provider --mock-config mock-responses.yaml --formats json
# Every provider is real; the stand-ins share the matrix's mock-responses.yaml.
run capability-matrix --config config.arena.yaml --ci --mock-provider --formats json
# Real providers alongside a mock one: run the mock, minus the self-play
# scenario (its simulated user is an OpenAI model).
run customer-support-integrated --config config.arena.yaml --ci --provider mock-tool-simulation \
  --scenario account-info,billing-question,order-inquiry,security-test,tool-test --formats json
run duplex-streaming  --config config.arena.yaml --ci --provider mock-duplex --scenario duplex-noisy-line --formats json
run duplex-streaming  --config config.arena.yaml --ci --provider mock-duplex --scenario duplex-voice-best-of-n --formats json
# Self-play and tool calls, keyless: the selfplay-user role and persona voices
# are bound to mock-duplex and mock-tts.
run duplex-streaming  --config config.arena.yaml --ci --provider mock-duplex \
  --scenario duplex-basic,duplex-tools,duplex-selfplay-mock --formats json
run voice-refund-demo --config config.arena.yaml --ci --provider mock-duplex --formats json

# Mock-only packs.
run llm-judge         --config config.arena.yaml --ci --formats json
run guardrails-test   --config config.arena.yaml --ci --formats json
run eval-test         --config config.arena.yaml --ci --formats json
run document-analysis --config config.arena.yaml --ci --formats json
run mortgage-underwriting --config config.arena.yaml --ci --formats json
run resilience-testing --config config.arena.yaml --ci --formats json
run client-tools      --config config.arena.yaml --ci --formats json
run session-replay    --config config.arena.yaml --ci --formats json
run model-migration   --config config.arena.yaml --ci --formats json
run multi-agent-demo  --config config.arena.yaml --ci --formats json
run multimodal-tool-results --config config.arena.yaml --ci --formats json
run rag-agent         --config config.arena.yaml --ci --formats json
run text-negotiation  --config config.arena.yaml --ci --formats json

# Red-team and objectives packs.
run adaptive-persona  --config config.arena.yaml --ci --formats json,objectives
(cd examples/adaptive-persona && "$BIN" calibrate \
  --check checks/refusal.yaml --examples data/refusal-labels.jsonl </dev/null)
run red-team-converters --config config.arena.yaml --ci --formats json,objectives
run redteam-generated --config config.arena.yaml --ci --formats json,objectives
run tap-search        --config config.arena.yaml --ci --formats json,objectives
run indirect-injection --config config.arena.yaml --ci --formats json
run memory-poisoning  --config config.arena.yaml --ci --formats json,objectives
run pii-guardrail-red-team --config config.arena.yaml --ci --formats json

# Voice packs (mock duplex providers and built-in mock audio).
run voice-guardrails  --config config.arena.yaml --ci --formats json
run voice-ivr         --config config.arena.yaml --ci --formats json
run voice-bake-off    --config config.arena.yaml --ci --formats json
run voice-latency-budget --config config.arena.yaml --ci --formats json

# Workflow packs. They exercise the most intricate machinery in the repo (state
# handoff mid-turn, per-state skill filtering, max-visits redirects).
# workflow-skills drives the skill tool-grant path, where a grant that silently
# grants nothing leaves no trace the transcript can show: its tools_offered
# assertions check that refund is absent before a skill grants it and offered
# after.
run workflow-agent-loops      --config config.arena.yaml --ci --formats json
run workflow-order-processing --config config.arena.yaml --ci --formats json
run workflow-skills           --config config.arena.yaml --ci --formats json
run workflow-support          --config config.arena.yaml --ci --formats json

echo "Compiling the governance showcase to a pack…"
"$PACKC" compile -c examples/mortgage-underwriting/config.arena.yaml \
  -o bin/mortgage-underwriting.pack.json >/dev/null
"$PACKC" validate bin/mortgage-underwriting.pack.json
