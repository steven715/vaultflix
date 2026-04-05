#!/bin/bash
# =============================================================================
# Integration test: 標籤管理 API
# 測試項目: 標籤 CRUD、影片標籤關聯、category 篩選、RBAC
# 前置條件: DB 中需有已匯入的影片（先跑 test_import.sh）
# =============================================================================

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/test_helpers.sh"

bold "=== Tags 標籤管理 API 測試 ==="
check_prerequisites

ADMIN_TOKEN=$(login_as "$ADMIN_USER" "$ADMIN_PASS")

register_user "test_tags_viewer" "test1234" >/dev/null 2>&1 || true
VIEWER_TOKEN=$(login_as "test_tags_viewer" "test1234")

# =====================================================================
# POST /api/tags — 建立標籤
# =====================================================================

echo ""
bold "[1] POST /api/tags — 建立標籤"

# 使用 timestamp 避免重複執行衝突
TS=$(date +%s)
TAG_NAME="inttest_action_${TS}"

RESP=$(curl -s -w "\n%{http_code}" -X POST "${API_BASE}/api/tags" \
    -H "Authorization: Bearer ${ADMIN_TOKEN}" \
    -H "Content-Type: application/json" \
    -d "{\"name\":\"${TAG_NAME}\",\"category\":\"genre\"}")
CODE=$(echo "$RESP" | tail -1)
BODY=$(echo "$RESP" | sed '$d')

assert_eq "建立標籤回 201" "201" "$CODE"

TAG_ID=$(echo "$BODY" | jq -r '.data.id // empty')
assert_not_empty "回傳 tag id" "$TAG_ID"

TAG_CATEGORY=$(echo "$BODY" | jq -r '.data.category // empty')
assert_eq "category 為 genre" "genre" "$TAG_CATEGORY"

# 建立第二個標籤（不同 category）
TAG_NAME2="inttest_studio_${TS}"
RESP2=$(curl -s -X POST "${API_BASE}/api/tags" \
    -H "Authorization: Bearer ${ADMIN_TOKEN}" \
    -H "Content-Type: application/json" \
    -d "{\"name\":\"${TAG_NAME2}\",\"category\":\"studio\"}")
TAG_ID2=$(echo "$RESP2" | jq -r '.data.id // empty')

# ---------------------------------------------------------------------------
echo ""
bold "[2] POST /api/tags — name 重複回 409"

CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "${API_BASE}/api/tags" \
    -H "Authorization: Bearer ${ADMIN_TOKEN}" \
    -H "Content-Type: application/json" \
    -d "{\"name\":\"${TAG_NAME}\",\"category\":\"genre\"}")
assert_eq "重複 name 回 409" "409" "$CODE"

# ---------------------------------------------------------------------------
echo ""
bold "[3] POST /api/tags — 預設 category 為 custom"

TAG_NAME3="inttest_custom_${TS}"
RESP=$(curl -s -X POST "${API_BASE}/api/tags" \
    -H "Authorization: Bearer ${ADMIN_TOKEN}" \
    -H "Content-Type: application/json" \
    -d "{\"name\":\"${TAG_NAME3}\"}")
CATEGORY=$(echo "$RESP" | jq -r '.data.category // empty')
assert_eq "未帶 category 預設 custom" "custom" "$CATEGORY"

# ---------------------------------------------------------------------------
echo ""
bold "[4] POST /api/tags — RBAC viewer 不能建立"

CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "${API_BASE}/api/tags" \
    -H "Authorization: Bearer ${VIEWER_TOKEN}" \
    -H "Content-Type: application/json" \
    -d '{"name":"viewer_should_fail"}')
assert_eq "viewer 建立標籤回 403" "403" "$CODE"

# =====================================================================
# GET /api/tags — 標籤列表
# =====================================================================

echo ""
bold "[5] GET /api/tags — 全部列表"

RESP=$(curl -s "${API_BASE}/api/tags" -H "Authorization: Bearer ${VIEWER_TOKEN}")
DATA_LEN=$(echo "$RESP" | jq '.data | length')
assert_gte "至少有我們建立的標籤" 1 "$DATA_LEN"

# 驗證每個 tag 帶有 video_count 欄位
HAS_VC=$(echo "$RESP" | jq '.data[0] | has("video_count")')
assert_eq "tag 包含 video_count 欄位" "true" "$HAS_VC"

# ---------------------------------------------------------------------------
echo ""
bold "[6] GET /api/tags?category=genre — 按 category 篩選"

RESP=$(curl -s "${API_BASE}/api/tags?category=genre" \
    -H "Authorization: Bearer ${VIEWER_TOKEN}")
ALL_GENRE=$(echo "$RESP" | jq '[.data[].category] | unique')
# 所有結果應該都是 genre
_TOTAL=$((_TOTAL + 1))
if echo "$ALL_GENRE" | jq -e '. == ["genre"]' >/dev/null 2>&1; then
    green "  [PASS] 所有結果 category 都是 genre"
    _PASS=$((_PASS + 1))
else
    red "  [FAIL] 包含非 genre 的結果: $ALL_GENRE"
    _FAIL=$((_FAIL + 1))
fi

# ---------------------------------------------------------------------------
echo ""
bold "[7] GET /api/tags?category=invalid — 無效 category"

CODE=$(curl -s -o /dev/null -w "%{http_code}" "${API_BASE}/api/tags?category=invalid" \
    -H "Authorization: Bearer ${VIEWER_TOKEN}")
assert_eq "無效 category 回 400" "400" "$CODE"

# =====================================================================
# POST /api/videos/:id/tags — 為影片加標籤
# =====================================================================

echo ""
bold "[8] POST /api/videos/:id/tags — 加標籤"

# 取一筆影片
VIDEO_ID=$(curl -s "${API_BASE}/api/videos?page_size=1" \
    -H "Authorization: Bearer ${ADMIN_TOKEN}" | jq -r '.data[0].id')
assert_not_empty "取得影片 ID" "$VIDEO_ID"

RESP=$(curl -s -w "\n%{http_code}" -X POST "${API_BASE}/api/videos/${VIDEO_ID}/tags" \
    -H "Authorization: Bearer ${ADMIN_TOKEN}" \
    -H "Content-Type: application/json" \
    -d "{\"tag_id\":${TAG_ID}}")
CODE=$(echo "$RESP" | tail -1)
assert_eq "加標籤回 201" "201" "$CODE"

# ---------------------------------------------------------------------------
echo ""
bold "[9] POST /api/videos/:id/tags — 重複加標籤（冪等）"

RESP=$(curl -s -w "\n%{http_code}" -X POST "${API_BASE}/api/videos/${VIDEO_ID}/tags" \
    -H "Authorization: Bearer ${ADMIN_TOKEN}" \
    -H "Content-Type: application/json" \
    -d "{\"tag_id\":${TAG_ID}}")
CODE=$(echo "$RESP" | tail -1)
assert_eq "重複加標籤回 200（冪等）" "200" "$CODE"

# ---------------------------------------------------------------------------
echo ""
bold "[10] 驗證影片詳情包含 tag"

RESP=$(curl -s "${API_BASE}/api/videos/${VIDEO_ID}" \
    -H "Authorization: Bearer ${ADMIN_TOKEN}")
TAG_COUNT=$(echo "$RESP" | jq '.data.tags | length')
assert_gte "影片至少有 1 個 tag" 1 "$TAG_COUNT"

# 驗證 tag 列表的 video_count 增加
RESP=$(curl -s "${API_BASE}/api/tags?category=genre" \
    -H "Authorization: Bearer ${ADMIN_TOKEN}")
VC=$(echo "$RESP" | jq --arg id "$TAG_ID" '[.data[] | select(.id == ($id | tonumber))][0].video_count // 0')
assert_gte "tag 的 video_count >= 1" 1 "$VC"

# ---------------------------------------------------------------------------
echo ""
bold "[11] POST /api/videos/:id/tags — 影片不存在回 404"

CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST \
    "${API_BASE}/api/videos/00000000-0000-0000-0000-000000000000/tags" \
    -H "Authorization: Bearer ${ADMIN_TOKEN}" \
    -H "Content-Type: application/json" \
    -d "{\"tag_id\":${TAG_ID}}")
assert_eq "影片不存在回 404" "404" "$CODE"

# ---------------------------------------------------------------------------
echo ""
bold "[12] POST /api/videos/:id/tags — 標籤不存在回 404"

CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST \
    "${API_BASE}/api/videos/${VIDEO_ID}/tags" \
    -H "Authorization: Bearer ${ADMIN_TOKEN}" \
    -H "Content-Type: application/json" \
    -d '{"tag_id":999999}')
assert_eq "標籤不存在回 404" "404" "$CODE"

# ---------------------------------------------------------------------------
echo ""
bold "[13] POST /api/videos/:id/tags — RBAC viewer 不能加標籤"

CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST \
    "${API_BASE}/api/videos/${VIDEO_ID}/tags" \
    -H "Authorization: Bearer ${VIEWER_TOKEN}" \
    -H "Content-Type: application/json" \
    -d "{\"tag_id\":${TAG_ID}}")
assert_eq "viewer 加標籤回 403" "403" "$CODE"

# =====================================================================
# GET /api/videos?tag_ids= — 按標籤篩選影片
# =====================================================================

echo ""
bold "[14] GET /api/videos?tag_ids= — 按標籤篩選"

RESP=$(curl -s "${API_BASE}/api/videos?tag_ids=${TAG_ID}" \
    -H "Authorization: Bearer ${VIEWER_TOKEN}")
FILTERED_TOTAL=$(echo "$RESP" | jq -r '.total')
assert_gte "篩選結果 total >= 1" 1 "$FILTERED_TOTAL"

# =====================================================================
# DELETE /api/videos/:id/tags/:tagId — 移除標籤
# =====================================================================

echo ""
bold "[15] DELETE /api/videos/:id/tags/:tagId — 移除標籤"

CODE=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE \
    "${API_BASE}/api/videos/${VIDEO_ID}/tags/${TAG_ID}" \
    -H "Authorization: Bearer ${ADMIN_TOKEN}")
assert_eq "移除標籤回 204" "204" "$CODE"

# ---------------------------------------------------------------------------
echo ""
bold "[16] DELETE /api/videos/:id/tags/:tagId — 已移除再刪回 404"

CODE=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE \
    "${API_BASE}/api/videos/${VIDEO_ID}/tags/${TAG_ID}" \
    -H "Authorization: Bearer ${ADMIN_TOKEN}")
assert_eq "重複移除回 404" "404" "$CODE"

# ---------------------------------------------------------------------------
echo ""
bold "[17] DELETE /api/videos/:id/tags/:tagId — RBAC viewer 不能移除"

# 先加回去再測 viewer 移除
curl -s -o /dev/null -X POST "${API_BASE}/api/videos/${VIDEO_ID}/tags" \
    -H "Authorization: Bearer ${ADMIN_TOKEN}" \
    -H "Content-Type: application/json" \
    -d "{\"tag_id\":${TAG_ID}}"

CODE=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE \
    "${API_BASE}/api/videos/${VIDEO_ID}/tags/${TAG_ID}" \
    -H "Authorization: Bearer ${VIEWER_TOKEN}")
assert_eq "viewer 移除標籤回 403" "403" "$CODE"

# 清理：移除測試用的 tag
curl -s -o /dev/null -X DELETE "${API_BASE}/api/videos/${VIDEO_ID}/tags/${TAG_ID}" \
    -H "Authorization: Bearer ${ADMIN_TOKEN}"

# ---------------------------------------------------------------------------
print_summary "Tags 標籤管理 API"
exit $?
