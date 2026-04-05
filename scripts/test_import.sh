#!/bin/bash
# =============================================================================
# Integration test: 影片匯入
# 測試項目: 匯入流程、冪等性、回應格式、數量一致性
# 前置條件: 需要有影片檔案在 IMPORT_DIR
# =============================================================================

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/test_helpers.sh"

IMPORT_DIR="${IMPORT_DIR:-/mnt/videos}"

bold "=== Import 影片匯入測試 ==="
check_prerequisites

ADMIN_TOKEN=$(login_as "$ADMIN_USER" "$ADMIN_PASS")

# ---------------------------------------------------------------------------
# 1. 未認證不能匯入
# ---------------------------------------------------------------------------
echo ""
bold "[1] 未認證不能匯入"

CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "${API_BASE}/api/videos/import")
assert_eq "未帶 token 回 401" "401" "$CODE"

# ---------------------------------------------------------------------------
# 2. 第一次匯入
# ---------------------------------------------------------------------------
echo ""
bold "[2] 第一次匯入"
yellow "  (可能需要一段時間...)"

IMPORT_RESP=$(curl -s -X POST "${API_BASE}/api/videos/import" \
    -H "Authorization: Bearer ${ADMIN_TOKEN}" \
    -H "Content-Type: application/json" \
    -d "{\"source_dir\":\"${IMPORT_DIR}\"}" \
    --max-time 3600)

echo "  回應: $(echo "$IMPORT_RESP" | jq -c '.')"

TOTAL_SCANNED=$(echo "$IMPORT_RESP" | jq -r '.data.total_scanned // 0')
IMPORTED=$(echo "$IMPORT_RESP" | jq -r '.data.imported // 0')
SKIPPED=$(echo "$IMPORT_RESP" | jq -r '.data.skipped // 0')
FAILED=$(echo "$IMPORT_RESP" | jq -r '.data.failed // 0')

assert_gte "total_scanned >= 1" 1 "$TOTAL_SCANNED"
assert_eq "failed == 0" "0" "$FAILED"

# imported + skipped 應該等於 total（首次跑全部 imported，重複跑全部 skipped）
SUM_IS=$((IMPORTED + SKIPPED))
assert_eq "imported + skipped == total_scanned" "$TOTAL_SCANNED" "$SUM_IS"

# ---------------------------------------------------------------------------
# 3. 冪等性 — 再次匯入應全部 skip
# ---------------------------------------------------------------------------
echo ""
bold "[3] 冪等性測試"

IMPORT2_RESP=$(curl -s -X POST "${API_BASE}/api/videos/import" \
    -H "Authorization: Bearer ${ADMIN_TOKEN}" \
    -H "Content-Type: application/json" \
    -d "{\"source_dir\":\"${IMPORT_DIR}\"}" \
    --max-time 3600)

IMPORTED2=$(echo "$IMPORT2_RESP" | jq -r '.data.imported // -1')
SKIPPED2=$(echo "$IMPORT2_RESP" | jq -r '.data.skipped // 0')

assert_eq "再次匯入 imported == 0" "0" "$IMPORTED2"
assert_eq "再次匯入 skipped == total_scanned" "$TOTAL_SCANNED" "$SKIPPED2"

# ---------------------------------------------------------------------------
# 4. 回應格式驗證
# ---------------------------------------------------------------------------
echo ""
bold "[4] 回應格式"

HAS_DATA=$(echo "$IMPORT_RESP" | jq 'has("data")')
assert_eq "有 data 欄位" "true" "$HAS_DATA"

for field in total_scanned imported skipped failed; do
    HAS=$(echo "$IMPORT_RESP" | jq ".data | has(\"$field\")")
    assert_eq "data 包含 $field" "true" "$HAS"
done

# ---------------------------------------------------------------------------
print_summary "Import 影片匯入"
exit $?
