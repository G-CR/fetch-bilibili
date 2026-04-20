# 本地一键构建与部署脚本设计规格

## 背景

当前项目已经具备完整的本地 Docker Compose 部署形态：

- 后端服务通过 `Dockerfile` 构建镜像，再由 `docker-compose.yml` 启动 `app` 服务。
- 前端控制台通过宿主机 `frontend/dist` 构建产物挂载到 `nginx` 容器中提供静态页面。
- 前端设置页目前支持保存 `config.yaml`，保存后会触发后端进程重启，使新配置生效。

但现阶段仍存在一个明显的运维断层：

- 「保存配置」只会重启当前后端进程，不会重新构建后端镜像，也不会重新构建前端静态资源。
- 当仓库代码已经更新，而本地容器仍运行旧镜像时，用户很容易误以为“已经重启 = 已经升级到最新代码”。
- 当前完整部署流程需要人工串行执行多条命令，路径包括前端测试、前端构建、`docker compose up -d --build` 等，步骤较多，容易漏掉。

用户希望新增一个仓库内的本地 CLI 脚本，用于在本机环境下一键完成“验证、构建、部署、校验”，降低日常自用维护成本。

## 目标

- 提供一个仓库内的本地脚本，统一封装前后端的构建与部署动作。
- 默认直接执行脚本时，执行一次完整部署：前端验证、前端构建、后端验证、`docker compose` 拉起。
- 默认采用严格模式：测试失败时中断部署，避免把明显损坏的版本部署到本地运行环境。
- 提供少量明确的子命令，覆盖最常用的部署、重启和状态查看场景。
- 输出全中文日志，便于快速判断当前处于哪一步、失败在哪一步。
- 明确区分“重启现有容器”和“重建后重新部署”这两类行为，减少误解。
- 第一版兼容：
  - macOS / Linux Shell
  - Windows + Docker Desktop + PowerShell

## 非目标

- 第一版不做远程服务器部署，不接入 SSH、CI/CD 或远端容器平台。
- 第一版不做前端按钮，不在 Web 页面里直接触发部署。
- 第一版不做后端部署 API，不把宿主机 Docker 权限暴露给 HTTP 接口。
- 第一版不自动执行 `git pull`、`git fetch`、分支切换或版本选择。
- 第一版不自动安装缺失依赖（例如 `npm install`、`go install`）；只做依赖检查并给出明确提示。
- 第一版不做复杂发布编排，例如灰度、回滚、多环境切换。
- 第一版不承诺兼容 Git Bash、Cygwin 或 WSL，只保证原生 PowerShell 路径可用。

## 方案选择

### 方案一：双入口脚本（推荐）

核心思路：

- 在仓库内新增 `scripts/deploy.sh`。
- 同时新增 `scripts/deploy.ps1`。
- 两个脚本共享同一套命令语义、参数和日志约定。
- `deploy.sh` 覆盖 macOS / Linux，`deploy.ps1` 覆盖 Windows + PowerShell。

优点：

- 兼容当前明确需要的 Windows 原生 PowerShell 场景。
- 仍然保持本地脚本的低成本优势，不需要新增后端模块，也不需要改前端权限模型。
- 可以针对不同平台分别处理命令发现、路径和错误输出，减少跨平台兼容黑盒。

缺点：

- 需要维护两份入口脚本。
- 两份脚本之间必须额外控制语义一致性。

### 方案二：单个 Shell 脚本

核心思路：

- 只保留 `scripts/deploy.sh`，Windows 用户自行通过 Git Bash / WSL 使用。

优点：

- 维护成本最低。
- 当前 macOS / Linux 路径最直接。

缺点：

- 不满足“Windows + PowerShell 原生兼容”的明确需求。
- 会把平台门槛转嫁给使用者。

### 方案三：`Makefile` 为主入口

核心思路：

- 通过 `make deploy`、`make deploy-app`、`make status` 等 target 暴露动作。

优点：

- 命令短。
- 对熟悉 `make` 的开发者友好。

缺点：

- 参数表达和错误处理能力较弱。
- 对 Windows 原生 PowerShell 不友好。

### 方案四：Go 运维 CLI

核心思路：

- 在仓库内新增独立 Go 工具，封装部署动作和状态查询。

优点：

- 结构化最好，后续扩展性强。
- 更容易做复杂参数、子命令和测试。

缺点：

- 对当前需求明显过重。
- 本质上是给本机部署包一层程序，收益不匹配实现成本。

## 最终决策

采用方案一：新增 `scripts/deploy.sh` 与 `scripts/deploy.ps1` 作为双入口、同语义实现。

理由：

- 当前需求边界非常明确：只服务本地开发机、只操作当前仓库、只控制本机 Docker Compose。
- 脚本已经足够覆盖“检查 -> 验证 -> 构建 -> 部署 -> 校验”的串行流程。
- 双入口可以直接满足 Windows + PowerShell 兼容要求，而不需要把用户逼到 Git Bash / WSL。
- 与其把复杂度投入到新 CLI，不如先把日常最容易出错的手工命令收敛成一个稳定入口。

`Makefile` 可以作为后续补充层，但不作为第一版主入口。

## 设计原则

### 1. 默认行为必须安全

- 直接执行 `./scripts/deploy.sh` 时，默认执行完整部署。
- 默认先验证再部署，测试失败即退出，避免把明显坏版本拉起。
- 只有用户显式传入 `--no-verify` 时才跳过验证。

### 2. 日志必须可读

- 全部步骤日志使用中文。
- 每一步都要明确打印“正在做什么”“成功 / 失败”“失败后建议下一步”。
- 失败时优先给出最短路径的排障提示，而不是只回显底层命令错误。

### 3. 重启与部署必须严格区分

- “重启”只针对现有容器，不构建新镜像，不代表升级到最新代码。
- “部署”必须显式包含构建动作，避免用户把重启误当成升级。

### 4. 依赖检查前置

- 脚本启动后第一步必须做环境检查。
- 如果 `docker`、`docker compose`、`go`、`npm`、`configs/config.yaml` 等关键前置条件缺失，立即失败并说明原因。
- `.env` 属于推荐项，不属于硬阻塞项；缺失时给出警告并继续。
- 不在隐式条件不满足时继续执行后续步骤。

### 5. 第一版只做最小闭环

- 只覆盖当前仓库已有的前后端部署路径。
- 只提供少量高频命令，不提前扩展复杂参数矩阵。
- 不把部署脚本和业务逻辑耦合到后端服务内部。

## 脚本入口与命令模型

### 文件位置

- 新增：`scripts/deploy.sh`
- 新增：`scripts/deploy.ps1`

要求：

- 两个文件的命令集合、默认行为、参数名、中文日志语义保持一致。
- 文档示例按平台分别给出，不要求用户自行做参数映射。

### 默认行为

直接执行：

```bash
./scripts/deploy.sh
```

等价于：

```bash
./scripts/deploy.sh deploy-all
```

Windows PowerShell 对应形式：

```powershell
.\scripts\deploy.ps1
```

等价于：

```powershell
.\scripts\deploy.ps1 deploy-all
```

带全局参数时也应保持这个语义，例如：

```bash
./scripts/deploy.sh --no-verify
```

等价于：

```bash
./scripts/deploy.sh deploy-all --no-verify
```

Windows PowerShell 对应形式：

```powershell
.\scripts\deploy.ps1 --no-verify
```

### 支持的子命令

#### 1. `deploy-all`

完整部署，覆盖前后端。

默认流程：

1. 环境检查
2. 后端测试
3. 前端快速测试
4. 前端构建
5. `docker compose up -d --build`
6. 健康检查
7. 输出部署摘要

#### 2. `deploy-app`

只重建并部署后端。

默认流程：

1. 环境检查
2. 后端测试
3. `docker compose up -d --build app`
4. 后端健康检查
5. 输出部署摘要

说明：

- 不重新构建前端。
- 不要求重启或重建 `mysql`。

#### 3. `restart`

只重启当前应用容器，不做构建。

默认流程：

1. 环境检查
2. `docker compose restart app frontend`
3. 健康检查
4. 输出摘要

说明：

- 这是“重启当前版本”，不是“升级到最新代码”。
- 若只需要后端重启，第一版不额外拆分成 `restart-app`，保持命令集最小。
- 如果当前尚未存在 `app` / `frontend` 容器，则直接失败，并提示先执行 `deploy-all`。

#### 4. `status`

输出当前本地部署状态。

默认内容：

- 当前 Git 分支与 `HEAD` 短提交号
- 当前 Git 工作区是否干净
- `docker compose ps`
- 后端 `GET /healthz` 结果
- 前端首页可访问性检查结果
- 后端镜像本地创建时间（若镜像存在）

说明：

- `status` 是诊断命令，不是健康门禁命令。
- 即使后端或前端当前不可访问，也应尽量输出已有信息，而不是过早失败。
- 第一版展示的“镜像创建时间”只能近似反映本地构建时间，不能精确等价于“当前运行代码提交号”。

### 跨平台语义要求

- `deploy.sh` 与 `deploy.ps1` 支持相同的子命令与参数。
- 默认行为一致：无子命令时都执行 `deploy-all`。
- `--no-verify` 语义一致。
- 中文日志主文案尽量保持一致，方便对照排障文档。
- 平台差异只允许存在于命令调用方式、路径分隔符和本机命令发现逻辑。

### 支持的参数

#### `--no-verify`

适用命令：

- `deploy-all`
- `deploy-app`

行为：

- 跳过后端测试与前端快速测试。
- `deploy-all` 仍保留前端构建，因为没有 `dist` 就无法发布最新前端页面。

不适用：

- `restart`
- `status`

## 详细执行流程

### 1. 环境检查

脚本启动后先完成以下检查：

- 当前工作目录是否位于仓库内；若不是，则自动切换到脚本所在仓库根目录。
- 是否存在 `docker-compose.yml`。
- 是否存在 `configs/config.yaml`。
- 是否能找到 `docker`、`go`、`npm`。
- 在 Windows 下额外确认当前运行环境为 PowerShell，并给出 Docker Desktop 前置提示。

考虑到当前项目在不同终端环境下可能存在 `PATH` 差异，脚本需要采用“`PATH` 优先 + 常见本机兜底路径”的方式寻找命令。

Unix 侧例如：

- `docker`
- `/usr/local/bin/docker`
- `/Applications/Docker.app/Contents/Resources/bin/docker`

Windows 侧例如：

- `docker.exe`
- `go.exe`
- `npm.cmd`

`go`、`npm` 也采用同类兜底策略，但第一版只覆盖当前项目已经实际遇到的常见本机路径，不追求任意平台全覆盖。

`.env` 在当前仓库中属于推荐项而不是硬前置项，因为 `docker-compose.yml` 已为镜像变量提供默认值。脚本处理策略为：

- 若 `.env` 存在，则按正常流程继续。
- 若 `.env` 不存在，则打印警告并继续执行，同时提示“将使用 Compose 默认镜像配置”。

如果缺失依赖，脚本立即退出，并输出例如：

```text
未找到 npm 命令，请先安装 Node.js，或确认 npm 已加入 PATH。
```

### 2. 验证阶段

#### 后端验证

命令：

```bash
go test ./... -count=1
```

说明：

- 优先使用当前环境中的 `go`。
- 如脚本已解析到可用绝对路径，则使用绝对路径执行。
- 任一测试失败则立即中断部署。

#### 前端快速验证

命令：

```bash
cd frontend
npm run test:state
npm run test:smoke
```

说明：

- 第一版不默认跑 `test:e2e`，避免把本地部署前验证做得过重。
- `test:vite-config` 也不作为默认严格门槛，保留为手工专项检查。

### 3. 构建阶段

#### `deploy-all`

前端构建：

```bash
cd frontend
npm run build
```

后端 / Compose 构建：

```bash
docker compose up -d --build
```

#### `deploy-app`

只构建并拉起后端：

```bash
docker compose up -d --build app
```

### 4. 校验阶段

#### 后端健康检查

命令：

```bash
curl -sf http://127.0.0.1:8080/healthz
```

行为：

- 采用带重试的等待机制，例如最多等待 60 秒。
- 成功后打印“后端健康检查通过”。
- 超时后打印失败，并附带 `docker compose logs app --tail=200` 的建议。

#### 前端可访问性检查

仅 `deploy-all` 和 `restart` 执行。

命令：

```bash
curl -sf http://127.0.0.1:5173
```

行为：

- 同样采用重试等待。
- 成功后打印“前端页面可访问”。

### 5. 摘要输出

部署成功后统一输出：

- 执行的命令类型
- 是否跳过验证
- 当前 Git 短提交号
- 后端地址：`http://localhost:8080`
- 前端地址：`http://localhost:5173`

## 错误处理

### 1. 验证失败

行为：

- 立即中断。
- 不执行任何部署动作。
- 输出“验证失败，已停止部署”。

### 2. 前端构建失败

行为：

- 立即中断。
- 不继续执行 `docker compose up`。
- 输出最近失败命令及建议排查方向。

### 3. Compose 构建或拉起失败

行为：

- 立即退出非零状态码。
- 输出建议命令：

```bash
docker compose logs app --tail=200
docker compose logs frontend --tail=200
```

### 4. 健康检查失败

行为：

- 视为部署失败。
- 输出容器状态和日志建议，不把“容器已启动”误判为“部署成功”。

## 日志与交互

第一版不做交互式问答，保持脚本适合直接复制、别名调用和后续自动化。

日志风格示例：

```text
[步骤 1/5] 检查部署环境
[步骤 2/5] 执行后端测试
[步骤 3/5] 执行前端快速测试
[步骤 4/5] 构建并拉起容器
[步骤 5/5] 校验服务健康状态
```

要求：

- 中文输出
- 明确步骤编号
- 失败时输出红色或醒目标识（若终端支持）

## 测试与验收标准

### 手工验收场景

#### 场景 1：默认完整部署

执行：

```bash
./scripts/deploy.sh
```

Windows 对应：

```powershell
.\scripts\deploy.ps1
```

预期：

- 先跑后端测试和前端快速测试。
- 前端完成构建。
- `app`、`frontend` 容器更新并保持可用。
- 后端 `healthz` 和前端首页检查通过。

#### 场景 2：跳过验证的快速部署

执行：

```bash
./scripts/deploy.sh --no-verify
```

Windows 对应：

```powershell
.\scripts\deploy.ps1 --no-verify
```

预期：

- 不跑测试。
- 仍然执行前端构建与 Compose 构建。

#### 场景 3：只部署后端

执行：

```bash
./scripts/deploy.sh deploy-app
```

Windows 对应：

```powershell
.\scripts\deploy.ps1 deploy-app
```

预期：

- 不重新构建前端。
- 后端镜像被重建并重新拉起。

#### 场景 4：只重启

执行：

```bash
./scripts/deploy.sh restart
```

Windows 对应：

```powershell
.\scripts\deploy.ps1 restart
```

预期：

- 不执行构建。
- 仅重启 `app`、`frontend`。
- 输出“当前版本重启完成”，避免误导为升级成功。
- 若容器尚未创建，则失败并提示先执行完整部署。

#### 场景 5：依赖缺失

预期：

- 任一关键依赖缺失时立即失败。
- 提示内容明确指向缺失项和处理建议。

### 脚本级验收要求

- 脚本退出码准确：成功返回 `0`，失败返回非 `0`。
- 默认命令和显式 `deploy-all` 行为一致。
- `--no-verify` 只影响验证阶段，不影响构建和健康检查。
- `status` 在服务异常但诊断信息可获取时，仍应输出状态摘要；只有在基础依赖缺失或关键查询命令不可执行时才返回失败。
- 所有输出均为中文。

## 文档联动

实现完成后需要同步更新：

- `README.md`
- `docs/container-deploy.md`
- `docs/runbook.md`

文档重点：

- 新增一键部署脚本的使用说明
- 分别给出 Unix Shell 与 PowerShell 的调用示例
- 说明“保存配置触发重启”与“重新构建部署”之间的区别
- 给出默认部署、后端单独部署、快速跳过验证部署的示例

## 后续扩展方向

第一版完成后，可视实际使用频率再考虑以下增强：

- 增加 `Makefile` 作为命令别名层
- 增加 `restart-app` / `restart-frontend` 细粒度命令
- 增加 `--tail-logs`、`--skip-frontend-build` 等更细参数
- 增加“运行版本信息”展示能力，让脚本和前端都能更准确地判断“当前部署是否等于仓库 HEAD”
