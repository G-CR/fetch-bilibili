# cleanup 清理链路实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 在本地存储超过安全阈值时，自动清理低价值且非核心的视频文件，把空间优先留给绝版视频。

**架构：** 通过仓储层提供 cleanup 候选查询与文件记录删除能力，worker 在执行 `cleanup` 任务时先计算当前存储占用，再按“是否绝版 -> 粉丝量 -> 播放量 -> 收藏量”的顺序排序候选并逐个删除，最后更新视频状态与中文日志。TODO 文档同步反映当前进度。

**技术栈：** Go、MySQL、net/http、os/filepath、sqlmock、单元测试

---

### 任务 1：落地动态 TODO 与 cleanup 计划

**文件：**
- 创建：`docs/todo.md`
- 创建：`docs/superpowers/plans/2026-04-13-cleanup-pipeline.md`

- [x] **步骤 1：写入优先级 TODO 文档**
- [x] **步骤 2：写入 cleanup 详细实施计划**

### 任务 2：扩展仓储接口与查询模型

**文件：**
- 修改：`internal/repo/models.go`
- 修改：`internal/repo/repo.go`
- 修改：`internal/repo/mysql/video_repo.go`
- 修改：`internal/repo/mysql/video_file_repo.go`
- 测试：`internal/repo/mysql/video_repo_test.go`
- 测试：`internal/repo/mysql/video_file_repo_test.go`

- [x] **步骤 1：先写失败测试，覆盖 cleanup 候选查询、文件记录删除、剩余记录统计**
- [x] **步骤 2：新增 cleanup 候选模型与仓储接口**
- [x] **步骤 3：实现 MySQL join 查询与删除逻辑**
- [x] **步骤 4：运行仓储层测试确认通过**

### 任务 3：实现 cleanup worker 流程

**文件：**
- 修改：`internal/worker/handler.go`
- 测试：`internal/worker/handler_test.go`
- 修改：`internal/app/app.go`

- [x] **步骤 1：先写失败测试，覆盖阈值内 no-op、排序规则、删除成功、候选不足、删除异常**
- [x] **步骤 2：为 handler 增加存储清理策略配置注入**
- [x] **步骤 3：实现 cleanup 流程与中文日志**
- [x] **步骤 4：运行 worker / app 测试确认通过**

### 任务 4：验证与文档同步

**文件：**
- 修改：`README.md`
- 修改：`docs/todo.md`

- [x] **步骤 1：运行 `go test ./... -count=1`**
- [x] **步骤 2：更新 README 中 cleanup 状态说明**
- [x] **步骤 3：更新 `docs/todo.md` 中 cleanup 状态**
