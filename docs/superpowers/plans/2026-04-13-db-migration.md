# 数据库迁移自动化实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 将当前依赖 `docs/mysql-schema.md` 手工执行的建表流程，升级为服务启动即可自动执行的数据库迁移机制，支持容器首次启动自举。

**架构：** 采用 `Goose + embed SQL`。新增 `migrations/` 作为 schema 唯一来源，在启动阶段于 MySQL 连接成功后执行迁移；通过 `mysql.auto_migrate` 控制是否自动迁移，默认开启。README、运行文档、容器文档统一改为“启动即迁移”，`docs/mysql-schema.md` 保留为结构说明，不再作为执行入口。

**技术栈：** Go、MySQL、Goose、embed、sqlmock、单元测试

---

### 任务 1：补齐迁移配置与失败测试

**文件：**
- 修改：`internal/config/config.go`
- 修改：`internal/config/config_test.go`
- 修改：`internal/app/app_test.go`
- 创建：`internal/db/migrate_test.go`

- [x] **步骤 1：先写失败测试，定义 `mysql.auto_migrate` 默认值与加载行为**
- [x] **步骤 2：先写失败测试，定义启动阶段调用迁移器与关闭开关的行为**

### 任务 2：实现迁移执行链路

**文件：**
- 创建：`internal/db/migrate.go`
- 创建：`migrations/00001_init.sql`
- 修改：`internal/app/app.go`
- 修改：`go.mod`
- 修改：`go.sum`

- [x] **步骤 1：实现基于 embed 的 Goose 迁移执行器**
- [x] **步骤 2：把现有初始化 SQL 收敛到首个 migration 文件**
- [x] **步骤 3：在应用启动阶段接入自动迁移，并保留关闭开关**

### 任务 3：文档与动态 TODO 同步

**文件：**
- 修改：`README.md`
- 修改：`docs/runbook.md`
- 修改：`docs/container-deploy.md`
- 修改：`docs/mysql-schema.md`
- 修改：`docs/todo.md`
- 修改：`docs/dev-standards.md`
- 修改：`docs/superpowers/plans/2026-04-13-db-migration.md`

- [x] **步骤 1：改写初始化说明，移除手工执行 SQL 作为主路径**
- [x] **步骤 2：同步动态 TODO 与计划状态**
- [x] **步骤 3：运行受影响测试与全量测试，记录验证结果**

## 当前结果

- 本轮已完成“数据库迁移自动化”。
- 当前数据库 schema 已收敛到 `migrations/00001_init.sql`，服务启动默认自动迁移。
- 验证结果：
  - `go test ./internal/config ./internal/app ./internal/db -count=1` 通过
  - `go test ./... -count=1` 通过
  - `go test ./internal/platform/bilibili -coverprofile=/tmp/bili-cover.out -count=1` 通过
  - `go test -p 1 ./... -coverprofile=/tmp/fetch-bilibili-cover.out` 在非沙箱环境通过，总覆盖率 `82.7%`
- 下一轮切换到“风控与 Cookie 观测增强”。
