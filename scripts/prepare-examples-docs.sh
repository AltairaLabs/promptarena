#!/bin/bash
# Script to dynamically prepare example READMEs for Astro Starlight documentation.
# This runs during the docs build (via the docs `prebuild` npm script) and copies
# example READMEs into the arena content collection.
#
#   examples/<name>/README.md  ->  docs/src/content/docs/arena/examples/<name>.md
#
# The generated pages are gitignored — they are regenerated on every build.
set -euo pipefail

# Resolve the repo root from this script's location so the generator works
# regardless of the caller's working directory (the docs `prebuild` runs it
# from the docs/ directory, CI and local `make`-style invocations from root).
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$REPO_ROOT"

ARENA_OUTPUT="docs/src/content/docs/arena/examples"
LINK_REWRITER="$SCRIPT_DIR/rewrite-example-links.mjs"

# Collected `dirname<TAB>title<TAB>blurb` rows, used to build the index page.
INDEX_ROWS=$(mktemp)
trap 'rm -f "$INDEX_ROWS"' EXIT

# Group an example into a section of the index page. Anything not listed here
# lands in "More examples", so a newly added example still shows up on the
# index without touching this script — it just isn't filed under a heading.
category_for() {
    case "$1" in
        customer-support|variables-demo|text-negotiation)
            echo "Start here" ;;
        assertions-test|resilience-testing|guardrails-test|rag-agent|eval-test|reasoning-test)
            echo "Assertions and validation" ;;
        customer-support-integrated|mcp-chatbot|mcp-filesystem-test|mcp-memory-test)
            echo "Tools and MCP" ;;
        workflow-support|workflow-order-processing|workflow-agent-loops|workflow-skills|document-analysis|multi-agent-demo)
            echo "Workflows and multi-agent" ;;
        context-management|memory-agent)
            echo "Context and memory" ;;
        multimodal-basics|multimodal-tool-results|arena-media-test)
            echo "Multimodal" ;;
        duplex-streaming|voice-console-vad|voice-console-asm|voice-refund-demo|voice-ivr|voice-guardrails|voice-red-team|voice-bake-off|voice-latency-budget)
            echo "Voice and duplex" ;;
        ollama-local|vllm-local)
            echo "Local and self-hosted models" ;;
        codegen-sandbox|codegen-anthropic|codegen-eval|test-a-codegen-agent)
            echo "Codegen agents" ;;
        model-migration|session-replay|load-testing)
            echo "Regression, replay and load" ;;
        vertex-test)
            echo "Deploy targets" ;;
        *)
            echo "More examples" ;;
    esac
}

# Section order for the index. Keep "More examples" last so unmapped examples
# collect at the bottom rather than interrupting the curated flow.
INDEX_SECTIONS=(
    "Start here"
    "Assertions and validation"
    "Tools and MCP"
    "Workflows and multi-agent"
    "Context and memory"
    "Multimodal"
    "Voice and duplex"
    "Local and self-hosted models"
    "Codegen agents"
    "Regression, replay and load"
    "Deploy targets"
    "More examples"
)

# Pull a one-line summary out of a README: the first paragraph of real prose,
# trimmed to its first sentence, with markdown links flattened to their text so
# the index never carries a repo-relative link that 404s on the docs host.
blurb_for() {
    awk '
      /^# / { seen_h1 = 1; next }
      !seen_h1 { next }
      {
        if (!started) {
          # Skip everything ahead of the first paragraph of prose: blank lines,
          # headings, badges, blockquotes, tables, code fences, lists.
          if ($0 ~ /^[[:space:]]*$/) next
          if ($0 ~ /^[#>|!]/) next
          if ($0 ~ /^(```|:::|<)/) next
          if ($0 ~ /^[-*+][[:space:]]/) next
          if ($0 ~ /^[0-9]+\./) next
          started = 1
        } else if ($0 ~ /^[[:space:]]*$/ || $0 ~ /^[-*+][[:space:]]/ || $0 ~ /^(```|:::|#)/) {
          exit
        }
        # READMEs hard-wrap their paragraphs, so join the whole paragraph before
        # trimming to a sentence — otherwise the blurb stops mid-clause.
        buf = buf (buf == "" ? "" : " ") $0
      }
      END { print buf }
    ' "$1" \
      | sed -E 's/\[([^]]*)\]\([^)]*\)/\1/g; s/\*\*//g; s/`//g' \
      | awk '{ if (match($0, /[.!?] [A-Z]/)) print substr($0, 1, RSTART); else print }' \
      | sed -E 's/[[:space:]]*:$//' \
      | tr -d '\r'
}

# Clean up any existing generated pages so removed examples don't linger.
rm -rf "$ARENA_OUTPUT"
mkdir -p "$ARENA_OUTPUT"

# Process examples from a directory into an output collection.
# Args: source_dir, output_path
process_examples() {
    local source_dir=$1
    local output_path=$2

    mkdir -p "$output_path"

    # Process each top-level example README (nested/sub-example dirs are ignored
    # because we only glob the immediate children of source_dir).
    for example_dir in "$source_dir"/*/; do
        if [ -f "${example_dir}README.md" ]; then
            dirname=$(basename "$example_dir")
            readme_path="${example_dir}README.md"

            # Extract title from the first H1 heading, defaulting to the dir name.
            title=$(grep -m 1 "^# " "$readme_path" | sed 's/^# //' || echo "$dirname")

            # Create the output page with Starlight frontmatter.
            output_file="$output_path/${dirname}.md"
            cat > "$output_file" << EOF
---
title: ${title}
description: Example demonstrating ${dirname}
sidebar:
  order: 100
---

EOF
            # Append the original content (skip the leading H1 since Starlight
            # renders the title from frontmatter) and rewrite repo-relative links
            # to absolute GitHub URLs so they stay valid once served from the docs
            # host. See rewrite-example-links.mjs for the rationale.
            #
            # awk is used instead of sed here because `sed '1{/^# /d}'` only works
            # on GNU sed — on BSD sed (macOS) it errors out, the pipeline swallows
            # the error, and pages end up with only frontmatter, masking broken
            # links locally that still fire in CI.
            example_rel_dir="${example_dir%/}"
            awk '
              !emitted && /^$/ { next }
              !emitted && /^# / { emitted = 1; next }
              { emitted = 1; print }
            ' "$readme_path" | node "$LINK_REWRITER" "$example_rel_dir" >> "$output_file"

            printf '%s\t%s\t%s\n' "$dirname" "$title" "$(blurb_for "$readme_path")" >> "$INDEX_ROWS"

            echo "  Processed: $dirname"
        fi
    done
}

# Build the /arena/examples/ landing page. Without it the section has no index
# route, so every link to /arena/examples/ 404s (see issue #101). Generating it
# here — rather than committing a hand-written page — keeps it in step with
# whatever is actually in examples/, and survives the rm -rf above.
write_index() {
    local output_path=$1
    local index_file="$output_path/index.md"
    local total
    total=$(wc -l < "$INDEX_ROWS" | tr -d ' ')

    cat > "$index_file" << EOF
---
title: Examples
description: Working PromptArena projects you can clone, run, and adapt.
sidebar:
  order: 0
---

${total} runnable example projects. Each one is a complete Arena config you can
copy into your own repo — most run against mock providers out of the box, so you
can try them without API keys.

Every example lives under [\`examples/\`](https://github.com/AltairaLabs/promptarena/tree/main/examples)
in the repository. To run one:

\`\`\`bash
git clone https://github.com/AltairaLabs/promptarena.git
cd promptarena/examples/customer-support
promptarena run
\`\`\`

New to Arena? Start with [Your First Arena Test](/arena/tutorials/01-first-test/).
EOF

    local section
    for section in "${INDEX_SECTIONS[@]}"; do
        local body
        body=$(
            while IFS=$'\t' read -r dirname title blurb; do
                [ "$(category_for "$dirname")" = "$section" ] || continue
                if [ -n "$blurb" ]; then
                    printf -- '- [%s](/arena/examples/%s/) — %s\n' "$title" "$dirname" "$blurb"
                else
                    printf -- '- [%s](/arena/examples/%s/)\n' "$title" "$dirname"
                fi
            done < "$INDEX_ROWS"
        )
        [ -n "$body" ] || continue
        printf '\n## %s\n\n%s\n' "$section" "$body" >> "$index_file"
    done

    echo "  Processed: index (${total} examples)"
}

echo "Processing PromptArena examples..."
process_examples "examples" "$ARENA_OUTPUT"
write_index "$ARENA_OUTPUT"

echo "✅ Example READMEs prepared for Starlight in arena/examples/"
