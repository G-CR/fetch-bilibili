# 风控与 Cookie 观测增强实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 增强 B 站接入链路的运行时观测能力，让驾驶舱能够明确展示 Cookie 是否已配置、来自哪里、最近一次检查/刷新结果，以及当前是否处于风控退避状态。

**架构：** 采用“运行时快照”方案，不新增数据库表。由 `internal/platform/bilibili` 在 `Client` / `AuthWatcher` 内维护线程安全的认证与风控状态快照，`internal/dashboard` 在 `GET /system/status` 中聚合输出，前端现有风控区块直接消费这些字段并展示。保留 `risk_level` 兼容字段，同时新增更细的 `risk`/`cookie` 子字段。

**技术栈：** Go、net/http、React、Vite、单元测试

---

### 任务 1：补齐后端失败测试

**文件：**
- 修改：`internal/platform/bilibili/client_test.go`
- 修改：`internal/platform/bilibili/auth_test.go`
- 修改：`internal/dashboard/service_test.go`
- 修改：`internal/api/http/router_test.go`

- [x] **步骤 1：先写失败测试，定义 Client/AuthWatcher 的运行时快照字段与更新时机**
- [x] **步骤 2：先写失败测试，定义 `/system/status` 返回的新 Cookie / 风控观测字段**

### 任务 2：实现后端运行时观测快照

**文件：**
- 修改：`internal/platform/bilibili/client.go`
- 修改：`internal/platform/bilibili/auth.go`
- 修改：`internal/dashboard/service.go`
- 修改：`internal/api/http/system.go`

- [x] **步骤 1：在 bilibili client 中维护 Cookie 来源、最近检查/刷新、最近错误、当前退避信息**
- [x] **步骤 2：让 AuthWatcher 把周期性刷新/检查结果回写到快照**
- [x] **步骤 3：扩展 dashboard system status 读模型与 HTTP 响应**

### 任务 3：接入前端风险区块

**文件：**
- 修改：`frontend/src/lib/state.js`
- 修改：`frontend/src/App.jsx`
- 修改：`frontend/scripts/test-dashboard-state.mjs`

- [x] **步骤 1：先写失败测试，定义前端状态层如何吸收新字段**
- [x] **步骤 2：在现有风控区块展示 Cookie 来源、最近检查/刷新、退避剩余与最近命中原因**
- [x] **步骤 3：保持 local 模式和旧字段兼容**

### 任务 4：文档与动态 TODO 同步

**文件：**
- 修改：`README.md`
- 修改：`docs/api.md`
- 修改：`docs/todo.md`
- 修改：`docs/superpowers/plans/2026-04-13-risk-cookie-observability.md`

- [x] **步骤 1：同步 API 文档与 README 里的系统状态说明**
- [x] **步骤 2：更新动态 TODO 的状态与下一优先级**
- [x] **步骤 3：运行 Go / 前端验证命令并记录结果**

## 当前结果

- 本轮已完成“风控与 Cookie 观测增强”。
- `GET /system/status` 已支持输出认证监控启用状态、Cookie 来源、最近检查/刷新结果、最近错误、风控退避剩余时间和最近命中原因。
- 前端现有“风控”区块已接入这些字段，无需额外耦合后端实现。
- 最新验证：
  - `go test ./... -count=1` 通过
  - `cd frontend && npm run test:vite-config && npm run test:state && npm run test:smoke && npm run build` 通过
  - `go test -p 1 ./... -coverprofile=...` 总覆盖率 `82.6%`
- 下一轮切换到“前端运维能力补全”。
