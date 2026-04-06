#!/bin/bash
# =============================================================================
# Integration test: 影片匯入
# 測試項目: media source 建立、匯入流程、冪等性、回應格式、數量一致性
# 前置條件: 需要有影片檔案在 IMPORT_DIR
# =============================================================================

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/test_helpers.sh"

IMPORT_DIR="${IMPORT_DIR:-/mnt/host/videos}"
SOURCE_LABEL="${SOURCE_LABEL:-integration-test}"

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
# 2. 建立或取得 media source
# ---------------------------------------------------------------------------
echo ""
bold "[2] 建立 Media Source"

# 先查有沒有同路徑的 source
EXISTING_SOURCES=$(curl -s -X GET "${API_BASE}/api/media-sources" \
    -H "Authorization: Bearer ${ADMIN_TOKEN}")
SOURCE_ID=$(echo "$EXISTING_SOURCES" | jq -r \
    ".data[] | select(.mount_path == \"${IMPORT_DIR}\") | .id // empty" 2>/dev/null || echo "")

if [ -n "$SOURCE_ID" ]; then
    yellow "  已存在同路徑的 media source: ${SOURCE_ID}"
else
    CREATE_RESP=$(curl -s -X POST "${API_BASE}/api/media-sources" \
        -H "Authorization: Bearer ${ADMIN_TOKEN}" \
        -H "Content-Type: application/json" \
        -d "{\"label\":\"${SOURCE_LABEL}\",\"mount_path\":\"${IMPORT_DIR}\"}")
    SOURCE_ID=$(echo "$CREATE_RESP" | jq -r '.data.id // empty')
    if [ -z "$SOURCE_ID" ]; then
        red "  建立 media source 失敗: $CREATE_RESP"
        exit 1
    fi
    green "  Media source 建立成功: ${SOURCE_ID}"
fi

assert_not_empty "source_id 不為空" "$SOURCE_ID"

# ---------------------------------------------------------------------------
# 3. 缺少 source_id 回 400
# ---------------------------------------------------------------------------
echo ""
bold "[3] 缺少 source_id 回 400"

CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "${API_BASE}/api/videos/import" \
    -H "Authorization: Bearer ${ADMIN_TOKEN}" \
    -H "Content-Type: application/json" \
    -d '{}')
assert_eq "缺少 source_id 回 400" "400" "$CODE"

# ---------------------------------------------------------------------------
# 4. source_id 不存在回 404
# ---------------------------------------------------------------------------
echo ""
bold "[4] source_id 不存在回 404"

CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "${API_BASE}/api/videos/import" \
    -H "Authorization: Bearer ${ADMIN_TOKEN}" \
    -H "Content-Type: application/json" \
    -d '{"source_id":"00000000-0000-0000-0000-000000000000"}')
assert_eq "不存在的 source_id 回 404" "404" "$CODE"

# ---------------------------------------------------------------------------
# 5. 第一次匯入
# ---------------------------------------------------------------------------
echo ""
bold "[5] 第一次匯入"
yellow "  (可能需要一段時間...)"

IMPORT_RESP=$(curl -s -X POST "${API_BASE}/api/videos/import" \
    -H "Authorization: Bearer ${ADMIN_TOKEN}" \
    -H "Content-Type: application/json" \
    -d "{\"source_id\":\"${SOURCE_ID}\"}" \
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
# 6. 冪等性 — 再次匯入應全部 skip
# ---------------------------------------------------------------------------
echo ""
bold "[6] 冪等性測試"

IMPORT2_RESP=$(curl -s -X POST "${API_BASE}/api/videos/import" \
    -H "Authorization: Bearer ${ADMIN_TOKEN}" \
    -H "Content-Type: application/json" \
    -d "{\"source_id\":\"${SOURCE_ID}\"}" \
    --max-time 3600)

IMPORTED2=$(echo "$IMPORT2_RESP" | jq -r '.data.imported // -1')
SKIPPED2=$(echo "$IMPORT2_RESP" | jq -r '.data.skipped // 0')

assert_eq "再次匯入 imported == 0" "0" "$IMPORTED2"
assert_eq "再次匯入 skipped == total_scanned" "$TOTAL_SCANNED" "$SKIPPED2"

# ---------------------------------------------------------------------------
# 7. 回應格式驗證
# ---------------------------------------------------------------------------
echo ""
bold "[7] 回應格式"

HAS_DATA=$(echo "$IMPORT_RESP" | jq 'has("data")')
assert_eq "有 data 欄位" "true" "$HAS_DATA"

for field in total_scanned imported skipped failed; do
    HAS=$(echo "$IMPORT_RESP" | jq ".data | has(\"$field\")")
    assert_eq "data 包含 $field" "true" "$HAS"
done

# ---------------------------------------------------------------------------
# 8. 匯入的影片有 source_id 和 file_path
# ---------------------------------------------------------------------------
echo ""
bold "[8] 影片記錄包含 source_id 和 file_path"

VIDEOS_RESP=$(curl -s -X GET "${API_BASE}/api/videos?page_size=1" \
    -H "Authorization: Bearer ${ADMIN_TOKEN}")

FIRST_VIDEO_SOURCE=$(echo "$VIDEOS_RESP" | jq -r '.data[0].source_id // empty')
FIRST_VIDEO_PATH=$(echo "$VIDEOS_RESP" | jq -r '.data[0].file_path // empty')

assert_not_empty "影片有 source_id" "$FIRST_VIDEO_SOURCE"
assert_not_empty "影片有 file_path" "$FIRST_VIDEO_PATH"
assert_eq "source_id 符合" "$SOURCE_ID" "$FIRST_VIDEO_SOURCE"

# ---------------------------------------------------------------------------
print_summary "Import 影片匯入"
exit $?
