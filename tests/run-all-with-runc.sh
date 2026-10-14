#!/bin/sh
# Run all tests/*.golf that have a matching .want file using runc.sh,
# with a 60-second timeout per test, and summarize passed/failed/timed-out tests.

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$ROOT_DIR" || exit 1

mkdir -p _tmp/runc_results

echo "Building minigolf and asm1802..."
go build -o minigolf main.go || exit 1
go build -o asm1802 ./cmd/asm1802 || exit 1

PASS_FILE="_tmp/runc_results/passed.list"
FAIL_FILE="_tmp/runc_results/failed.list"
TIMEOUT_FILE="_tmp/runc_results/timeout.list"
: > "$PASS_FILE"
: > "$FAIL_FILE"
: > "$TIMEOUT_FILE"

pass_count=0
fail_count=0
timeout_count=0

trap 'echo ""; echo "Interrupted."; exit 130' INT TERM

for golf in tests/*.golf; do
    case "$golf" in
        *nomoto*)
            continue
            ;;
        # Known heap size exhaustion (*PANIC*: 4002 on 1802):
        tests/forth_count.golf | tests/test_btree.golf)
            continue
            ;;
        # Known code+data size exceeding the 64KB 1802 address space:
        tests/test_big.golf | tests/test_complex.golf | tests/test_math.golf | \
        tests/test_fft.golf | tests/test_fft_sine64.golf | tests/test_floating.golf | \
        tests/test_longlong.golf)
            continue
            ;;
    esac
    want="${golf%.golf}.want"
    if [ ! -f "$want" ]; then
        continue
    fi

    name="$(basename "$golf")"
    base="${name%.golf}"
    got="_tmp/runc_results/${base}.got"
    err="_tmp/runc_results/${base}.err"
    dif="_tmp/runc_results/${base}.diff"
    want_clean="_tmp/runc_results/${base}.want.clean"
    got_clean="_tmp/runc_results/${base}.got.clean"

    printf "%-42s ... " "$golf"

    timeout -k 2s 60s sh ./runc.sh "$golf" </dev/null >"$got" 2>"$err"
    rc=$?

    sed 's/^[[:space:]]*//;s/[[:space:]]*$//' "$want" | grep -a -v '^#' | grep -a -v '^$' > "$want_clean"
    sed 's/^[[:space:]]*//;s/[[:space:]]*$//' "$got"  | grep -a -v '^#' | grep -a -v '^$' > "$got_clean"

    if [ "$rc" -eq 130 ]; then
        echo "INTERRUPTED"
        exit 130
    elif [ "$rc" -eq 124 ] || [ "$rc" -eq 137 ] || grep -q "reached maximum execution time" "$err" 2>/dev/null; then
        echo "TIMEOUT (>60s)"
        echo "$golf" >> "$TIMEOUT_FILE"
        timeout_count=$((timeout_count + 1))
    elif [ "$rc" -ne 0 ]; then
        echo "FAIL (exit $rc)"
        echo "$golf (exit $rc)" >> "$FAIL_FILE"
        fail_count=$((fail_count + 1))
    elif diff -u "$want_clean" "$got_clean" >"$dif" 2>&1; then
        echo "PASS"
        echo "$golf" >> "$PASS_FILE"
        pass_count=$((pass_count + 1))
    else
        echo "FAIL (output mismatch)"
        echo "$golf (output mismatch)" >> "$FAIL_FILE"
        fail_count=$((fail_count + 1))
    fi
done

echo ""
echo "=================================================================="
echo "COSMAC 1802 Test Summary (runc.sh)"
echo "=================================================================="
echo "Passed:    $pass_count"
echo "Failed:    $fail_count"
echo "Timed out: $timeout_count"
echo ""

if [ "$pass_count" -gt 0 ]; then
    echo "--- PASSED ($pass_count) ---"
    sed 's/^/  PASS: /' "$PASS_FILE"
    echo ""
fi

if [ "$fail_count" -gt 0 ]; then
    echo "--- FAILED ($fail_count) ---"
    sed 's/^/  FAIL: /' "$FAIL_FILE"
    echo ""
fi

if [ "$timeout_count" -gt 0 ]; then
    echo "--- TIMED OUT ($timeout_count) ---"
    sed 's/^/  TIMEOUT: /' "$TIMEOUT_FILE"
    echo ""
fi

if [ "$fail_count" -gt 0 ] || [ "$timeout_count" -gt 0 ]; then
    exit 1
fi
exit 0
