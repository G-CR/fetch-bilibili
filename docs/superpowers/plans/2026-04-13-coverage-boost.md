# 覆盖率补强专项计划

> **面向 AI 代理的工作者：** 优先遵循测试优先与验证先行原则。此轮目标是不改业务行为，只补测试与必要文档，同步维护动态 TODO。

**目标：** 将仓库总体 Go 单元测试覆盖率从当前 `82.6%` 提升到开发规范要求的 `85%` 及以上。

**约束：**
- 不修改现有业务语义，不为测试牺牲生产代码可读性。
- 优先补已有逻辑的错误分支、参数分支、降级分支。
- 每一批补测后都要重新执行受影响包测试、全量测试、全量覆盖率统计。

**当前覆盖率分析结论：**
- 优先级最高的薄弱包：
  - `internal/api/http`
  - `internal/dashboard`
  - `internal/app`
- 优先补测的低覆盖函数：
  - `internal/api/http/videos.go`
  - `internal/api/http/storage.go`
  - `internal/api/http/system.go`
  - `internal/api/http/creators.go`
  - `internal/dashboard/service.go`
  - `internal/app/app.go`

---

### 任务 1：补齐接口层错误分支与参数分支

**文件：**
- 修改：`internal/api/http/router_test.go`

- [x] 步骤 1：补 `GET /videos` 的非法参数、服务错误、方法错误分支
- [x] 步骤 2：补 `GET /videos/{id}`、`POST /videos/{id}/download`、`POST /videos/{id}/check` 的不存在、服务未就绪、内部错误分支
- [x] 步骤 3：补 `PATCH /creators/{id}`、`GET /system/status`、`GET /storage/stats`、`POST /storage/cleanup` 的缺失分支
- [x] 步骤 4：补 `parsePathID` 辅助函数边界分支

### 任务 2：补齐 dashboard 服务降级与辅助分支

**文件：**
- 修改：`internal/dashboard/service_test.go`

- [x] 步骤 1：补 `checkCookie` 的 `not_configured`、`unknown`、`invalid`、`error` 分支
- [x] 步骤 2：补 `runtimeStatus`、`GetVideo`、`ListJobs`、`ListVideos` 的空仓库或非 provider 分支
- [x] 步骤 3：补 `scanStorage` 和 `percent` 的边界场景
- [x] 步骤 4：补 `GetStorageStats` / `GetSystemStatus` 的降级路径

### 任务 3：补齐 app 启动与恢复辅助分支

**文件：**
- 修改：`internal/app/app_test.go`
- 修改：`internal/app/recovery_test.go`

- [x] 步骤 1：补 `jobPayloadInt64` 的多类型与异常类型分支
- [x] 步骤 2：补 `storageVideoPath` 默认平台分支
- [x] 步骤 3：补恢复逻辑中的空文件、空仓库、错误路径等恢复分支
- [x] 步骤 4：如仍未达标，再补 `New` / `Run` 的边缘分支

### 任务 4：验证与文档同步

**文件：**
- 修改：`docs/todo.md`
- 修改：`docs/superpowers/plans/2026-04-13-coverage-boost.md`

- [x] 步骤 1：执行受影响包测试
- [x] 步骤 2：执行 `go test ./... -count=1`
- [x] 步骤 3：执行全量覆盖率统计并记录结果
- [x] 步骤 4：根据结果更新 TODO 状态与专项结论

## 当前结果

- 本轮未修改生产逻辑，只新增和扩展了测试与文档。
- 覆盖率补强重点落在 `internal/api/http`、`internal/dashboard`、`internal/app`。
- 关键提升点：
  - `internal/api/http/videos.go`、`internal/api/http/storage.go`、`internal/api/http/system.go` 已补齐主要错误分支
  - `internal/dashboard/service.go` 的 `checkCookie`、`runtimeStatus`、`percent` 已覆盖完整边界
  - `internal/app/app.go` 的 `jobPayloadInt64`、`storageVideoPath`、恢复链路边界已补齐
- 最新验证：
  - `go test ./internal/api/http -count=1` 通过
  - `go test ./internal/dashboard -count=1` 通过
  - `go test ./internal/app -count=1` 通过
  - `go test ./... -count=1` 通过
  - `go test -p 1 ./... -coverprofile=...` 总覆盖率 `86.8%`
- 结论：已达到 `docs/dev-standards.md` 规定的 `85%` 覆盖率基线。
- 下一优先级：回到“前端运维能力补全”。
