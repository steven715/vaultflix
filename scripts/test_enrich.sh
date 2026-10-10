#!/bin/bash
# =============================================================================
# Integration test: enrichment backfill with NULL code (regression for 500 bug)
# 測試項目:
#   1. 直接插入一筆 code=NULL 的 legacy 影片 (模擬 migration-013 前的舊資料)
#   2. 啟動 Code Backfill Job (POST /api/admin/backfill-jobs {"kind":"code"}) 回 202
#   3. 輪詢到 job completed，且 succeeded >= 1、failed = 0 (legacy 行被 seed 成 pending 並寫入 code)
#   4. GET /api/videos/:id 帶出已存的 code（EnrichVideo 依賴此路徑）
# =============================================================================

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/test_helpers.sh"

bold "=== Enrichment backfill (NULL code regression) 測試 ==="
check_prerequisites

# ---------------------------------------------------------------------------
# 確認 psql 可用 (alpine test-runner 需要先 apk add)
# ---------------------------------------------------------------------------
if ! command -v psql &>/dev/null; then
    yellow "  psql 未安裝，嘗試 apk add postgresql-client ..."
    apk add --no-cache postgresql-client >/dev/null 2>&1 || {
        red "  無法安裝 postgresql-client，跳過 NULL-code 測試"
        exit 1
    }
fi

ADMIN_TOKEN=$(login_as "$ADMIN_USER" "$ADMIN_PASS")

# ---------------------------------------------------------------------------
# 1. 插入 legacy 影片：code 欄位刻意省略 → 資料庫存 NULL
#    original_filename 含番號，讓 backfill 能解析出 DASD-700
# ---------------------------------------------------------------------------
echo ""
bold "[1] 插入 code=NULL 的 legacy 影片"

DB_DSN="postgres://${DB_USER}:${DB_PASSWORD}@postgres:5432/${DB_NAME}?sslmode=disable"

# Use a separate SELECT after INSERT to avoid RETURNING output parsing issues
psql "$DB_DSN" -q -c \
    "INSERT INTO videos (title, original_filename, minio_object_key, file_size_bytes, mime_type, enrichment_status)
     VALUES ('legacy-null-code', 'DASD-700.mp4', 'legacy/DASD-700.mp4', 1, 'video/mp4', 'none');" 2>&1 || {
    red "  插入 legacy 影片失敗"
    exit 1
}

# Fetch the just-inserted row by unique minio_object_key
LEGACY_ID=$(psql "$DB_DSN" -t -A -c \
    "SELECT id FROM videos WHERE minio_object_key = 'legacy/DASD-700.mp4' LIMIT 1;" 2>/dev/null | head -1 | tr -d '[:space:]')

if [ -z "$LEGACY_ID" ]; then
    red "  插入 legacy 影片後找不到 id"
    exit 1
fi
green "  插入 legacy 影片成功 id=${LEGACY_ID}"

# 確認 code 真的是 NULL
CODE_VAL=$(psql "$DB_DSN" -t -A -c \
    "SELECT code IS NULL FROM videos WHERE id = '${LEGACY_ID}';" 2>/dev/null | head -1 | tr -d '[:space:]')
assert_eq "legacy 影片 code IS NULL" "t" "$CODE_VAL"

# ---------------------------------------------------------------------------
# 2. 啟動 Code Backfill Job，應回 HTTP 202（修復前同步版會 500）
# ---------------------------------------------------------------------------
echo ""
bold "[2] POST /api/admin/backfill-jobs {kind: code} 回 202"

BACKFILL_HTTP=$(curl -s -o /tmp/backfill_body.json -w "%{http_code}" -X POST \
    "${API_BASE}/api/admin/backfill-jobs" \
    -H "Authorization: Bearer ${ADMIN_TOKEN}" \
    -H "Content-Type: application/json" \
    -d '{"kind":"code"}')
JOB_ID=$(jq -r '.data.job_id // empty' /tmp/backfill_body.json)

assert_eq "code backfill 回 202" "202" "$BACKFILL_HTTP"

# ---------------------------------------------------------------------------
# 3. 輪詢 job 直到結束：completed、succeeded >= 1、failed = 0
# ---------------------------------------------------------------------------
echo ""
bold "[3] Code Backfill Job completed 且 succeeded >= 1"

JOB_STATUS="running"
for _ in $(seq 1 50); do
    ACTIVE=$(curl -s "${API_BASE}/api/admin/backfill-jobs/active" -H "Authorization: Bearer ${ADMIN_TOKEN}")
    if [ "$(echo "$ACTIVE" | jq -r '.data.id // empty')" = "$JOB_ID" ]; then
        JOB_STATUS=$(echo "$ACTIVE" | jq -r '.data.status')
        [ "$JOB_STATUS" != "running" ] && break
    fi
    sleep 0.2
done

assert_eq "code backfill job completed" "completed" "$JOB_STATUS"
assert_gte "succeeded >= 1" 1 "$(echo "$ACTIVE" | jq -r '.data.succeeded // 0')"
assert_eq "failed = 0" "0" "$(echo "$ACTIVE" | jq -r '.data.failed // -1')"

# ---------------------------------------------------------------------------
# 4. DB 中 legacy 影片 code 已被寫入 DASD-700，enrichment_status 變 pending
# ---------------------------------------------------------------------------
echo ""
bold "[4] legacy 影片 code 已 seed 為 DASD-700"

CODE_AFTER=$(psql "$DB_DSN" -t -A -c \
    "SELECT code FROM videos WHERE id = '${LEGACY_ID}';" 2>/dev/null | head -1 | tr -d '[:space:]')
assert_eq "legacy 影片 code = DASD-700" "DASD-700" "$CODE_AFTER"

STATUS_AFTER=$(psql "$DB_DSN" -t -A -c \
    "SELECT enrichment_status FROM videos WHERE id = '${LEGACY_ID}';" 2>/dev/null | head -1 | tr -d '[:space:]')
assert_eq "legacy 影片 enrichment_status = pending" "pending" "$STATUS_AFTER"

# ---------------------------------------------------------------------------
# 5. GET /api/videos/:id 回傳已存的 code（EnrichVideo 靠 GetByID 讀 Code，
#    GetByID 漏 select code 時 enrichment 會退回解析檔名）
# ---------------------------------------------------------------------------
echo ""
bold "[5] GET /api/videos/:id 帶出已存的 code"

DETAIL_CODE=$(curl -s "${API_BASE}/api/videos/${LEGACY_ID}" \
    -H "Authorization: Bearer ${ADMIN_TOKEN}" | jq -r '.data.code // empty')
assert_eq "video detail code = DASD-700" "DASD-700" "$DETAIL_CODE"

# ---------------------------------------------------------------------------
# 6. 清除測試資料，避免污染其他 suite（legacy 影片無 source_id，會讓 import 的
#    data[0] 檢查誤以為影片沒有 source_id）
# ---------------------------------------------------------------------------
echo ""
bold "[6] 清除 legacy 測試影片"

psql "$DB_DSN" -q -c \
    "DELETE FROM videos WHERE minio_object_key = 'legacy/DASD-700.mp4';" 2>/dev/null && \
    green "  清除完成" || yellow "  清除失敗（非致命，不影響測試結果）"

# ---------------------------------------------------------------------------
print_summary "Enrichment backfill (NULL code regression)"
exit $?
