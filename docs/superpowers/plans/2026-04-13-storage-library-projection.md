# 存储投影层实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 在保持数据库为唯一信源的前提下，引入 `store/` 主存储和 `library/` 浏览投影目录，用符号链接和实时 `json` 索引提升人工浏览体验。

**架构：** 业务层只写数据库和主存储；投影层独立维护 `library/` 目录、符号链接和 `_meta/*.json`。业务链路通过进程内通知触发按博主重建，启动与定时任务执行全量对账，保证最终一致。

**技术栈：** Go、MySQL、文件系统、符号链接、单元测试

---

### 任务 1：定义投影层边界与失败测试

**文件：**
- 创建：`internal/library/projector.go`
- 创建：`internal/library/projector_test.go`
- 创建：`internal/library/path.go`
- 创建：`internal/library/types.go`
- 修改：`internal/worker/handler_test.go`
- 修改：`internal/app/app_test.go`

- [x] **步骤 1：先写失败测试，定义主存储路径和浏览目录路径规则**
- [x] **步骤 2：先写失败测试，覆盖普通视频投影到 `videos/`、绝版视频投影到 `rare/`**
- [x] **步骤 3：先写失败测试，覆盖 `creator.json`、`index.json` 的原子更新**
- [x] **步骤 4：先写失败测试，覆盖博主改名后的目录重建与旧目录清理**

### 任务 2：实现主存储路径与下载链路切换

**文件：**
- 修改：`internal/worker/handler.go`
- 修改：`internal/app/app.go`
- 修改：`internal/worker/handler_test.go`
- 修改：`internal/app/recovery_test.go`

- [x] **步骤 1：新增 `store/` 主存储路径构建函数，替换现有扁平路径**
- [x] **步骤 2：让下载成功后的 `video_files.path` 指向主存储真实路径**
- [x] **步骤 3：同步修复启动恢复、缺失文件检查、cleanup 路径判断**
- [x] **步骤 4：运行受影响测试，确认主存储路径切换不破坏现有恢复逻辑**

### 任务 3：实现浏览投影层与实时通知

**文件：**
- 创建：`internal/library/queue.go`
- 创建：`internal/library/export.go`
- 修改：`internal/app/app.go`
- 修改：`internal/creator/service.go`
- 修改：`internal/worker/handler.go`

- [x] **步骤 1：实现按博主重建浏览目录、符号链接和 `_meta/*.json` 的核心逻辑**
- [x] **步骤 2：在博主新增 / 更新、下载成功、状态变更、cleanup 删除后发送投影通知**
- [x] **步骤 3：为单博主重建增加目录级锁和原子写入保护**
- [x] **步骤 4：运行投影层测试，验证重复通知和并发重建的幂等性**

### 任务 4：补仓储查询与全量重建能力

**文件：**
- 修改：`internal/repo/repo.go`
- 修改：`internal/repo/models.go`
- 修改：`internal/repo/mysql/video_repo.go`
- 修改：`internal/repo/mysql/video_repo_test.go`
- 修改：`internal/repo/mysql/creator_repo.go`
- 修改：`internal/repo/mysql/creator_repo_test.go`
- 修改：`internal/library/projector.go`

- [x] **步骤 1：新增“按博主列出当前本地库存视频”的仓储查询**
- [x] **步骤 2：新增“列出全部活跃博主用于全量重建”的仓储扫描能力**
- [x] **步骤 3：实现 `RebuildAll`，用于启动时和定时对账时重建投影目录**
- [x] **步骤 4：验证全量重建可以修复丢失链接、脏目录和缺失 `json`**

### 任务 5：接入启动重建、运维入口与文档

**文件：**
- 修改：`internal/app/app.go`
- 修改：`internal/app/app_test.go`
- 修改：`docs/todo.md`
- 修改：`README.md`
- 修改：`docs/runbook.md`
- 修改：`docs/config.md`
- 修改：`docs/data-model.md`
- 修改：`docs/superpowers/plans/2026-04-13-storage-library-projection.md`

- [x] **步骤 1：在应用启动后执行一次投影全量重建**
- [x] **步骤 2：补运行文档，说明 `store/` 与 `library/` 的职责边界**
- [x] **步骤 3：补运维说明，明确浏览目录和 `json` 不作为业务真源**
- [x] **步骤 4：更新动态 TODO 和本计划状态**

### 任务 6：完整验证

**文件：**
- 修改：`docs/superpowers/plans/2026-04-13-storage-library-projection.md`

- [x] **步骤 1：运行 `go test ./internal/library ./internal/worker ./internal/app -count=1`**
- [x] **步骤 2：运行 `go test ./... -count=1`**
- [ ] **步骤 3：在容器内验证下载成功后 `store/` 与 `library/` 同步更新**
- [ ] **步骤 4：验证博主改名后浏览目录重建成功，旧目录被清理**

## 当前结果

- 本轮已确认“数据库真源 + 主存储 + 浏览投影”的设计方向。
- 已完成任务 1：落地 `internal/library` 路径 / 快照 / 投影器基础能力，并用失败测试锁定路径、目录投影、元数据原子写入和改名清理行为。
- 已完成任务 2：主存储真实文件路径切换到 `storage.root_dir/store/{platform}/{video_id}.mp4`，并同步修复下载、启动恢复、缺失文件检查、cleanup 统计和仪表盘存储统计。
- 已完成任务 3：补齐 `internal/library/export.go` / `internal/library/queue.go`，接入 `creator.changed` / `video.changed` 实时通知，并通过按博主队列避免重复重建。
- 已完成任务 4：补齐仓储层 `ListLibraryByCreator` / `ListForLibraryAfter`，支持启动重建和全量对账，并在无本地库存时清理空博主目录。
- 已完成任务 5：应用启动时先执行一次浏览目录全量重建，再启动实时同步；运行、配置、数据模型和动态 TODO 文档已同步更新。
- 当前落盘结构已升级为：
  - `store/`：真实文件
  - `library/`：符号链接与 `json` 视图
- 已完成验证：
  - `go test ./internal/library ./internal/repo/mysql -count=1`
  - `go test ./internal/library ./internal/worker ./internal/app ./internal/dashboard ./internal/creator -count=1`
  - `go test ./... -count=1`
- 剩余待环境联调项：
  - 在容器内验证下载成功后 `store/` 与 `library/` 实时同步
  - 验证博主改名后的浏览目录迁移与旧目录清理
