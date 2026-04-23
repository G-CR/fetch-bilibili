# 开发细则

本文件是仓库级 `AGENTS.md` 的补充细则，只记录当前仓库已经确认的开发约束。
若与 `AGENTS.md` 冲突，以 `AGENTS.md` 为准。

## 1. 适用边界

- 本文件不替代 `docs/requirements.md`、`docs/architecture.md` 或任务级
  `spec` / `plan`。
- 涉及提交、分支、文档优先级、未提交改动处理、分级验证与验收时，以
  `AGENTS.md` 为入口。
- 本文件只补充更贴近当前代码库的测试、契约、依赖和同步细则。

## 2. 测试与覆盖率

- 覆盖率基线：`go test ./... -cover` 总体不低于 85%。
- 若本地沙箱限制导致测试无法监听端口或写默认 Go cache，必须明确记录阻塞
  原因；可使用 `GOCACHE=/tmp/fetch_bilibili_gocache` 这类临时缓存路径复验。
- 新增或修改核心业务逻辑时，必须补最贴近行为边界的测试：
  - HTTP / SSE：`internal/api/http`
  - 配置解析与写回：`internal/config`
  - 任务、worker、恢复：`internal/jobs`、`internal/worker`、`internal/app`
  - MySQL 查询与迁移：`internal/repo/mysql`、`internal/db`
  - B 站适配：`internal/platform/bilibili`
  - 前端状态与交互：`frontend/scripts/*` 与 Playwright E2E
- 允许单个包低于 85%，但最终交付说明必须解释原因，并确保总体覆盖率不低于
  基线。

## 3. 前端验证入口

- 当前前端没有独立 `npm run lint` 脚本，不能把 lint 写成已存在校验入口。
- 命中前端代码、脚本或构建配置时，当前最小验证入口为：
  - `cd frontend && npm run test:state`
  - `cd frontend && npm run test:vite-config`
  - `cd frontend && npm run test:smoke`
  - `cd frontend && npm run build`
- 涉及浏览器端真实流程、配置保存、SSE 重连、候选池操作或端到端联动时，再补：
  - `cd frontend && npm run test:e2e`
- `test:e2e` 默认使用 mock API 与前端 dev server；真实联调需要显式设置
  `E2E_MODE=live`、`E2E_BASE_URL` 与 `E2E_API_BASE`。

## 4. API、SSE 与状态契约

- 后端对外 JSON 字段统一使用 `snake_case`。
- 前端内部状态可使用 `camelCase`，但转换与兼容逻辑应集中在
  `frontend/src/lib/state.js`。
- HTTP 与 SSE 的时间字段都使用 RFC 3339 字符串，但空值表现不完全一致：
  - HTTP 快照中带 `omitempty` 的字段，零值时会被省略。
  - 未加 `omitempty` 的 HTTP 字段会返回空字符串。
  - 当前由 `map` 手工构造的 SSE payload，空时间字段通常保留为空字符串。
- `creator.status`、`candidate.status`、`job.status`、`video.state` 与
  `/system/status` 相关字段属于对外契约；新增、删除或重命名必须同步改后
  端、前端映射、`docs/api.md` 与测试。
- `video.changed` 当前存在 `state="FAILED"` 的运行时例外；不要把这个例外误
  写成 `GET /videos` 公开状态基线已扩展。
- 触达 `/events/stream` 事件类型或 payload 时，必须同时核对快照接口和前端
  `applyLiveEvent` 逻辑。

## 5. 错误、日志与敏感信息

- 日志统一使用中文，记录最小必要上下文；优先带 `id`、`uid`、任务类型、状
  态、页码和接口动作。
- 禁止在日志、HTTP 响应、SSE 事件、示例配置或任务文档中写入原始
  `cookie`、`SESSDATA`、数据库密码、完整 DSN、Token 或完整上游响应体。
- 多数业务错误路径返回 `{"error":"..."}`，但当前部分 `405` 和 SSE 非 JSON
  错误路径会直接写状态码，可能没有 JSON body。
- 触达新增错误路径时，优先返回稳定、可操作的中文摘要；如果保留
  `err.Error()`，必须先确认不会带出凭证、完整 DSN、完整上游响应体或本机敏
  感路径。

## 6. 数据库与迁移

- 数据库 schema 变更必须通过 `migrations/*.sql` 落地。
- `migrations/*.sql` 是执行来源；`docs/mysql-schema.md` 是结构说明，不是执
  行入口。
- 新迁移文件保持当前序号风格：`000NN_<slug>.sql`。
- 每个迁移必须包含 `-- +goose Up` 与 `-- +goose Down`。
- 触达迁移、自动迁移开关或 schema 字段语义时，至少同步：
  - `docs/mysql-schema.md`
  - 命中的 repo / service / API 文档
  - 相关测试

## 7. 作业、worker 与恢复

- 任务类型和状态统一以 `internal/jobs/types.go` 为准。
- 当前任务类型：`fetch`、`download`、`check`、`cleanup`、`discover`。
- 当前任务状态：`queued`、`running`、`success`、`failed`。
- 活动任务去重只把 `queued` 与 `running` 视为活动任务。
- 命中 `jobs.ErrJobAlreadyActive` 时，上层应按“无需重复创建”处理。
- 启动恢复当前只修复：
  - `running` 任务重新入队为 `queued`
  - 无活动下载支撑的 `DOWNLOADING` 视频修正为 `NEW` 或 `DOWNLOADED`
  - 缺失有效文件的 `DOWNLOADED` 视频回退为 `NEW`

## 8. 配置写回与部署语义

- `PUT /system/config` 是整份配置文档写回。
- 写回前必须先比对内容并完成解析校验；未变化时返回 `changed=false`，不得
  触发重启。
- `restart_scheduled=true` 只表示写回成功且重启请求已发出，不表示服务已恢
  复，也不等价于重新部署最新代码。
- Docker Compose 下，前端设置页保存配置后的重启当前主要依赖容器内后端进程
  自重启，不依赖 Docker 重拉容器。
- 触达配置写回、重启调度或前端重启提示时，至少同步：
  - `docs/api.md`
  - `docs/config.md`
  - `docs/runbook.md`
  - `README.md`
  - 若容器部署语义受影响，再补 `docs/container-deploy.md`

## 9. 依赖与脚本

- 新增 Go 依赖时，同步更新 `go.mod` 与 `go.sum`。
- 新增前端依赖时，同步更新 `frontend/package.json` 与
  `frontend/package-lock.json`。
- 前端辅助脚本优先放 `frontend/scripts/` 并通过 `package.json` 暴露入口。
- 运维 / 部署脚本优先放 `scripts/`，正式入口必须配套 `scripts/tests/`
  smoke 测试。
- Shell 与 PowerShell 部署脚本的命令语义应保持一致；若当前环境无法执行
  `pwsh`，必须在验收说明中标记为未执行，而不是写成通过。

## 10. 文档同步矩阵

- 触达 API / SSE：至少同步 `docs/api.md`。
- 触达配置、默认值、保存与重启：至少同步 `docs/config.md`、
  `docs/runbook.md`、`README.md`。
- 触达容器、镜像、部署脚本或挂载路径：至少同步 `docs/container-deploy.md`
  与 `README.md`。
- 触达 worker、scheduler、恢复或任务状态：至少同步 `docs/worker.md`、
  `docs/job-scheduler.md`、`docs/runbook.md`。
- 触达存储、cleanup 或浏览投影：至少同步 `docs/storage-policy.md`、
  `docs/data-model.md`、`docs/runbook.md`。
