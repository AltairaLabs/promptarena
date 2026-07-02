#!/usr/bin/env bash
#
# Pre-commit hook for PromptArena
# Runs fast, developer-friendly checks on changed code only.
#
# PromptArena is a single Go module, so (unlike PromptKit) there is no
# per-module resolution — everything runs from the repo root.
#
# Coverage gate: changed non-test files must hit 80%. Only *_test.go and
# *_interactive.go are exempt. *_integration.go is DELIBERATELY NOT exempt — it
# holds testable seam logic (engine/provider/tool/MCP wiring) and must be tested,
# matching sonar-project.properties (see its comments).
#
# To skip this hook, include "[skip-pre-commit]" in your commit message.
#

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

print_info()    { echo -e "${BLUE}ℹ ${NC}$1"; }
print_success() { echo -e "${GREEN}✓${NC} $1"; }
print_warning() { echo -e "${YELLOW}⚠${NC} $1"; }
print_error()   { echo -e "${RED}✗${NC} $1"; }
print_header()  {
    echo ""
    echo -e "${BLUE}═══════════════════════════════════════════════════${NC}"
    echo -e "${BLUE}  $1${NC}"
    echo -e "${BLUE}═══════════════════════════════════════════════════${NC}"
}

# Honour [skip-pre-commit] from the commit message file or -m args.
COMMIT_MSG_FILE=".git/COMMIT_EDITMSG"
if [ -f "$COMMIT_MSG_FILE" ] && grep -q "\[skip-pre-commit\]" "$COMMIT_MSG_FILE"; then
    print_warning "Skipping pre-commit checks (found [skip-pre-commit] in commit message)"
    exit 0
fi
for arg in "$@"; do
    if [[ "$arg" == *"[skip-pre-commit]"* ]]; then
        print_warning "Skipping pre-commit checks (found [skip-pre-commit] in commit message)"
        exit 0
    fi
done

print_header "Pre-Commit Checks"
print_info "Running fast checks on changed code only..."

CHECKS_FAILED=0
REPO_ROOT=$(git rev-parse --show-toplevel)
cd "$REPO_ROOT"

STAGED_GO_FILES=$(git diff --cached --name-only --diff-filter=ACM | grep '\.go$' || true)
if [ -z "$STAGED_GO_FILES" ]; then
    print_info "No Go files staged for commit. Skipping checks."
    exit 0
fi
print_info "Found $(echo "$STAGED_GO_FILES" | wc -l | tr -d ' ') Go file(s) to check"

# A repo whose build is entirely behind build tags, or with no tests, has
# nothing to analyze/build/test — not a failure.
is_empty_output() {
    echo "$1" | grep -qE "no go files to analyze|matched no packages|build constraints exclude all Go files|no test files"
}

#
# 1. Lint changed code only
#
print_header "Linting Changed Code"
if ! command -v golangci-lint &> /dev/null; then
    print_error "golangci-lint is not installed (brew install golangci-lint)"
    CHECKS_FAILED=1
else
    set +e
    lint_out=$(golangci-lint run --new-from-rev=HEAD --timeout=3m ./... 2>&1)
    lint_rc=$?
    set -e
    [ -n "$lint_out" ] && echo "$lint_out"
    if [ $lint_rc -eq 0 ] || is_empty_output "$lint_out"; then
        print_success "Lint passed"
    else
        print_error "Lint found issues in new code"
        CHECKS_FAILED=1
    fi
fi

#
# 2. Build
#
print_header "Building"
set +e
build_out=$(go build ./... 2>&1)
build_rc=$?
set -e
[ -n "$build_out" ] && echo "$build_out"
if [ $build_rc -eq 0 ] || is_empty_output "$build_out"; then
    print_success "Build succeeded"
else
    print_error "Build failed"
    CHECKS_FAILED=1
fi

#
# 3. Test with coverage, then gate coverage on changed files
#
print_header "Testing & Coverage"
TEMP_COVERAGE_DIR=$(mktemp -d)
trap 'rm -rf "$TEMP_COVERAGE_DIR"' EXIT
MERGED_COVERAGE="$TEMP_COVERAGE_DIR/coverage.out"

# Test only the packages that own the changed files — fast local feedback
# rather than the whole (single-module) suite. `-coverpkg` on those same
# packages captures cross-file coverage within each. The authoritative
# whole-repo gate is CI/SonarCloud.
CHANGED_PKGS=$(for f in $STAGED_GO_FILES; do d=$(dirname "$f"); printf './%s\n' "$d"; done | sort -u)
print_info "Testing changed packages:"
echo "$CHANGED_PKGS" | sed 's/^/    /'

set +e
# shellcheck disable=SC2086
test_out=$(go test -coverprofile="$MERGED_COVERAGE" -covermode=set $CHANGED_PKGS 2>&1)
test_rc=$?
set -e
echo "$test_out" | grep -v "no test files" || true

if [ $test_rc -ne 0 ] && ! is_empty_output "$test_out"; then
    print_error "Tests failed"
    CHECKS_FAILED=1
elif [ -f "$MERGED_COVERAGE" ]; then
    print_success "Tests passed"
    echo ""
    print_info "Checking coverage on changed files (threshold 80%)..."

    COVERAGE_THRESHOLD=80.0
    COVERAGE_FAILED=0
    COVERAGE_RESULTS=()

    for file in $STAGED_GO_FILES; do
        # Exempt tests and genuinely-interactive I/O only. *_integration.go is
        # intentionally gated (see header).
        case "$file" in
            *_test.go|*_interactive.go) continue ;;
            *_windows.go|*_darwin.go) continue ;;
            */testdata/*|testdata/*) continue ;;
        esac

        FILE_COV_PERCENT=$(grep "/$file:" "$MERGED_COVERAGE" 2>/dev/null | awk '
        {
            n = split($0, parts, " ")
            if (n >= 2) {
                count = parts[n]; stmts = parts[n-1]
                if (stmts ~ /^[0-9]+$/ && count ~ /^[0-9]+$/) {
                    total += stmts
                    if (count > 0) covered += stmts
                }
            }
        }
        END { if (total > 0) printf "%.1f", (covered / total) * 100 }')

        if [ -n "$FILE_COV_PERCENT" ]; then
            COV_OK=$(echo "$FILE_COV_PERCENT $COVERAGE_THRESHOLD" | awk '{print ($1 >= $2) ? "1" : "0"}')
            if [ "$COV_OK" = "1" ]; then
                COVERAGE_RESULTS+=("✓ $file: ${FILE_COV_PERCENT}%")
            else
                COVERAGE_RESULTS+=("✗ $file: ${FILE_COV_PERCENT}% (below ${COVERAGE_THRESHOLD}%)")
                COVERAGE_FAILED=1
            fi
        else
            # No coverage data — only a problem if the file has executable code.
            if grep -q "^func " "$REPO_ROOT/$file" 2>/dev/null; then
                COVERAGE_RESULTS+=("✗ $file: 0.0% (no coverage data)")
                COVERAGE_FAILED=1
            else
                COVERAGE_RESULTS+=("○ $file: N/A (no executable code)")
            fi
        fi
    done

    if [ ${#COVERAGE_RESULTS[@]} -eq 0 ]; then
        print_info "No non-test Go files to check"
    else
        for result in "${COVERAGE_RESULTS[@]}"; do echo "  $result"; done
        echo ""
        if [ $COVERAGE_FAILED -eq 1 ]; then
            print_error "Some changed files are below ${COVERAGE_THRESHOLD}% coverage"
            print_info "Add tests, or move genuinely-untestable I/O to a *_interactive.go file."
            CHECKS_FAILED=1
        else
            print_success "All changed files meet the coverage threshold"
        fi
    fi
fi

#
# Summary
#
print_header "Summary"
if [ $CHECKS_FAILED -eq 0 ]; then
    print_success "All pre-commit checks passed!"
    exit 0
else
    echo ""
    print_error "Pre-commit checks failed."
    print_info "Fix the issues above, or use [skip-pre-commit] in the commit message to bypass (not recommended)."
    exit 1
fi
