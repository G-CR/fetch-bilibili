# 存储投影层设计规格

## 背景

当前视频文件直接落在 `storage.root_dir/{platform}/{video_id}.mp4` 下，数据库中的 `video_files.path` 指向真实文件路径。这个方案对业务链路简单，但对人工浏览、按博主管理、归档检查和离线审计都不友好。

用户希望把“业务存储”和“人工浏览目录”解耦：数据库继续作为唯一信源，真实文件保留稳定主存储路径；面向人工浏览的目录结构、分类和 `json` 索引由独立投影层实时生成。

## 目标

构建一个“数据库真源 + 主存储 + 浏览投影”的结构，满足以下要求：

- 数据库继续作为唯一信源。
- 真实视频文件放在稳定主存储路径，不再直接以博主目录组织。
- 人工浏览目录按平台、博主、普通视频、绝版视频清晰分类。
- 浏览目录中的视频使用符号链接，不要求是真实文件实体。
- 每个博主目录下实时维护 `creator.json` 和 `index.json`。
- `index.json` 只展示当前磁盘上仍存在的视频，不保留已被清理的视频历史。
- 博主改名时，浏览目录名需要跟随变化。
- 投影层与业务逻辑解耦；投影失败不应阻塞核心抓取、下载、检查、清理链路。

## 非目标

- 第一版不引入跨进程消息队列或数据库 Outbox。
- 第一版不把 `json` 作为业务判定依据。
- 第一版不保留已删除视频的浏览历史。
- 第一版不做多平台统一抽象，只先覆盖当前 B 站链路。

## 设计原则

### 1. 唯一信源原则

- `creators`、`videos`、`video_files` 仍是权威数据来源。
- 浏览目录、符号链接、`creator.json`、`index.json` 全部视为派生产物。
- 任意投影层数据损坏，都必须可以从数据库重建。

### 2. 业务与投影解耦原则

- 业务层只负责数据库状态流转和主存储写入。
- 投影层只负责“给人看”的目录结构和元数据导出。
- 投影更新采用异步通知 + 定时全量对账，不要求与业务提交强一致，只要求最终一致。

### 3. 人工浏览优先原则

- 浏览目录需要按博主可读化组织。
- 普通视频与绝版视频物理分桶。
- 元数据文件集中在 `_meta/` 目录，降低阅读成本。

### 4. 稳定主键原则

- 博主的唯一身份仍然是 `platform + uid`。
- 目录名允许带博主名称，但必须由 `uid` 锚定。
- 视频真实文件主键仍然是 `platform + video_id`。

## 存储拓扑

### 主存储（业务真实文件）

```text
/data/
  store/
    bilibili/
      BV1xxx.mp4
      BV1yyy.mp4
```

说明：

- 主存储只按 `platform/video_id` 组织。
- 数据库中的 `video_files.path` 指向主存储真实路径。
- cleanup 删除的是主存储文件，而不是浏览目录中的链接。

### 浏览投影（人工浏览目录）

```text
/data/
  library/
    bilibili/
      creators/
        352981594_我是猫南北/
          _meta/
            creator.json
            index.json
          videos/
            BV1xxx.mp4 -> /data/store/bilibili/BV1xxx.mp4
          rare/
            BV1yyy.mp4 -> /data/store/bilibili/BV1yyy.mp4
```

说明：

- 浏览目录中的视频文件全部使用符号链接。
- `videos/` 存放普通本地库存。
- `rare/` 存放 `OUT_OF_PRINT` 状态的视频。
- `DELETED` 视频不出现在浏览目录中。

## 目录命名规则

### 主存储目录

- 平台目录：`{platform}`
- 文件名：`{video_id}.mp4`

### 浏览目录中的博主目录

格式：`{uid}_{safe_name}`

约束：

- `uid` 必须存在。
- `safe_name` 由博主名称清洗得到：保留中文、英文、数字、下划线和短横线；其他字符统一替换为 `_`。
- 名称为空时，回退为 `unknown`。
- 博主改名后，目录名跟随更新。

## JSON 设计

### `_meta/creator.json`

用途：展示博主当前快照。

建议字段：

```json
{
  "manifest_version": 1,
  "generated_at": "2026-04-13T12:00:00Z",
  "platform": "bilibili",
  "uid": "352981594",
  "name": "我是猫南北_",
  "status": "active",
  "follower_count": 0,
  "local_video_count": 12,
  "local_rare_count": 3,
  "storage_bytes": 1234567890,
  "directory": "352981594_我是猫南北"
}
```

### `_meta/index.json`

用途：展示该博主当前本地库存索引。

建议字段：

```json
{
  "manifest_version": 1,
  "generated_at": "2026-04-13T12:00:00Z",
  "platform": "bilibili",
  "uid": "352981594",
  "videos": [
    {
      "video_id": "BV1xxx",
      "title": "示例视频",
      "state": "DOWNLOADED",
      "publish_time": "2026-04-01T10:00:00Z",
      "out_of_print_at": "",
      "stable_at": "",
      "relative_path": "videos/BV1xxx.mp4",
      "size_bytes": 123456789
    }
  ]
}
```

约束：

- `index.json` 只保留当前磁盘上仍然存在的视频。
- cleanup 删除后，对应记录从 `index.json` 中移除。
- `relative_path` 使用相对博主目录的路径，避免绝对路径绑定。

## 状态与目录映射

- `DOWNLOADED`：投影到 `videos/`
- `STABLE`：投影到 `videos/`
- `OUT_OF_PRINT`：投影到 `rare/`
- `NEW` / `DOWNLOADING`：不出现在浏览目录中
- `DELETED`：不出现在浏览目录中

说明：

- 视频从普通状态变成 `OUT_OF_PRINT` 时，只重建符号链接位置，不移动真实文件。
- 视频从 `OUT_OF_PRINT` 重新变为可访问时，回到 `videos/`。

## 架构设计

### 1. 业务层

职责：

- 维护 `creators`、`videos`、`video_files`、`jobs`
- 下载真实文件到主存储目录
- 处理下架检查、清理、恢复、重试

约束：

- 业务层不直接创建浏览目录和 `json`
- 业务层只在关键状态变化后发布投影更新事件

### 2. 投影层

职责：

- 根据数据库当前状态构建浏览目录
- 创建 / 删除符号链接
- 实时更新 `creator.json` 和 `index.json`
- 处理博主改名后的目录迁移

建议接口：

```go
type LibraryProjector interface {
    NotifyCreator(ctx context.Context, creatorID int64)
    NotifyVideo(ctx context.Context, videoID int64)
    RebuildCreator(ctx context.Context, creatorID int64) error
    RebuildAll(ctx context.Context) error
}
```

### 3. 事件通知

第一版采用进程内异步通知：

- 业务层提交成功后，调用 `NotifyCreator` / `NotifyVideo`
- 投影 worker 从缓冲队列异步消费
- 同一博主的事件允许合并，避免短时间重复重建

说明：

- 该方案不保证事件零丢失
- 通过“启动全量重建 + 定时全量对账”保证最终一致

### 4. 全量对账

需要两个入口：

- 启动完成后执行一次 `RebuildAll`
- 调度器按固定周期执行一次全量对账（例如每 6 小时）

目的：

- 修补因异常退出、磁盘操作失败、事件丢失导致的投影偏差
- 清理失效符号链接和空目录

## 关键流程

### 下载成功

1. 业务层把真实文件写入 `/data/store/{platform}/{video_id}.mp4`
2. 业务层写入 `video_files.path`
3. 业务层把 `videos.state` 更新为 `DOWNLOADED`
4. 业务层发布 `NotifyVideo(videoID)`
5. 投影层重建对应博主目录：
   - 创建或刷新 `videos/{video_id}.mp4` 符号链接
   - 更新 `creator.json`
   - 更新 `index.json`

### 视频变绝版

1. 业务层把 `videos.state` 更新为 `OUT_OF_PRINT`
2. 业务层发布 `NotifyVideo(videoID)`
3. 投影层删除 `videos/` 中旧链接
4. 投影层创建 `rare/` 中新链接
5. 投影层重写 `index.json`

### cleanup 删除

1. 业务层删除主存储真实文件
2. 业务层删除 `video_files` 记录并更新 `videos.state = DELETED`
3. 业务层发布 `NotifyVideo(videoID)`
4. 投影层移除对应符号链接
5. 投影层从 `index.json` 移除该视频

### 博主改名

1. 业务层更新 `creators.name`
2. 业务层发布 `NotifyCreator(creatorID)`
3. 投影层计算新的目录名
4. 投影层基于数据库全量重建该博主目录
5. 投影层清理旧目录

## 一致性与容错

### 原则

- 数据库优先于投影目录。
- 投影失败不回滚业务事务。
- 投影层必须支持重复执行和全量重建。

### 文件写入策略

- `creator.json` 和 `index.json` 使用“临时文件写入 + `rename` 原子替换”。
- 符号链接创建前先删除旧链接，再创建新链接。
- 单博主目录更新时加锁，避免并发写坏目录结构。

### 容错策略

- 如果符号链接创建失败，只记录日志并等待下次事件 / 对账修复。
- 如果目录重命名失败，保留旧目录并在下次对账时重试。
- 如果 `_meta/` 写入失败，不影响主业务状态。

## 对现有代码的影响

### 路径构建

当前：

- `internal/worker/handler.go`：`buildVideoPath`
- `internal/app/app.go`：`storageVideoPath`

调整后：

- 真实文件路径改为 `store/` 路径构建函数
- 浏览目录路径改由投影层统一计算

### 仓储查询

投影层需要新增查询能力：

- 按博主列出当前仍有本地文件的视频及其 `video_files.path`
- 读取博主当前名称、状态、粉丝数
- 支持按 `creator_id` 重建投影

### 事件接入点

建议在以下链路接入通知：

- 博主新增 / 更新 / 停用
- 下载成功
- 下架检查更新状态
- cleanup 删除
- 启动恢复修复状态

## 迁移策略

### 第一阶段

- 保留现有数据库结构
- 保留现有 `video_files.path` 字段
- 只调整真实文件落盘路径到 `store/`
- 新增 `library/` 投影目录

### 第二阶段

- 启动时全量扫描数据库，重建全部浏览目录和 `json`
- 不要求迁移旧浏览目录，直接以新投影结构为准

### 第三阶段

- 稳定后再考虑增加投影健康检查、重建管理接口、后台重放能力

## 风险与边界

- 符号链接依赖宿主文件系统能力，容器挂载路径必须保持一致。
- 博主改名会触发整目录重建，单博主视频很多时会有瞬时 I/O 波动。
- 第一版使用进程内事件队列，异常退出时可能丢失局部投影事件，因此必须依赖全量对账兜底。
- 若未来引入多进程 worker 或多实例部署，需要升级为持久化事件或 Outbox 模式。

## 结论

本方案采用“数据库真源 + 主存储 + 浏览投影”的三层结构：

- 数据库负责业务真相
- `store/` 负责真实文件落盘
- `library/` 负责人工浏览目录和 `json` 索引

该设计既能满足人工浏览清晰、博主目录跟名更新、绝版与普通视频分桶、`json` 实时更新等诉求，也能把目录投影与业务链路保持解耦，避免目录结构反向绑架核心业务逻辑。
