#!/bin/bash
# =============================================================================
# Integration test: 播放遙測 ingest → 聚合讀取
# 測試項目: POST /api/playback/telemetry (204)、同 session_id upsert 不重複計數、
#           非法 play_mode (400)、缺 video (404)、GET /api/admin/playback/telemetry
#           聚合、viewer 讀 admin 端點 RBAC (403)
# 用 before/after DELTA 斷言 sessions，對「重跑/共用 DB」具隔離性。
# =============================================================================

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/test_helpers.sh"

bold "=== Playback Telemetry 遙測 API 測試 ==="
check_prerequisites

ADMIN_TOKEN=$(login_as "$ADMIN_USER" "$ADMIN_PASS")
register_user "test_telemetry_viewer" "test1234" >/dev/null 2>&1 || true
VIEWER_TOKEN=$(login_as "test_telemetry_viewer" "test1234")

VIDEO_ID=$(ensure_video "$ADMIN_TOKEN")
assert_not_empty "取得影片 ID" "$VIDEO_ID"

SESSION_ID=$(cat /proc/sys/kernel/random/uuid)

# --- baseline: direct 分組的 sessions 數 ---
baseline_sessions() {
    curl -s "${API_BASE}/api/admin/playback/telemetry?days=7" \
        -H "Authorization: Bearer ${ADMIN_TOKEN}" \
        | grep -o '"play_mode":"direct","sessions":[0-9]*' | grep -o '[0-9]*$' || echo 0
}
BEFORE=$(baseline_sessions)
[ -z "$BEFORE" ] && BEFORE=0

echo ""
bold "[1] POST /api/playback/telemetry — viewer ingest 回 204"
CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "${API_BASE}/api/playback/telemetry" \
    -H "Authorization: Bearer ${VIEWER_TOKEN}" -H "Content-Type: application/json" \
    -d "{\"session_id\":\"${SESSION_ID}\",\"video_id\":\"${VIDEO_ID}\",\"play_mode\":\"direct\",\"ttff_ms\":900,\"watched_ms\":60000,\"rebuffer_count\":1,\"rebuffer_ms\":500}")
assert_eq "ingest 回 204" "204" "$CODE"

echo ""
bold "[2] 同 session_id 再送一次 — upsert 不新增列"
CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "${API_BASE}/api/playback/telemetry" \
    -H "Authorization: Bearer ${VIEWER_TOKEN}" -H "Content-Type: application/json" \
    -d "{\"session_id\":\"${SESSION_ID}\",\"video_id\":\"${VIDEO_ID}\",\"play_mode\":\"direct\",\"ttff_ms\":950,\"watched_ms\":65000}")
assert_eq "重送回 204" "204" "$CODE"

echo ""
bold "[3] 非法 play_mode 回 400"
CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "${API_BASE}/api/playback/telemetry" \
    -H "Authorization: Bearer ${VIEWER_TOKEN}" -H "Content-Type: application/json" \
    -d "{\"session_id\":\"$(cat /proc/sys/kernel/random/uuid)\",\"video_id\":\"${VIDEO_ID}\",\"play_mode\":\"bogus\"}")
assert_eq "非法 play_mode 回 400" "400" "$CODE"

echo ""
bold "[4] 缺 video 回 404"
CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "${API_BASE}/api/playback/telemetry" \
    -H "Authorization: Bearer ${VIEWER_TOKEN}" -H "Content-Type: application/json" \
    -d "{\"session_id\":\"$(cat /proc/sys/kernel/random/uuid)\",\"video_id\":\"$(cat /proc/sys/kernel/random/uuid)\",\"play_mode\":\"direct\"}")
assert_eq "缺 video 回 404" "404" "$CODE"

echo ""
bold "[5] GET /api/admin/playback/telemetry — 聚合含 direct，DELTA == 1"
RESP=$(curl -s -w "\n%{http_code}" "${API_BASE}/api/admin/playback/telemetry?days=7" \
    -H "Authorization: Bearer ${ADMIN_TOKEN}")
CODE=$(echo "$RESP" | tail -1)
assert_eq "聚合回 200" "200" "$CODE"
AFTER=$(baseline_sessions)
assert_eq "direct sessions DELTA == 1 (兩次同 session 只算一列)" "1" "$((AFTER - BEFORE))"

echo ""
bold "[6] viewer 讀 admin 端點回 403"
CODE=$(curl -s -o /dev/null -w "%{http_code}" "${API_BASE}/api/admin/playback/telemetry?days=7" \
    -H "Authorization: Bearer ${VIEWER_TOKEN}")
assert_eq "viewer 讀聚合回 403" "403" "$CODE"

# ---------------------------------------------------------------------------
print_summary "Playback Telemetry 遙測 API"
exit $?
