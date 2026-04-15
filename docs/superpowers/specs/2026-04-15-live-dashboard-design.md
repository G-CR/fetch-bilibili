# 实时驾驶舱设计规格

## 背景

当前前端驾驶舱通过 `loadDashboardSnapshot` 一次性拉取博主、任务、视频、系统状态和存储统计。页面首次打开会同步一次，用户点击“立即拉取”“检查下架”“清理存储”等操作后也会主动刷新一次。

这个方案已经能满足基本运维，但存在明显体验缺口：

- 任务从 `queued` 进入 `running`、再进入 `success/failed` 时，页面不会自动变化。
- 用户点完“立即拉取”后，需要手动再次点击“同步数据”才能看到后续状态。
- 风控退避、Cookie 状态、视频状态、博主状态虽然已经能通过后端接口拿到，但没有持续动态更新机制。

用户希望把这个页面升级为“自动变化的实时驾驶舱”，尽量减少手动刷新，同时保留系统实现上的可控复杂度。

## 目标

- 页面打开后自动进入“实时跟随”状态，不依赖手动刷新查看任务进度。
- 任务列表、任务详情、视频状态、博主列表、Cookie/风控状态自动更新。
- 存储统计尽量实时更新，但允许通过低频全量对账修正聚合漂移。
- 前端在断线、重连、页面隐藏、后端重启等场景下行为可预期。
- 保持现有 HTTP 快照接口不变，新增实时通道而不是替换现有接口。
- 第一版继续适配当前单后端实例部署，不为未来多实例架构过度设计。

## 非目标

- 第一版不引入 WebSocket。
- 第一版不引入 Redis、Kafka、数据库 Outbox 或跨实例消息总线。
- 第一版不做历史事件回放。
- 第一版不保证前端仅靠实时事件即可恢复完整状态；仍保留快照兜底。
- 第一版不在服务端按秒推送倒计时数字。

## 方案选择

### 方案一：SSE + 前端本地派生 + 低频全量对账

这是推荐方案。

核心思路：

- 后端新增 `SSE` 长连接接口，推送离散状态变化事件。
- 前端继续保留现有快照加载逻辑，作为首屏初始化和断线重连兜底。
- 任务、视频、博主、Cookie/风控等离散变更通过实时事件推送。
- 存储统计和总览卡片通过“事件增量更新 + 定时快照校正”实现最终一致。
- 风控退避剩余秒数由前端根据 `backoff_until` 本地倒计时，不让后端每秒推送。

优点：

- 交互体验上接近“全实时”。
- 服务端改动可控，容易接入当前单实例架构。
- 保留现有 HTTP 快照接口，迁移风险低。
- 以后如果扩成多实例，前端协议可以不变，只替换后端事件来源。

缺点：

- 前端状态管理会比现在复杂。
- 存储统计和总览聚合不是纯事件强一致，而是“事件优先 + 快照修正”。

### 方案二：所有字段都走服务端强实时推送

核心思路：

- 不仅任务和视频，连存储统计、风险倒计时、聚合指标也全部通过后端持续推送。

优点：

- 概念简单，所有变化都依赖单一实时通道。

缺点：

- 有些字段天然不适合强实时推送，例如退避剩余秒数。
- 后端需要承担更多高频聚合与广播逻辑。
- 复杂度和维护成本明显高于真实收益。

### 方案三：增强轮询

核心思路：

- 只要存在活跃任务就每 3 秒轮询一次，没有活跃任务就每 30 秒轮询一次。

优点：

- 实现最快。
- 风险最小。

缺点：

- 交互上仍不是事件驱动。
- 请求冗余较多。
- 页面“实时感”弱于 SSE。

## 最终决策

采用方案一：`SSE + 前端本地派生 + 低频全量对账`。

原则：

- 用户视角：整个页面自动更新。
- 后端实现：只对真正适合事件化的状态做 SSE 推送。
- 聚合型或倒计时型字段通过前端派生或低频校正实现。

## 设计原则

### 1. 快照仍然是权威读取入口

- 现有 `/creators`、`/jobs`、`/videos`、`/system/status`、`/storage/stats` 保持不变。
- 页面首次加载和重连后恢复状态时，仍然依赖快照接口。
- 实时通道只负责“把变化尽快告诉前端”，不替代初始化和纠偏能力。

### 2. 实时事件只传离散变化

- 适合事件化的状态：
  - 任务入队、开始、成功、失败
  - 视频状态变化
  - 博主被添加、暂停、停止追踪、名称补齐
  - Cookie 状态变化
  - 风控命中、恢复
- 不适合高频服务端推送的状态：
  - 风控剩余秒数
  - 热点目录等扫描型聚合结果

### 3. 页面效果实时，内部实现分层

- 第一层：SSE 离散事件更新
- 第二层：前端本地派生状态
- 第三层：低频快照对账

这样既保留实时体验，也控制系统复杂度。

### 4. 单实例优先，协议可迁移

- 第一版使用进程内事件总线。
- 不做跨实例广播。
- 事件协议、前端消费逻辑保持可迁移，后续扩多实例时仅替换事件源。

## 后端架构

### 1. 新增进程内事件总线

新增包建议：`internal/live`

建议接口：

```go
type Broker interface {
    Publish(event Event)
    Subscribe(ctx context.Context, buffer int) <-chan Event
}

type Event struct {
    ID      string
    Type    string
    At      time.Time
    Payload any
}
```

要求：

- 采用广播模式，一个发布事件可以被多个 SSE 连接消费。
- 每个订阅者有独立缓冲区。
- 某个慢订阅者阻塞时，不能拖垮整个系统。
- 当订阅者通道写满时，允许丢弃该订阅者并让前端自动重连，不做无限堆积。

### 2. 新增 SSE 接口

新增接口：`GET /events/stream`

行为：

- 响应头：
  - `Content-Type: text/event-stream`
  - `Cache-Control: no-cache`
  - `Connection: keep-alive`
- 建立连接后先推送一条 `hello`
- 每 15 秒发送一次 `heartbeat`
- 后续持续推送业务事件

示例：

```text
event: hello
data: {"server_time":"2026-04-15T09:00:00+08:00"}

event: job.changed
data: {"id": 101, "status": "running", "type": "download"}
```

### 3. 事件类型

第一版定义以下事件：

- `hello`
- `heartbeat`
- `job.changed`
- `video.changed`
- `creator.changed`
- `system.changed`
- `storage.changed`
- `snapshot.required`

其中：

- `job.changed`：任务状态变化
- `video.changed`：视频状态或关键指标变化
- `creator.changed`：博主新增、状态切换、名称补齐
- `system.changed`：Cookie、风控、认证状态变化
- `storage.changed`：已用空间、文件数、热点目录、绝版数等统计变化
- `snapshot.required`：当后端判断事件可能丢失、状态难以局部修复时，主动通知前端补一次快照

### 4. 事件负载设计

原则：

- 事件负载只包含前端局部更新所需字段。
- 同时保留 `id` 和业务主键，方便前端定位。
- 不强制让前端为每类事件重新请求详情接口。

建议结构：

#### `job.changed`

```json
{
  "id": 101,
  "type": "download",
  "status": "running",
  "payload": {"video_id": 12},
  "error_msg": "",
  "started_at": "2026-04-15T09:00:10+08:00",
  "finished_at": "",
  "updated_at": "2026-04-15T09:00:10+08:00"
}
```

#### `video.changed`

```json
{
  "id": 12,
  "video_id": "BV1xx",
  "creator_id": 7,
  "state": "DOWNLOADED",
  "title": "示例视频",
  "out_of_print_at": "",
  "stable_at": "",
  "last_check_at": "2026-04-15T09:00:20+08:00"
}
```

#### `creator.changed`

```json
{
  "id": 7,
  "platform": "bilibili",
  "uid": "352981594",
  "name": "我是猫南北_",
  "status": "active"
}
```

#### `system.changed`

```json
{
  "cookie": {
    "configured": true,
    "is_login": true,
    "uname": "我是猫南北_",
    "status": "valid",
    "source": "config",
    "last_check_at": "2026-04-15T09:01:00+08:00",
    "last_check_result": "valid"
  },
  "risk": {
    "level": "低",
    "active": false,
    "backoff_until": "",
    "last_hit_at": "",
    "last_reason": ""
  }
}
```

#### `storage.changed`

```json
{
  "used_bytes": 123456789,
  "file_count": 18,
  "hottest_bucket": "store/bilibili",
  "rare_videos": 3,
  "usage_percent": 12
}
```

## 事件发布点

### 1. 任务入队

位置：

- `internal/jobs/service.go`

时机：

- `EnqueueFetch`
- `EnqueueCheck`
- `EnqueueCleanup`
- `EnqueueDownload`
- `EnqueueCheckVideo`

行为：

- 任务成功入队后立即发布 `job.changed`

### 2. 任务进入运行态和结束态

位置：

- `internal/worker/worker.go`
- `internal/repo/mysql/job_repo.go`

时机：

- `FetchQueued` 将任务置为 `running` 后
- worker 将任务置为 `success/failed` 后

行为：

- 发布 `job.changed`
- 如果任务状态会导致概览卡片变化，则同时触发概览相关更新

### 3. 视频状态变化

位置：

- `internal/worker/handler.go`

时机：

- 抓取任务发现新视频
- 视频状态从 `NEW -> DOWNLOADING -> DOWNLOADED`
- 视频被标记为 `OUT_OF_PRINT`
- 视频被标记为 `STABLE`
- 视频被 cleanup 删除

行为：

- 发布 `video.changed`
- 需要时同步发布 `storage.changed`

### 4. 博主状态变化

位置：

- `internal/creator/service.go`
- `internal/creator/syncer.go`

时机：

- 新增博主
- 启用 / 暂停
- 停止追踪
- 名称补齐
- 文件同步触发状态变化

行为：

- 发布 `creator.changed`

### 5. 系统状态变化

位置：

- `internal/platform/bilibili/auth.go`
- `internal/platform/bilibili/client.go`

时机：

- Cookie 刷新结果变化
- Cookie 校验成功 / 失败
- 风控命中
- 风控恢复

行为：

- 发布 `system.changed`

### 6. 存储统计变化

位置：

- `internal/worker/handler.go`

时机：

- 下载成功落盘
- cleanup 删除成功

行为：

- 以增量方式发布 `storage.changed`
- 不强制每次重新扫描整个目录

## 前端架构

### 1. 初始化流程

页面首次加载：

1. 调用现有 `loadDashboardSnapshot`
2. 初始化页面状态
3. 建立 `EventSource`
4. 收到实时事件后做局部更新

### 2. 状态管理

建议在 `frontend/src/lib/state.js` 中新增事件应用函数：

```js
applyLiveEvent(previous, event)
```

按事件类型做局部更新：

- `job.changed`：更新任务列表和当前选中任务
- `video.changed`：更新视频列表
- `creator.changed`：更新博主列表
- `system.changed`：更新 Cookie / 风控状态
- `storage.changed`：更新存储统计和总览卡片
- `snapshot.required`：触发一次全量快照同步

### 3. 连接状态

前端新增连接状态字段：

- `connecting`
- `live`
- `reconnecting`
- `offline`

页面需要明确展示：

- 已连接
- 重连中
- 已退回快照模式

### 4. 重连策略

- `EventSource` 断开后自动重连
- 前端同时保留一个退避定时器，避免瞬时重连风暴
- 每次重连成功后主动补一次快照，修正可能遗漏的事件

### 5. 页面隐藏策略

- 页面隐藏时不关闭 SSE
- 但停止本地高频 UI 派生定时器，例如退避秒数倒计时
- 页面恢复可见时立即刷新派生显示

### 6. 低频对账

即使 SSE 正常，也保留低频快照同步：

- `system/status`：每 30 秒
- `storage/stats`：每 60 秒
- 必要时可以整页快照每 60 秒做一次完整对账

作用：

- 修正可能遗漏的事件
- 修正存储聚合增量误差
- 在长连接失效但浏览器未及时感知时保持页面不完全失真

## 存储与风控的“动态效果”

### 存储

用户感知上要求“变化及时”，但不要求每个字段绝对强一致。

实现：

- 下载成功：即时推送 `used_bytes/file_count`
- 删除成功：即时推送 `used_bytes/file_count`
- `hottest_bucket` 等扫描字段通过低频快照校正

### 风控

实现：

- 后端通过 `system.changed` 推送：
  - 是否处于风控退避
  - 风险等级
  - 命中时间
  - 命中原因
  - `backoff_until`
- 前端根据 `backoff_until` 每秒本地刷新剩余时间

这样页面看起来是连续动态变化的，但服务端只在真正状态变化时发一次事件。

## 错误处理

### 1. SSE 连接失败

- 前端展示“实时连接已断开”
- 自动进入重连状态
- 同时回退到低频快照模式

### 2. 订阅者过慢

- 后端允许断开该 SSE 连接
- 前端自动重连并补快照

### 3. 事件消费失败

- 单条坏事件不应导致整个连接崩溃
- 前端记录日志并继续消费后续事件

### 4. 状态不一致

- 后端可推送 `snapshot.required`
- 前端收到后立即全量同步

## 安全与边界

- SSE 为只读接口，不暴露写操作。
- 第一版沿用现有 CORS 逻辑。
- 如果后续需要鉴权，再统一接入前端管理台认证。
- 由于当前是自用部署，第一版不额外引入用户维度权限控制。

## 性能考虑

- 单后端实例下，SSE 连接数量预期较小，进程内广播足够。
- 事件负载只传增量，不做大对象广播。
- `storage.changed` 避免每次都扫描磁盘，优先走业务侧增量更新。
- 对账快照频率控制在秒级或分钟级，避免请求风暴。

## 测试策略

### 后端

- `internal/live`：订阅、广播、慢消费者断开、关闭清理
- `internal/api/http/events.go`：SSE 响应头、hello、heartbeat、断线释放
- `internal/jobs/service.go`：入队后发布 `job.changed`
- `internal/worker/worker.go`：任务状态变化事件
- `internal/worker/handler.go`：视频与存储事件
- `internal/creator/service.go`：博主变更事件
- `internal/platform/bilibili/auth.go`：系统状态事件

### 前端

- `state.js`：按事件类型的 reducer 单测
- `api.js`：SSE 连接封装与重连逻辑测试
- `App.jsx`：连接状态、局部更新、快照对账联动测试
- E2E：
  - 点击“立即拉取”后任务从 `queued -> running -> success` 自动变化
  - SSE 断线后 UI 进入重连态
  - 重连成功后页面恢复

## 实施拆分建议

### 阶段 1：后端实时事件基础设施

- 新增 `internal/live`
- 新增 `/events/stream`
- 接入任务状态事件

### 阶段 2：前端接入任务实时更新

- 新增 SSE 客户端
- 实时更新任务列表与任务详情
- 增加连接状态展示

### 阶段 3：扩展到视频、博主、系统状态

- 接入 `video.changed`
- 接入 `creator.changed`
- 接入 `system.changed`

### 阶段 4：存储增量更新与低频对账

- 接入 `storage.changed`
- 增加低频快照校正
- 完成前端倒计时本地派生

## 涉及文件

- `internal/api/http/router.go`
- `internal/api/http/events.go`
- `internal/api/http/router_test.go`
- `internal/live/broker.go`
- `internal/live/broker_test.go`
- `internal/jobs/service.go`
- `internal/jobs/service_test.go`
- `internal/worker/worker.go`
- `internal/worker/worker_test.go`
- `internal/worker/handler.go`
- `internal/worker/handler_test.go`
- `internal/creator/service.go`
- `internal/creator/service_test.go`
- `internal/platform/bilibili/auth.go`
- `internal/platform/bilibili/auth_test.go`
- `internal/app/app.go`
- `internal/app/app_test.go`
- `frontend/src/App.jsx`
- `frontend/src/lib/api.js`
- `frontend/src/lib/state.js`
- `frontend/e2e/dashboard.spec.js`

## 风险与后续演进

### 当前风险

- 进程内事件总线只适合单实例。
- 存储统计的增量事件与磁盘真实状态之间可能出现短暂偏差。
- 页面状态管理会从“纯快照”升级为“快照 + 实时增量”，复杂度增加。

### 后续演进

- 若后端扩成多实例，可把事件源替换为数据库 Outbox 或消息队列。
- 若需要更强的跨端同步能力，可在 SSE 协议不变的前提下替换内部发布机制。
- 若后续前端要 App 化，这套事件协议也可以复用。

## 验收标准

- 页面打开后，无需手动点击刷新即可看到任务状态持续变化。
- 点击“立即拉取”后，任务从入队到完成在页面上自动反映。
- 视频状态变化会自动同步到页面。
- 博主新增、暂停、停止追踪、名称补齐会自动同步到页面。
- Cookie 和风控状态变化会自动同步到页面。
- 存储占用和文件数量会在下载/删除后尽快变化，并能通过低频对账修正。
- SSE 断线后页面能自动重连，重连成功后状态恢复正常。
