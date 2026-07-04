#!/bin/bash
# =============================================================================
# Integration test: 觀看紀錄 heartbeat → 分析儀表板 API
# 測試項目: heartbeat 累積(同一 session_id 兩次心跳只算一次 view)、
#           GET /api/admin/analytics 的 total_views/daily_trend/top_videos、RBAC
# 前置條件: DB 中需有已匯入的影片（沒有的話 ensure_video 會自動匯入 fixture）
#
# 用 before/after 的 DELTA 而非絕對值斷言 total_views，因為 DB 在重跑套件時
# 可能已累積過往測試留下的 views；DELTA 對「重跑/共用 DB」具備隔離性，同時仍然
# 證明了 heartbeat 有真的累積進 analytics（兩次同 session 心跳只新增 1 次 view）。
# =============================================================================

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/test_helpers.sh"

bold "=== Analytics 觀看紀錄與分析 API 測試 ==="
check_prerequisites

ADMIN_TOKEN=$(login_as "$ADMIN_USER" "$ADMIN_PASS")

register_user "test_analytics_viewer" "test1234" >/dev/null 2>&1 || true
VIEWER_TOKEN=$(login_as "test_analytics_viewer" "test1234")

VIDEO_ID=$(ensure_video "$ADMIN_TOKEN")
assert_not_empty "取得影片 ID" "$VIDEO_ID"

# =====================================================================
# Baseline — 記錄 heartbeat 前的 total_views
# =====================================================================

echo ""
bold "[1] GET /api/admin/analytics?days=7 — 取得 baseline"

RESP=$(curl -s -w "\n%{http_code}" "${API_BASE}/api/admin/analytics?days=7" \
    -H "Authorization: Bearer ${ADMIN_TOKEN}")
CODE=$(echo "$RESP" | tail -1)
BODY=$(echo "$RESP" | sed '$d')
assert_eq "baseline 分析回 200" "200" "$CODE"

BASELINE_VIEWS=$(echo "$BODY" | jq '.data.total_views')
assert_not_empty "baseline total_views" "$BASELINE_VIEWS"

# =====================================================================
# POST /api/watch-sessions/heartbeat — 同一 session 兩次心跳應累積
# =====================================================================

echo ""
bold "[2] POST /api/watch-sessions/heartbeat — 第一次心跳"

SID=$(cat /proc/sys/kernel/random/uuid 2>/dev/null || uuidgen)
assert_not_empty "產生 session id" "$SID"

CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST \
    "${API_BASE}/api/watch-sessions/heartbeat" \
    -H "Authorization: Bearer ${VIEWER_TOKEN}" \
    -H "Content-Type: application/json" \
    -d "{\"session_id\":\"${SID}\",\"video_id\":\"${VIDEO_ID}\",\"played_delta\":15,\"position_seconds\":15}")
assert_eq "第一次心跳回 204" "204" "$CODE"

# ---------------------------------------------------------------------------
echo ""
bold "[3] POST /api/watch-sessions/heartbeat — 同 session 第二次心跳（累積）"

CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST \
    "${API_BASE}/api/watch-sessions/heartbeat" \
    -H "Authorization: Bearer ${VIEWER_TOKEN}" \
    -H "Content-Type: application/json" \
    -d "{\"session_id\":\"${SID}\",\"video_id\":\"${VIDEO_ID}\",\"played_delta\":15,\"position_seconds\":30}")
assert_eq "第二次心跳回 204" "204" "$CODE"

# =====================================================================
# GET /api/admin/analytics — 驗證累積與各項欄位
# =====================================================================

echo ""
bold "[4] GET /api/admin/analytics?days=7 — 驗證累積後結果"

RESP=$(curl -s -w "\n%{http_code}" "${API_BASE}/api/admin/analytics?days=7" \
    -H "Authorization: Bearer ${ADMIN_TOKEN}")
CODE=$(echo "$RESP" | tail -1)
AFTER_BODY=$(echo "$RESP" | sed '$d')
assert_eq "累積後分析回 200" "200" "$CODE"

AFTER_VIEWS=$(echo "$AFTER_BODY" | jq '.data.total_views')
TREND_LEN=$(echo "$AFTER_BODY" | jq '.data.daily_trend | length')

assert_eq "兩次同 session 心跳只新增 1 次 view" "1" "$((AFTER_VIEWS - BASELINE_VIEWS))"
assert_eq "daily_trend 有 7 個資料點" "7" "$TREND_LEN"
assert_contains "top_videos 包含我們的影片" "$AFTER_BODY" "$VIDEO_ID"

# =====================================================================
# RBAC — viewer 不能存取 admin analytics
# =====================================================================

echo ""
bold "[5] GET /api/admin/analytics — RBAC viewer 不能存取"

CODE=$(curl -s -o /dev/null -w "%{http_code}" "${API_BASE}/api/admin/analytics" \
    -H "Authorization: Bearer ${VIEWER_TOKEN}")
assert_eq "viewer 存取 analytics 回 403" "403" "$CODE"

# ---------------------------------------------------------------------------
print_summary "Analytics 觀看紀錄與分析 API"
exit $?
