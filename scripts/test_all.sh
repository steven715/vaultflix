#!/bin/bash
# =============================================================================
# 執行所有 integration tests
#
# 使用方式:
#   bash scripts/test_all.sh              # 跑全部
#   bash scripts/test_all.sh auth tags    # 只跑指定的
#
# 可用的 test suites: auth, import, videos, tags
# =============================================================================

set -uo pipefail
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

ALL_SUITES="auth import videos tags"
SUITES="${*:-$ALL_SUITES}"

TOTAL_PASS=0
TOTAL_FAIL=0
RESULTS=""

green()  { printf "\033[32m%s\033[0m\n" "$1"; }
red()    { printf "\033[31m%s\033[0m\n" "$1"; }
bold()   { printf "\033[1m%s\033[0m\n" "$1"; }

for suite in $SUITES; do
    script="${SCRIPT_DIR}/test_${suite}.sh"
    if [ ! -f "$script" ]; then
        red "找不到 ${script}，跳過"
        RESULTS="${RESULTS}\n  SKIP  ${suite}"
        continue
    fi

    echo ""
    bold "================================================================"
    bold "  Running: test_${suite}.sh"
    bold "================================================================"
    echo ""

    if bash "$script"; then
        RESULTS="${RESULTS}\n  $(green "PASS")  ${suite}"
        TOTAL_PASS=$((TOTAL_PASS + 1))
    else
        RESULTS="${RESULTS}\n  $(red "FAIL")  ${suite}"
        TOTAL_FAIL=$((TOTAL_FAIL + 1))
    fi
done

echo ""
bold "================================================================"
bold "  Integration Test Summary"
bold "================================================================"
printf "%b\n" "$RESULTS"
echo ""

if [ "$TOTAL_FAIL" -eq 0 ]; then
    green "All ${TOTAL_PASS} suites passed!"
else
    red "${TOTAL_FAIL} suite(s) failed, ${TOTAL_PASS} passed"
fi

exit "$TOTAL_FAIL"
