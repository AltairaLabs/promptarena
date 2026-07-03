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

            echo "  Processed: $dirname"
        fi
    done
}

echo "Processing PromptArena examples..."
process_examples "examples" "$ARENA_OUTPUT"

echo "✅ Example READMEs prepared for Starlight in arena/examples/"
