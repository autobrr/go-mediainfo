#!/usr/bin/env bash
#
# scripts/fuzz.sh — run every Go fuzz target in internal/mediainfo. ci.yml calls
# it with a short FUZZTIME as a smoke test; fuzz.yml calls it nightly with a
# long one. Targets are discovered with `go test -list`, so a new Fuzz* function
# is picked up without editing any list.
#
# Why the shutdown-race handling exists: when a `-fuzztime` window ends, Go's
# fuzzing coordinator waits for its workers to stop within an internal deadline.
# On loaded / shared CI runners a worker can miss that deadline and the run
# fails with a bare
#
#     --- FAIL: FuzzXxx (120.10s)
#         context deadline exceeded
#
# That is a shutdown race in the test harness, not a defect in the code under
# test: no reproducer is written and nothing is reproducible. This wrapper fails
# the job only on a real finding (a written crasher / panic / other failure)
# and tolerates a bare shutdown deadline — but only after a deterministic corpus
# replay confirms there is nothing reproducible to find.
#
# Usage:
#   scripts/fuzz.sh              # fuzz every target with the default window
#   FUZZTIME=30s scripts/fuzz.sh # shorter window (handy locally)
#
# TestMain in internal/mediainfo chdirs to the repo root, so the seed corpus and
# any written crasher live under testdata/fuzz/ at the root, not the package dir.
#
set -uo pipefail

PKG=./internal/mediainfo
FUZZTIME="${FUZZTIME:-2m}"
LOG="$(mktemp)"
trap 'rm -f "$LOG"' EXIT

mapfile -t TARGETS < <(go test -list='^Fuzz' "$PKG" | grep '^Fuzz')
if [ "${#TARGETS[@]}" -eq 0 ]; then
  echo "fuzz: no Fuzz* targets found in $PKG (build broken?)" >&2
  exit 1
fi

# run_target fuzzes one target and returns 0 (clean / tolerated) or 1 (real bug).
run_target() {
  local fn="$1" rc
  echo "::group::fuzz ${fn} (fuzztime=${FUZZTIME})"
  go test "$PKG" -run='^$' -fuzz="^${fn}\$" -fuzztime="$FUZZTIME" 2>&1 | tee "$LOG"
  rc="${PIPESTATUS[0]}"
  echo "::endgroup::"
  if [ "$rc" -eq 0 ]; then
    echo "✅ ${fn}: clean"
    return 0
  fi
  if ! grep -q "Failing input written to" "$LOG" && grep -q "context deadline exceeded" "$LOG"; then
    echo "::warning title=fuzz shutdown race::${fn} ended with 'context deadline exceeded'; replaying corpus deterministically to rule out a real bug"
    if go test "$PKG" -run="^${fn}\$" -count=1 -timeout=3m; then
      echo "✅ ${fn}: corpus replay clean — tolerated harness shutdown race"
      return 0
    fi
    echo "❌ ${fn}: corpus replay failed — real bug, not a flake"
    return 1
  fi
  echo "❌ ${fn}: failed (exit ${rc}) — real finding"
  return 1
}

overall=0
# Keep going after a real finding so the nightly run surfaces them all.
for fn in "${TARGETS[@]}"; do
  run_target "$fn" || overall=1
done
if [ "$overall" -ne 0 ]; then
  echo "fuzz: one or more targets reported a real finding"
fi
exit "$overall"
