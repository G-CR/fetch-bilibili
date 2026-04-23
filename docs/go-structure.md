# Go 目录结构与模块边界（当前实现）

本文档只描述当前仓库已经存在的 Go 目录、模块职责和依赖方向，不再沿用早期
“建议结构”或未落地的抽象设计。

## 1. 当前目录落点

后端相关目录当前以如下结构为主：

```text
cmd/
  server/
    main.go

configs/
  config.example.yaml
  creators.example.yaml

internal/
  api/http/
  app/
  config/
  creator/
  dashboard/
  db/
  discovery/
  jobs/
  library/
  live/
  platform/bilibili/
  repo/
  repo/mysql/
  scheduler/
  worker/

migrations/
  00001_init.sql
  00002_creator_removed_status.sql
  00003_candidate_discovery.sql
```

当前仓库不存在以下早期文档里提过的目录：

- `internal/domain`
- `internal/storage`
- `internal/observability`

因此，阅读或扩展当前实现时，不应再假设这些中间层已经存在。

## 2. 启动装配链路

当前后端启动链路是：

1. `cmd/server/main.go`
2. `internal/config.Load`
3. `internal/app.New`
4. `internal/db.NewMySQL`
5. `internal/db.RunMySQLMigrations`（当 `mysql.auto_migrate=true`）
6. `internal/repo/mysql.New`
7. `internal/live.NewBroker`
8. `internal/jobs.NewService`
9. `internal/scheduler.New`
10. `internal/platform/bilibili.New`
11. `internal/creator.NewService`
12. `internal/discovery.NewService`
13. `internal/worker.NewDefaultHandler`
14. `internal/worker.New`
15. `internal/config.NewEditor`
16. `internal/api/http.NewRouter`

`internal/app.Run` 启动后，会依次拉起：

- 启动恢复逻辑
- `library` 全量重建
- 调度器
- worker pool
- 认证 watcher
- 博主文件同步器
- HTTP 服务

## 3. 模块职责

### 3.1 `cmd/server`

- 负责读取配置路径、装配 `context`、处理重启循环。
- 不承载业务逻辑、SQL 或 HTTP 细节。

### 3.2 `internal/app`

- 负责整个应用的装配与生命周期管理。
- 统一创建 DB、Repo、Service、Scheduler、Worker、Broker、Router。
- 管理启动恢复、优雅退出和“配置写回后重启”。

### 3.3 `internal/api/http`

- 负责 HTTP / SSE 协议层。
- 包含：
  - 路由注册
  - 参数解析
  - JSON 编码
  - SSE 输出
  - CORS 处理
- 当前通过接口依赖上层服务，不直接 import `internal/repo/mysql`。

### 3.4 `internal/config`

- 负责配置解析、默认值回填、业务校验。
- 提供运行配置写回能力：
  - 读取当前配置全文
  - 保存整份配置文本
  - 触发异步重启请求

### 3.5 `internal/db`

- 负责 MySQL 初始化与迁移执行。
- 只处理数据库连接和 goose 迁移入口，不承载业务规则。

### 3.6 `internal/repo`

- 定义仓储接口、共享模型和过滤器。
- 这里的结构体是持久化与服务层共用的数据形状，不是单独的 `domain` 层。

### 3.7 `internal/repo/mysql`

- 负责 MySQL 持久化实现。
- 当前实现覆盖：
  - `creators`
  - `videos`
  - `video_files`
  - `jobs`
  - `candidate_creators`
- 这里负责：
  - SQL 语句
  - 行扫描
  - 查询过滤
  - 去重与状态更新
- 不负责 HTTP、调度、下载或事件分发。

### 3.8 `internal/jobs`

- 负责任务入队服务。
- 当前承认的任务类型定义在 `internal/jobs/types.go`：
  - `fetch`
  - `download`
  - `check`
  - `cleanup`
  - `discover`
- 入队成功后会发布 `job.changed` 事件。
- 去重错误会被收敛为幂等成功，不直接上抛给接口层。

### 3.9 `internal/scheduler`

- 负责周期性入队。
- 当前使用 `time.Ticker`，不是 cron 表达式。
- 当前调度项：
  - `fetch`
  - `check`
  - `cleanup`
  - `discover`（仅当 `discovery.enabled=true` 且 `interval>0`）

### 3.10 `internal/worker`

- 负责消费任务并执行具体业务。
- 核心由两部分组成：
  - `WorkerPool`：拉任务、更新状态、发布 `job.changed`
  - `DefaultHandler`：按任务类型执行 `fetch`、`download`、`check`、
    `cleanup`、`discover`
- 这里还承载：
  - 全局限速
  - 按博主限速
  - cleanup 执行
  - 下载与检查状态写回

### 3.11 `internal/platform/bilibili`

- 负责 B 站平台适配。
- 当前包含：
  - 鉴权与 Cookie 观察
  - WBI 相关处理
  - 博主与视频拉取
  - 下载
  - 可用性检查
  - discovery 搜索能力
- 当前仓库没有单独抽象出通用 `Platform` 接口层。

### 3.12 `internal/creator`

- 负责正式追踪博主的业务逻辑。
- 包含：
  - 新增 / 更新博主
  - patch / remove
  - 名称解析
  - 文件同步
  - `creator.changed` 事件发布

### 3.13 `internal/discovery`

- 负责候选池发现、评分、审核流转。
- 当前包含：
  - `KeywordDiscoverer`
  - `RelatedDiscoverer`
  - `Scorer`
  - 候选审核服务

### 3.14 `internal/dashboard`

- 负责驾驶舱查询聚合。
- 当前提供：
  - 任务列表
  - 视频列表 / 单视频
  - 系统状态
  - 存储统计

### 3.15 `internal/library`

- 负责 `library/` 浏览投影。
- 当前包含：
  - 路径规则
  - 快照导出
  - 单博主投影重建
  - 基于事件的增量同步
  - 定时全量对账

### 3.16 `internal/live`

- 负责进程内事件总线。
- 当前事件会被：
  - HTTP SSE 消费
  - `library` 投影同步器消费
- 这里不是业务真源，只是广播通道。

## 4. 当前依赖方向

当前依赖方向大致如下：

- `cmd/server` -> `internal/config`、`internal/app`
- `internal/app` -> 具体实现模块
- `internal/api/http` -> service interface
- `internal/jobs` / `internal/creator` / `internal/discovery` /
  `internal/dashboard` -> `internal/repo`
- `internal/repo/mysql` -> `internal/repo`、`internal/jobs`
- `internal/worker` -> `internal/repo`、`internal/jobs`、
  `internal/platform/bilibili`、`internal/library`

当前需要特别注意的边界：

- 只有 `internal/app` 等装配层直接依赖 `internal/repo/mysql`。
- `internal/api/http` 当前不直接拼 SQL，也不直接依赖 MySQL 实现。
- `internal/library` 只维护浏览投影，不回写业务主状态。
- `internal/live` 只负责广播，不做持久化。

## 5. 当前并发模型

### 5.1 Worker Pool

- 当前是共享 worker pool，不是按任务类型拆成多套独立 pool。
- worker 数量来自 `limits.download_concurrency`。
- 每个 worker 按固定轮询周期拉任务，默认 `2s` 一次。
- 任务源是真实的 MySQL `jobs` 表，不存在“内存队列作为主队列”的实现。

### 5.2 取任务方式

- `internal/repo/mysql/job_repo.go` 当前通过事务取任务。
- 取数 SQL 使用：
  - `status = queued`
  - `not_before <= NOW()`
  - `FOR UPDATE SKIP LOCKED`
- 取到任务后，在同一事务里把状态改成 `running`。

### 5.3 限速

- 全局限速由 `internal/worker/limiter.go` 提供简单等待器。
- 按博主限速由 `DefaultHandler` 内部维护每个博主的 limiter。
- 当前 `limits.check_concurrency` 仍未真正接入 worker 并发控制。

### 5.4 调度与旁路 goroutine

除了 worker pool 外，当前还会并发运行：

- scheduler goroutine
- auth watcher goroutine
- creator file syncer goroutine
- library syncer goroutine
- HTTP 服务 goroutine

## 6. 当前没有落地的旧设想

以下内容曾出现在早期结构文档中，但当前仓库没有按这些方式实现：

- 独立的 `internal/domain` 领域层
- 独立的 `internal/storage` 清理策略包
- 独立的 `internal/observability` 日志 / 指标封装层
- 通用平台抽象接口及多平台实现
- “内存队列 + MySQL 持久化”双队列模型
- 在 worker 主循环里统一做自动重试退避编排

其中要特别说明：

- `jobs.not_before` 字段和相关读取逻辑已经存在。
- 但当前主流程还没有形成完整的自动重试 / 退避闭环。

## 7. 维护建议

若后续真实目录或模块职责发生变化，至少同步更新：

- `docs/architecture.md`
- `docs/go-structure.md`
- 触达运行链路时，再同步 `docs/runbook.md`
