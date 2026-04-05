#!/bin/bash
# =============================================================================
# Integration test: 影片瀏覽 API
# 測試項目: 列表分頁/排序/搜尋、影片詳情、更新、刪除、RBAC
# 前置條件: DB 中需有已匯入的影片（先跑 test_import.sh）
# =============================================================================

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/test_helpers.sh"

bold "=== Videos 影片瀏覽 API 測試 ==="
check_prerequisites

ADMIN_TOKEN=$(login_as "$ADMIN_USER" "$ADMIN_PASS")

# 準備 viewer 帳號
register_user "test_videos_viewer" "test1234" >/dev/null 2>&1 || true
VIEWER_TOKEN=$(login_as "test_videos_viewer" "test1234")

# =====================================================================
# GET /api/videos — 列表
# =====================================================================

echo ""
bold "[1] GET /api/videos — 預設分頁"

RESP=$(curl -s "${API_BASE}/api/videos" -H "Authorization: Bearer ${VIEWER_TOKEN}")

PAGE=$(echo "$RESP" | jq -r '.page')
PAGE_SIZE=$(echo "$RESP" | jq -r '.page_size')
TOTAL=$(echo "$RESP" | jq -r '.total')
DATA_LEN=$(echo "$RESP" | jq '.data | length')

assert_eq "page == 1" "1" "$PAGE"
assert_eq "page_size == 20" "20" "$PAGE_SIZE"
assert_gte "total >= 1（需有已匯入影片）" 1 "$TOTAL"
assert_gte "data 有資料" 1 "$DATA_LEN"

# 驗證每筆影片帶有 tags 欄位（即使是空陣列）
FIRST_HAS_TAGS=$(echo "$RESP" | jq '.data[0] | has("tags")')
assert_eq "影片項目包含 tags 欄位" "true" "$FIRST_HAS_TAGS"

# ---------------------------------------------------------------------------
echo ""
bold "[2] GET /api/videos — 自訂分頁"

RESP=$(curl -s "${API_BASE}/api/videos?page=1&page_size=2" \
    -H "Authorization: Bearer ${VIEWER_TOKEN}")
DATA_LEN=$(echo "$RESP" | jq '.data | length')
RET_PAGE_SIZE=$(echo "$RESP" | jq -r '.page_size')

assert_eq "page_size == 2" "2" "$RET_PAGE_SIZE"
assert_gte "data length >= 1" 1 "$DATA_LEN"
# data length 不應超過 page_size
_TOTAL=$((_TOTAL + 1))
if [ "$DATA_LEN" -le 2 ]; then
    green "  [PASS] data length <= page_size"
    _PASS=$((_PASS + 1))
else
    red "  [FAIL] data length ($DATA_LEN) > page_size (2)"
    _FAIL=$((_FAIL + 1))
fi

# ---------------------------------------------------------------------------
echo ""
bold "[3] GET /api/videos — 排序"

RESP=$(curl -s "${API_BASE}/api/videos?sort_by=title&sort_order=asc&page_size=5" \
    -H "Authorization: Bearer ${VIEWER_TOKEN}")
CODE_CHECK=$(echo "$RESP" | jq -r '.page // empty')
assert_not_empty "sort_by=title&sort_order=asc 回應正常" "$CODE_CHECK"

# ---------------------------------------------------------------------------
echo ""
bold "[4] GET /api/videos — 無效參數驗證"

CODE=$(curl -s -o /dev/null -w "%{http_code}" "${API_BASE}/api/videos?page_size=999" \
    -H "Authorization: Bearer ${VIEWER_TOKEN}")
assert_eq "page_size=999 回 400" "400" "$CODE"

CODE=$(curl -s -o /dev/null -w "%{http_code}" "${API_BASE}/api/videos?sort_by=invalid" \
    -H "Authorization: Bearer ${VIEWER_TOKEN}")
assert_eq "sort_by=invalid 回 400" "400" "$CODE"

CODE=$(curl -s -o /dev/null -w "%{http_code}" "${API_BASE}/api/videos?tag_ids=abc" \
    -H "Authorization: Bearer ${VIEWER_TOKEN}")
assert_eq "tag_ids=abc 回 400" "400" "$CODE"

# =====================================================================
# GET /api/videos/:id — 詳情
# =====================================================================

echo ""
bold "[5] GET /api/videos/:id — 影片詳情"

# 取第一筆影片的 ID
VIDEO_ID=$(curl -s "${API_BASE}/api/videos?page_size=1" \
    -H "Authorization: Bearer ${VIEWER_TOKEN}" | jq -r '.data[0].id')
assert_not_empty "取得影片 ID" "$VIDEO_ID"

RESP=$(curl -s "${API_BASE}/api/videos/${VIDEO_ID}" \
    -H "Authorization: Bearer ${VIEWER_TOKEN}")

DETAIL_ID=$(echo "$RESP" | jq -r '.data.id')
STREAM_URL=$(echo "$RESP" | jq -r '.data.stream_url // empty')
HAS_TAGS=$(echo "$RESP" | jq '.data | has("tags")')

assert_eq "回傳的 id 一致" "$VIDEO_ID" "$DETAIL_ID"
assert_not_empty "stream_url 有值（pre-signed URL）" "$STREAM_URL"
assert_eq "包含 tags 欄位" "true" "$HAS_TAGS"

# ---------------------------------------------------------------------------
echo ""
bold "[6] GET /api/videos/:id — url_expiry_minutes 參數"

RESP=$(curl -s "${API_BASE}/api/videos/${VIDEO_ID}?url_expiry_minutes=30" \
    -H "Authorization: Bearer ${VIEWER_TOKEN}")
STREAM_URL2=$(echo "$RESP" | jq -r '.data.stream_url // empty')
assert_not_empty "url_expiry_minutes=30 回應有 stream_url" "$STREAM_URL2"

CODE=$(curl -s -o /dev/null -w "%{http_code}" "${API_BASE}/api/videos/${VIDEO_ID}?url_expiry_minutes=9999" \
    -H "Authorization: Bearer ${VIEWER_TOKEN}")
assert_eq "url_expiry_minutes=9999 回 400" "400" "$CODE"

# ---------------------------------------------------------------------------
echo ""
bold "[7] GET /api/videos/:id — 不存在的影片"

CODE=$(curl -s -o /dev/null -w "%{http_code}" \
    "${API_BASE}/api/videos/00000000-0000-0000-0000-000000000000" \
    -H "Authorization: Bearer ${VIEWER_TOKEN}")
assert_eq "不存在的影片回 404" "404" "$CODE"

# =====================================================================
# PUT /api/videos/:id — 更新
# =====================================================================

echo ""
bold "[8] PUT /api/videos/:id — 更新影片 metadata"

ORIGINAL_TITLE=$(curl -s "${API_BASE}/api/videos/${VIDEO_ID}" \
    -H "Authorization: Bearer ${ADMIN_TOKEN}" | jq -r '.data.title')

RESP=$(curl -s -w "\n%{http_code}" -X PUT "${API_BASE}/api/videos/${VIDEO_ID}" \
    -H "Authorization: Bearer ${ADMIN_TOKEN}" \
    -H "Content-Type: application/json" \
    -d '{"title":"Integration Test Updated Title","description":"Updated by test"}')
CODE=$(echo "$RESP" | tail -1)
BODY=$(echo "$RESP" | sed '$d')

assert_eq "更新回 200" "200" "$CODE"

UPDATED_TITLE=$(echo "$BODY" | jq -r '.data.title // empty')
assert_eq "title 已更新" "Integration Test Updated Title" "$UPDATED_TITLE"

# 還原 title
curl -s -o /dev/null -X PUT "${API_BASE}/api/videos/${VIDEO_ID}" \
    -H "Authorization: Bearer ${ADMIN_TOKEN}" \
    -H "Content-Type: application/json" \
    -d "{\"title\":\"${ORIGINAL_TITLE}\",\"description\":\"\"}"

# ---------------------------------------------------------------------------
echo ""
bold "[9] PUT /api/videos/:id — RBAC viewer 不能更新"

CODE=$(curl -s -o /dev/null -w "%{http_code}" -X PUT "${API_BASE}/api/videos/${VIDEO_ID}" \
    -H "Authorization: Bearer ${VIEWER_TOKEN}" \
    -H "Content-Type: application/json" \
    -d '{"title":"Hacked"}')
assert_eq "viewer 更新回 403" "403" "$CODE"

# =====================================================================
# DELETE /api/videos/:id — 刪除（用最後一筆影片測試）
# =====================================================================

echo ""
bold "[10] DELETE /api/videos/:id — 刪除影片"

# 取最後一筆影片（避免刪掉其他測試需要的資料）
LAST_PAGE_RESP=$(curl -s "${API_BASE}/api/videos?sort_by=created_at&sort_order=desc&page_size=1" \
    -H "Authorization: Bearer ${ADMIN_TOKEN}")
DELETE_VIDEO_ID=$(echo "$LAST_PAGE_RESP" | jq -r '.data[0].id')
TOTAL_BEFORE=$(echo "$LAST_PAGE_RESP" | jq -r '.total')

assert_not_empty "取得待刪除影片 ID" "$DELETE_VIDEO_ID"

# viewer 不能刪
CODE=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE "${API_BASE}/api/videos/${DELETE_VIDEO_ID}" \
    -H "Authorization: Bearer ${VIEWER_TOKEN}")
assert_eq "viewer 刪除回 403" "403" "$CODE"

# admin 可以刪
CODE=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE "${API_BASE}/api/videos/${DELETE_VIDEO_ID}" \
    -H "Authorization: Bearer ${ADMIN_TOKEN}")
assert_eq "admin 刪除回 204" "204" "$CODE"

# 刪除後查詢回 404
CODE=$(curl -s -o /dev/null -w "%{http_code}" "${API_BASE}/api/videos/${DELETE_VIDEO_ID}" \
    -H "Authorization: Bearer ${ADMIN_TOKEN}")
assert_eq "刪除後查詢回 404" "404" "$CODE"

# 總數減少
TOTAL_AFTER=$(curl -s "${API_BASE}/api/videos?page_size=1" \
    -H "Authorization: Bearer ${ADMIN_TOKEN}" | jq -r '.total')
EXPECTED_TOTAL=$((TOTAL_BEFORE - 1))
assert_eq "total 減少 1" "$EXPECTED_TOTAL" "$TOTAL_AFTER"

# ---------------------------------------------------------------------------
print_summary "Videos 影片瀏覽 API"
exit $?
