#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT=""
DOCKER_CMD=""
CURL_CMD=""
GO_CMD=""
NPM_CMD=""
NO_VERIFY=0
CURRENT_CMD=""
STEP_NO=0
HEALTH_RETRY_ATTEMPTS="${HEALTH_RETRY_ATTEMPTS:-30}"
HEALTH_RETRY_INTERVAL="${HEALTH_RETRY_INTERVAL:-1}"

log_info() {
  echo "$*"
}

log_warn() {
  echo "[警告] $*"
}

log_step() {
  STEP_NO=$((STEP_NO + 1))
  echo "[步骤 ${STEP_NO}] $*"
}

fail() {
  echo "[失败] $1" >&2
  exit 1
}

resolve_repo_root() {
  local script_dir
  script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
  REPO_ROOT="$(cd "$script_dir/.." && pwd -P)"
}

resolve_cmd() {
  local name="$1"
  shift

  if command -v "$name" >/dev/null 2>&1; then
    command -v "$name"
    return 0
  fi

  while [[ $# -gt 0 ]]; do
    if [[ -x "$1" ]]; then
      echo "$1"
      return 0
    fi
    shift
  done

  return 1
}

check_repo_files() {
  [[ -f "$REPO_ROOT/docker-compose.yml" ]] || fail "失败原因：未找到 $REPO_ROOT/docker-compose.yml"
  [[ -f "$REPO_ROOT/configs/config.yaml" ]] || fail "失败原因：未找到 $REPO_ROOT/configs/config.yaml"

  if [[ ! -f "$REPO_ROOT/.env" ]]; then
    log_warn "未找到 .env，将使用 Compose 默认镜像配置"
  fi
}

resolve_runtime_commands() {
  DOCKER_CMD="$(resolve_cmd docker /usr/local/bin/docker /Applications/Docker.app/Contents/Resources/bin/docker)" || fail "失败原因：未找到 docker 命令"
  CURL_CMD="$(resolve_cmd curl /usr/bin/curl /usr/local/bin/curl)" || fail "失败原因：未找到 curl 命令"
}

resolve_go_command() {
  GO_CMD="$(resolve_cmd go /usr/local/go/bin/go)" || fail "失败原因：未找到 go 命令"
}

resolve_npm_command() {
  NPM_CMD="$(resolve_cmd npm /usr/local/bin/npm)" || fail "失败原因：未找到 npm 命令"
}

docker_compose() {
  "$DOCKER_CMD" compose "$@"
}

run_verify_backend() {
  log_step "执行后端测试"
  if ! (cd "$REPO_ROOT" && "$GO_CMD" test ./... -count=1); then
    fail "失败原因：后端测试未通过"
  fi
}

run_verify_frontend() {
  log_step "执行前端快速测试"
  if ! (cd "$REPO_ROOT/frontend" && "$NPM_CMD" run test:state && "$NPM_CMD" run test:smoke); then
    fail "失败原因：前端快速测试未通过"
  fi
}

run_build_frontend() {
  log_step "执行前端构建"
  if ! (cd "$REPO_ROOT/frontend" && "$NPM_CMD" run build); then
    fail "失败原因：前端构建失败"
  fi
}

wait_for_health() {
  local url="$1"
  local failure_message="$2"
  local attempt=1

  while [[ $attempt -le $HEALTH_RETRY_ATTEMPTS ]]; do
    if "$CURL_CMD" -sf "$url" >/dev/null; then
      return 0
    fi

    if [[ $attempt -eq $HEALTH_RETRY_ATTEMPTS ]]; then
      fail "$failure_message"
    fi

    sleep "$HEALTH_RETRY_INTERVAL"
    attempt=$((attempt + 1))
  done

  fail "$failure_message"
}

run_health_backend() {
  log_step "执行后端健康检查"
  wait_for_health "http://127.0.0.1:8080/healthz" "失败原因：后端健康检查失败，请执行 docker compose logs app --tail=200 排查"
}

run_health_frontend() {
  log_step "执行前端健康检查"
  wait_for_health "http://127.0.0.1:5173" "失败原因：前端健康检查失败，请执行 docker compose logs frontend --tail=200 排查"
}

print_summary() {
  local mode="$1"
  log_info "状态摘要"
  log_info "- 命令：$mode"
  if [[ $NO_VERIFY -eq 1 ]]; then
    log_info "- 校验：已跳过 (--no-verify)"
  else
    log_info "- 校验：已执行"
  fi
  log_info "- 后端地址：http://localhost:8080"
  log_info "- 前端地址：http://localhost:5173"
}

cmd_deploy_all() {
  log_step "检查部署环境"
  check_repo_files
  resolve_runtime_commands

  if [[ $NO_VERIFY -eq 0 ]]; then
    resolve_go_command
    resolve_npm_command
    run_verify_backend
    run_verify_frontend
  else
    resolve_npm_command
    log_step "跳过验证阶段 (--no-verify)"
  fi

  run_build_frontend

  log_step "构建并启动全部容器"
  if ! (cd "$REPO_ROOT" && docker_compose up -d --build); then
    fail "失败原因：docker compose up 执行失败"
  fi

  run_health_backend
  run_health_frontend
  print_summary "deploy-all"
}

cmd_deploy_app() {
  log_step "检查部署环境"
  check_repo_files
  resolve_runtime_commands

  if [[ $NO_VERIFY -eq 0 ]]; then
    resolve_go_command
    run_verify_backend
  else
    log_step "跳过后端验证 (--no-verify)"
  fi

  log_step "构建并启动 app 容器"
  if ! (cd "$REPO_ROOT" && docker_compose up -d --build app); then
    fail "失败原因：app 容器部署失败"
  fi

  run_health_backend
  print_summary "deploy-app"
}

check_restart_containers() {
  local ps_output
  if ! ps_output="$(cd "$REPO_ROOT" && docker_compose ps --format json)"; then
    fail "失败原因：无法获取容器状态"
  fi

  if [[ "$ps_output" != *'"Service":"app"'* ]] || [[ "$ps_output" != *'"Service":"frontend"'* ]]; then
    fail "失败原因：未找到 app/frontend 容器，请先执行 deploy-all"
  fi
}

cmd_restart() {
  log_step "检查部署环境"
  check_repo_files
  resolve_runtime_commands
  check_restart_containers

  log_step "重启 app 与 frontend 容器"
  if ! (cd "$REPO_ROOT" && docker_compose restart app frontend); then
    fail "失败原因：容器重启失败"
  fi

  run_health_backend
  run_health_frontend
  print_summary "restart"
}

cmd_status() {
  local backend_ok="失败"
  local frontend_ok="失败"
  local branch="unknown"
  local commit="unknown"
  local dirty="unknown"
  local ps_output=""

  log_step "检查部署环境"
  check_repo_files
  resolve_runtime_commands

  if command -v git >/dev/null 2>&1; then
    if branch="$(cd "$REPO_ROOT" && git rev-parse --abbrev-ref HEAD 2>/dev/null)"; then :; else branch="unknown"; fi
    if commit="$(cd "$REPO_ROOT" && git rev-parse --short HEAD 2>/dev/null)"; then :; else commit="unknown"; fi
    if [[ -z "$(cd "$REPO_ROOT" && git status --porcelain 2>/dev/null || true)" ]]; then
      dirty="干净"
    else
      dirty="有改动"
    fi
  fi

  log_step "收集 Docker Compose 状态"
  if ! ps_output="$(cd "$REPO_ROOT" && docker_compose ps 2>&1)"; then
    fail "失败原因：docker compose ps 执行失败"
  fi

  log_step "执行健康检查"
  if "$CURL_CMD" -sf http://127.0.0.1:8080/healthz >/dev/null; then
    backend_ok="通过"
  fi
  if "$CURL_CMD" -sf http://127.0.0.1:5173 >/dev/null; then
    frontend_ok="通过"
  fi

  log_info "状态摘要"
  log_info "- Git 分支：$branch"
  log_info "- Git 提交：$commit"
  log_info "- 工作区：$dirty"
  log_info "- 后端健康检查：$backend_ok"
  log_info "- 前端健康检查：$frontend_ok"
  log_info "- Docker Compose 状态："
  printf '%s\n' "$ps_output"
}

parse_args() {
  local arg
  CURRENT_CMD=""

  for arg in "$@"; do
    case "$arg" in
      --no-verify)
        NO_VERIFY=1
        ;;
      deploy-all|deploy-app|restart|status)
        if [[ -n "$CURRENT_CMD" ]]; then
          fail "失败原因：只能指定一个子命令"
        fi
        CURRENT_CMD="$arg"
        ;;
      *)
        fail "失败原因：不支持的参数或命令 '$arg'"
        ;;
    esac
  done

  if [[ -z "$CURRENT_CMD" ]]; then
    CURRENT_CMD="deploy-all"
  fi

  if [[ $NO_VERIFY -eq 1 && "$CURRENT_CMD" != "deploy-all" && "$CURRENT_CMD" != "deploy-app" ]]; then
    fail "失败原因：--no-verify 仅适用于 deploy-all 与 deploy-app"
  fi
}

main() {
  resolve_repo_root
  cd "$REPO_ROOT"
  parse_args "$@"

  case "$CURRENT_CMD" in
    deploy-all)
      cmd_deploy_all
      ;;
    deploy-app)
      cmd_deploy_app
      ;;
    restart)
      cmd_restart
      ;;
    status)
      cmd_status
      ;;
    *)
      fail "失败原因：未知命令 '$CURRENT_CMD'"
      ;;
  esac
}

main "$@"
