#!/bin/bash
# =============================================================================
# Integration test: 認證與授權
# 測試項目: 註冊、登入、JWT 驗證、RBAC 角色
# =============================================================================

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/test_helpers.sh"

bold "=== Auth 認證與授權測試 ==="
check_prerequisites

# ---------------------------------------------------------------------------
# 1. 註冊 viewer 帳號
# ---------------------------------------------------------------------------
echo ""
bold "[1] 註冊 viewer 帳號"

RESP=$(curl -s -w "\n%{http_code}" -X POST "${API_BASE}/api/auth/register" \
    -H "Content-Type: application/json" \
    -d '{"username":"test_auth_viewer","password":"test1234"}')
CODE=$(echo "$RESP" | tail -1)
BODY=$(echo "$RESP" | sed '$d')

# 201 = 新建, 400 = 已存在（重複執行時）
if [ "$CODE" = "201" ] || [ "$CODE" = "400" ]; then
    green "  [PASS] POST /api/auth/register (HTTP $CODE)"
    _PASS=$((_PASS + 1)); _TOTAL=$((_TOTAL + 1))
else
    red "  [FAIL] POST /api/auth/register (expected 201 or 400, got $CODE)"
    _FAIL=$((_FAIL + 1)); _TOTAL=$((_TOTAL + 1))
fi

# ---------------------------------------------------------------------------
# 2. 重複註冊應回 400
# ---------------------------------------------------------------------------
echo ""
bold "[2] 重複註冊同一帳號"

CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "${API_BASE}/api/auth/register" \
    -H "Content-Type: application/json" \
    -d '{"username":"test_auth_viewer","password":"test1234"}')
assert_eq "重複註冊回 400" "400" "$CODE"

# ---------------------------------------------------------------------------
# 3. 正確密碼登入
# ---------------------------------------------------------------------------
echo ""
bold "[3] Viewer 登入"

VIEWER_TOKEN=$(login_as "test_auth_viewer" "test1234")
assert_not_empty "取得 viewer JWT token" "$VIEWER_TOKEN"

# ---------------------------------------------------------------------------
# 4. 錯誤密碼登入
# ---------------------------------------------------------------------------
echo ""
bold "[4] 錯誤密碼登入"

CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "${API_BASE}/api/auth/login" \
    -H "Content-Type: application/json" \
    -d '{"username":"test_auth_viewer","password":"wrongpassword"}')
assert_eq "錯誤密碼回 401" "401" "$CODE"

# ---------------------------------------------------------------------------
# 5. Viewer 存取 /api/me
# ---------------------------------------------------------------------------
echo ""
bold "[5] Viewer 存取 /api/me"

RESP=$(curl -s "${API_BASE}/api/me" -H "Authorization: Bearer ${VIEWER_TOKEN}")
ROLE=$(echo "$RESP" | jq -r '.data.role // empty')
assert_eq "role 為 viewer" "viewer" "$ROLE"

# ---------------------------------------------------------------------------
# 6. 不帶 token
# ---------------------------------------------------------------------------
echo ""
bold "[6] 不帶 token 存取 /api/me"

CODE=$(curl -s -o /dev/null -w "%{http_code}" "${API_BASE}/api/me")
assert_eq "無 token 回 401" "401" "$CODE"

# ---------------------------------------------------------------------------
# 7. 無效 token
# ---------------------------------------------------------------------------
echo ""
bold "[7] 無效 token"

CODE=$(curl -s -o /dev/null -w "%{http_code}" "${API_BASE}/api/me" \
    -H "Authorization: Bearer invalid.token.here")
assert_eq "無效 token 回 401" "401" "$CODE"

# ---------------------------------------------------------------------------
# 8. Admin 登入與驗證
# ---------------------------------------------------------------------------
echo ""
bold "[8] Admin 登入與驗證"

ADMIN_TOKEN=$(login_as "$ADMIN_USER" "$ADMIN_PASS")
assert_not_empty "取得 admin JWT token" "$ADMIN_TOKEN"

RESP=$(curl -s "${API_BASE}/api/me" -H "Authorization: Bearer ${ADMIN_TOKEN}")
ROLE=$(echo "$RESP" | jq -r '.data.role // empty')
assert_eq "role 為 admin" "admin" "$ROLE"

# ---------------------------------------------------------------------------
# 9. RBAC 測試 — viewer 不能呼叫 admin 端點
# ---------------------------------------------------------------------------
echo ""
bold "[9] RBAC — viewer 不能 POST /api/videos/import"

CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "${API_BASE}/api/videos/import" \
    -H "Authorization: Bearer ${VIEWER_TOKEN}" \
    -H "Content-Type: application/json" \
    -d '{"source_dir":"/mnt/videos"}')
assert_eq "viewer 呼叫 import 回 403" "403" "$CODE"

# ---------------------------------------------------------------------------
print_summary "Auth 認證與授權"
exit $?
