# Worker 消费端设计（当前实现）

## 1. 定位

当前 worker 层负责从 `jobs` 表中消费任务，并执行实际的后台工作。它不是一套
独立的消息中间件，也不是按任务类型拆分的多队列系统，而是单进程内的一组共
享 goroutine。

当前 worker 负责处理的任务类型包括：

- `fetch`
- `download`
- `check`
- `cleanup`
- `discover`

## 2. 运行模型

### 2.1 共享 worker 池

- 应用启动时由 `internal/app` 创建一个 `WorkerPool`。
- 当前 worker 数量取自 `limits.download_concurrency`。
- 这个值当前实际控制的是“整个共享 worker 池大小”，不是“仅下载任务并发”。
- `pollEvery` 当前固定为 `2s`。

也就是说，当前系统并没有为 `download`、`check`、`cleanup`、`discover` 建立
独立 worker pool；所有任务共享同一批消费线程。

### 2.2 取任务方式

worker 每轮只拉取 1 条待执行任务：

1. 调用 `jobs.FetchQueued(ctx, 1)`。
2. 任务仓储在事务内查询 `queued` 且 `not_before <= NOW()` 的任务。
3. 查询使用 `FOR UPDATE SKIP LOCKED`，避免多个 worker 重复抢到同一条任务。
4. 取出后立即在事务内更新为 `running`。

因此，任务“开始执行”的真实边界是在仓储层完成 `queued -> running` 更新之
后，而不是 handler 开始处理时。

### 2.3 状态流转

当前 worker 只负责以下任务状态流转：

- `queued -> running`
- `running -> success`
- `running -> failed`

执行过程中会发布 `job.changed` 事件：

- 入队时由 `jobs.Service` 发布一次
- 被 worker 抢到并进入 `running` 时发布一次
- 执行完成进入 `success` / `failed` 时再发布一次

## 3. 当前 handler 行为

### 3.1 `fetch`

`fetch` handler 的职责是“发现新视频并补下载任务”，不是直接下载文件。

执行流程：

1. 若 payload 中存在 `creator_id`，则只拉该博主；否则遍历当前启用博主。
2. 对每个博主先走限流等待。
3. 调用 B 站客户端拉取最新视频列表。
4. 对视频做 `Upsert`，新视频以 `NEW` 状态入库。
5. 只有在视频当前仍为 `NEW` 时，才继续创建 `download` 任务。
6. 若 `download` 活动任务已存在，则跳过重复创建。

当前 `fetch` 的直接输出是：

- 补齐视频元数据
- 为新视频创建下载任务

它不会直接推进到文件落盘。

### 3.2 `download`

`download` handler 的职责是把单个视频落到 `store/`，并写入文件记录。

执行流程：

1. 从 payload 中读取 `video_id`。
2. 读取视频记录并计算目标落盘路径。
3. 若视频状态已是 `DOWNLOADED` 且本地文件存在且非空，则直接视为完成。
4. 若数据库显示已下载但文件不存在，则先删除脏 `video_files` 记录。
5. 初始化存储快照，确保后续能正确增量更新 `storage.changed`。
6. 将视频状态更新为 `DOWNLOADING`。
7. 走限流等待后调用 B 站客户端下载。
8. 下载成功后写入 `video_files`，再把视频状态更新为 `DOWNLOADED`。
9. 发布 `video.changed` 与 `storage.changed`。

失败语义：

- 普通下载错误：把视频状态回退为 `NEW`，任务记为 `failed`。
- 文件记录写入失败：删除已下载文件并把视频状态回退为 `NEW`。
- 永久错误（`PermanentError`）：当前实现会把视频状态写为 `FAILED`，并发
  布 `video.changed`。

说明：

- `FAILED` 目前是 worker 实现中的状态写法，但尚未进入仓库公开需求基线和
  API 状态枚举，应视为当前实现细节，后续仍需统一。

### 3.3 `check`

`check` handler 的职责是检查视频当前是否仍可访问，并维护 `OUT_OF_PRINT` /
`STABLE` 状态。

执行流程：

1. 若 payload 中有 `video_id`，则只检查单视频；否则按批量范围取待检查视
   频。
2. 对每个视频按博主走限流等待。
3. 调用 B 站客户端检查可访问性。
4. 根据检查结果更新：
   - 不可访问 -> `OUT_OF_PRINT`
   - 超过稳定阈值仍可访问 -> `STABLE`
   - 其余情况只刷新检查时间
5. 若状态发生需要广播的变化，则发布 `video.changed`。

当前 `check` 不会为每个视频维护独立定时器，而是依赖调度器周期性批量触发。

### 3.4 `cleanup`

`cleanup` handler 的职责是在存储超过阈值时删除部分本地文件并修正状态。

执行流程：

1. 扫描 `store/` 当前实际使用量。
2. 若未超过安全阈值，则直接跳过。
3. 从数据库查询清理候选。
4. 先按保留期过滤近期文件。
5. 按当前实现排序候选：
   - 绝版优先保留
   - 粉丝量越低越优先删
   - 播放量越低越优先删
   - 收藏量越低越优先删
   - 同优先级下再参考文件大小、标题、文件记录 ID
6. 删除真实文件。
7. 删除 `video_files` 记录。
8. 若该视频已无剩余文件，则把视频状态更新为 `DELETED`。
9. 发布 `video.changed` 与 `storage.changed`。

当前 cleanup 不直接依赖配置里的权重表进行动态算分，而是使用代码中的固定比
较逻辑。

### 3.5 `discover`

`discover` handler 本身较薄，只负责调用 discovery runner。

真正的发现逻辑位于 `internal/discovery/`，当前包含：

- 关键词发现
- 一跳关系扩散

worker 在这里承担的是“任务执行入口”角色，而不是评分与审核逻辑的承载者。

## 4. 并发与限流

### 4.1 当前已接入的限制

worker 当前已接入两类限流：

- 全局 QPS：`limits.global_qps`
- 按博主 QPS：`limits.per_creator_qps`

这两类限制通过 `waitForCreator` 生效，当前会影响：

- `fetch`
- `check`
- `download`

`cleanup` 和 `discover` 当前不走这套按博主限流。

### 4.2 当前未落地的限制

配置中存在：

- `limits.download_concurrency`
- `limits.check_concurrency`

但当前真正接入执行面的只有 `download_concurrency`，且它被用作整个共享 worker
池大小。

`check_concurrency` 目前只体现在系统状态输出里，没有真正作用到 worker 调
度或检查执行并发。

## 5. 任务幂等与去重边界

### 5.1 当前已落地

- `fetch`、`check`、`cleanup`、`discover`
  当前按“同类型存在活动任务”进行去重。
- `download`
  当前按 `video_id` 对活动任务去重。
- `download` 对已存在且非空的本地文件具备幂等短路。

### 5.2 当前未落地

以下能力当前没有在 worker 层完整实现：

- 自动重试
- 指数退避
- 按任务类型独立并发
- 基于 `schedule_window` 的细粒度 fetch/check 去重
- handler 内统一超时包装

虽然仓储层支持 `not_before` 字段，`FetchQueued` 也会尊重它，但默认调度链路
和当前 worker 流程还没有把它真正用于重试 / 延迟执行。

## 6. 事件与派生状态

worker 除了执行任务，还会驱动多个派生观察面更新：

- `job.changed`
- `video.changed`
- `storage.changed`

这些事件的直接作用包括：

- 驱动前端 SSE 增量更新
- 驱动 `library/` 浏览投影同步器增量重建
- 更新前端中的存储、任务、视频状态展示

因此，worker 不只是“后台干活线程”，也是运行态变更事件的重要来源。

## 7. 与其他模块的边界

### 7.1 worker 负责的事

- 消费 `jobs` 表中的待执行任务
- 执行具体后台动作
- 更新任务最终状态
- 发布执行过程中的状态变化事件

### 7.2 worker 不负责的事

- 任务调度策略：由 `internal/scheduler` 负责
- 启动恢复：由 `internal/app` 负责
- 候选池评分与审核：由 `internal/discovery` 负责
- 前端状态合并：由前端本地状态层负责
- 浏览目录重建：由 `internal/library` 负责

## 8. 当前限制与后续收口点

当前 worker 设计的主要限制如下：

- 所有任务共享一个 worker 池，容易互相争抢执行位。
- `check_concurrency` 尚未接线，配置与实现不完全一致。
- `not_before` 已有仓储支持，但默认执行链路未形成完整重试语义。
- `FAILED` 视频状态当前只存在于下载失败实现与 `video.changed` 的 SSE 运行时
  例外中，不属于 `/videos` 快照状态基线。
- 当前没有按任务类型隔离的 timeout / backoff / retry 机制。

因此，后续若继续演进 worker 层，优先级应是：

1. 先统一当前公开契约与实现语义。
2. 再决定是否拆分独立 worker pool。
3. 最后再引入自动重试、退避和更细粒度并发控制。
