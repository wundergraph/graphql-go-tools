#!/usr/bin/env bash
# Compares the benchmarks of this package between a base ref and the working tree.
#
# Usage: ./bench-compare.sh
#
# Environment:
#   BASE       git ref to compare against (default: previous v2.* tag)
#   BENCH      benchmark regexp (default: Scenarios)
#   COUNT      number of runs for each side (default: 10)
#   BENCHTIME  -benchtime for each run (default: 200ms)
#   OUT        directory for the raw results (default: new temp directory)
#
# The script copies the current benchmark file into the base checkout.
# Thus both sides run the same scenarios. The base must compile with this file.
set -euo pipefail

PKG_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(git -C "$PKG_DIR" rev-parse --show-toplevel)
PKG_REL=${PKG_DIR#"$REPO_ROOT"/}
BENCH_FILE=grpc_datasource_bench_test.go

BASE=${BASE:-$(git -C "$REPO_ROOT" describe --tags --abbrev=0 --match 'v2.*' HEAD^)}
BENCH=${BENCH:-Scenarios}
COUNT=${COUNT:-10}
BENCHTIME=${BENCHTIME:-200ms}
TEMP_PATH=${TMPDIR:-/tmp}
TEMP_PATH=${TEMP_PATH%/}
OUT=${OUT:-$(mktemp -d "$TEMP_PATH/grpc-bench.XXXXXX")}
mkdir -p "$OUT"

BASE_TREE=$(mktemp -d "$TEMP_PATH/grpc-bench-base.XXXXXX")
cleanup() {
	git -C "$REPO_ROOT" worktree remove --force "$BASE_TREE" 2>/dev/null || true
	rm -rf "$BASE_TREE"
}
trap cleanup EXIT

BASE_SHA=$(git -C "$REPO_ROOT" rev-parse --short "$BASE^{commit}")
echo "base:         $BASE ($BASE_SHA)"
echo "working tree: $(git -C "$REPO_ROOT" describe --tags --always --dirty)"
echo "results:      $OUT"

git -C "$REPO_ROOT" worktree add --quiet --detach "$BASE_TREE" "$BASE_SHA"
cp "$PKG_DIR/$BENCH_FILE" "$BASE_TREE/$PKG_REL/"

echo "building test binaries..."
(cd "$BASE_TREE/$PKG_REL" && go test -c -o "$OUT/base.test" .)
(cd "$PKG_DIR" && go test -c -o "$OUT/head.test" .)

# Alternate the runs. Thus thermal drift and background load affect both sides equally.
rm -f "$OUT/base.txt" "$OUT/head.txt"
for i in $(seq 1 "$COUNT"); do
	echo "run $i/$COUNT"
	(cd "$BASE_TREE/$PKG_REL" && "$OUT/base.test" -test.run '^$' -test.bench "$BENCH" -test.benchmem -test.benchtime "$BENCHTIME" >>"$OUT/base.txt")
	(cd "$PKG_DIR" && "$OUT/head.test" -test.run '^$' -test.bench "$BENCH" -test.benchmem -test.benchtime "$BENCHTIME" >>"$OUT/head.txt")
done
rm -f "$OUT/base.test" "$OUT/head.test"

(cd "$PKG_DIR" && go tool benchstat "$BASE=$OUT/base.txt" "working-tree=$OUT/head.txt" | tee "$OUT/benchstat.txt")
