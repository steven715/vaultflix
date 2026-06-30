#!/bin/bash
# =============================================================================
# Integration test: mkv/h264/aac remux → HLS playback chain
# 測試項目:
#   1. 確認 sample_h264.mkv 已匯入且 play_mode == "remux"
#   2. 取得 stream token
#   3. GET /api/videos/:id/hls/seg00000.ts（無 token）→ 401
#   4. GET /api/videos/:id/hls/index.m3u8?token= → 200 且 body 含 #EXTM3U
#   5. 確認 playlist 的 segment 行含有 token= 參數
#   6. 從 playlist 取出含 token 的 segment URI，GET → 200
# 前置條件: 先跑 test_import.sh（sample_h264.mkv 在 /mnt/host/videos）
# =============================================================================

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/test_helpers.sh"

IMPORT_DIR="${IMPORT_DIR:-/mnt/host/videos}"
SOURCE_LABEL="${SOURCE_LABEL:-integration-test}"

bold "=== HLS remux 播放鏈整合測試 ==="
check_prerequisites

ADMIN_TOKEN=$(login_as "$ADMIN_USER" "$ADMIN_PASS")

# =====================================================================
# 前置準備：確保 sample_h264.mkv 已匯入（冪等；補救 test_videos.sh 可能刪掉它）
# =====================================================================
echo ""
bold "[pre] 確保 sample_h264.mkv 已匯入（自我佈建）"

EXISTING_SOURCES=$(curl -s "${API_BASE}/api/media-sources" \
    -H "Authorization: Bearer ${ADMIN_TOKEN}")
SOURCE_ID=$(echo "$EXISTING_SOURCES" | jq -r \
    ".data[] | select(.mount_path == \"${IMPORT_DIR}\") | .id // empty" 2>/dev/null | head -1)

if [ -z "$SOURCE_ID" ]; then
    yellow "  media source 不存在，建立中..."
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
else
    yellow "  使用既有 media source: ${SOURCE_ID}"
fi

yellow "  觸發匯入（冪等，已存在的檔案會 skip）..."
run_import "$ADMIN_TOKEN" "$SOURCE_ID" >/dev/null
green "  匯入完成（或全部跳過）"

# =====================================================================
# [1] 找到已匯入的 sample_h264.mkv — 靠 original_filename 過濾
# =====================================================================
echo ""
bold "[1] 找到已匯入的 sample_h264.mkv"

# 列出所有影片，找 original_filename 含 sample_h264.mkv 的那筆
MKV_ID=$(curl -s "${API_BASE}/api/videos?page_size=100" \
    -H "Authorization: Bearer ${ADMIN_TOKEN}" \
    | jq -r '.data[] | select(.original_filename | test("sample_h264\\.mkv")) | .id' \
    | head -1)

assert_not_empty "找到 sample_h264.mkv 的影片 ID" "$MKV_ID"

# =====================================================================
# [2] 確認 play_mode == "remux"
# =====================================================================
echo ""
bold "[2] GET /api/videos/:id — play_mode == remux"

DETAIL_RESP=$(curl -s "${API_BASE}/api/videos/${MKV_ID}" \
    -H "Authorization: Bearer ${ADMIN_TOKEN}")

PLAY_MODE=$(echo "$DETAIL_RESP" | jq -r '.data.play_mode // empty')
assert_eq "play_mode 為 remux" "remux" "$PLAY_MODE"

# =====================================================================
# [3] 取得 stream token
# =====================================================================
echo ""
bold "[3] GET /api/videos/:id/stream-token — 取得 stream token"

TOKEN_RESP=$(curl -s "${API_BASE}/api/videos/${MKV_ID}/stream-token" \
    -H "Authorization: Bearer ${ADMIN_TOKEN}")

STREAM_TOKEN=$(echo "$TOKEN_RESP" | jq -r '.data.token // empty')
assert_not_empty "stream token 不為空" "$STREAM_TOKEN"

# =====================================================================
# [4] 無 token 請求 segment → 401
# =====================================================================
echo ""
bold "[4] GET /api/videos/:id/hls/seg00000.ts（無 token）→ 401"

NO_TOKEN_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
    "${API_BASE}/api/videos/${MKV_ID}/hls/seg00000.ts")
assert_eq "無 token 請求 segment 回 401" "401" "$NO_TOKEN_CODE"

# =====================================================================
# [5] GET HLS playlist — 200 + body 含 #EXTM3U
# =====================================================================
echo ""
bold "[5] GET /api/videos/:id/hls/index.m3u8 — 200 + #EXTM3U"

PLAYLIST_CODE=$(curl -s -o /tmp/hls_playlist.m3u8 -w "%{http_code}" \
    "${API_BASE}/api/videos/${MKV_ID}/hls/index.m3u8?token=${STREAM_TOKEN}")
assert_eq "HLS playlist 回 200" "200" "$PLAYLIST_CODE"

PLAYLIST_BODY=$(cat /tmp/hls_playlist.m3u8)
assert_contains "playlist 包含 #EXTM3U" "$PLAYLIST_BODY" "#EXTM3U"

# =====================================================================
# [6] 確認 playlist 的 segment 行含有 token= 參數
#     輪詢直到 playlist 包含至少一個 segment 行
# =====================================================================
echo ""
bold "[6] 確認 playlist segment 行含有 token= 參數"

FIRST_SEG_URI=""
for i in $(seq 1 20); do
    PLAYLIST_NOW=$(curl -s \
        "${API_BASE}/api/videos/${MKV_ID}/hls/index.m3u8?token=${STREAM_TOKEN}" 2>/dev/null || echo "")
    # segment URI 行：含 token= 的 seg*.ts 行
    FIRST_SEG_URI=$(echo "$PLAYLIST_NOW" | grep -E '^seg[0-9]{5}\.ts\?token=' | head -1 || echo "")
    if [ -n "$FIRST_SEG_URI" ]; then
        break
    fi
    sleep 0.5
done

assert_not_empty "playlist 含有帶 token= 的 segment 行" "$FIRST_SEG_URI"

# =====================================================================
# [7] 用 playlist 中帶 token 的完整 URI 請求 segment → 200
# =====================================================================
echo ""
bold "[7] GET 帶 token 的 segment URI → 200"

# FIRST_SEG_URI 格式: seg00000.ts?token=<value>
# 需要拆出 segment 名稱和 query 部分
FIRST_SEG_NAME=$(echo "$FIRST_SEG_URI" | cut -d'?' -f1)
FIRST_SEG_QUERY=$(echo "$FIRST_SEG_URI" | cut -d'?' -f2-)

SEG_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
    "${API_BASE}/api/videos/${MKV_ID}/hls/${FIRST_SEG_NAME}?${FIRST_SEG_QUERY}")
assert_eq "第一個 segment (${FIRST_SEG_NAME}) 帶 token 回 200" "200" "$SEG_CODE"

# ---------------------------------------------------------------------------
print_summary "HLS remux 播放鏈"
exit $?
