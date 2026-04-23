# 存储策略与清理规则（当前实现）

本文档描述当前仓库已经落地的存储边界、容量统计与 cleanup 行为，不再沿用早
期的“建议模型”或示意权重。

## 1. 存储边界

- `storage.root_dir` 是存储根目录。
- 当前服务在根目录下维护两层结构：
  - `store/`：真实文件目录，业务上的“本地文件真源”。
  - `library/`：人工浏览投影目录，只包含符号链接与 `_meta/*.json`。
- 下载文件实际写入路径为：
  `storage.root_dir/store/{platform}/{video_id}.mp4`。
- `library/` 是派生产物，不参与容量统计，也不是 cleanup 的删除对象。

## 2. 当前会影响存储行为的配置项

- `storage.max_bytes`：总容量上限。
- `storage.safe_bytes`：安全阈值；cleanup 的目标是把已用空间降到这个值以下。
- `storage.keep_out_of_print`：是否优先保留 `OUT_OF_PRINT` 视频。
- `storage.cleanup_retention_hours`：下载文件的最短保留时长。

### 当前尚未完全接线的配置

- `storage.delete_weights` 已定义在配置结构中，也有默认值。
- 但当前 cleanup 主流程没有按 `delete_weights` 做动态算分。
- 因此，这组字段目前更像“保留接口”，不是已经生效的排序开关。

## 3. 容量统计口径

- cleanup 与驾驶舱存储统计当前都只扫描 `storage.root_dir/store/`。
- 统计结果至少包含：
  - `used_bytes`
  - `file_count`
  - `max_bytes`
  - `safe_bytes`
  - `usage_percent`
  - `hottest_bucket`
  - `rare_videos`
  - `cleanup_rule`
- 若 `storage.root_dir` 未配置、不存在，或 `store/` 目录尚未建立，当前统计会按
  `0` 处理，而不是直接报错。

## 4. cleanup 触发与目标

- cleanup 可以由两条链路触发：
  - 调度器周期性入队 `cleanup` 任务。
  - 人工调用 `POST /storage/cleanup`。
- worker 执行 cleanup 时会先扫描 `store/` 的当前占用。
- 若当前占用未超过安全阈值，任务直接跳过并记日志。
- 目标释放量计算规则：
  - 优先使用 `storage.safe_bytes`。
  - 若 `safe_bytes <= 0`，回退到 `storage.max_bytes`。
  - 只有 `used_bytes > threshold` 时，才需要释放空间。
  - 目标释放字节数为 `used_bytes - threshold`。

## 5. cleanup 候选范围

- 当前候选来自 `videos`、`creators`、`video_files` 三表联查。
- 只考虑 `video_files.type='video'` 的真实视频文件。
- 只考虑以下视频状态：
  - `DOWNLOADED`
  - `STABLE`
  - `OUT_OF_PRINT`
- 当 `storage.keep_out_of_print=true` 时，候选查询会直接排除
  `OUT_OF_PRINT`。
- 候选查询当前先按 `vf.created_at ASC, vf.id ASC` 取前 `500` 条，再由 worker
  继续过滤与排序。

## 6. 最短保留期过滤

- worker 会按 `storage.cleanup_retention_hours` 再做一轮过滤。
- 当该值大于 `0` 时：
  - 若 `file_created_at + retention` 仍晚于当前时间，这个文件本轮不会被删。
- 当该值小于等于 `0` 时，不启用这层保留期过滤。

## 7. 当前实际排序规则

候选集合经过保留期过滤后，会按下面的固定规则排序，越靠前越先删：

1. 非 `OUT_OF_PRINT` 先于 `OUT_OF_PRINT`
2. 博主粉丝量更少的优先
3. 播放量更低的优先
4. 收藏量更低的优先
5. 文件更大的优先
6. 标题字典序
7. `file_id`

这意味着当前实现虽然保留了“绝版优先保留 -> 粉丝量 -> 播放量 -> 收藏量”的
主排序方向，但没有接入以下早期文档里提过的维度：

- `age_days`
- `last_access_days`
- 动态权重比例
- 可配置打分公式

## 8. 删除后的写回行为

对单个 cleanup 候选，当前执行顺序是：

1. 尝试删除 `video_files.path` 指向的真实文件。
2. 删除对应的 `video_files` 记录。
3. 若该视频已没有剩余文件记录：
   - 把 `videos.state` 写成 `DELETED`
   - 若原状态是 `OUT_OF_PRINT`，同步把绝版统计减 `1`
4. 发布事件：
   - 必要时发布 `video.changed`
   - cleanup 完成后统一发布一次 `storage.changed`
5. 记录中文日志，包含标题、视频 ID、状态、粉丝量、播放量、收藏量、释放字
   节数和文件路径

## 9. 与浏览投影的关系

- cleanup 删除的是 `store/` 下的真实文件，不直接删除 `library/` 中的符号链
  接。
- `library/` 由独立投影层维护，依赖：
  - 启动时全量重建
  - 运行时消费 `creator.changed`、`video.changed`
- 因此 cleanup 后浏览目录的收敛方式是：
  - 业务层先改真实文件与数据库
  - 事件驱动投影层再重建对应博主目录

## 10. 当前未实现的能力

以下内容曾在早期文档中出现，但当前仓库没有按这些方式实现：

- 基于 `delete_weights` 的动态打分 cleanup
- 手工锁定视频为“永不清理”
- 独立的 cleanup 审计记录表作为当前主结果面
- 基于访问频率或最近访问时间的真实排序
