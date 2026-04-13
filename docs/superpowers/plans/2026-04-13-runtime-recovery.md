# 任务幂等与异常恢复实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 提升任务系统在服务重启、异常中断、状态残留场景下的可恢复性，避免 `running` 任务卡死、`DOWNLOADING` 视频长时间悬空，并为后续重复任务保护打基础。

**架构：** 应用启动时先执行恢复流程：将残留的 `running` 任务重新入队，再扫描 `DOWNLOADING` 视频并结合活动下载任务与本地文件状态恢复为 `NEW` 或 `DOWNLOADED`。仓储层同步保证任务重新入队时状态字段语义一致。主 TODO 文档动态反映当前进度。

**技术栈：** Go、MySQL、net/http、os/filepath、sqlmock、单元测试

---

### 任务 1：补齐恢复场景测试

**文件：**
- 创建：`internal/app/recovery_test.go`
- 修改：`internal/repo/mysql/job_repo_test.go`

- [x] **步骤 1：收敛启动恢复边界与目标行为**
- [x] **步骤 2：先写失败测试，覆盖 running 任务重入队、DOWNLOADING 状态恢复、活动下载任务保护**
- [x] **步骤 3：补仓储层失败测试，约束 queued 状态下时间字段清理语义**

### 任务 2：实现启动恢复与仓储语义

**文件：**
- 修改：`internal/app/app.go`
- 修改：`internal/repo/mysql/job_repo.go`

- [x] **步骤 1：补齐启动恢复细节，确保活动下载任务识别稳定**
- [x] **步骤 2：修正任务重新入队时的时间字段更新语义**

### 任务 3：验证与动态文档更新

**文件：**
- 修改：`docs/todo.md`
- 修改：`docs/superpowers/plans/2026-04-13-runtime-recovery.md`

- [x] **步骤 1：运行针对性测试**
- [x] **步骤 2：运行 `go test ./... -count=1`**
- [x] **步骤 3：更新动态 TODO 与计划状态，准备切换到下一子任务**

## 当前结果

- 本轮已完成“启动恢复 + 任务重入队”子任务。
- 后续“重复任务保护”子任务已在独立计划中完成。
- 当前“任务幂等与异常恢复”主题已完成，下一优先级切换为“清理后的数据一致性”。
