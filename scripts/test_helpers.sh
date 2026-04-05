#!/bin/bash
# =============================================================================
# Integration test 共用函式
# 被其他 test_*.sh source 引用，不直接執行
# =============================================================================

API_BASE="${API_BASE:-http://vaultflix-api:8080}"
ADMIN_USER="${ADMIN_USER:-admin}"
ADMIN_PASS="${ADMIN_PASS:-admin123}"

_PASS=0
_FAIL=0
_TOTAL=0

green()  { printf "\033[32m%s\033[0m\n" "$1"; }
red()    { printf "\033[31m%s\033[0m\n" "$1"; }
yellow() { printf "\033[33m%s\033[0m\n" "$1"; }
bold()   { printf "\033[1m%s\033[0m\n" "$1"; }

assert_eq() {
    local label="$1" expected="$2" actual="$3"
    _TOTAL=$((_TOTAL + 1))
    if [ "$expected" = "$actual" ]; then
        green "  [PASS] $label"
        _PASS=$((_PASS + 1))
    else
        red "  [FAIL] $label (expected: $expected, got: $actual)"
        _FAIL=$((_FAIL + 1))
    fi
}

assert_not_empty() {
    local label="$1" value="$2"
    _TOTAL=$((_TOTAL + 1))
    if [ -n "$value" ] && [ "$value" != "null" ]; then
        green "  [PASS] $label"
        _PASS=$((_PASS + 1))
    else
        red "  [FAIL] $label (value is empty or null)"
        _FAIL=$((_FAIL + 1))
    fi
}

assert_gte() {
    local label="$1" expected="$2" actual="$3"
    _TOTAL=$((_TOTAL + 1))
    if [ "$actual" -ge "$expected" ] 2>/dev/null; then
        green "  [PASS] $label ($actual >= $expected)"
        _PASS=$((_PASS + 1))
    else
        red "  [FAIL] $label (expected >= $expected, got: $actual)"
        _FAIL=$((_FAIL + 1))
    fi
}

assert_contains() {
    local label="$1" haystack="$2" needle="$3"
    _TOTAL=$((_TOTAL + 1))
    if echo "$haystack" | grep -q "$needle"; then
        green "  [PASS] $label"
        _PASS=$((_PASS + 1))
    else
        red "  [FAIL] $label (not found: $needle)"
        _FAIL=$((_FAIL + 1))
    fi
}

# 確認前置條件（curl, jq, API 啟動）
check_prerequisites() {
    if ! command -v curl &>/dev/null; then
        red "curl 未安裝"; exit 1
    fi
    if ! command -v jq &>/dev/null; then
        red "jq 未安裝"; exit 1
    fi
    local health
    health=$(curl -s -o /dev/null -w "%{http_code}" "${API_BASE}/health" 2>/dev/null || echo "000")
    if [ "$health" != "200" ]; then
        red "API 服務未啟動 (${API_BASE}, HTTP $health)"; exit 1
    fi
}

# 登入並回傳 token，失敗時 exit
login_as() {
    local user="$1" pass="$2"
    local resp token
    resp=$(curl -s -X POST "${API_BASE}/api/auth/login" \
        -H "Content-Type: application/json" \
        -d "{\"username\":\"${user}\",\"password\":\"${pass}\"}")
    token=$(echo "$resp" | jq -r '.data.token // empty')
    if [ -z "$token" ]; then
        red "登入失敗 (user=$user): $resp"; exit 1
    fi
    echo "$token"
}

# 註冊帳號，回傳 HTTP status code（靜默處理 duplicate）
register_user() {
    local user="$1" pass="$2"
    curl -s -o /dev/null -w "%{http_code}" -X POST "${API_BASE}/api/auth/register" \
        -H "Content-Type: application/json" \
        -d "{\"username\":\"${user}\",\"password\":\"${pass}\"}"
}

# 印出測試結果摘要，回傳失敗數作為 exit code
print_summary() {
    local label="$1"
    echo ""
    bold "=== ${label} 結果 ==="
    if [ "$_FAIL" -eq 0 ]; then
        green "全部通過！ ($_PASS/$_TOTAL)"
    else
        red "有 $_FAIL 個測試失敗 ($_PASS/$_TOTAL 通過)"
    fi
    return "$_FAIL"
}
