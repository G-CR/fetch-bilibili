# 本地一键构建与部署脚本实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 为当前仓库补齐本地一键构建与部署脚本，支持 macOS / Linux、Windows Git Bash 和 Windows PowerShell，并把“验证、构建、部署、健康检查、状态查看”收敛为统一入口。

**架构：** 采用双入口脚本：`scripts/deploy.sh` 负责 Unix Shell 与 Windows Git Bash，`scripts/deploy.ps1` 负责 Windows PowerShell。两者保持相同命令语义，通过各自的测试脚本在临时工作区中注入假的 `docker/go/npm/curl` 命令，验证命令解析、环境检查、执行顺序和失败语义。最后同步 `README`、容器部署文档和运行手册。

**技术栈：** POSIX Shell、PowerShell、Docker Compose、Git、curl、仓库内脚本级 smoke 测试、现有中文文档体系

---

## 文件边界

### 脚本入口

- 创建：`scripts/deploy.sh`
  - Unix Shell / Windows Git Bash 主入口
  - 负责参数解析、环境检查、验证、构建部署、健康检查和状态输出
- 创建：`scripts/deploy.ps1`
  - Windows PowerShell 主入口
  - 与 `deploy.sh` 保持相同命令和参数语义

### 脚本测试

- 创建：`scripts/tests/test-deploy.sh`
  - Unix / Git Bash smoke 测试
  - 在临时工作区和 fake PATH 中验证 `deploy.sh` 的命令分发、依赖检查、日志输出和退出码
- 创建：`scripts/tests/test-deploy.ps1`
  - PowerShell smoke 测试
  - 在临时工作区和 fake PATH 中验证 `deploy.ps1` 的同语义行为

### 文档

- 修改：`README.md`
  - 增加一键部署脚本的入口说明和平台示例
- 修改：`docs/container-deploy.md`
  - 把当前手工命令串改为“脚本优先、手工命令兜底”
- 修改：`docs/runbook.md`
  - 增加“重启现有版本”和“重建部署最新代码”的区别说明

---

### 任务 1：落地 Unix / Git Bash 部署入口

**文件：**
- 创建：`scripts/deploy.sh`
- 创建：`scripts/tests/test-deploy.sh`

- [ ] **步骤 1：先写 `deploy.sh` 的失败 smoke 测试**

在 `scripts/tests/test-deploy.sh` 中至少覆盖这些场景：
- 无参数时等价于 `deploy-all`
- `--no-verify` 会跳过 `go test` 与前端快速测试，但不会跳过前端构建
- `deploy-app` 不会触发前端构建
- `restart` 在找不到 `app` / `frontend` 容器时返回失败
- `status` 在健康检查失败时仍返回状态摘要
- 健康检查确实调用了 `curl`

测试建议形态：

```bash
#!/usr/bin/env bash
set -euo pipefail

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

mkdir -p "$tmp/repo/scripts" "$tmp/repo/configs" "$tmp/repo/frontend" "$tmp/bin"
cp scripts/deploy.sh "$tmp/repo/scripts/deploy.sh"
printf 'services: {}\n' >"$tmp/repo/docker-compose.yml"
printf 'storage:\n  root_dir: /data\nmysql:\n  dsn: test\n' >"$tmp/repo/configs/config.yaml"

cat >"$tmp/bin/docker" <<'EOF'
#!/usr/bin/env bash
echo "docker $*" >>"$TMP_LOG"
if [[ "$1 $2" == "compose ps" ]]; then
  printf '{"Service":"app","Name":"fetch_bilibili"}\n{"Service":"frontend","Name":"fetch_bilibili_frontend"}\n'
fi
EOF
chmod +x "$tmp/bin/docker"

cat >"$tmp/bin/curl" <<'EOF'
#!/usr/bin/env bash
echo "curl $*" >>"$TMP_LOG"
exit 0
EOF
chmod +x "$tmp/bin/curl"
```

- [ ] **步骤 2：运行 shell smoke 测试，确认当前失败**

运行：

```bash
bash scripts/tests/test-deploy.sh
```

预期：FAIL，提示缺少 `scripts/deploy.sh`，或缺少命令分发 / 日志输出能力。

- [ ] **步骤 3：实现 `scripts/deploy.sh` 的最小可用版本**

要求：
- 默认命令为 `deploy-all`
- 支持 `deploy-all`、`deploy-app`、`restart`、`status`
- 支持全局参数 `--no-verify`
- 先解析仓库根目录，再检查 `docker-compose.yml`、`configs/config.yaml`
- 优先从 `PATH` 寻找命令，找不到时兜底尝试：
  - `docker`
  - `/usr/local/bin/docker`
  - `/Applications/Docker.app/Contents/Resources/bin/docker`
- 显式解析 `curl`，因为 `deploy.sh` 的健康检查依赖它
- 中文日志至少包含步骤编号和失败原因

函数建议最少拆成：

```bash
resolve_repo_root() { ... }
resolve_cmd() { ... }
run_verify_backend() { ... }
run_verify_frontend() { ... }
run_build_frontend() { ... }
run_health_backend() { ... }
run_health_frontend() { ... }
cmd_deploy_all() { ... }
cmd_deploy_app() { ... }
cmd_restart() { ... }
cmd_status() { ... }
main() { ... }
```

- [ ] **步骤 4：重新运行 shell smoke 测试，确认通过**

运行：

```bash
bash scripts/tests/test-deploy.sh
```

预期：PASS，输出类似 `deploy.sh smoke ok`。

- [ ] **步骤 5：补一个 Git Bash 兼容断言**

在 `scripts/tests/test-deploy.sh` 中补一条约束，确保 `deploy.sh` 不依赖纯 Linux 独占语法，例如：
- 不使用 Bash 5 独有特性作为硬前提
- 不依赖 GNU-only `readlink -f`
- 健康检查和 PATH 解析在 Git Bash 下仍能走通

建议增加针对 `.env` 缺失只告警不失败的断言：

```bash
output="$(PATH="$tmp/bin:$PATH" TMP_LOG="$tmp/log" bash "$tmp/repo/scripts/deploy.sh" status || true)"
grep -q "未找到 .env，将使用 Compose 默认镜像配置" <<<"$output"
```

- [ ] **步骤 6：Commit**

```bash
git add scripts/deploy.sh scripts/tests/test-deploy.sh
git commit -m "feat(运维): 增加 Unix 与 Git Bash 部署脚本"
```

### 任务 2：补齐 PowerShell 部署入口

**文件：**
- 创建：`scripts/deploy.ps1`
- 创建：`scripts/tests/test-deploy.ps1`

- [ ] **步骤 1：先写 `deploy.ps1` 的失败 smoke 测试**

在 `scripts/tests/test-deploy.ps1` 中至少覆盖这些场景：
- 无参数时等价于 `deploy-all`
- `--no-verify` 只跳过验证，不跳过前端构建
- `restart` 在容器缺失时失败
- `status` 在健康检查失败时继续输出摘要
- 健康检查不依赖 Windows 上语义不稳定的 `curl` 别名

测试建议形态：

```powershell
$tmp = Join-Path ([System.IO.Path]::GetTempPath()) ("deploy-ps1-" + [guid]::NewGuid())
New-Item -ItemType Directory -Force -Path $tmp | Out-Null
$repo = Join-Path $tmp "repo"
$bin = Join-Path $tmp "bin"
New-Item -ItemType Directory -Force -Path $repo, $bin | Out-Null

@'
services: {}
'@ | Set-Content -Path (Join-Path $repo "docker-compose.yml")
```

- [ ] **步骤 2：运行 PowerShell smoke 测试，确认当前失败**

Windows PowerShell 运行：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/tests/test-deploy.ps1
```

若当前机器装有 `pwsh`，也可以运行：

```bash
pwsh -NoProfile -File scripts/tests/test-deploy.ps1
```

预期：FAIL，提示缺少 `scripts/deploy.ps1` 或命令行为未实现。

- [ ] **步骤 3：实现 `scripts/deploy.ps1` 的最小可用版本**

要求：
- 与 `deploy.sh` 保持同样的命令与参数语义
- 使用 PowerShell 原生函数处理：
  - 参数解析
  - 命令发现（`Get-Command` + 常见兜底路径）
  - 临时重试健康检查
  - JSON / 文本输出
- 健康检查优先使用 `Invoke-WebRequest`，不要依赖 Windows 下语义不稳定的 `curl` 别名
- 中文日志语义与 `deploy.sh` 尽量一致

建议函数最少拆成：

```powershell
function Resolve-RepoRoot { ... }
function Resolve-CommandPath { ... }
function Invoke-BackendVerify { ... }
function Invoke-FrontendVerify { ... }
function Invoke-FrontendBuild { ... }
function Test-BackendHealth { ... }
function Test-FrontendHealth { ... }
function Invoke-DeployAll { ... }
function Invoke-DeployApp { ... }
function Invoke-Restart { ... }
function Invoke-Status { ... }
```

- [ ] **步骤 4：运行 PowerShell smoke 测试，确认通过**

Windows PowerShell 运行：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/tests/test-deploy.ps1
```

预期：PASS，输出类似 `deploy.ps1 smoke ok`。

- [ ] **步骤 5：做一次同语义核对**

对照 `scripts/deploy.sh`，逐项核对：
- 支持的子命令完全一致
- 默认行为一致
- `--no-verify` 语义一致
- 关键日志主文案一致

必要时在 `scripts/tests/test-deploy.ps1` 中补一个 parity 断言，例如：

```powershell
if ($summary.Command -ne 'deploy-all') { throw '默认命令不是 deploy-all' }
```

- [ ] **步骤 6：Commit**

```bash
git add scripts/deploy.ps1 scripts/tests/test-deploy.ps1
git commit -m "feat(运维): 增加 PowerShell 部署脚本"
```

### 任务 3：同步文档并完成验收

**文件：**
- 修改：`README.md`
- 修改：`docs/container-deploy.md`
- 修改：`docs/runbook.md`

- [ ] **步骤 1：先更新 README 的使用入口**

补充内容：
- 默认 `./scripts/deploy.sh`
- Windows Git Bash 示例
- Windows PowerShell 示例
- `deploy-app`、`restart`、`status`、`--no-verify` 示例

- [ ] **步骤 2：更新容器部署文档**

把当前“前端手工构建 + `docker compose up -d --build`”的说明调整为：
- 脚本为主入口
- 手工命令为兜底
- 明确“保存配置触发重启 ≠ 重新构建部署最新代码”

- [ ] **步骤 3：更新运行手册**

补充：
- 什么时候用 `restart`
- 什么时候用 `deploy-all`
- 什么时候用 `deploy-app`
- 常见故障排查：
  - 缺少 `docker` / `npm` / `go`
  - 健康检查超时
  - 容器未创建导致 `restart` 失败

- [ ] **步骤 4：运行 Unix / Git Bash 侧最终验证**

运行：

```bash
bash scripts/tests/test-deploy.sh
```

预期：PASS。

- [ ] **步骤 5：运行 PowerShell 侧最终验证**

Windows PowerShell 运行：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/tests/test-deploy.ps1
```

若当前执行环境存在 `pwsh`，可额外运行：

```bash
pwsh -NoProfile -File scripts/tests/test-deploy.ps1
```

预期：PASS。

- [ ] **步骤 6：做最终仓库级检查**

运行：

```bash
git diff --check
```

预期：PASS，无多余空格、无格式错误。

- [ ] **步骤 7：Commit**

```bash
git add README.md docs/container-deploy.md docs/runbook.md
git commit -m "docs(运维): 补充一键部署脚本使用说明"
```

## 执行注意事项

- 严格遵循 TDD：先写测试，再实现，再验证通过。
- `deploy.sh` 需要兼顾 macOS / Linux 和 Windows Git Bash，不要依赖 `readlink -f`、GNU-only `sed -r` 之类的实现细节。
- `deploy.ps1` 不要调用 `bash` 去复用逻辑；PowerShell 路径必须独立可用。
- `status` 是诊断命令，不是健康门禁命令；服务异常时也要尽量输出可用信息。
- `.env` 缺失只能告警，不能直接阻塞。
- 不要把脚本做成会自动修改配置、自动拉代码或自动安装依赖的“黑箱运维器”。

## 建议的提交顺序

1. `feat(运维): 增加 Unix 与 Git Bash 部署脚本`
2. `feat(运维): 增加 PowerShell 部署脚本`
3. `docs(运维): 补充一键部署脚本使用说明`
