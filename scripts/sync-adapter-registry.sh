#!/usr/bin/env bash
# Refresh the embedded adapter registry's `latest` versions from each adapter's
# newest GitHub release.
#
# `promptarena deploy adapter install <name>` asks the Releases API first and
# only falls back to these constants when it cannot (offline, rate-limited,
# firewalled) — which is exactly the moment a user cannot check for themselves,
# so the fallback must not lag several releases behind. Nothing updated it
# before this; it drifted within the hour of every bump (#172, #173).
#
# Usage: scripts/sync-adapter-registry.sh [registry.json]
# Exit 0 whether or not anything changed; the caller diffs the file.
# Requires gh (authenticated) and jq.
set -euo pipefail

registry="${1:-arena/cmd/promptarena/adapter_registry.json}"

for name in $(jq -r '.adapters | keys[]' "$registry"); do
  repo="$(jq -r --arg n "$name" '.adapters[$n].repo' "$registry")"
  current="$(jq -r --arg n "$name" '.adapters[$n].latest' "$registry")"

  # tag_name is "vX.Y.Z"; the registry stores "X.Y.Z" and the installer
  # re-adds the v when it builds the download URL.
  if ! tag="$(gh api "repos/$repo/releases/latest" --jq '.tag_name' 2>/dev/null)"; then
    echo "warning: could not resolve latest release of $repo; leaving $name at $current" >&2
    continue
  fi
  latest="${tag#v}"

  if [[ ! "$latest" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo "warning: $repo latest tag $tag is not X.Y.Z; leaving $name at $current" >&2
    continue
  fi

  if [ "$latest" != "$current" ]; then
    echo "$name: $current -> $latest"
    tmp="$(mktemp)"
    jq --indent 2 --arg n "$name" --arg v "$latest" '.adapters[$n].latest = $v' "$registry" > "$tmp"
    mv "$tmp" "$registry"
  else
    echo "$name: $current (current)"
  fi
done
