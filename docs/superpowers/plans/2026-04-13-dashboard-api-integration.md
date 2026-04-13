# 驾驶舱真实接口对接实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 为前端驾驶舱补齐真实后端查询接口，并将总览、任务、视频、存储、风控区块从占位数据切换为 API 实时数据。

**架构：** 后端新增只读查询接口 `/jobs`、`/videos`、`/system/status`、`/storage/stats`，通过 `internal/dashboard` 聚合服务统一组装读模型；前端通过独立 `api.js` 拉取资源并更新驾驶舱状态，保持前后端仅通过 HTTP 交互。

**技术栈：** Go、MySQL、net/http、React、Vite

---

### 任务 1：补齐计划文档与接口边界

**文件：**
- 创建：`docs/superpowers/plans/2026-04-13-dashboard-api-integration.md`
- 修改：`docs/api.md`

- [x] **步骤 1：写明目标接口与前端真实联动范围**
- [x] **步骤 2：在实现完成后同步更新 API 文档**

### 任务 2：扩展仓储查询能力

**文件：**
- 修改：`internal/repo/repo.go`
- 修改：`internal/repo/mysql/job_repo.go`
- 修改：`internal/repo/mysql/video_repo.go`
- 测试：`internal/repo/mysql/job_repo_test.go`
- 测试：`internal/repo/mysql/video_repo_test.go`

- [x] **步骤 1：先写失败测试，覆盖任务列表、任务计数、视频列表、视频计数**
- [x] **步骤 2：为 JobRepository / VideoRepository 增加读接口声明**
- [x] **步骤 3：实现 MySQL 查询**
- [x] **步骤 4：运行相关 Go 测试并确认通过**

### 任务 3：新增 dashboard 聚合服务

**文件：**
- 创建：`internal/dashboard/service.go`
- 创建：`internal/dashboard/service_test.go`

- [x] **步骤 1：先写失败测试，覆盖系统状态、存储统计、总览汇总**
- [x] **步骤 2：实现聚合服务，整合 creators/videos/jobs/config/auth 状态**
- [x] **步骤 3：运行测试确认通过**

### 任务 4：新增 HTTP 查询接口

**文件：**
- 修改：`internal/api/http/router.go`
- 修改：`internal/api/http/jobs.go`
- 创建：`internal/api/http/videos.go`
- 创建：`internal/api/http/system.go`
- 创建：`internal/api/http/storage.go`
- 测试：`internal/api/http/router_test.go`

- [x] **步骤 1：先写失败测试，覆盖新接口返回结构与错误处理**
- [x] **步骤 2：将 `/jobs` 改为支持 GET + POST**
- [x] **步骤 3：新增 `/videos`、`/system/status`、`/storage/stats`**
- [x] **步骤 4：运行 HTTP 层测试确认通过**

### 任务 5：接入应用装配

**文件：**
- 修改：`internal/app/app.go`
- 测试：`internal/app/app_test.go`

- [x] **步骤 1：先写失败测试，验证新 router 依赖接入**
- [x] **步骤 2：把 dashboard service 注入 router**
- [x] **步骤 3：运行 app 测试确认通过**

### 任务 6：前端 API 层真实对接

**文件：**
- 修改：`frontend/src/lib/api.js`
- 修改：`frontend/src/lib/state.js`
- 修改：`frontend/src/App.jsx`

- [x] **步骤 1：先写或扩展前端 smoke 测试需要的 API 适配逻辑**
- [x] **步骤 2：新增 `listJobs`、`listVideos`、`getSystemStatus`、`getStorageStats`**
- [x] **步骤 3：将总览、任务、存储、风控区块切为 API 数据驱动**
- [x] **步骤 4：保留 local 模式，API 模式下默认真实联动**

### 任务 7：验证与文档

**文件：**
- 修改：`README.md`
- 修改：`docs/api.md`
- 修改：`docs/container-deploy.md`

- [x] **步骤 1：运行 `go test ./...`**
- [x] **步骤 2：运行 `cd frontend && npm run test:vite-config && npm run test:smoke && npm run build`**
- [x] **步骤 3：运行 `docker compose up -d --build` 并验证前后端联通**
- [x] **步骤 4：更新 README 与 API 文档**
