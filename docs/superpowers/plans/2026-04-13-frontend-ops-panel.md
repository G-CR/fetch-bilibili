# 前端运维能力补全计划

> **面向 AI 代理的工作者：** 本轮以前端实现为主，不新增后端接口。先补测试，再补实现，最后同步 TODO 和验证结果。

**目标：** 在现有驾驶舱基础上补齐首批运维工作面板，让前端能够更直接地查看任务详情、失败原因，以及当前存储压力下的清理候选预览。

**首版设计决策：**
- 不新增后端 API，完全基于现有 `/jobs`、`/videos`、`/system/status`、`/storage/stats`、`/videos/{id}`、单视频动作接口完成。
- 任务详情基于当前任务列表中的字段展开，包括状态、时间、payload、失败原因。
- 清理预览基于前端当前可见视频数据做近似排序，优先考虑：
  - 是否绝版
  - 播放量
  - 收藏量
- 由于现有 API 未暴露博主粉丝量，界面需要明确提示该维度暂未纳入预览计算。

**涉及文件：**
- 修改：`frontend/src/App.jsx`
- 修改：`frontend/src/lib/state.js`
- 修改：`frontend/src/styles.css`
- 修改：`frontend/scripts/test-dashboard-state.mjs`
- 修改：`frontend/scripts/smoke-render.mjs`
- 修改：`docs/todo.md`
- 修改：`docs/superpowers/plans/2026-04-13-frontend-ops-panel.md`

---

### 任务 1：补前端状态与交互测试

- [x] 为清理预览派生逻辑补失败测试
- [x] 为任务详情 / 失败原因展示补失败测试
- [x] 为首屏 smoke render 补新面板存在性断言

### 任务 2：实现运维面板

- [x] 在任务区增加选中态与详情面板
- [x] 在任务详情中突出失败原因、payload、时间轴
- [x] 在存储区增加清理预览列表和限制说明
- [x] 保持本地模式和 API 模式都可用

### 任务 3：回归验证与文档同步

- [x] 执行 `cd frontend && npm run test:state`
- [x] 执行 `cd frontend && npm run test:smoke`
- [x] 执行 `cd frontend && npm run build`
- [x] 更新 TODO 与本计划的完成状态

## 当前结果

- 本轮未新增后端接口，完全基于现有前端状态和已开放 HTTP 接口补齐运维面板。
- 已完成：
  - 任务列表选中态与任务详情面板
  - 失败原因显式展示
  - 清理预览面板
  - 存储压力与候选数量展示
- 清理预览说明已在界面标注：
  - 当前按“非绝版 → 播放量 → 收藏量”做前端近似推演
  - 博主粉丝量维度待后端接口补齐
- 最新验证：
  - `cd frontend && npm run test:state` 通过
  - `cd frontend && npm run test:smoke` 通过
  - `cd frontend && npm run test:vite-config` 通过
  - `cd frontend && npm run build` 通过
- 下一优先级：浏览器级 E2E 联调测试。
