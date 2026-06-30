#!/bin/bash
# =============================================================================
# Integration test: mkv/h264/aac remux → HLS playback chain
# 測試項目:
#   1. 確認 sample_h264.mkv 已匯入且 play_mode == "remux"
#   2. 取得 stream token
#   3. GET /api/videos/:id/hls/index.m3u8?token= → 200 且 body 含 #EXTM3U
#   4. 從 playlist 解出第一個 segment，GET /api/videos/:id/hls/:segment?token= → 200
# 前置條件: 先跑 test_import.sh（sample_h264.mkv 在 /mnt/host/videos）
# =============================================================================

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/test_helpers.sh"

bold "=== HLS remux 播放鏈整合測試 ==="
check_prerequisites

ADMIN_TOKEN=$(login_as "$ADMIN_USER" "$ADMIN_PASS")

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
# [4] GET HLS playlist — 200 + body 含 #EXTM3U
# =====================================================================
echo ""
bold "[4] GET /api/videos/:id/hls/index.m3u8 — 200 + #EXTM3U"

PLAYLIST_CODE=$(curl -s -o /tmp/hls_playlist.m3u8 -w "%{http_code}" \
    "${API_BASE}/api/videos/${MKV_ID}/hls/index.m3u8?token=${STREAM_TOKEN}")
assert_eq "HLS playlist 回 200" "200" "$PLAYLIST_CODE"

PLAYLIST_BODY=$(cat /tmp/hls_playlist.m3u8)
assert_contains "playlist 包含 #EXTM3U" "$PLAYLIST_BODY" "#EXTM3U"

# =====================================================================
# [5] 解出第一個 segment 並 GET 它 → 200
# =====================================================================
echo ""
bold "[5] GET 第一個 HLS segment → 200"

# 從 playlist 取第一個 seg*.ts 行（playlist 可能是 event 型，會持續更新）
# 給 ffmpeg 最多 10s 產出第一個 segment，輪詢直到解析到 segment 名稱
FIRST_SEG=""
for i in $(seq 1 20); do
    PLAYLIST_NOW=$(curl -s \
        "${API_BASE}/api/videos/${MKV_ID}/hls/index.m3u8?token=${STREAM_TOKEN}" 2>/dev/null || echo "")
    FIRST_SEG=$(echo "$PLAYLIST_NOW" | grep -E '^seg[0-9]{5}\.ts$' | head -1 || echo "")
    if [ -n "$FIRST_SEG" ]; then
        break
    fi
    sleep 0.5
done

assert_not_empty "playlist 包含至少一個 segment 行" "$FIRST_SEG"

SEG_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
    "${API_BASE}/api/videos/${MKV_ID}/hls/${FIRST_SEG}?token=${STREAM_TOKEN}")
assert_eq "第一個 segment (${FIRST_SEG}) 回 200" "200" "$SEG_CODE"

# ---------------------------------------------------------------------------
print_summary "HLS remux 播放鏈"
exit $?
