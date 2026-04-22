# AGENTS.md

**版本**：v0.10.0

**最后更新**：2026-04-22

**适用范围**：本仓库内所有代码、配置、文档、测试与脚本改动。

**当前目标**：持续维护仓库级开发规范总入口，新增细则按项评审、落地与验
收。

## 快速目录

可直接搜索下列二级标题快速跳转：

1. 仓库知识导航（当前版本）
2. 事实源与优先级
3. 未提交改动处理
4. 文档冲突记录
5. 版本控制与提交流程
6. 任务追溯与执行记录
7. 分级验证与验收
8. 代码结构与模块边界
9. 配置、迁移与部署变更规则
10. 日志、错误与敏感信息处理
11. 对外接口、状态枚举与前后端契约
12. 测试分层与补测规则
13. 作业状态流转、幂等与恢复规范
14. 仓储、SQL 与事务边界
15. 配置写回与重启语义
16. 文档同步矩阵
17. 依赖与脚本引入规范
18. 文档头字段维护
19. 核心入口

## 仓库知识导航（当前版本）

| 类别 | 路径 | 用途 |
| :--- | :--- | :--- |
| 项目入口 | `README.md` | 项目范围、运行方式、当前能力边界 |
| 需求边界 | `docs/requirements.md` | 产品目标、范围与验收基线 |
| 架构总览 | `docs/architecture.md` | 系统组件、数据流与整体职责划分 |
| Go 结构 | `docs/go-structure.md` | Go 目录结构与模块职责边界 |
| 配置设计 | `docs/config.md` | 配置项定义、默认值与运行约束 |
| API 事实 | `docs/api.md` | HTTP 接口定义与当前已实现能力 |
| 数据模型 | `docs/data-model.md` | 主要实体与字段关系说明 |
| 存储规则 | `docs/storage-policy.md` | 存储容量、清理策略与保留规则 |
| Worker 设计 | `docs/worker.md` | 作业消费模型与当前 worker 行为 |
| 调度设计 | `docs/job-scheduler.md` | 调度链路与任务触发规则 |
| 平台适配 | `docs/bilibili-adapter.md` | B 站适配器行为与接入约束 |
| 数据库结构 | `docs/mysql-schema.md` | MySQL 表结构说明与字段语义 |
| 运行手册 | `docs/runbook.md` | 本地运行、排障与恢复步骤 |
| 容器部署 | `docs/container-deploy.md` | Docker / Compose 部署约束与流程 |
| 开发细则 | `docs/dev-standards.md` | 仓库级规范的补充细则；测试与覆盖率要求当前以本文件第 2 节为入口 |
| 需求设计记录 | `docs/superpowers/specs/` | 需求设计；命名遵循 `YYYY-MM-DD-slug.md` |
| 执行计划记录 | `docs/superpowers/plans/` | 执行计划与验收上下文；命名遵循 `YYYY-MM-DD-slug.md` |
| 规划参考 | `docs/roadmap.md`、`docs/todo.md` | 路线图与待办，供优先级讨论参考 |

- `docs/superpowers/specs/` 与 `docs/superpowers/plans/` 下的新文件统一使用 `YYYY-MM-DD-slug.md`。
- `slug` 使用小写 `kebab-case`，只描述当前主题，不重复日期。

## 事实源与优先级

- 仓库内已版本化的代码、配置、文档、测试是唯一事实源。
- 禁止把聊天结论、口头约定、临时文件或未提交草稿当作高于仓库内容的依据。
- 判断当前行为时，优先查看代码、测试和可复现的命令结果。
- 判断目标边界、设计约束与文档优先级时，按以下顺序处理：
  1. `AGENTS.md`
  2. `docs/requirements.md`
  3. `docs/architecture.md`
  4. 主题文档：`docs/config.md`、`docs/api.md`、`docs/runbook.md`、`docs/go-structure.md`、`docs/storage-policy.md`、`docs/data-model.md`、`docs/worker.md`、`docs/job-scheduler.md`、`docs/bilibili-adapter.md`、`docs/mysql-schema.md`、`docs/dev-standards.md`
  5. `docs/superpowers/specs/` 与 `docs/superpowers/plans/` 中的任务级设计与执行记录
  6. `README.md`
- 测试、覆盖率与变更后的最小验收要求，当前统一从 `docs/dev-standards.md` 进入；若仓库后续新增 CI 或门禁工作流，以仓库内实际配置文件为准，禁止凭空假设。
- 若文档与当前实现冲突，先以代码、测试和运行结果确认现状，再在本次变更中显式记录冲突点；禁止按记忆或猜测处理。

## 未提交改动处理

- 执行任何改动前，必须先运行 `git status --short` 检查工作树状态。
- 允许在有未提交改动的工作树上继续，但必须兼容这些改动。
- 这里的“兼容”指：不覆盖、不回滚、不绕开已有未提交改动，且最终结果同时保留原改动意图与本次任务目标。
- 若当前任务触达同一文件，但改动区域可明确区分，允许直接在现状基础上继续修改。
- 若当前任务与已有未提交改动发生同一区域重叠，且无法确定如何同时保留双方意图，
  必须停止并先询问，禁止自行 `stash`、`reset`、`checkout`、`restore`
  或覆盖式改写。
- 禁止覆盖、回滚或忽略与当前任务相关的未提交内容。

## 文档冲突记录

- 若仓库文档与当前实现不一致，先用代码、测试或可复现命令确认现状，再决定裁决方向。
- 记录冲突与裁决时，优先写在本次实际修改的目标文档中，确保后续阅读者能直接看到结果。
- 若本次任务已有对应的 `docs/superpowers/plans/YYYY-MM-DD-slug.md`，同步在该计划中补一条决策记录，说明冲突来源、裁决依据与落地结果。
- 仅当冲突本身影响仓库级治理规则时，才直接修改 `AGENTS.md`。

## 版本控制与提交流程

### 默认分支

- `master` 是当前稳定分支。
- 非明确授权场景下，禁止把非琐碎开发改动直接提交到 `master`。
- 错字、纯排版这类极小文档修正，只有在任务明确要求时才允许直接落
  `master`。

### 开发分支

- 所有非琐碎开发任务默认在短期分支上完成。
- 新分支命名统一使用 `<type>/<slug>`。
- `type` 与提交类型保持一致，当前允许：
  `feat`、`fix`、`docs`、`refactor`、`test`、`chore`、`build`、`ci`、
  `perf`、`revert`。
- `slug` 使用小写 `kebab-case`，只描述任务主题，不重复日期、作者或工
  具名。
- 已存在的历史分支命名不追溯修改；本规范只约束新增分支。
- 分支合并完成后，应及时删除短期分支，避免长期漂移。

### 提交信息格式

- 提交信息统一使用 `type(scope): summary`。
- `scope` 可选，但推荐填写；优先使用仓库内真实模块或领域名，可使用中
  文，如 `发现`、`运维`、`前端`、`博主管理`、`存储`、`调度`、`配置`。
- `summary` 只描述本次提交的直接结果，不写空话，不以句号结尾。
- 一个 commit 只解决一个清晰目标，禁止把互不相关的代码、文档、部署改
  动混在同一提交。
- 若本次提交按 `docs/superpowers/plans/` 分阶段落地，允许在摘要末尾补充
  阶段标记，如 `（任务 3/8）`。

### 提交粒度与自验

- 若无法用一句话说清 commit 做了什么，说明粒度过大，应该继续拆分。
- 配置变更、数据库迁移、接口变更、部署脚本调整属于高风险提交，禁止与
  无关重构混提。
- 提交前至少执行本次变更命中的最小验证；测试与覆盖率要求以
  `docs/dev-standards.md` 为准。
- 提交人必须能说明本次实际跑过的命令；若未运行，必须明确标注未验证及
  原因。
- 文档或规范变更至少通过对应的 Markdown 校验。

### 评审与合并

- 默认流程为：建分支 -> 完成改动 -> 自验 -> 审核或评审 -> 合并。
- 若通过远端协作合并到 `master`，应优先使用 PR 承载评审；若当前任务明
  确要求本地直提，可跳过 PR，但不能跳过自验。
- 当前仓库虽未看到 PR 模板或流水线配置，但 PR 描述至少应包含：
  1. 目标与背景
  2. 主要改动点
  3. 验证命令与结果
  4. 是否涉及配置、迁移、接口或部署影响
  5. 文档是否已同步

### 历史文档兼容

- 既有提交历史保持不改写。
- 既有旧风格分支如 `codex-delete-creator-api` 不强制重命名。
- 本规范只约束生效后的新增分支与新增提交。

## 任务追溯与执行记录

### 目标

- 中大型改动必须能回答 3 个问题：
  1. 为什么做
  2. 打算怎么做
  3. 实际怎么验收
- 任务追溯以 `docs/superpowers/specs/` 和 `docs/superpowers/plans/` 为主入口。

### 文档分工

- `docs/superpowers/specs/YYYY-MM-DD-slug.md`
  用于记录需求背景、约束、方案比较、最终设计与非目标。
- `docs/superpowers/plans/YYYY-MM-DD-slug.md`
  用于记录实施拆分、涉及文件、验证步骤、阶段提交与完成状态。
- `spec` 解决“为什么这样设计”。
- `plan` 解决“准备怎么落地和怎么验收”。

### 何时必须创建 spec

- 出现以下任一情况，必须先创建或补齐 `spec`：
  - 新功能或新页面
  - 对外接口语义变更
  - 数据模型、迁移或存储策略变更
  - 调度、恢复、部署、风控等跨模块行为变更
  - 需要在 2 种以上方案间做取舍的任务
- 以下情况可不单独创建 `spec`：
  - 错字、排版、注释修正
  - 不改变行为的局部重构
  - 已有 `spec` 明确覆盖范围内的小步实现

### 何时必须创建 plan

- 出现以下任一情况，必须创建或补齐 `plan`：
  - 需要分 2 步以上实施
  - 涉及多个文件或跨前后端
  - 需要数据库迁移、配置变更或部署动作
  - 需要分阶段提交或分阶段验收
  - 需要记录“已完成 / 待完成 / 放弃”的执行状态
- 以下情况可不单独创建 `plan`：
  - 单文件小修且可以在一个 commit 内闭环
  - 纯文档微调，且无需阶段性验收

### 关联规则

- 有 `spec` 的任务，`plan` 应尽量使用相同主题 `slug`。
- 实现提交、PR 描述或本地验收说明，应显式引用对应的 `spec` / `plan`
  路径。
- 若某次改动没有单独建 `spec` / `plan`，提交说明中必须能说清目标、范围
  与验证结果。

### 最小内容要求

- `spec` 最少包含：
  - 背景
  - 目标
  - 非目标
  - 方案选择
  - 风险或约束
- `plan` 最少包含：
  - 目标
  - 涉及文件
  - 分步任务
  - 验证命令
  - 完成状态

### 验收闭环

- 任务完成后，实际执行的验证结果必须回写到 `plan`、提交说明或 PR 描述
  中的至少一个位置。
- 若实现过程中偏离原 `spec`，必须同步更新 `spec` 或在 `plan` 中记录偏
  差与原因。
- 已完成但仍有效的 `spec` / `plan` 不删除，继续作为后续任务的事实源。

### 追溯历史兼容

- 历史已有 `spec` / `plan` 文件不追溯改模板。
- 本规范只约束新增任务与被重新触达的活动任务。

## 分级验证与验收

### 验收原则

- 任何改动在提交前都必须命中至少一组与变更类型对应的最小验证。
- 验证命令只以仓库内现有脚本、测试命令和运行入口为准，禁止自造“等
  价”替代命令。
- 若受环境限制无法执行某项验证，必须明确记录未执行项、阻塞原因和潜在
  风险；禁止把未执行写成“已验证”。

### 命中规则

- 只改 Markdown 文档：命中文档验证。
- 修改 `frontend/` 下的源码、脚本、构建配置或前端文档：命中前端验证。
- 修改 `*.go`、`go.mod`、`go.sum`：命中后端验证。
- 修改 `configs/`、`migrations/`、`Dockerfile`、`docker-compose.yml`、
  `scripts/deploy.*`、部署或运行手册：命中运行与部署验证。
- 若同时跨前后端或跨代码与部署，必须分别执行对应集合；不能只跑一边。

### 最小验证集合

#### 文档验证

- 至少校验本次修改的 Markdown 文件。
- 推荐命令：`markdownlint <changed-md-files>`

#### 前端验证

- 命中前端变更时，至少执行：
  1. `cd frontend && npm run test:state`
  2. `cd frontend && npm run test:vite-config`
  3. `cd frontend && npm run test:smoke`
  4. `cd frontend && npm run build`
- 若改动涉及前后端接口联动、实时事件、E2E 脚本或驾驶舱集成链路，再补：
  `cd frontend && npm run test:e2e`

#### 后端验证

- 命中 Go 代码变更时，至少执行：`go test ./...`
- 若改动涉及核心业务逻辑、状态流转、调度、恢复、数据库或 API 语义，
  追加：`go test ./... -cover`
- 覆盖率门槛仍以 `docs/dev-standards.md` 为准。

#### 运行与部署验证

- 命中配置、迁移、启动链路、 Compose 或部署脚本时，至少执行：
  1. `go test ./...`
  2. `docker compose config`
- 若改动涉及部署脚本：
  1. `bash scripts/tests/test-deploy.sh`
  2. `pwsh -NoProfile -File scripts/tests/test-deploy.ps1`
- 当前环境若不具备 `pwsh`，PowerShell smoke 不算通过，必须记录为“未执行”
  并说明原因。
- 若改动直接影响服务启动、配置加载、迁移或健康检查，并且本地依赖已就
  绪，再追加运行时验收：
  1. `go run ./cmd/server`
  2. `curl -sf http://127.0.0.1:8080/healthz`

### 阻断规则

- 命中的最小验证集合中，只要有一项新增失败，就不得声称任务完成。
- 纯因本地环境缺依赖而无法执行的验证，不算通过；必须显式标记为“未执
  行”。
- 文档、代码、配置、部署类改动都必须在验收说明中写明实际跑过的命令。

### 验收记录位置

- 优先记录在对应的 `docs/superpowers/plans/...` 中。
- 若没有单独 `plan`，则记录在 PR 描述或最终交付说明中。
- 记录内容至少包括：
  - 实际执行的命令
  - 结果（通过 / 失败 / 未执行）
  - 未执行原因（如有）

### 验证历史兼容

- 既有历史任务不追溯补跑验证。
- 本规范只约束生效后的新增任务和被重新触达的活动任务。

## 代码结构与模块边界

### 结构原则

- 文件落点优先遵循仓库当前实际结构，而不是理想化重构后的目录图。
- 新需求优先扩展已有模块，禁止因为一个局部需求平行新开一套职责相近的
  目录。
- 若需要引入新的顶级目录或跨模块搬迁文件，必须先有 `spec` 或 `plan`
  说明原因与边界。

### 后端目录职责

- `cmd/server`
  - 只负责进程入口、配置路径解析、信号处理和启动循环。
  - 禁止写业务逻辑、SQL 或 HTTP 路由细节。
- `internal/app`
  - 负责依赖装配、启动顺序、恢复流程和服务组装。
  - 可以引用具体实现，如 `internal/repo/mysql`。
  - 不负责解析 HTTP 请求或实现具体仓储 SQL。
- `internal/api/http`
  - 只负责 HTTP 协议层：路由、参数解析、响应编码、协议级错误。
  - 应依赖 service interface，禁止直接依赖 `internal/repo/mysql`、
    `database/sql` 细节或外部平台调用细节。
- `internal/repo`
  - 放仓储接口、共享查询过滤器和持久化层共用模型。
  - 不放 HTTP、调度或平台适配逻辑。
- `internal/repo/mysql`
  - 只放 MySQL 实现。
  - 禁止承担业务编排、HTTP 响应或调度入口职责。
- `internal/platform/bilibili`
  - 只处理 B 站外部能力接入、鉴权、风控、请求与响应适配。
  - 禁止直接写数据库、拼 HTTP 返回或调度任务生命周期。
- `internal/creator`、`internal/dashboard`、`internal/discovery`、
  `internal/jobs`、`internal/scheduler`、`internal/worker`、
  `internal/library`、`internal/live`
  - 按各自主题承载业务行为。
  - 新代码应优先放回对应主题模块，不要绕过模块边界把逻辑堆到 `app`
    或 `api/http`。

### 依赖约束

- 除 `internal/app`、启动相关代码和测试外，其他业务代码默认不应直接
  import `internal/repo/mysql`。
- HTTP handler 只面向 service interface 编程；若需要新增能力，优先扩
  service interface，而不是在 handler 中下沉到具体实现。
- `scheduler` 只负责任务触发与编排，不直接执行下载、检查、清理等具体
  I/O。
- `worker` 负责消费和执行任务，不负责暴露 HTTP 接口或定义持久化实现。
- `live` 事件流只用于推送与订阅，不得作为业务真源替代数据库。
- `config` / `db` 目录只处理配置加载与数据库初始化、迁移，不承载业务规
  则。

### 前端边界

- `frontend/src/lib/api.js`
  - 只负责请求封装、接口路径、流连接和错误格式化。
  - 不放 JSX、DOM 操作或页面级状态。
- `frontend/src/lib/state.js`
  - 只负责状态归一化、派生计算、持久化和事件应用。
  - 不发请求、不渲染 UI。
- `frontend/src/App.jsx`
  - 负责页面编排、交互流程和把 `api.js` / `state.js` 串起来。
  - 若同类展示块明显增多，应优先拆组件，而不是继续把纯展示逻辑堆大。
- `frontend/src/styles.css`
  - 只负责样式与视觉规则，不写业务语义。

### 变更边界

- 功能改动默认只触达完成该目标所需的最小模块集合。
- 若当前任务只是功能修复，不得顺手做无关目录重组。
- 若结构问题已经明显阻碍当前任务，可在 `plan` 中明确记录后做“顺手整
  治”，但范围必须受控。

### 文档同步

- 新增顶级目录、改变模块职责或引入新的主要依赖方向时，必须同步更新：
  - `docs/go-structure.md`
  - `docs/architecture.md`
- 若只是模块内部小改，不强制改结构文档。

### 结构历史兼容

- 当前仓库已有结构按现状承认，不追溯按理想图重排。
- 本规范只约束新增代码和被重新触达的模块。

## 配置、迁移与部署变更规则

### 变更目标

- 配置、数据库和部署链路属于高风险改动，必须保证代码、迁移、脚本、文
  档与验证同步收敛。
- 任何会影响启动、建库、运行参数、Compose 行为或本地部署路径的改动，
  都不能只改其中一层。

### 配置变更

- 运行配置以 `configs/config.yaml` 为实际入口，以
  `configs/config.example.yaml` 为示例入口。
- 新增、删除或修改配置项时，必须同步更新：
  1. `docs/config.md`
  2. `configs/config.example.yaml`（若该项属于示例配置）
  3. 涉及启动或部署的文档，如 `README.md`、`docs/runbook.md`、
     `docs/container-deploy.md`
- 禁止在示例配置、README 或部署文档中写入真实密钥、真实 Cookie 或只适
  用于单人机器的私有路径。
- 若配置变更会影响前端设置页保存、重启或恢复行为，必须同步核对
  `/system/config` 相关实现与文档。
- 删除配置项或改变默认值时，必须在 `spec` 或 `plan` 中写明兼容策略与
  影响范围。

### 数据库迁移

- 数据库结构变更必须通过 `migrations/*.sql` 落地，禁止只改
  `docs/mysql-schema.md` 或只靠手工 SQL。
- 当前权威 schema 以迁移文件为准，`docs/mysql-schema.md` 只负责说明，
  不是执行入口。
- 新迁移文件命名保持当前序号风格：`migrations/000NN_<slug>.sql`。
- 每个迁移都必须同时提供 `-- +goose Up` 与 `-- +goose Down`；若确实不
  可逆，必须在 `spec` 或 `plan` 中明确说明原因。
- 结构变更应优先选择向后兼容路径：
  - 新字段尽量可空或带默认值
  - 枚举扩展优先兼容旧值
  - 高风险表结构调整先写兼容方案，再落迁移
- 若迁移修改了状态枚举、任务类型、字段语义或索引假设，必须同步更新：
  1. `docs/mysql-schema.md`
  2. 对应 repo / service / API 文档
  3. 相关测试

### 部署与运行链路

- 修改 `Dockerfile`、`docker-compose.yml`、`.env.example`、
  `scripts/deploy.sh`、`scripts/deploy.ps1` 时，必须同步检查：
  1. `README.md`
  2. `docs/container-deploy.md`
  3. `docs/runbook.md`
- `scripts/deploy.sh` 与 `scripts/deploy.ps1` 的命令语义必须保持一致；
  新增子命令、参数或默认行为时，不能只改一边。
- 若变更影响端口、健康检查地址、镜像变量、挂载路径或前端构建前置条
  件，必须同步更新全部相关文档与 smoke 测试。
- 运行文档中出现的手工命令应与脚本默认行为保持一致，禁止脚本文档和
  README 各说各话。

### 最小验证要求

- 配置变更至少命中：
  1. 对应的文档同步检查
  2. `go test ./...`
- 迁移变更至少命中：
  1. `go test ./...`
  2. 与迁移或启动链路相关的测试或启动验收
- 部署脚本变更至少命中：
  1. `bash scripts/tests/test-deploy.sh`
  2. `pwsh -NoProfile -File scripts/tests/test-deploy.ps1`
- `Dockerfile` 或 `docker-compose.yml` 变更至少命中：
  1. `docker compose config`
  2. “分级验证与验收”中的运行与部署验证

### 配置与部署阻断规则

- 只改说明文档而不改实际迁移、脚本或示例配置，不算完成。
- 只改迁移或脚本而不更新相关文档，不算完成。
- 无法说明配置、迁移或部署改动对启动与兼容性的影响，不得声称验收通过。

### 配置与迁移历史兼容

- 历史迁移文件不因本规范生效而回补命名或模板。
- 本规范只约束新增迁移、被重新触达的配置项和新增部署改动。

## 日志、错误与敏感信息处理

### 处理目标

- 在保证可排障的前提下，统一日志、HTTP 错误、SSE 事件、状态接口与文档
  示例中的信息边界。
- 这项规则重点约束 `internal/api/http`、`internal/dashboard`、
  `internal/platform/bilibili`、`internal/config` 及命中的相关文档。

### 敏感信息边界

- 禁止把原始 `cookie`、`SESSDATA`、鉴权 Header、数据库密码、完整 DSN、
  第三方 Token、完整上游响应体写入日志、HTTP 响应、SSE 事件、示例配置、
  `spec`、`plan`、README 或运行手册。
- `GET /system/status` 与 `system.changed` 可以继续暴露驾驶舱所需的摘要运
  行态，例如：
  - 是否已配置
  - 是否登录
  - `mid`、`uname`
  - cookie 来源
  - 最近检查结果
  - 最近重载结果
  - 风控是否激活
  - 退避截止时间
- 现有兼容字段如 `error`、`last_error`、`last_reason` 若继续保留，新增或
  修改时必须先确认其中不包含凭证原文、上游完整报文、本地绝对路径、完整
  SQL 或其他环境敏感细节；若命中，必须先脱敏再暴露。

### 返回与记录规则

- HTTP `4xx` 可以返回稳定、可操作的中文错误，前提是该信息属于参数校验、
  资源不存在、状态冲突等可公开业务语义。
- HTTP `5xx`、数据库错误、网络错误、外部平台错误默认只返回稳定摘要，
  不直接把 `err.Error()` 原样透传给前端。
- 日志统一使用中文，并记录最小必要上下文；优先补充 `id`、`uid`、任务
  类型、状态、页码、接口动作等排障字段，禁止整对象打印配置、请求头、响
  应体或凭证相关结构。
- 同一失败链路优先在边界层记录一次关键日志；底层继续返回带上下文的错
  误即可，避免多层重复打印造成噪音和重复泄露面。

### 文档与示例约束

- `configs/config.example.yaml`、README、`docs/config.md`、
  `docs/runbook.md`、`docs/container-deploy.md` 与任务文档中，只允许使
  用占位符或脱敏示例。
- 若改动 `/system/status`、`system.changed`、B 站鉴权状态、风控状态或
  错误返回语义，必须同步更新命中的说明文档；通常至少检查：
  1. `docs/api.md`
  2. `docs/config.md`
  3. `docs/bilibili-adapter.md`

### 契约验证入口

- 触达 `internal/api/http`、`internal/dashboard`、
  `internal/platform/bilibili` 的错误返回、日志打印或敏感字段暴露时，至
  少命中“分级验证与验收”中的后端验证。
- 若同时改动了接口返回字段或状态字段说明，必须同步更新对应 handler /
  service 测试与 `docs/api.md`。
- 若只做脱敏、摘要化或日志边界调整，也不能跳过验证；至少需要说明实际跑
  过的测试命令。

### 日志规则历史兼容

- 历史日志与错误返回不做追溯式重写。
- 本规范只约束新增代码、被重新触达的错误返回路径、状态接口字段和文档示
  例。

## 对外接口、状态枚举与前后端契约

### 契约范围

- 除 `GET /healthz`、`GET /readyz` 这类基础探针外，当前仓库对前端、脚
  本或人工运维暴露的 HTTP 返回都属于对外契约。
- `/events/stream` 中的事件类型与 payload 与 HTTP API 同级，视为稳定契
  约，不得当作“内部实现细节”随意改动。
- 前端通过 `frontend/src/lib/api.js` 消费的数据结构，以及
  `frontend/src/lib/state.js` 中的归一化映射，是当前前后端契约的落地
  面。

### 字段与格式约束

- 后端对外 JSON 字段统一使用 `snake_case`。
- 前端内部状态可继续使用 `camelCase`，但字段兼容、默认值回填和旧字段
  兜底逻辑应集中收敛在 `frontend/src/lib/state.js`，不要把契约转换散落
  到页面组件。
- HTTP 与 SSE 中的时间字段统一输出 RFC 3339 字符串；空时间保持空字符
  串，不混用 Unix 时间戳、中文格式时间或多套格式。
- 查询参数名、默认值、分页规则和过滤语义属于契约的一部分；改动时必须同
  步更新 handler、前端调用处、`docs/api.md` 与相关测试。

### 状态枚举约束

- `creator.status`、`candidate.status`、`job.status`、`video.state`、
  `/system/status` 中的状态字段，以及 SSE 中对应的增量状态，统一以当前
  实现与 `docs/api.md` 为准。
- 新增、删除、重命名状态值时，必须同步修改：
  1. 对应 handler / service 返回
  2. 前端归一化与展示逻辑
  3. `docs/api.md`
  4. 命中的测试
- 不得只改前端展示文案而默认假设后端状态语义未变；也不得只改后端枚举而
  让前端靠兜底逻辑“猜”新状态。

### 兼容与变更策略

- 对外字段变更默认优先追加，不直接删除或重命名既有字段。
- 若必须做破坏性变更，必须先有对应的 `spec` / `plan`，明确兼容期、影响
  面、迁移路径与验收方式。
- 若同一语义同时存在 HTTP 全量快照和 SSE 增量事件，变更时必须同步核对
  两边，禁止只改一侧。

### 命中验证入口

- 触达 `internal/api/http`、`docs/api.md`、`frontend/src/lib/api.js`、
  `frontend/src/lib/state.js` 或 SSE 事件 payload 时，至少命中“分级验证
  与验收”中的后端验证。
- 若前端消费链路、页面状态归一化或事件应用逻辑也被触达，再补：
  1. `cd frontend && npm run test:state`
  2. 视影响面补 `cd frontend && npm run test:smoke`
  3. 涉及实时事件或端到端交互时补 `cd frontend && npm run test:e2e`
- 只改接口文档而不核对实现，或只改实现而不更新 `docs/api.md`，都不算完
  成。

### 接口历史兼容

- 历史接口不因本规范生效而追溯重写。
- 本规范只约束新增接口、被重新触达的字段、状态值、查询参数和 SSE
  payload。

## 测试分层与补测规则

### 分层原则

- 测试类型必须与变更层次对应，优先补最靠近行为边界的测试；不能只靠一次
  笼统的 `go test ./...` 通过，就声称命中的实现细节已被覆盖。
- 当前仓库至少承认以下测试层：
  1. HTTP / SSE 协议层：`internal/api/http/router_test.go`
  2. 配置解析与写回：`internal/config/*_test.go`
  3. jobs / worker / recovery：`internal/jobs/*_test.go`、
     `internal/worker/*_test.go`、`internal/app/recovery_test.go`
  4. MySQL 仓储层：`internal/repo/mysql/*_test.go`
  5. 前端状态与快速渲染：`frontend/scripts/test-dashboard-state.mjs`、
     `frontend/scripts/smoke-render.mjs`
  6. 前端端到端与专项检查：`frontend/scripts/e2e/*`、
     `frontend/scripts/check-vite-react-plugin.mjs`
- 修复 bug、回归或状态漂移问题时，优先补能稳定复现该问题的测试；若当前
  场景确实无法稳定测试，必须在验收说明里解释原因。

### 补测要求

- 触达 `internal/api/http` 中的路由、参数解析、状态码、错误返回、SSE
  payload 或 `/system/config` 语义时，必须同步更新对应 router 测试。
- 触达 `internal/repo/mysql` 中的查询条件、排序、分页、状态过滤、去重或
  事务行为时，必须同步更新该包下的仓储测试。
- 触达 `internal/jobs`、`internal/worker`、`internal/app` 中的任务编排、
  状态流转、恢复或事件发布时，必须同步更新对应 service / worker /
  recovery 测试。
- 触达 `internal/config` 中的解析、默认值、保存、重启调度行为时，必须同
  步更新 `config_test.go` 或 `editor_test.go`。
- 触达 `frontend/src/lib/api.js`、`frontend/src/lib/state.js`、
  `frontend/src/App.jsx` 中的前后端映射、快照归一化、SSE 应用或配置保存
  交互时，至少补 `cd frontend && npm run test:state`；再按影响面补
  `test:smoke`、`test:e2e`。
- 若只是重构且理论上不改行为，也必须确认现有测试已覆盖命中路径；不能以
  “只是重构”为由跳过核对。

### 测试规则历史兼容

- 历史任务不因本规范生效而追溯补齐测试矩阵。
- 本规范只约束新增代码和被重新触达的测试命中路径。

## 作业状态流转、幂等与恢复规范

### 状态与类型边界

- 任务类型统一以 `internal/jobs/types.go` 为准，当前仅承认：
  `fetch`、`download`、`check`、`cleanup`、`discover`。
- 任务状态统一以 `internal/jobs/types.go` 为准，当前仅承认：
  `queued`、`running`、`success`、`failed`。
- 对外 API、SSE、数据库记录、前端展示与文档必须使用同一套任务类型和状
  态值，禁止在单个模块内私自引入别名或同义新值。

### 幂等与事件规则

- 新任务统一从 `queued` 入队；worker 抢占后转为 `running`，执行结束后
  只能进入 `success` 或 `failed`。
- 当前“活动任务去重”的事实边界是：`queued` 与 `running` 视为活动任
  务；命中重复任务时返回 `jobs.ErrJobAlreadyActive`，上层应按“已存在，
  无需重复创建”处理，而不是当作硬失败。
- 去重键、payload 字段与“什么算重复任务”的判断，统一收敛在仓储层；若
  调整去重语义，必须同步更新 jobs / repo 测试与命中文档。
- `job.changed` 事件只在真实入队成功或真实状态变化后发布；不得为被去重
  拦截的任务、失败的状态写入或未落库的过渡态发布“幽灵事件”。

### 启动恢复规则

- 启动恢复只允许修复仓库当前已定义的残留状态：
  1. `running` 任务重新入队为 `queued`
  2. 无活动下载任务支撑的 `DOWNLOADING` 视频修复为 `NEW` 或 `DOWNLOADED`
  3. 缺失有效文件的 `DOWNLOADED` 视频回退为 `NEW`
- 恢复逻辑必须满足幂等：同一次修复可以重复执行，不应越修越乱或制造重复
  任务。
- 新增恢复动作、状态修正规则或跨表回滚逻辑时，必须先有 `spec` / `plan`
  说明触发条件、边界与验证方式。

### 作业历史兼容

- 历史任务记录不因本规范生效而重写。
- 本规范只约束新增任务类型、被重新触达的状态流转逻辑和恢复路径。

## 仓储、SQL 与事务边界

### 仓储职责

- `internal/repo` 定义接口、过滤器和共享模型；`internal/repo/mysql`
  只负责持久化、查询拼装、行映射与数据库级约束处理。
- 仓储层禁止承担 HTTP 协议、外部平台调用、事件发布、文件删除、重启调
  度等业务编排职责。
- HTTP handler、平台适配器和前端状态逻辑不得直接拼 SQL；所有数据库访
  问都必须经由仓储接口或初始化/迁移入口。

### SQL 与错误映射

- 数据库访问统一使用带 `Context` 的 `ExecContext` / `QueryContext` /
  `QueryRowContext`。
- 查询默认值、排序顺序、分页上限和状态过滤必须在 SQL 或过滤器中显式表
  达，不能依赖 MySQL 的隐式顺序。
- `NULL`、JSON 字段、时间字段和枚举值的扫描 / 反序列化，应在仓储层完
  成，不把底层扫描细节泄露到 service / handler。
- 需要让上层做分支判断的数据库错误，优先映射为稳定的 repo / domain
  错误，例如 `repo.ErrConflict`、`repo.ErrNotFound`；不要让 handler 直
  接依赖 MySQL 错误码分支。

### 事务使用规则

- 只有当同一仓储操作需要保证多条 SQL 原子性时，才在仓储层开事务；当前
  已有模式包括任务抢占和候选来源 / 评分明细替换。
- 事务范围必须尽量小；事务内只做数据库读写，不做事件发布、网络请求、磁
  盘 I/O 或长时间计算。
- 事务中的所有失败路径都必须显式回滚；提交后再做事务外副作用。
- 若需要跨多个仓储接口保证原子性，必须先有 `spec` / `plan` 明确边界，
  禁止在 handler 或 service 中临时拼接“半事务”流程。

### 仓储历史兼容

- 历史 SQL 风格不做追溯式统一重写。
- 本规范只约束新增仓储实现和被重新触达的 SQL / 事务路径。

## 配置写回与重启语义

### 写回前提

- `PUT /system/config` 当前是整份配置文档写回，不是字段级 patch。
- 写回前必须先读取当前文件内容；若内容未变化，返回 `changed=false`，
  且不得安排重启。
- 写回前必须先通过 YAML 与业务配置解析校验；校验失败时不得部分落盘。
- 写回时应尽量保留现有文件权限；不得因为一次保存把配置文件权限改成不可
  预期状态。

### 重启语义

- `restart_scheduled=true` 只表示配置已成功写回，且重启请求已发出；不表
  示服务已经重启完成或新配置已经对外可用。
- “保存配置触发重启”只针对当前后端进程生效，不等价于重新构建镜像、重新
  部署最新代码或刷新前端静态资源。
- 重启窗口内接口短暂失败属于当前仓库承认的正常现象；前端与运行文档应按
  此事实处理，不得宣称“保存后立即无缝生效”。

### 配置写回校验

- 触达 `internal/config/editor.go`、`/system/config` 返回语义、前端配置
  保存提示或重启状态逻辑时，至少同步执行：
  1. 命中的 `internal/config/*_test.go`
  2. 命中的 `internal/api/http/router_test.go`
  3. “分级验证与验收”中的后端验证
- 若前端保存态、重启态或恢复提示也被触达，再补：
  1. `cd frontend && npm run test:state`
  2. `cd frontend && npm run test:smoke`
- 同步更新命中的文档，通常至少检查：
  1. `docs/api.md`
  2. `docs/config.md`
  3. `docs/runbook.md`
  4. `README.md`
  5. `docs/container-deploy.md`（若 Compose 行为受影响）

### 配置写回历史兼容

- 历史配置保存记录不追溯补写语义说明。
- 本规范只约束新增写回逻辑和被重新触达的配置保存 / 重启路径。

## 文档同步矩阵

### 同步规则

- 文档同步是交付物的一部分；对实现、接口、运维动作或用户可见行为的改
  动，不得只改代码不改文档。
- 同一主题的代码、配置、脚本和文档应在同一批改动中收敛，避免形成“代码
  已变、文档下一次再补”的长期漂移。

### 常见命中矩阵

- 触达 HTTP API、SSE、状态字段、查询参数：
  - 至少检查 `docs/api.md`
- 触达配置项、默认值、配置保存、重启行为：
  - 至少检查 `docs/config.md`、`docs/runbook.md`、`README.md`
  - 若部署方式受影响，再补 `docs/container-deploy.md`
- 触达 worker、scheduler、恢复、任务状态流转：
  - 至少检查 `docs/worker.md`、`docs/job-scheduler.md`、`docs/runbook.md`
- 触达存储策略、清理规则、浏览目录或投影产物：
  - 至少检查 `docs/storage-policy.md`、`docs/data-model.md`、
    `docs/runbook.md`
- 触达数据库结构、状态枚举、迁移语义：
  - 至少检查 `docs/mysql-schema.md`
  - 若字段对外暴露，再补 `docs/api.md`、`docs/data-model.md`
- 触达部署脚本、端口、健康检查、容器行为：
  - 至少检查 `README.md`、`docs/container-deploy.md`、
    `docs/runbook.md`
- 若变更已经影响系统边界、模块职责或关键数据流，再额外检查
  `docs/architecture.md`、`docs/go-structure.md`

### 文档同步历史兼容

- 历史任务不因本规范生效而追溯补齐所有说明文档。
- 本规范只约束新增改动和被重新触达的文档命中面。

## 依赖与脚本引入规范

### 依赖引入

- 新增 Go 依赖时，必须同步更新 `go.mod` 与 `go.sum`；优先使用标准库或
  当前仓库已存在依赖，避免为单一小需求引入重量级库。
- 新增前端依赖时，必须同步更新 `frontend/package.json` 与
  `frontend/package-lock.json`。
- 引入非显而易见的新依赖时，应在 `spec` / `plan`、提交说明或验收说明中
  交代引入原因、替代方案为何未采用，以及对构建 / 运行面的影响。
- 触达相关模块时，应顺手清理已确认无用的依赖或废弃脚本入口；不要把死依
  赖长期留在仓库里。

### 脚本引入

- 前端开发、验证或专项辅助脚本优先放在 `frontend/scripts/`，并通过
  `frontend/package.json` 暴露稳定入口。
- 运维、部署或仓库级辅助脚本优先放在 `scripts/`；若脚本承担正式部署或
  日常运维职责，必须同步补 `scripts/tests/` 下的 smoke 测试。
- 新脚本名称要直接表达行为，避免出现多个名称不同但语义重叠的入口。
- 涉及跨平台正式支持的部署 / 运维脚本，必须明确 Bash / PowerShell 的边
  界与是否需要同语义支持，不能默认只改一侧。

### 依赖脚本校验

- 新增或升级 Go 依赖，至少命中“分级验证与验收”中的后端验证。
- 新增或升级前端依赖，至少执行：
  1. `cd frontend && npm run test:vite-config`
  2. `cd frontend && npm run build`
  3. 按影响面补 `cd frontend && npm run test:state`
- 新增或修改 `scripts/deploy.*`、`scripts/tests/*` 或用户会直接执行的仓
  库级脚本时，必须同步更新命中的 README / 部署文档与脚本 smoke 测试。
- 不得引入新的强依赖运行工具却不说明安装前提、调用方式和验证入口。

### 依赖脚本历史兼容

- 历史依赖和旧脚本入口不因本规范生效而追溯重命名。
- 本规范只约束新增依赖、被重新触达的依赖清单和新增脚本入口。

## 文档头字段维护

- `最后更新`：只要 `AGENTS.md` 的语义内容发生变更，就必须同步更新日期。
- 当前处于 `0.x` 阶段；版本号只表示规范语义迭代次数，不表示成熟度等级
  或完成度评分。
- `版本`：仅在规则、优先级、入口范围或执行要求发生语义变化时更新；纯排版、错字、措辞润色或失效链接修正，只更新日期即可。
- 仅修正引用、补充解释或做小范围收口时，优先递增补丁版本；新增一项独立
  规则或明显扩展执行要求时，再递增次版本。

## 核心入口

- `AGENTS.md`：仓库级开发规范总入口。
- `docs/requirements.md` + `docs/architecture.md`：定义需求边界与系统级约束。
- `docs/dev-standards.md`：当前已沉淀的补充细则；测试与覆盖率要求以第 2 节为入口。
- `docs/superpowers/specs/`、`docs/superpowers/plans/`：任务级设计与执行记录，不替代仓库级规范。
