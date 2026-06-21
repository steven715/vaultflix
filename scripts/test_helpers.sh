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

# 觸發匯入並輪詢直到 job 結束，回傳「最終」job JSON（內含 data）。
# 匯入是非同步的：POST 立即回傳一個 status=running 的 snapshot，真正的
# total/imported/skipped/failed 要等背景 goroutine 跑完後從 job 端點取得。
# 用法: FINAL_JOB=$(run_import "$ADMIN_TOKEN" "$SOURCE_ID")
run_import() {
    local token="$1" source_id="$2"
    local start_resp job_id status job_resp
    start_resp=$(curl -s -X POST "${API_BASE}/api/videos/import" \
        -H "Authorization: Bearer ${token}" \
        -H "Content-Type: application/json" \
        -d "{\"source_id\":\"${source_id}\"}" \
        --max-time 3600)
    job_id=$(echo "$start_resp" | jq -r '.data.id // empty')
    if [ -z "$job_id" ]; then
        # 非 2xx（例如 400/404）— 直接回傳原始回應讓呼叫端判斷
        echo "$start_resp"
        return 0
    fi

    local i
    for i in $(seq 1 120); do
        job_resp=$(curl -s -X GET "${API_BASE}/api/import-jobs/${job_id}" \
            -H "Authorization: Bearer ${token}")
        status=$(echo "$job_resp" | jq -r '.data.status // empty')
        if [ "$status" != "running" ] && [ -n "$status" ]; then
            echo "$job_resp"
            return 0
        fi
        sleep 1
    done
    # 逾時：回傳最後一次的 job 狀態
    echo "$job_resp"
}

# 確保 DB 中至少有一筆影片，回傳第一筆影片 ID。
# 若目前沒有影片（例如 videos 套件刪光了唯一的 fixture 影片），就用 fixture
# 重新匯入一次，讓依賴影片的套件（tags）不受套件執行順序影響。
ensure_video() {
    local token="$1"
    local import_dir="${IMPORT_DIR:-/mnt/host/videos}"
    local source_label="${SOURCE_LABEL:-integration-test}"
    local video_id source_id

    video_id=$(curl -s "${API_BASE}/api/videos?page_size=1" \
        -H "Authorization: Bearer ${token}" | jq -r '.data[0].id // empty')
    if [ -n "$video_id" ]; then
        echo "$video_id"
        return 0
    fi

    # 找現有 source 或建立一個
    source_id=$(curl -s "${API_BASE}/api/media-sources" \
        -H "Authorization: Bearer ${token}" | jq -r \
        ".data[] | select(.mount_path == \"${import_dir}\") | .id // empty" | head -1)
    if [ -z "$source_id" ]; then
        source_id=$(curl -s -X POST "${API_BASE}/api/media-sources" \
            -H "Authorization: Bearer ${token}" \
            -H "Content-Type: application/json" \
            -d "{\"label\":\"${source_label}\",\"mount_path\":\"${import_dir}\"}" \
            | jq -r '.data.id // empty')
    fi
    if [ -z "$source_id" ]; then
        echo ""
        return 0
    fi

    run_import "$token" "$source_id" >/dev/null
    curl -s "${API_BASE}/api/videos?page_size=1" \
        -H "Authorization: Bearer ${token}" | jq -r '.data[0].id // empty'
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
