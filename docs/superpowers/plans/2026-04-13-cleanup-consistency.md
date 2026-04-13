# 清理后数据一致性实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 消除“本地文件、`video_files`、`videos.state` 三者不一致”的高频场景，尤其是文件缺失但数据库仍显示 `DOWNLOADED`，以及重下载时残留脏文件记录的问题。

**架构：** 通过仓储层补 `DeleteByVideoID` 能力，在应用启动恢复时扫描 `DOWNLOADED` 且本地文件缺失的视频并修复为 `NEW`；在下载链路里，重下载前先清理旧文件记录，若写 `video_files` 失败则删除本地文件并回滚状态，保证文件系统与数据库同步变化。

**技术栈：** Go、MySQL、sqlmock、单元测试

---

### 任务 1：补齐失败测试

**文件：**
- 修改：`internal/app/recovery_test.go`
- 修改：`internal/worker/handler_test.go`
- 修改：`internal/repo/mysql/video_file_repo_test.go`

- [x] **步骤 1：先写失败测试，覆盖启动时修复缺失文件的 `DOWNLOADED` 视频**
- [x] **步骤 2：先写失败测试，覆盖重下载前清理脏 `video_files` 记录**
- [x] **步骤 3：先写失败测试，覆盖文件记录写入失败时的回滚行为**

### 任务 2：实现一致性修复逻辑

**文件：**
- 修改：`internal/repo/repo.go`
- 修改：`internal/repo/mysql/video_file_repo.go`
- 修改：`internal/app/app.go`
- 修改：`internal/worker/handler.go`

- [x] **步骤 1：为 `VideoFileRepository` 增加按视频删除文件记录能力**
- [x] **步骤 2：在启动恢复中补 `DOWNLOADED` 缺失文件修复**
- [x] **步骤 3：在下载链路中补脏记录清理与失败回滚**

### 任务 3：验证与文档同步

**文件：**
- 修改：`docs/todo.md`
- 修改：`docs/superpowers/plans/2026-04-13-cleanup-consistency.md`

- [x] **步骤 1：运行受影响模块测试**
- [x] **步骤 2：运行 `go test ./... -count=1`**
- [x] **步骤 3：更新动态 TODO，标记当前完成情况**

## 当前结果

- 本轮已完成“清理后的数据一致性”主题。
- 下一轮切换到“补全运维接口”，重点补单视频操作和手动 cleanup 入口。
