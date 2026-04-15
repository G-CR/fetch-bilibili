# 项目 TODO（动态更新）

最后更新时间：2026-04-15（已完成“实时驾驶舱 SSE 与低频对账”的联调，继续推进“存储投影层与人工浏览目录”）

说明：
- 本文档用于维护当前项目的实施优先级、执行状态和下一步动作。
- 状态说明：
  - `todo`：未开始
  - `doing`：进行中
  - `done`：已完成
  - `blocked`：有阻塞

## P0

- [x] `done` 落地 `cleanup` 清理链路
  - 目标：在本地存储超过安全阈值时，按“是否绝版 -> 博主粉丝量 -> 播放量 -> 收藏量”规则清理低价值文件。
  - 完成情况：
    - 已补 cleanup 候选查询
    - 已补文件记录删除与剩余统计
    - 已实现 worker cleanup 流程
    - 已补中文日志与单元测试
  - 结果要求：
    - 支持手动触发和调度触发
    - 中文日志
    - 单元测试覆盖主流程与异常分支

- [x] `done` 任务幂等与异常恢复
  - 目标：解决任务重启、状态卡死、重复执行等问题。
  - 范围：
    - `DOWNLOADING` / `running` 卡死恢复
    - 启动后续跑恢复
    - 重复任务保护
  - 已完成：
    - 启动时将残留 `running` 任务重新入队
    - 启动时修复无活动下载任务的 `DOWNLOADING` 视频状态
    - 支持按本地文件存在性恢复为 `NEW` / `DOWNLOADED`
    - 重新入队时清理任务时间戳字段，避免状态语义残留
    - `fetch/check/cleanup` 已拦截重复活动任务
    - `download` 已按 `video_id` 拦截重复活动任务
  - 结果要求：
    - 调度重复触发不会持续堆积同类活动任务
    - 启动恢复后任务状态与时间戳语义一致
    - 单元测试覆盖重复入队与恢复分支

- [x] `done` 清理后的数据一致性
  - 目标：文件、数据库状态、存储统计保持一致。
  - 范围：
    - 文件删除后同步删除 `video_files`
    - 更新视频状态
    - 避免“文件不存在但数据库仍显示已下载”
  - 已完成：
    - cleanup 删除后同步删除 `video_files` 并在最后一个文件删除后回写 `DELETED`
    - 启动时修复 `DOWNLOADED` 且本地文件缺失的视频，状态回写为 `NEW`
    - 重下载前清理该视频残留的脏 `video_files` 记录
    - 文件记录写入失败时删除本地文件并回滚状态，避免“文件有了但数据库没记住”
  - 结果要求：
    - 本地文件、`video_files`、`videos.state` 保持同步
    - 避免文件已缺失但视频仍长期显示 `DOWNLOADED`
    - 单元测试覆盖恢复、重下载、回滚分支

## P1

- [x] `done` 测试覆盖率补强（`>= 85%`）
  - 目标：在不修改业务行为的前提下，补齐低覆盖率分支测试，让仓库总体覆盖率达到开发规范基线。
  - 已完成：
    - 已补 `internal/api/http` 的视频运维接口、存储接口、系统状态接口、`parsePathID` 边界分支测试
    - 已补 `internal/dashboard` 的 Cookie 状态、风险分级、存储扫描、边界辅助函数测试
    - 已补 `internal/app` 的鉴权 watcher 创建、`jobPayloadInt64`、恢复流程边界测试
  - 验证情况：
    - `go test ./internal/api/http -count=1` 通过
    - `go test ./internal/dashboard -count=1` 通过
    - `go test ./internal/app -count=1` 通过
    - `go test ./... -count=1` 通过
    - `go test -p 1 ./... -coverprofile=...` 总覆盖率为 `86.8%`
  - 结果要求：
    - `go test ./... -count=1` 通过
    - `go test -p 1 ./... -coverprofile=...` 总覆盖率不低于 `85%`
    - 同步更新专项计划与 TODO 状态

- [x] `done` 补全运维接口
  - `PATCH /creators/{id}`
  - `GET /videos/{id}`
  - `POST /videos/{id}/download`
  - `POST /videos/{id}/check`
  - `POST /storage/cleanup`
  - 已完成：
    - 支持单博主 patch 更新 `name` / `status`
    - 支持单视频详情查询
    - 支持单视频手动重下与单视频手动检查
    - 支持手动触发 cleanup
    - worker 已支持按 `video_id` 执行单视频检查
  - 结果要求：
    - 驾驶舱和运维调用方可以不依赖全局调度，直接操作单条资源
    - 单元测试覆盖接口成功、参数错误和单视频检查执行分支
- [x] `done` 数据库迁移自动化
  - 目标：从手工 SQL 初始化升级到迁移工具。
  - 已完成：
    - 已接入 `Goose + embed SQL`
    - 已新增 `migrations/00001_init.sql` 作为权威 schema 来源
    - 已在启动阶段自动执行迁移
    - 已新增 `mysql.auto_migrate` 配置开关，默认开启
    - 已同步 README、运行文档、容器文档与配置示例
    - 已补迁移链路单元测试与启动阶段测试
  - 验证情况：
    - `go test ./... -count=1` 已通过
    - `go test -p 1 ./... -coverprofile=...` 已通过
  - 备注：
    - 当时全仓覆盖率已低于开发规范中的 `85%` 基线，属于当前仓库的存量质量缺口，需要后续专项补齐

- [x] `done` 风控与 Cookie 观测增强
  - 目标：更清楚地展示 Cookie 是否有效、风险是否升高、请求是否退避。
  - 已完成：
    - 已在 bilibili client 内维护认证与风控运行时快照
    - 已输出 Cookie 来源、最近检查/刷新结果、最近错误
    - 已输出风控退避剩余秒数、退避截止时间、最近命中原因
    - 已增强 AuthWatcher 中文日志
    - 前端风控区块已展示新增观测字段
  - 验证情况：
    - `go test ./internal/platform/bilibili ./internal/dashboard ./internal/api/http -count=1` 已通过
    - `cd frontend && npm run test:state` 已通过
    - `go test -p 1 ./... -coverprofile=...` 总覆盖率当前为 `82.6%`

- [x] `done` 前端运维能力补全
  - 目标：补齐清理预览、任务详情、失败原因和更完整的系统观测。
  - 已完成：
    - 已新增任务详情面板，支持查看状态、时间轴、payload
    - 已在任务列表和详情面板中展示失败原因
    - 已新增清理预览面板，按现有可见数据近似推演候选顺序
    - 已补存储压力和预览候选数量展示
  - 约束说明：
    - 本轮未新增后端接口
    - 清理预览当前基于现有视频 / 任务 / 存储接口做前端近似推演
    - 博主粉丝量权重暂无法从现有 API 获取，界面已明确标注
  - 验证情况：
    - `cd frontend && npm run test:state` 已通过
    - `cd frontend && npm run test:smoke` 已通过
    - `cd frontend && npm run test:vite-config` 已通过
    - `cd frontend && npm run build` 已通过

- [x] `done` 实时驾驶舱 SSE 与低频对账
  - 目标：驾驶舱在不手动刷新的前提下，自动看到任务、视频、博主、存储和系统状态变化；断线后自动恢复。
  - 已完成：
    - 已打通后端 `/events/stream` 与前端 `EventSource` 增量更新链路
    - 已展示实时连接状态：连接中 / 实时同步中 / 重连中 / 连接中断
    - 已增加手动重建之外的低频对账：`GET /system/status` 每 30 秒一次，整页快照每 60 秒一次
    - 已支持 SSE 断线自动重连，重连成功后自动补一次快照
    - 已补 Playwright 用例，覆盖任务自动推进、断线重连、恢复补快照
    - 已修复局部 SSE 事件合并会清空旧字段的问题，并补状态脚本验证
    - 已增加快照 / 轮询版本保护，避免旧 HTTP 响应回退新状态
  - 验证情况：
    - `go test ./... -count=1` 已通过
    - `cd frontend && npm run test:state` 已通过
    - `cd frontend && npm run test:smoke` 已通过
    - `cd frontend && npm run build` 已通过
    - `cd frontend && npm run test:e2e` 已通过
  - 结果要求：
    - 驾驶舱默认依赖实时事件更新，不再要求用户手动点刷新
    - 断线、后端重启、短时事件丢失后，都能自动回到一致状态

## P2

- [x] `done` 浏览器级 E2E 联调测试
  - 目标：锁定前后端真实联动行为，避免界面可访问但数据链路失效。
  - 已完成：
    - 已接入 Playwright 浏览器级 E2E
    - 已提供 `mock` 模式，可在仓库内自起 mock API + 前端 dev server
    - 已覆盖页面打开、同步数据、添加博主、触发任务、任务详情 4 条链路
    - 已提供 `live` 模式入口，可直连本机 `frontend/app`
    - `live` 模式添加博主用例已改为显式提交 `uid + name`，不再依赖真实环境中的名称解析能力
  - 验证情况：
    - `cd frontend && npm run test:e2e` 通过（`mock` 模式）
    - `cd frontend && E2E_MODE=live E2E_BASE_URL=http://127.0.0.1:5173 E2E_API_BASE=http://127.0.0.1:8080 npm run test:e2e` 通过（`live` 模式）

- [ ] `doing` 存储投影层与人工浏览目录
  - 目标：将数据库继续作为唯一信源，把真实文件落盘与人工浏览目录解耦，按平台 / 博主 / 普通视频 / 绝版视频组织浏览视图。
  - 设计要点：
    - 真实文件进入 `store/` 主存储目录
    - 浏览目录进入 `library/` 投影目录
    - 浏览目录中的视频使用符号链接，不复制真实文件
    - 每个博主目录实时维护 `_meta/creator.json` 和 `_meta/index.json`
    - `index.json` 只保留当前磁盘上仍存在的视频，不保留已删除历史
    - 博主改名时浏览目录名需要同步更新
  - 已完成：
    - 已形成设计规格：`docs/superpowers/specs/2026-04-13-storage-library-projection-design.md`
    - 已形成实现计划：`docs/superpowers/plans/2026-04-13-storage-library-projection.md`
    - 已新增 `internal/library` 基础投影层，覆盖主存储路径、浏览目录路径、符号链接投影、`creator.json` / `index.json` 原子写入、博主改名旧目录清理
    - 已把真实文件路径切换到 `storage.root_dir/store/{platform}/{video_id}.mp4`
    - 已同步修复下载、启动恢复、缺失文件修复、cleanup 容量统计和 dashboard 存储统计对 `store/` 的识别
    - 已通过验证：`go test ./internal/library ./internal/worker ./internal/app ./internal/dashboard -count=1`
  - 当前进行中：
    - 接入投影层实时通知
    - 增加按博主重建的队列与幂等保护
    - 增加仓储查询与启动 / 定时全量重建
  - 结果要求：
    - 业务层只认数据库和主存储
    - 投影层失败不阻塞主流程
    - 支持启动重建和定时对账修复投影偏差

## P3

- [ ] `todo` 多平台扩展
  - 抖音
  - 快手
  - 小红书

- [ ] `todo` App 化
  - 独立客户端
  - 多用户 / 权限
  - 云端或分布式存储

## 当前推荐执行顺序

1. 存储投影层与人工浏览目录
2. 多平台扩展
3. App 化
