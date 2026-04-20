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

echo "docker $*" >>"${TMP_LOG:?}"

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
  run_deploy "$fixture" deploy-app >/dev/null

  assert_file_not_contains "$fixture/log" "npm run build" "deploy-app 不应执行前端构建"
  assert_file_contains "$fixture/log" "docker compose up -d --build app" "deploy-app 应仅部署 app"
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
  set +e
  output="$(MOCK_CURL_FAIL=1 run_deploy "$fixture" status 2>&1)"
  set -e

  assert_contains "$output" "状态摘要" "status 在健康检查失败时仍需输出摘要"
  assert_file_contains "$fixture/log" "docker compose ps" "status 应查询 compose 状态"
}

smoke_healthcheck_calls_curl() {
  local fixture="$1"
  : >"$fixture/log"
  run_deploy "$fixture" deploy-app >/dev/null

  assert_file_contains "$fixture/log" "curl -sf http://127.0.0.1:8080/healthz" "后端健康检查应调用 curl"
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

  make_fixture "$TMP_DIR"
  smoke_default_command_equals_deploy_all "$TMP_DIR"
  smoke_no_verify_skips_tests_but_not_build "$TMP_DIR"
  smoke_deploy_app_without_frontend_build "$TMP_DIR"
  smoke_restart_fails_when_container_missing "$TMP_DIR"
  smoke_status_prints_summary_on_health_failure "$TMP_DIR"
  smoke_healthcheck_calls_curl "$TMP_DIR"
  smoke_git_bash_compat_and_env_warning "$TMP_DIR"

  echo "deploy.sh smoke ok"
}

main "$@"
