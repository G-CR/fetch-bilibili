# 调度与任务设计（当前实现）

## 1. 定位

当前调度层的职责很简单：按配置周期创建后台任务，把具体执行交给 worker。

它不是：

- cron 表达式驱动的复杂调度系统
- 带重试 / 补偿 / 回放能力的任务编排器
- 多队列、多优先级、多租户的调度中心

当前实现是一个进程内 ticker 调度器，核心目标是稳定地产生以下任务：

- `fetch`
- `check`
- `cleanup`
- `discover`（可选）

## 2. 当前调度模型

### 2.1 进程内 ticker

调度器位于 `internal/scheduler`，由 `internal/app` 在应用启动时创建并启动。

当前行为：

- `fetch_interval` 控制 `fetch` 周期
- `check_interval` 控制 `check` 周期
- `cleanup_interval` 控制 `cleanup` 周期
- `discovery.interval` 控制 `discover` 周期

实现方式是标准 `time.Ticker`，不是日历式调度，也不是“每天固定几点执行”。

### 2.2 启动条件

当前创建调度器时要求：

- `scheduler.fetch_interval > 0`
- `scheduler.check_interval > 0`
- `scheduler.cleanup_interval > 0`

否则调度器初始化直接失败。

`discover` 是可选调度项，只有同时满足以下条件才会启用对应 ticker：

- `discovery.enabled = true`
- `discovery.interval > 0`

### 2.3 时间语义

当前 ticker 调度有几个需要明确的语义：

- 进程启动后不会立刻执行一次任务，而是等首个 interval 到期后再触发。
- 进程重启后，ticker 重新从当前时刻开始计时，不保留上一次运行的相位。
- 当前没有“错过窗口后补跑”的机制。
- 当前没有随机抖动、错峰或动态退避逻辑。

## 3. 当前任务类型与来源

### 3.1 周期任务

调度器当前只会周期性创建以下任务：

- `fetch`
- `check`
- `cleanup`
- `discover`（仅在 discovery 调度开启时）

其中：

- `fetch` 用于批量拉取启用博主的新视频列表。
- `check` 用于批量检查视频可访问性。
- `cleanup` 只是周期性发起一次清理尝试，真正是否删除文件要由 worker 在执
  行时根据存储占用判断。
- `discover` 用于运行候选池自动发现。

### 3.2 非周期任务

当前仓库里还有多条“非调度器直接创建”的任务入口：

- `POST /jobs`
  支持手工触发 `fetch`、`check`、`cleanup`
- `POST /storage/cleanup`
  手工触发 `cleanup`
- `POST /videos/{id}/download`
  创建单视频 `download`
- `POST /videos/{id}/check`
  创建单视频 `check`
- `POST /candidate-creators/discover`
  手工触发 `discover`
- 候选批准后
  若开启 `discovery.auto_enqueue_fetch_on_approve`，会尝试创建带
  `creator_id` 的定向 `fetch`

因此，当前任务来源不只有周期调度器，还包括多个 HTTP 运维入口和候选审核流
程。

### 3.3 当前未被调度器直接创建的任务

以下任务当前不会由 ticker 直接创建：

- `download`

`download` 只会从其他链路派生出来，例如：

- `fetch` 发现新视频后补下游下载任务
- 手工单视频重下

## 4. 入队与去重语义

### 4.1 当前入队行为

调度器本身不直接操作数据库，而是调用 `jobs.Service`：

- `EnqueueFetch`
- `EnqueueCheck`
- `EnqueueCleanup`
- `EnqueueDiscover`

`jobs.Service` 再调用 `JobRepository.Enqueue` 把任务写入 `jobs` 表，并在成功
后发布 `job.changed` 事件。

### 4.2 当前去重粒度

当前入队去重规则由 `internal/repo/mysql/job_repo.go` 决定：

- `download`
  按 `video_id` 对活动任务去重
- 其他任务类型
  只按 `type + status in (queued, running)` 去重

这意味着当前真实粒度是：

- 所有活动中的 `fetch` 任务彼此互斥，不区分是否带 `creator_id`
- 所有活动中的 `check` 任务彼此互斥，不区分是否为单视频检查
- 所有活动中的 `cleanup` 任务彼此互斥
- 所有活动中的 `discover` 任务彼此互斥

这不是文档层面的“设计目标”，而是当前仓库真实实现。

### 4.3 去重后的返回语义

若入队命中活动任务去重：

- 仓储层返回 `ErrJobAlreadyActive`
- `jobs.Service` 会把这个错误吞掉并返回 `nil`

因此，上层调用方通常只会看到“请求成功”，而不一定真的插入了新任务。

这会影响：

- 定时调度重复触发时的表现
- 手工点击触发接口时的返回理解
- 候选批准后定向 `fetch` 的触发预期

当前应把它理解为“幂等接受请求”，而不是“保证创建了一条新任务”。

## 5. 当前配置与实际生效关系

### 5.1 当前真正生效

调度层当前真正生效的配置包括：

- `scheduler.fetch_interval`
- `scheduler.check_interval`
- `scheduler.cleanup_interval`
- `scheduler.check_stable_days`
- `discovery.enabled`
- `discovery.interval`

其中：

- 前三个控制 ticker 频率
- `check_stable_days` 不影响调度是否触发，但会影响 `check` 任务执行后的状态
  判定
- `discovery.enabled` 和 `discovery.interval` 共同决定是否开启 discover 调
  度

### 5.2 当前未形成完整调度语义

虽然任务模型和接口中存在 `not_before` 字段，但当前默认链路里：

- 调度器不会主动设置 `not_before`
- `jobs.Service` 不负责重试排程
- 当前没有自动指数退避
- 当前没有最大重试次数控制

仓储层只是在取任务时尊重 `not_before <= NOW()` 这一条件。

## 6. 当前执行结果的真实含义

### 6.1 `fetch`

调度器触发 `fetch` 只代表：

- 尝试创建一次批量拉取任务

真正的后续效果还取决于：

- 是否命中活动任务去重
- worker 是否成功执行
- 是否发现新视频
- 是否成功派生 `download`

### 6.2 `check`

调度器触发 `check` 只代表：

- 尝试创建一次批量检查任务

它不会保证：

- 所有视频本轮都被检查
- 状态一定发生变化
- 本轮一定会产生 `OUT_OF_PRINT` / `STABLE`

### 6.3 `cleanup`

调度器触发 `cleanup` 只代表：

- 尝试创建一次清理任务

是否真的删除文件，由 worker 在执行时决定：

- 若当前占用未超过安全阈值，会直接跳过
- 若候选不足，也可能执行失败

因此，“调度了 cleanup” 不等于 “本轮一定发生了清理”。

### 6.4 `discover`

调度器触发 `discover` 只代表：

- 尝试创建一次候选池发现任务

是否发现新候选，取决于：

- discovery 是否启用
- 当前关键词、关系扩散输入与 B 站返回结果
- 是否被已有候选、状态规则或评分逻辑过滤

## 7. 当前缺失的能力

以下内容在旧文档里常被写成“已有设计”，但当前实现并未落地：

- 基于 cron 表达式的日历调度
- 每日固定时刻的 cleanup
- 指数退避自动重试
- 最大重试次数
- 基于 `schedule_window` 的细粒度去重
- 下载并发与检查并发分别由调度器独立控制
- 任务超时控制与超时后自动恢复

这些内容如果后续要引入，应作为新的实现任务处理，不能视为当前既有能力。

## 8. 当前限制与后续收口点

当前调度层最关键的限制有：

- 调度粒度粗，只是固定 interval ticker。
- `fetch/check/cleanup/discover` 当前是按任务类型全局去重，粒度偏粗。
- `download` 不由调度器直接创建，链路分散在多个上游入口。
- `not_before` 已有字段支持，但没有形成完整重试系统。
- 通用 `POST /jobs` 与专用触发入口并存，入口语义还不完全统一。

后续若继续收口调度层，优先级建议为：

1. 先统一文档、接口和当前实现的触发语义。
2. 再决定是否需要更细粒度的任务去重。
3. 最后再考虑重试、退避、独立并发和更复杂的时间编排。
