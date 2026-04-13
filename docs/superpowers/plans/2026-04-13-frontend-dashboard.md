# 前端驾驶舱重构实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 删除旧 `web/` 静态原型，使用 `React + Vite` 重建一个 `Raycast` 风格的单页驾驶舱前端。

**架构：** 前端放入独立 `frontend/` 目录，采用组件化结构、轻量状态层和独立 API 封装。第一版保留现有后端联动范围，其他模块用前端状态模拟，后续再逐步替换为真实接口。

**技术栈：** React、Vite、JavaScript、CSS

---

### 任务 1：建立前端工程骨架

**文件：**
- 创建：`frontend/package.json`
- 创建：`frontend/index.html`
- 创建：`frontend/src/main.jsx`
- 创建：`frontend/src/App.jsx`
- 创建：`frontend/src/styles.css`
- 删除：`web/index.html`
- 删除：`web/app.js`
- 删除：`web/styles.css`

- [ ] 创建最小 Vite + React 工程配置
- [ ] 删除旧 `web/` 静态入口
- [ ] 确认新的前端入口结构清晰可维护

### 任务 2：实现驾驶舱布局与视觉系统

**文件：**
- 修改：`frontend/src/App.jsx`
- 修改：`frontend/src/styles.css`

- [ ] 搭建左侧边栏 + 主工作区骨架
- [ ] 实现 Raycast 风格的深色主题和 B 站粉蓝强调
- [ ] 实现指标区、运行态区、博主管理区、存储区、风控区、设置区
- [ ] 实现单页锚点导航与基础动效

### 任务 3：接入现有 API 模式与本地状态

**文件：**
- 创建：`frontend/src/lib/api.js`
- 创建：`frontend/src/lib/state.js`
- 修改：`frontend/src/App.jsx`

- [ ] 抽离 API 请求函数
- [ ] 保留本地模式和 API 模式切换
- [ ] 接入现有 `GET /creators`、`POST /creators`、`POST /jobs`
- [ ] 为无接口模块提供前端占位状态

### 任务 4：补充运行文档与容器后续说明

**文件：**
- 修改：`README.md`

- [ ] 更新前端目录与启动方式说明
- [ ] 明确前后端解耦关系
- [ ] 说明第一版数据联动边界

### 任务 5：验证可运行性

**文件：**
- 无

- [ ] 安装前端依赖
- [ ] 运行前端构建或最小校验命令
- [ ] 如有必要修复路径或语法问题
