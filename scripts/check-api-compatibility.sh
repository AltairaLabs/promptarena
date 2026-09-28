#!/usr/bin/env bash
#
# Fail a release whose Go API changes do not match the version bump it claims.
#
# PromptArena is imported as a library (deploy adapters build on
# deploy/adaptersdk, and its other exported packages are importable too), so
# removing or re-typing an exported symbol in a minor breaks someone's build.
# PromptKit shipped exactly that in v1.8.0 and the first signal was a downstream
# ticket. This is the same gate, for this module.
#
# It is not a warning. A breaking change in a minor is a bug: if the API must
# break, that is a major, and the /vN module-path change it forces is the
# friction that stops it happening casually.
#
# Usage: scripts/check-api-compatibility.sh vX.Y.Z [base-vX.Y.Z]
set -euo pipefail

VERSION="${1:?usage: $0 <new-version> [base-version]}"
BASE="${2:-}"

MAJOR="${VERSION#v}"
MAJOR="${MAJOR%%.*}"

if [ -z "$BASE" ]; then
  # Previous release on the same major. A different major is a different
  # module path, so it is not a baseline. `|| true` because grep exits 1 on an
  # empty list, which is a legitimate answer: the first release of a major.
  BASE=$(git tag -l "v${MAJOR}.[0-9]*.[0-9]*" --sort=-v:refname \
         | grep -v "^${VERSION}$" | head -1 || true)
fi

if [ -z "$BASE" ]; then
  echo "::warning::${VERSION} is the first release of v${MAJOR}; there is no published API to compare against."
  exit 0
fi

echo "Comparing the published API against ${BASE}, claiming ${VERSION}."
echo

out=$(env GOWORK=off go run golang.org/x/exp/cmd/gorelease@latest \
        -base="$BASE" -version="$VERSION" 2>&1 || true)
echo "$out" | sed 's/^/   /'
echo

# Key on the verdict, not the exit code: gorelease exits non-zero for
# diagnostics too, and a diagnostic is not a breaking change.
if echo "$out" | grep -q "is not a valid semantic version"; then
  echo "::error::API changes since ${BASE} are incompatible with ${VERSION}."
  echo "::error::gorelease lists the changes and a suggested version above."
  echo "::error::Additions need at least a minor. If it suggests a major, something"
  echo "::error::exported was removed or re-typed: restore it (keeping the old name"
  echo "::error::as a wrapper) or release a major, which means a /vN module path."
  exit 1
elif echo "$out" | grep -q "is a valid semantic version"; then
  echo "✓ ${VERSION} carries the API changes since ${BASE}"
else
  # No verdict is a failure: a gate that passes when it could not look reports
  # success for a release nobody checked.
  echo "::error::gorelease reached no verdict, so the API is UNVERIFIED."
  exit 1
fi
