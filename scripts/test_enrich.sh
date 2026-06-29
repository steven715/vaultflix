#!/bin/bash
# =============================================================================
# Integration test: enrichment backfill with NULL code (regression for 500 bug)
# 測試項目:
#   1. 直接插入一筆 code=NULL 的 legacy 影片 (模擬 migration-013 前的舊資料)
#   2. 呼叫 POST /api/enrich-jobs/backfill-codes 確認 HTTP 200 (不再 500)
#   3. 確認 seeded >= 1 (legacy 行被 seed 成 pending 並寫入 code)
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
# 2. 呼叫 backfill-codes，應回 HTTP 200（修復前會 500）
# ---------------------------------------------------------------------------
echo ""
bold "[2] POST /api/enrich-jobs/backfill-codes 回 200"

BACKFILL_HTTP=$(curl -s -o /tmp/backfill_body.json -w "%{http_code}" -X POST \
    "${API_BASE}/api/enrich-jobs/backfill-codes" \
    -H "Authorization: Bearer ${ADMIN_TOKEN}")
BACKFILL_BODY=$(cat /tmp/backfill_body.json)

assert_eq "backfill-codes 回 200 (not 500)" "200" "$BACKFILL_HTTP"

# ---------------------------------------------------------------------------
# 3. 回應 JSON 含 seeded >= 1
# ---------------------------------------------------------------------------
echo ""
bold "[3] 回應 seeded >= 1"

SEEDED=$(echo "$BACKFILL_BODY" | jq -r '.data.seeded // .seeded // 0' 2>/dev/null || echo "0")
assert_gte "seeded >= 1" 1 "$SEEDED"

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
# 5. 清除測試資料，避免污染其他 suite（legacy 影片無 source_id，會讓 import 的
#    data[0] 檢查誤以為影片沒有 source_id）
# ---------------------------------------------------------------------------
echo ""
bold "[5] 清除 legacy 測試影片"

psql "$DB_DSN" -q -c \
    "DELETE FROM videos WHERE minio_object_key = 'legacy/DASD-700.mp4';" 2>/dev/null && \
    green "  清除完成" || yellow "  清除失敗（非致命，不影響測試結果）"

# ---------------------------------------------------------------------------
print_summary "Enrichment backfill (NULL code regression)"
exit $?
