#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SOURCE_SCRIPT="$ROOT_DIR/scripts/deploy.sh"
TMP_DIR=""

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

assert_contains() {
  local haystack="$1"
  local needle="$2"
  local message="$3"
  if [[ "$haystack" != *"$needle"* ]]; then
    fail "$message"
  fi
}

assert_file_contains() {
  local file="$1"
  local needle="$2"
  local message="$3"
  if ! grep -Fq "$needle" "$file"; then
    fail "$message"
  fi
}

assert_file_not_contains() {
  local file="$1"
  local needle="$2"
  local message="$3"
  if grep -Fq "$needle" "$file"; then
    fail "$message"
  fi
}

make_fixture() {
  local dir="$1"
  mkdir -p "$dir/repo/scripts" "$dir/repo/configs" "$dir/repo/frontend" "$dir/bin"

  if [[ ! -f "$SOURCE_SCRIPT" ]]; then
    fail "缺少 $SOURCE_SCRIPT"
  fi

  cp "$SOURCE_SCRIPT" "$dir/repo/scripts/deploy.sh"
  chmod +x "$dir/repo/scripts/deploy.sh"

  cat > "$dir/repo/docker-compose.yml" <<'YAML'
services:
  app: {}
  frontend: {}
YAML

  cat > "$dir/repo/configs/config.yaml" <<'YAML'
storage:
  root_dir: /data
mysql:
  dsn: test
YAML

  cat > "$dir/repo/frontend/package.json" <<'JSON'
{
  "name": "fixture-frontend",
  "private": true,
  "scripts": {
    "test:state": "echo state",
    "build": "echo build",
    "test:smoke": "echo smoke"
  }
}
JSON

  cat > "$dir/bin/docker" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail

echo "DOCKER_BUILDKIT=${DOCKER_BUILDKIT:-} docker $*" >>"${TMP_LOG:?}"

if [[ "${1:-} ${2:-}" == "compose ps" ]]; then
  if [[ "${MOCK_PS_FAIL:-0}" == "1" ]]; then
    exit 1
  fi
  if [[ -n "${MOCK_PS_JSON:-}" ]]; then
    printf '%s\n' "$MOCK_PS_JSON"
  else
    printf '{"Service":"app","Name":"fetch_bilibili"}\n'
    printf '{"Service":"frontend","Name":"fetch_bilibili_frontend"}\n'
  fi
  exit 0
fi

if [[ "${1:-} ${2:-}" == "compose up" ]]; then
  case "${MOCK_COMPOSE_UP_FAIL_MODE:-}" in
    proxy)
      if [[ "${DOCKER_BUILDKIT:-}" != "0" ]]; then
        printf '%s\n' 'failed to fetch anonymous token: Get "https://example.invalid": proxyconnect tcp: dial tcp 127.0.0.1:7890: connect: connection refused' >&2
        exit 1
      fi
      ;;
    normal)
      printf '%s\n' 'normal compose failure' >&2
      exit 1
      ;;
  esac
  exit 0
fi

if [[ "${1:-} ${2:-}" == "compose restart" ]]; then
  if [[ "${MOCK_RESTART_FAIL:-0}" == "1" ]]; then
    exit 1
  fi
  exit 0
fi

if [[ "${1:-} ${2:-}" == "compose logs" ]]; then
  printf 'logs mock\n'
  exit 0
fi

exit 0
SCRIPT

  cat > "$dir/bin/curl" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail

echo "curl $*" >>"${TMP_LOG:?}"

COUNT_FILE="${TMP_CURL_COUNT_FILE:-}"
if [[ -n "$COUNT_FILE" ]]; then
  current=0
  if [[ -f "$COUNT_FILE" ]]; then
    current="$(cat "$COUNT_FILE")"
  fi
  current=$((current + 1))
  printf '%s\n' "$current" >"$COUNT_FILE"
fi

if [[ -n "${MOCK_CURL_FAIL_COUNT_FILE:-}" ]]; then
  remaining=0
  if [[ -f "$MOCK_CURL_FAIL_COUNT_FILE" ]]; then
    remaining="$(cat "$MOCK_CURL_FAIL_COUNT_FILE")"
  fi
  if [[ "$remaining" -gt 0 ]]; then
    remaining=$((remaining - 1))
    printf '%s\n' "$remaining" >"$MOCK_CURL_FAIL_COUNT_FILE"
    exit 22
  fi
fi

if [[ "$*" == *"8080/healthz"* && "${MOCK_CURL_BACKEND_FAIL:-0}" == "1" ]]; then
  exit 22
fi
if [[ "$*" == *"5173"* && "${MOCK_CURL_FRONTEND_FAIL:-0}" == "1" ]]; then
  exit 22
fi
if [[ "${MOCK_CURL_FAIL:-0}" == "1" ]]; then
  exit 22
fi

exit 0
SCRIPT

  cat > "$dir/bin/go" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail

echo "go $*" >>"${TMP_LOG:?}"
exit 0
SCRIPT

  cat > "$dir/bin/npm" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail

echo "npm $*" >>"${TMP_LOG:?}"
exit 0
SCRIPT

  chmod +x "$dir/bin/docker" "$dir/bin/curl" "$dir/bin/go" "$dir/bin/npm"
}

run_deploy() {
  local fixture="$1"
  shift
  (
    cd "$fixture/repo"
    PATH="$fixture/bin:$PATH" TMP_LOG="$fixture/log" bash "$fixture/repo/scripts/deploy.sh" "$@"
  )
}

run_deploy_with_path() {
  local fixture="$1"
  local path_value="$2"
  shift 2
  (
    cd "$fixture/repo"
    PATH="$path_value" TMP_LOG="$fixture/log" bash "$fixture/repo/scripts/deploy.sh" "$@"
  )
}

remove_fixture_command() {
  local fixture="$1"
  local name="$2"
  if [[ -f "$fixture/bin/$name" ]]; then
    mv "$fixture/bin/$name" "$fixture/bin/$name.disabled"
  fi
}

disable_fixture_fallbacks() {
  local fixture="$1"
  local script_path="$fixture/repo/scripts/deploy.sh"
  local fake_go="$fixture/fallbacks/go"
  local fake_npm="$fixture/fallbacks/npm"
  local script_content
  local search_go="/usr/local/go/bin/go"
  local search_npm="/usr/local/bin/npm"
  mkdir -p "$fixture/fallbacks"

  script_content="$(cat "$script_path")"
  script_content="${script_content//$search_go/$fake_go}"
  script_content="${script_content//$search_npm/$fake_npm}"
  printf '%s' "$script_content" > "$script_path"
}

shadow_perl_unavailable() {
  local fixture="$1"
  cat > "$fixture/bin/perl" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail
echo "unexpected perl $*" >>"${TMP_LOG:?}"
exit 127
SCRIPT
  chmod +x "$fixture/bin/perl"
}

write_failing_verify_commands() {
  local fixture="$1"

  cat > "$fixture/bin/go" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail
echo "unexpected go $*" >>"${TMP_LOG:?}"
exit 99
SCRIPT

  cat > "$fixture/bin/npm" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail
echo "unexpected npm $*" >>"${TMP_LOG:?}"
exit 99
SCRIPT

  chmod +x "$fixture/bin/go" "$fixture/bin/npm"
}

smoke_default_command_equals_deploy_all() {
  local fixture="$1"
  : >"$fixture/log"
  local output
  output="$(run_deploy "$fixture")"

  assert_contains "$output" "执行前端构建" "默认 deploy-all 未执行前端构建"
  assert_file_contains "$fixture/log" "go test ./..." "默认 deploy-all 应执行 go test"
  assert_file_contains "$fixture/log" "npm run test:state" "默认 deploy-all 应执行前端状态测试"
  assert_file_contains "$fixture/log" "npm run test:smoke" "默认 deploy-all 应执行前端快速测试"
  assert_file_contains "$fixture/log" "npm run build" "默认 deploy-all 应执行前端构建"
  assert_file_contains "$fixture/log" "docker compose up -d --build" "默认 deploy-all 应启动 compose"
}

smoke_no_verify_skips_tests_but_not_build() {
  local fixture="$1"
  : >"$fixture/log"
  run_deploy "$fixture" --no-verify >/dev/null

  assert_file_not_contains "$fixture/log" "go test ./..." "--no-verify 不应执行 go test"
  assert_file_not_contains "$fixture/log" "npm run test:state" "--no-verify 不应执行前端状态测试"
  assert_file_not_contains "$fixture/log" "npm run test:smoke" "--no-verify 不应执行前端快速测试"
  assert_file_contains "$fixture/log" "npm run build" "--no-verify 仍应执行前端构建"
}

smoke_deploy_app_without_frontend_build() {
  local fixture="$1"
  : >"$fixture/log"
  local output
  output="$(run_deploy "$fixture" deploy-app)"

  assert_file_not_contains "$fixture/log" "npm run build" "deploy-app 不应执行前端构建"
  assert_file_contains "$fixture/log" "docker compose up -d --build app" "deploy-app 应仅部署 app"
  if [[ "$output" == *"前端地址：http://localhost:5173"* ]]; then
    fail "deploy-app 摘要不应误导输出前端地址"
  fi
}

smoke_proxy_error_retries_without_buildkit() {
  local fixture="$1"
  : >"$fixture/log"
  local output
  output="$(MOCK_COMPOSE_UP_FAIL_MODE=proxy run_deploy "$fixture" deploy-app --no-verify 2>&1)"

  assert_contains "$output" "关闭 BuildKit 后重试" "遇到失效本地代理时应提示关闭 BuildKit 重试"
  assert_file_contains "$fixture/log" "DOCKER_BUILDKIT= docker compose up -d --build app" "首次 compose up 应先按默认 BuildKit 执行"
  assert_file_contains "$fixture/log" "DOCKER_BUILDKIT=0 docker compose up -d --build app" "代理异常时应关闭 BuildKit 重试"
}

smoke_restart_fails_when_container_missing() {
  local fixture="$1"
  : >"$fixture/log"
  local output
  set +e
  output="$(
    MOCK_PS_JSON='{"Service":"app","Name":"fetch_bilibili"}' run_deploy "$fixture" restart 2>&1
  )"
  local code=$?
  set -e

  if [[ $code -eq 0 ]]; then
    fail "restart 在 frontend 容器缺失时应失败"
  fi
  assert_contains "$output" "请先执行 deploy-all" "restart 缺少容器时应提示先 deploy-all"
}

smoke_status_prints_summary_on_health_failure() {
  local fixture="$1"
  : >"$fixture/log"
  local output
  local code
  set +e
  output="$(MOCK_CURL_FAIL=1 run_deploy "$fixture" status 2>&1)"
  code=$?
  set -e

  if [[ $code -ne 0 ]]; then
    fail "status 在健康检查失败时仍应返回 0"
  fi
  assert_contains "$output" "状态摘要" "status 在健康检查失败时仍需输出摘要"
  assert_file_contains "$fixture/log" "docker compose ps" "status 应查询 compose 状态"
}

smoke_healthcheck_calls_curl() {
  local fixture="$1"
  : >"$fixture/log"
  run_deploy "$fixture" deploy-app >/dev/null

  assert_file_contains "$fixture/log" "curl -sf http://127.0.0.1:8080/healthz" "后端健康检查应调用 curl"
}

smoke_restart_and_status_do_not_require_go_or_npm() {
  local fixture="$1"
  : >"$fixture/log"
  write_failing_verify_commands "$fixture"

  run_deploy "$fixture" restart >/dev/null
  run_deploy "$fixture" status >/dev/null

  assert_file_not_contains "$fixture/log" "unexpected go" "restart/status 不应调用 go"
  assert_file_not_contains "$fixture/log" "unexpected npm" "restart/status 不应调用 npm"
}

smoke_deploy_healthcheck_retries_before_success() {
  local fixture="$1"
  : >"$fixture/log"
  local count_file="$fixture/curl-count"
  local fail_count_file="$fixture/curl-fail-count"
  printf '2\n' >"$fail_count_file"

  TMP_CURL_COUNT_FILE="$count_file" MOCK_CURL_FAIL_COUNT_FILE="$fail_count_file" run_deploy "$fixture" deploy-app >/dev/null

  if [[ ! -f "$count_file" ]]; then
    fail "健康检查重试测试未记录 curl 调用次数"
  fi
  if [[ "$(cat "$count_file")" -lt 3 ]]; then
    fail "deploy-app 健康检查应在失败后重试"
  fi
}

smoke_deploy_all_no_verify_without_go() {
  local fixture="$1"
  : >"$fixture/log"
  shadow_perl_unavailable "$fixture"
  disable_fixture_fallbacks "$fixture"
  remove_fixture_command "$fixture" "go"

  run_deploy_with_path "$fixture" "$fixture/bin:/usr/bin:/bin" --no-verify >/dev/null

  assert_file_not_contains "$fixture/log" "go " "deploy-all --no-verify 缺少 go 时不应调用 go"
  assert_file_contains "$fixture/log" "npm run build" "deploy-all --no-verify 缺少 go 时仍应执行前端构建"
}

smoke_deploy_app_no_verify_without_go_or_npm() {
  local fixture="$1"
  : >"$fixture/log"
  disable_fixture_fallbacks "$fixture"
  remove_fixture_command "$fixture" "go"
  remove_fixture_command "$fixture" "npm"

  run_deploy_with_path "$fixture" "$fixture/bin:/usr/bin:/bin" deploy-app --no-verify >/dev/null

  assert_file_not_contains "$fixture/log" "go " "deploy-app --no-verify 缺少 go 时不应调用 go"
  assert_file_not_contains "$fixture/log" "npm " "deploy-app --no-verify 缺少 npm 时不应调用 npm"
  assert_file_contains "$fixture/log" "docker compose up -d --build app" "deploy-app --no-verify 缺少 go/npm 时仍应部署 app"
}

smoke_git_bash_compat_and_env_warning() {
  local fixture="$1"
  : >"$fixture/log"

  if grep -Fq "readlink -f" "$fixture/repo/scripts/deploy.sh"; then
    fail "deploy.sh 不应依赖 readlink -f"
  fi

  local output
  output="$(run_deploy "$fixture" status 2>&1 || true)"
  assert_contains "$output" "未找到 .env，将使用 Compose 默认镜像配置" ".env 缺失时应仅告警"
}

main() {
  TMP_DIR="$(mktemp -d)"
  trap 'rm -rf "$TMP_DIR"' EXIT

  make_fixture "$TMP_DIR/default"
  smoke_default_command_equals_deploy_all "$TMP_DIR/default"

  make_fixture "$TMP_DIR/no-verify"
  smoke_no_verify_skips_tests_but_not_build "$TMP_DIR/no-verify"

  make_fixture "$TMP_DIR/no-verify-no-go"
  smoke_deploy_all_no_verify_without_go "$TMP_DIR/no-verify-no-go"

  make_fixture "$TMP_DIR/deploy-app"
  smoke_deploy_app_without_frontend_build "$TMP_DIR/deploy-app"

  make_fixture "$TMP_DIR/proxy-fallback"
  smoke_proxy_error_retries_without_buildkit "$TMP_DIR/proxy-fallback"

  make_fixture "$TMP_DIR/deploy-app-no-verify-no-build-tools"
  smoke_deploy_app_no_verify_without_go_or_npm "$TMP_DIR/deploy-app-no-verify-no-build-tools"

  make_fixture "$TMP_DIR/restart-missing"
  smoke_restart_fails_when_container_missing "$TMP_DIR/restart-missing"

  make_fixture "$TMP_DIR/status"
  smoke_status_prints_summary_on_health_failure "$TMP_DIR/status"

  make_fixture "$TMP_DIR/health-call"
  smoke_healthcheck_calls_curl "$TMP_DIR/health-call"

  make_fixture "$TMP_DIR/no-verify-deps"
  smoke_restart_and_status_do_not_require_go_or_npm "$TMP_DIR/no-verify-deps"

  make_fixture "$TMP_DIR/health-retry"
  smoke_deploy_healthcheck_retries_before_success "$TMP_DIR/health-retry"

  make_fixture "$TMP_DIR/git-bash"
  smoke_git_bash_compat_and_env_warning "$TMP_DIR/git-bash"

  echo "deploy.sh smoke ok"
}

main "$@"
