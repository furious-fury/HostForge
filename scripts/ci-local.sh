#!/usr/bin/env bash
# Run the same checks as .github/workflows/ci.yml, so a branch can be verified
# before it is pushed.
#
# Usage:
#   scripts/ci-local.sh          # Go checks, then web checks
#   scripts/ci-local.sh go       # Go checks only
#   scripts/ci-local.sh web      # web checks only
#
# The race detector needs cgo and a C compiler. When neither is available the
# step is reported as skipped rather than passed, because CI still runs it.

set -uo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

target="${1:-all}"
failed=()
skipped=()

step() {
  local name="$1"
  shift
  printf '\n\033[1m==> %s\033[0m\n' "$name"
  if "$@"; then
    printf '\033[32mPASS\033[0m  %s\n' "$name"
  else
    printf '\033[31mFAIL\033[0m  %s\n' "$name"
    failed+=("$name")
  fi
}

check_gofmt() {
  local unformatted
  unformatted="$(gofmt -l ./cmd ./internal ./pkg)"
  if [ -n "$unformatted" ]; then
    echo "Not gofmt-formatted. Run: gofmt -w ./cmd ./internal ./pkg"
    echo "$unformatted"
    return 1
  fi
}

run_race() {
  if ! command -v gcc >/dev/null 2>&1 && ! command -v clang >/dev/null 2>&1; then
    printf '\n\033[1m==> Go race detector\033[0m\n'
    printf '\033[33mSKIP\033[0m  Go race detector (no C compiler; CI still runs it)\n'
    skipped+=("Go race detector")
    return
  fi
  step "Go race detector" env CGO_ENABLED=1 go test -race ./...
}

if [ "$target" = "all" ] || [ "$target" = "go" ]; then
  step "Go formatting" check_gofmt
  step "Go vet"        go vet ./...
  step "Go build"      go build ./...
  step "Go test"       go test ./...
  run_race
fi

if [ "$target" = "all" ] || [ "$target" = "web" ]; then
  step "Web install"           npm --prefix web ci
  step "Web lint"              npm --prefix web run lint
  step "Web typecheck + build" npm --prefix web run build
  step "Web test"              npm --prefix web run test
fi

printf '\n────────────────────────────────\n'
for name in "${skipped[@]:-}"; do
  [ -n "$name" ] && printf '\033[33mskipped\033[0m %s\n' "$name"
done
if [ "${#failed[@]}" -gt 0 ]; then
  printf '\033[31m%d check(s) failed:\033[0m\n' "${#failed[@]}"
  printf '  %s\n' "${failed[@]}"
  exit 1
fi
printf '\033[32mAll checks passed.\033[0m\n'
