# 实时驾驶舱 SSE 实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 为当前驾驶舱补齐 SSE 实时更新能力，让任务、视频、博主、系统状态和存储统计在不手动刷新的情况下自动变化，同时保留现有快照接口作为初始化与纠偏兜底。

**架构：** 后端新增进程内事件总线 `internal/live` 和只读 SSE 接口 `/events/stream`，在任务、视频、博主、Cookie/风控、存储关键状态变化时发布离散事件；前端在首屏快照基础上建立 `EventSource` 长连接消费增量事件，并用低频快照对账修正漂移。风控倒计时由前端根据 `backoff_until` 本地派生，不做服务端高频广播。

**技术栈：** Go、net/http、MySQL、React、Vite、EventSource、单元测试、E2E

---

## 文件边界

### 后端实时基础设施

- 创建：`internal/live/broker.go`
  - 进程内广播事件总线，负责 `Publish` / `Subscribe`
- 创建：`internal/live/broker_test.go`
  - 覆盖订阅、广播、慢消费者断开、上下文退出
- 创建：`internal/api/http/events.go`
  - SSE handler，负责 `hello`、`heartbeat`、事件写出、连接关闭
- 修改：`internal/api/http/router.go`
  - 注册 `/events/stream`
- 修改：`internal/api/http/router_test.go`
  - 覆盖 SSE 路由与基础响应

### 事件发布接入

- 修改：`internal/jobs/service.go`
  - 入队成功后发布 `job.changed`
- 修改：`internal/jobs/service_test.go`
  - 覆盖入队事件发布
- 修改：`internal/worker/worker.go`
  - 任务进入 `running/success/failed` 时发布 `job.changed`
- 修改：`internal/worker/worker_test.go`
  - 覆盖任务状态事件
- 修改：`internal/worker/handler.go`
  - 发布 `video.changed`、`storage.changed`
- 修改：`internal/worker/handler_test.go`
  - 覆盖下载、清理、状态变化事件
- 修改：`internal/creator/service.go`
  - 发布 `creator.changed`
- 修改：`internal/creator/service_test.go`
  - 覆盖新增、暂停、停止追踪、名称补齐事件
- 修改：`internal/platform/bilibili/auth.go`
  - 发布 `system.changed`
- 修改：`internal/platform/bilibili/client.go`
  - 必要时暴露构造 `system.changed` 所需运行态快照
- 修改：`internal/app/app.go`
  - 组装 broker 并注入 router / service / worker / auth watcher
- 修改：`internal/app/app_test.go`
  - 覆盖 broker 注入和装配链路

### 前端实时接入

- 修改：`frontend/src/lib/api.js`
  - 新增 SSE 客户端封装与快照对账触发器
- 修改：`frontend/src/lib/state.js`
  - 新增 `applyLiveEvent`、连接状态、倒计时派生支持
- 修改：`frontend/src/App.jsx`
  - 首屏快照 + EventSource 连接 + 连接状态 UI + 低频对账
- 修改：`frontend/src/styles.css`
  - 连接状态、实时刷新高亮、断线提示样式
- 修改：`frontend/scripts/test-dashboard-state.mjs`
  - 覆盖状态 reducer 的实时事件适配
- 修改：`frontend/scripts/smoke-render.mjs`
  - 覆盖实时状态 UI 基本渲染
- 修改：`frontend/e2e/dashboard.spec.js`
  - 覆盖任务自动刷新、断线重连
- 修改：`frontend/scripts/e2e/mock-api.mjs`
  - 提供可控 SSE mock 流

### 文档

- 修改：`README.md`
  - 增加“实时模式”说明
- 修改：`docs/api.md`
  - 增加 `/events/stream` 协议说明
- 修改：`docs/todo.md`
  - 更新实时驾驶舱事项状态

---

### 任务 1：实现进程内事件总线与 SSE HTTP 边界

**文件：**
- 创建：`internal/live/broker.go`
- 创建：`internal/live/broker_test.go`
- 创建：`internal/api/http/events.go`
- 修改：`internal/api/http/router.go`
- 修改：`internal/api/http/router_test.go`

- [ ] **步骤 1：先写 broker 失败测试**

覆盖用例：
- 一个事件能被多个订阅者同时收到
- 订阅者 `ctx.Done()` 后自动移除
- 慢消费者缓冲写满时不会阻塞全局发布

运行：`go test ./internal/live -run 'TestBroker' -count=1`
预期：FAIL，提示缺少 broker 实现

- [ ] **步骤 2：实现最小 broker**

实现边界：
- 公开 `Event` 结构体：

```go
type Event struct {
	ID      string
	Type    string
	At      time.Time
	Payload any
}
```

- 公开 `Broker`：

```go
type Broker struct {
	// 维护订阅者集合，Publish 时广播
}
```

- `Subscribe(ctx, buffer)` 返回只读通道
- 慢消费者写满时直接关闭并移除该订阅者

- [ ] **步骤 3：运行 broker 测试确认通过**

运行：`go test ./internal/live -count=1`
预期：PASS

- [ ] **步骤 4：先写 SSE handler 失败测试**

覆盖用例：
- `/events/stream` 返回 `text/event-stream`
- 建连后能收到 `hello`
- broker 发布事件后响应体出现 `event:` 与 `data:`

运行：`go test ./internal/api/http -run 'TestEventsStream' -count=1`
预期：FAIL，提示缺少路由或 handler

- [ ] **步骤 5：实现 SSE handler 与路由注册**

实现要求：
- 新增 `/events/stream`
- 响应头包含：
  - `Content-Type: text/event-stream`
  - `Cache-Control: no-cache`
  - `Connection: keep-alive`
- 建连立即发送 `hello`
- 每 `15s` 发送 `heartbeat`
- 订阅 broker，按 `event: <type>\ndata: <json>\n\n` 写出

- [ ] **步骤 6：运行 HTTP 层测试确认通过**

运行：`go test ./internal/api/http -run 'TestEventsStream|TestRouter' -count=1`
预期：PASS

- [ ] **步骤 7：Commit**

```bash
git add internal/live/broker.go internal/live/broker_test.go internal/api/http/events.go internal/api/http/router.go internal/api/http/router_test.go
git commit -m "feat(实时驾驶舱): 增加 SSE 基础设施"
```

### 任务 2：接入任务生命周期事件

**文件：**
- 修改：`internal/jobs/service.go`
- 修改：`internal/jobs/service_test.go`
- 修改：`internal/worker/worker.go`
- 修改：`internal/worker/worker_test.go`
- 修改：`internal/app/app.go`
- 修改：`internal/app/app_test.go`

- [ ] **步骤 1：先写 jobs/service 失败测试**

覆盖用例：
- `EnqueueFetch` / `EnqueueCheck` / `EnqueueCleanup` / `EnqueueDownload` / `EnqueueCheckVideo` 成功后发布 `job.changed`
- 命中 `ErrJobAlreadyActive` 时不重复发布

运行：`go test ./internal/jobs -run 'TestEnqueue.*PublishesEvent' -count=1`
预期：FAIL，提示缺少事件发布

- [ ] **步骤 2：扩展 jobs.Service 依赖并实现入队事件**

实现要求：
- `jobs.New(...)` 扩展为可注入 broker 或事件发布器
- 只在真实成功入队后发布 `job.changed`
- 事件负载至少包含：
  - `id`
  - `type`
  - `status`
  - `payload`
  - `updated_at`

- [ ] **步骤 3：先写 worker 失败测试**

覆盖用例：
- `FetchQueued` 产生 `running` 状态时发布 `job.changed`
- 成功完成时发布 `success`
- 失败时发布 `failed` 且带 `error_msg`

运行：`go test ./internal/worker -run 'TestWorker.*PublishesJobEvent' -count=1`
预期：FAIL，提示未发布事件

- [ ] **步骤 4：实现 worker 任务状态事件**

实现要求：
- 任务进入 `running` 的发布点优先放在 worker 侧拿到 job 后立即广播当前 job 快照
- 任务结束后依据结果广播 `success/failed`
- 避免在 repo 层引入广播逻辑，保持仓储纯数据访问

- [ ] **步骤 5：补 app 装配测试并完成 broker 注入**

运行：`go test ./internal/app -run 'TestNew.*Broker' -count=1`
预期：先 FAIL，再 PASS

- [ ] **步骤 6：运行相关测试确认通过**

运行：`go test ./internal/jobs ./internal/worker ./internal/app -count=1`
预期：PASS

- [ ] **步骤 7：Commit**

```bash
git add internal/jobs/service.go internal/jobs/service_test.go internal/worker/worker.go internal/worker/worker_test.go internal/app/app.go internal/app/app_test.go
git commit -m "feat(实时驾驶舱): 推送任务生命周期事件"
```

### 任务 3：接入视频、存储与博主实时事件

**文件：**
- 修改：`internal/worker/handler.go`
- 修改：`internal/worker/handler_test.go`
- 修改：`internal/creator/service.go`
- 修改：`internal/creator/service_test.go`

- [ ] **步骤 1：先写 worker/handler 失败测试**

覆盖用例：
- 下载成功后发布 `video.changed`
- 下载成功或 cleanup 成功后发布 `storage.changed`
- 视频标记绝版、稳定、删除时发布对应 `video.changed`

运行：`go test ./internal/worker -run 'TestHandle.*Publishes(Video|Storage)Event' -count=1`
预期：FAIL

- [ ] **步骤 2：实现视频与存储事件**

实现要求：
- 只在数据库状态变更真正完成后发事件
- `storage.changed` 事件负载至少包含：
  - `used_bytes`
  - `file_count`
  - `rare_videos`
  - `usage_percent`
- 第一版允许 `hottest_bucket` 继续通过低频快照校正，不强制实时精确

- [ ] **步骤 3：先写 creator/service 失败测试**

覆盖用例：
- 新增博主发布 `creator.changed`
- `Patch` 更新状态发布 `creator.changed`
- `Delete` 停止追踪发布 `creator.changed`
- 名称补齐后发布 `creator.changed`

运行：`go test ./internal/creator -run 'TestService.*PublishesCreatorEvent' -count=1`
预期：FAIL

- [ ] **步骤 4：实现博主事件**

实现要求：
- 保持 `creator.Service` 仍是唯一的业务入口
- 从该层统一发出 `creator.changed`
- 文件同步触发的博主回填或状态变化也复用同一发布函数

- [ ] **步骤 5：运行相关测试确认通过**

运行：`go test ./internal/worker ./internal/creator -count=1`
预期：PASS

- [ ] **步骤 6：Commit**

```bash
git add internal/worker/handler.go internal/worker/handler_test.go internal/creator/service.go internal/creator/service_test.go
git commit -m "feat(实时驾驶舱): 推送视频存储与博主事件"
```

### 任务 4：接入系统状态实时事件

**文件：**
- 修改：`internal/platform/bilibili/auth.go`
- 修改：`internal/platform/bilibili/client.go`
- 修改：`internal/platform/bilibili/client_test.go`
- 新建或修改：`internal/platform/bilibili/auth_test.go`

- [ ] **步骤 1：先写 auth watcher 失败测试**

覆盖用例：
- Cookie 刷新结果变化时发布 `system.changed`
- Cookie 校验成功、失效、报错时发布 `system.changed`
- 风控命中或恢复后发布 `system.changed`

运行：`go test ./internal/platform/bilibili -run 'TestAuthWatcher.*PublishesSystemEvent|TestClient.*Risk.*Event' -count=1`
预期：FAIL

- [ ] **步骤 2：实现系统状态事件**

实现要求：
- 事件负载与 `dashboard.SystemStatus` 中前端已消费的字段对齐：
  - `cookie`
  - `risk`
- 不要求每秒推送倒计时
- 风控相关只在状态切换点发布，前端使用 `backoff_until` 自行倒计时

- [ ] **步骤 3：运行 bilibili 相关测试确认通过**

运行：`go test ./internal/platform/bilibili -count=1`
预期：PASS

- [ ] **步骤 4：Commit**

```bash
git add internal/platform/bilibili/auth.go internal/platform/bilibili/client.go internal/platform/bilibili/client_test.go internal/platform/bilibili/auth_test.go
git commit -m "feat(实时驾驶舱): 推送系统状态事件"
```

### 任务 5：前端接入 SSE 与局部状态更新

**文件：**
- 修改：`frontend/src/lib/api.js`
- 修改：`frontend/src/lib/state.js`
- 修改：`frontend/src/App.jsx`
- 修改：`frontend/src/styles.css`
- 修改：`frontend/scripts/test-dashboard-state.mjs`
- 修改：`frontend/scripts/smoke-render.mjs`

- [ ] **步骤 1：先写前端状态失败测试**

覆盖用例：
- `job.changed` 更新任务列表与已选详情
- `video.changed` 更新视频状态
- `creator.changed` 更新博主列表
- `system.changed` 更新 Cookie/风控状态
- `storage.changed` 更新存储统计

运行：`cd frontend && node ./scripts/test-dashboard-state.mjs`
预期：FAIL，提示缺少实时事件 reducer

- [ ] **步骤 2：实现 SSE 客户端封装**

实现要求：
- 在 `frontend/src/lib/api.js` 新增：
  - `createDashboardEventStream(baseURL, handlers)`
- 支持：
  - 建连
  - 断线回调
  - 基础重连
  - `snapshot.required` 回调

- [ ] **步骤 3：实现 state reducer**

实现要求：
- 在 `frontend/src/lib/state.js` 新增 `applyLiveEvent(previous, event)`
- 新增连接状态字段：
  - `connecting`
  - `live`
  - `reconnecting`
  - `offline`

- [ ] **步骤 4：在 App.jsx 接入 EventSource**

实现要求：
- 首屏继续先跑 `loadDashboardSnapshot`
- 然后建立 SSE
- 收到事件后只做局部更新
- 断线后展示连接状态
- 重连成功后主动补一次快照

- [ ] **步骤 5：补连接状态和动态效果样式**

实现要求：
- 顶部显示实时连接状态
- 活跃任务更新时增加轻量高亮
- 风控退避剩余秒数本地倒计时

- [ ] **步骤 6：运行前端测试确认通过**

运行：

```bash
cd frontend
node ./scripts/test-dashboard-state.mjs
node ./scripts/smoke-render.mjs
npm run build
```

预期：全部 PASS

- [ ] **步骤 7：Commit**

```bash
git add frontend/src/lib/api.js frontend/src/lib/state.js frontend/src/App.jsx frontend/src/styles.css frontend/scripts/test-dashboard-state.mjs frontend/scripts/smoke-render.mjs
git commit -m "feat(前端): 接入实时驾驶舱 SSE 更新"
```

### 任务 6：补低频对账与 E2E 验证

**文件：**
- 修改：`frontend/src/App.jsx`
- 修改：`frontend/e2e/dashboard.spec.js`
- 修改：`frontend/scripts/e2e/mock-api.mjs`
- 修改：`README.md`
- 修改：`docs/api.md`
- 修改：`docs/todo.md`

- [ ] **步骤 1：先写 E2E 失败测试**

覆盖用例：
- 点击“立即拉取”后，任务状态自动从 `queued -> running -> success`
- SSE 断线后页面显示重连中
- 重连成功后自动恢复

运行：`cd frontend && npm run test:e2e`
预期：FAIL

- [ ] **步骤 2：实现低频快照对账**

实现要求：
- SSE 正常时仍保留低频对账：
  - `system/status`：30 秒
  - `storage/stats`：60 秒
  - 必要时整页快照：60 秒
- 页面隐藏时暂停本地高频派生，不暂停 SSE 连接

- [ ] **步骤 3：完善 mock SSE 流并让 E2E 通过**

运行：`cd frontend && npm run test:e2e`
预期：PASS

- [ ] **步骤 4：更新文档**

同步说明：
- README 增加“实时连接状态 / SSE”说明
- `docs/api.md` 增加 `/events/stream`
- `docs/todo.md` 更新该事项状态

- [ ] **步骤 5：全量验证**

运行：

```bash
go test ./... -count=1
cd frontend && npm run test:e2e && npm run build
```

预期：全部 PASS

- [ ] **步骤 6：Commit**

```bash
git add frontend/src/App.jsx frontend/e2e/dashboard.spec.js frontend/scripts/e2e/mock-api.mjs README.md docs/api.md docs/todo.md
git commit -m "feat(实时驾驶舱): 完成 SSE 联动与对账"
```

## 交付检查

- [ ] 任务、视频、博主、Cookie/风控状态在页面中自动更新
- [ ] 存储统计能在下载/清理后尽快更新，并由低频快照校正
- [ ] SSE 断线后页面展示明确状态，并能自动重连
- [ ] 首屏初始化、断线恢复、后端重启后恢复都能回到一致状态
- [ ] `go test ./... -count=1` 通过
- [ ] `cd frontend && npm run test:e2e && npm run build` 通过
