# 开发细则

本文件是仓库级 [AGENTS.md](../AGENTS.md) 的补充细则，不再承担规范总入口角色。

优先级约束：

- 若与 `AGENTS.md` 冲突，以 `AGENTS.md` 为准。
- 若与 `docs/requirements.md`、`docs/architecture.md` 冲突，以这两份文档为准。
- 本文件当前只沉淀已确认的细则项，不替代任务级设计文档与执行计划。

## 1. 代码与结构

- 目录结构遵循 `docs/go-structure.md` 的模块划分。
- 业务逻辑与基础设施分层清晰，避免跨层耦合。
- 关键逻辑必须有单元测试覆盖。

## 2. 测试与覆盖率

- 覆盖率基线：`go test ./... -cover` 总体不低于 **85%**。
- 新增/修改的核心逻辑必须补充对应单测。
- 允许个别包覆盖率低于阈值，但需说明原因。

## 3. 日志、错误与敏感信息处理

- 日志统一使用中文，记录最小必要上下文；优先带 `id`、`uid`、任务类型、
  页码、状态、接口动作。
- 禁止在日志、HTTP 响应、SSE 事件、示例配置或任务文档中写入原始
  `cookie`、`SESSDATA`、数据库密码、完整 DSN、Token 或完整上游响应体。
- `4xx` 返回稳定、可操作的中文错误；`5xx`、网络、数据库、上游平台错
  误默认返回摘要，不直接透传 `err.Error()`。
- `/system/status` 与 `system.changed` 只暴露驾驶舱所需的摘要运行态；
  兼容字段若可能带出原始上游错误或本地环境细节，后续触达时应先脱敏再保
  留。
- 触达 `internal/api/http`、`internal/dashboard`、
  `internal/platform/bilibili` 的错误返回或状态字段时，同步更新命中的
  `docs/api.md`、`docs/config.md`、`docs/bilibili-adapter.md`，并补相
  应测试。

## 4. 对外接口、状态枚举与前后端契约

- 后端对外 JSON 字段统一使用 `snake_case`；前端内部状态可用
  `camelCase`，但转换与兼容逻辑应集中收敛在
  `frontend/src/lib/state.js`。
- HTTP 与 SSE 中的时间字段统一使用 RFC 3339 字符串；空时间保持空字符
  串，不混用时间戳或多种格式。
- `creator.status`、`candidate.status`、`job.status`、`video.state`
  与 `/system/status` 相关状态字段属于稳定契约。新增、删除、重命名状态
  值时，必须同步修改后端返回、前端映射、`docs/api.md` 与测试。
- 查询参数名、默认值、分页规则、过滤语义，以及 `/events/stream` 的事件
  类型和 payload，都按对外契约处理，禁止只改一侧。
- 触达 `internal/api/http`、`frontend/src/lib/api.js`、
  `frontend/src/lib/state.js` 或 SSE payload 时，至少执行后端验证；若前
  端消费链路也受影响，再补 `cd frontend && npm run test:state`，并按
  影响面追加 `test:smoke` / `test:e2e`。
- 只改实现不更新 `docs/api.md`，或只改 `docs/api.md` 不核对实现，都不
  算完成。

## 5. 数据库变更

- 数据库 schema 变更必须通过 `migrations/` 中的迁移文件提交，不允许只改文档或手工 SQL。
- `docs/mysql-schema.md` 是结构说明，`migrations/*.sql` 才是执行来源。
- 迁移文件需要配套单元测试或启动链路测试，至少覆盖自动迁移开关与失败分支。

## 6. 安全与合规

- 遵守平台服务条款，不进行超频请求。
- 必要时添加限速、重试与风控处理逻辑。

## 7. 测试分层与补测规则

- 测试层与变更层必须对应：HTTP / SSE 改动补 `internal/api/http` 测试，
  repo/mysql 改动补仓储测试，jobs / worker / recovery 改动补对应包测
  试，配置解析 / 写回改动补 `internal/config` 测试。
- 触达 `frontend/src/lib/api.js`、`frontend/src/lib/state.js`、
  `frontend/src/App.jsx` 的契约映射、SSE 应用或配置保存交互时，至少补
  `cd frontend && npm run test:state`，再按影响面补 `test:smoke` /
  `test:e2e`。
- 不能只以一次笼统的 `go test ./...` 通过代替命中层级的补测。

## 8. 作业状态流转、幂等与恢复

- 任务类型与状态统一以 `internal/jobs/types.go` 为准；当前只承认
  `fetch`、`download`、`check`、`cleanup`、`discover` 与
  `queued`、`running`、`success`、`failed`。
- 新任务统一从 `queued` 入队，worker 执行后只能落到 `success` /
  `failed`；去重判断统一收敛在仓储层，命中 `jobs.ErrJobAlreadyActive`
  时按“无需重复创建”处理。
- 启动恢复只允许修复当前已定义的残留态：`running` 任务重新入队，
  `DOWNLOADING` / `DOWNLOADED` 视频按现有恢复逻辑修正；新增恢复规则前
  必须先有 `spec` / `plan`。

## 9. 仓储、SQL 与事务边界

- `internal/repo/mysql` 只负责持久化、查询拼装、行映射和数据库级错误处
  理，不承载 HTTP、事件发布、外部调用或文件系统副作用。
- SQL 的排序、分页、过滤和状态条件必须显式表达，不能依赖 MySQL 隐式顺
  序；扫描 `NULL`、JSON、时间字段的细节应留在仓储层。
- 只有在同一仓储操作需要保证多条 SQL 原子性时，才允许在仓储层开事务；
  事务内不做网络、事件或磁盘 I/O。

## 10. 配置写回与重启语义

- `PUT /system/config` 当前是整份配置文档写回；写回前必须先比对内容并通
  过解析校验，未变化时返回 `changed=false` 且不得重启。
- `restart_scheduled=true` 只表示写回成功且重启请求已发出，不表示服务已
  完成重启，更不等价于重新部署最新代码。
- 触达配置写回、重启调度、前端重启提示或 `/system/config` 返回语义时，
  同步更新 `docs/api.md`、`docs/config.md`、`docs/runbook.md`、
  `README.md`，必要时再补 `docs/container-deploy.md`。

## 11. 文档同步矩阵

- 触达 API / SSE：至少同步 `docs/api.md`。
- 触达配置、默认值、保存与重启：至少同步 `docs/config.md`、
  `docs/runbook.md`、`README.md`；若部署方式受影响，再补
  `docs/container-deploy.md`。
- 触达 worker、scheduler、恢复、任务状态：至少同步 `docs/worker.md`、
  `docs/job-scheduler.md`、`docs/runbook.md`。
- 触达存储、清理、浏览目录：至少同步 `docs/storage-policy.md`、
  `docs/data-model.md`、`docs/runbook.md`。

## 12. 依赖与脚本引入规范

- 新增 Go 依赖时，同步更新 `go.mod` 与 `go.sum`；新增前端依赖时，同步更
  新 `frontend/package.json` 与 `frontend/package-lock.json`。
- 前端辅助脚本优先放 `frontend/scripts/` 并通过 `package.json` 暴露入
  口；运维 / 部署脚本优先放 `scripts/`，正式入口要配套 `scripts/tests/`
  smoke 测试。
- 新依赖或新脚本不能只落文件不补验证与文档；至少说明引入原因、运行前
  提与命中的验证入口。
