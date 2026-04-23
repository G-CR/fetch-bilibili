# 数据模型说明（当前实现）

## 1. 说明与事实源

本文档描述当前仓库正在使用的数据模型，不再沿用早期“6 张核心表”的简化版说
法。

判断当前数据模型时，应同时参考以下事实源：

- `internal/repo/models.go`
- `internal/repo/mysql/*.go`
- `migrations/00001_init.sql`
- `migrations/00002_creator_removed_status.sql`
- `migrations/00003_candidate_discovery.sql`

注意：

- 不能只看 `migrations/00001_init.sql`。
- 当前有效 schema 是上述迁移按顺序执行后的结果。

## 2. 当前核心实体

### 2.1 creators（正式追踪博主）

用途：

- 保存正式追踪博主的基础资料和运行状态。
- 作为 `videos` 的上游实体。

当前主要字段：

- `id`
- `platform`
- `uid`
- `name`
- `follower_count`
- `status`
- `created_at`
- `updated_at`

当前状态值：

- `active`
- `paused`
- `removed`

说明：

- `removed` 不是物理删除，而是手工移除后的保留态。
- 文件同步只会把缺失博主自动改为 `paused`，不会自动恢复 `removed`。

### 2.2 videos（视频）

用途：

- 保存视频元数据、生命周期状态和检查结果。

当前主要字段：

- `id`
- `platform`
- `video_id`
- `creator_id`
- `title`
- `description`
- `publish_time`
- `duration`
- `cover_url`
- `view_count`
- `favorite_count`
- `state`
- `out_of_print_at`
- `stable_at`
- `last_check_at`
- `created_at`
- `updated_at`

当前公开状态基线：

- `NEW`
- `DOWNLOADING`
- `DOWNLOADED`
- `OUT_OF_PRINT`
- `STABLE`
- `DELETED`

说明：

- 当前下载实现里仍存在把视频写成 `FAILED`，并通过 `video.changed` 事件发出
  去的例外路径；它尚未进入仓库公开状态基线，此处仍以当前公开契约为准。

### 2.3 video_files（本地文件记录）

用途：

- 记录视频相关本地文件的真实落盘信息。
- 当前主流程主要使用 `type = 'video'` 的记录。

当前主要字段：

- `id`
- `video_id`
- `path`
- `size_bytes`
- `checksum`
- `type`
- `created_at`

说明：

- `path` 指向 `storage.root_dir/store/...` 下的真实文件。
- `library/` 中的符号链接不是 `video_files` 的真源。

### 2.4 jobs（后台任务）

用途：

- 承载调度器、手工运维入口和候选审核链路产生的后台任务。

当前主要字段：

- `id`
- `type`
- `status`
- `payload_json`
- `error_message`
- `not_before`
- `started_at`
- `finished_at`
- `created_at`
- `updated_at`

当前任务类型：

- `fetch`
- `download`
- `check`
- `cleanup`
- `discover`

当前任务状态：

- `queued`
- `running`
- `success`
- `failed`

说明：

- `discover` 不是初始 schema 自带，而是由 `00003_candidate_discovery.sql` 扩展
  进来的。
- `not_before` 当前已存在于表结构，但默认主链路尚未形成完整重试调度语义。

### 2.5 candidate_creators（候选博主）

用途：

- 保存自动发现链路产出的候选博主。
- 作为“正式追踪池”的上游补种池。

当前主要字段：

- `id`
- `platform`
- `uid`
- `name`
- `avatar_url`
- `profile_url`
- `follower_count`
- `status`
- `score`
- `score_version`
- `last_discovered_at`
- `last_scored_at`
- `approved_at`
- `ignored_at`
- `blocked_at`
- `created_at`
- `updated_at`

当前 schema 允许的状态值：

- `new`
- `reviewing`
- `approved`
- `ignored`
- `blocked`

当前业务主流程主要使用的状态值：

- `reviewing`
- `approved`
- `ignored`
- `blocked`

说明：

- `new` 目前主要是 schema 层保留值，当前自动发现实现默认写入的是
  `reviewing`。

### 2.6 candidate_creator_sources（候选来源）

用途：

- 保存候选是如何被发现的。
- 一个候选可以关联多条来源记录。

当前主要字段：

- `id`
- `candidate_creator_id`
- `source_type`
- `source_value`
- `source_label`
- `weight`
- `detail_json`
- `created_at`

当前常见来源类型：

- `keyword`
- `related_creator`

说明：

- 来源是候选明细页和评分解释的重要组成部分。
- `(candidate_creator_id, source_type, source_value)` 当前要求唯一。

### 2.7 candidate_creator_score_details（候选评分明细）

用途：

- 保存候选分数的拆解项，方便审核时解释“为什么给这个分”。

当前主要字段：

- `id`
- `candidate_creator_id`
- `factor_key`
- `factor_label`
- `score_delta`
- `detail_json`
- `created_at`

说明：

- 这张表不保存最终总分，只保存各评分因子的拆解结果。
- 最终总分仍存放在 `candidate_creators.score`。

## 3. 当前关系结构

### 3.1 主业务关系

- 一个 `creator` 可以有多个 `video`
- 一个 `video` 可以有多个 `video_file`
- 一个 `video_file` 当前通常对应一个真实落盘文件

### 3.2 候选池关系

- 一个 `candidate_creator` 可以有多条 `candidate_creator_source`
- 一个 `candidate_creator` 可以有多条 `candidate_creator_score_detail`
- `candidate_creator` 批准后会转化为正式 `creator`，但两者不是同表同记录

### 3.3 派生关系

- `library/` 浏览目录由 `creators + videos + video_files` 派生出来
- 前端控制台中的聚合指标由查询结果和 SSE 事件派生出来

这些派生结果都不是业务真源。

## 4. Go 模型与数据库字段的区别

`internal/repo/models.go` 中有一部分字段是查询投影，不是数据库表中的真实列。

典型例子：

- `repo.Creator.LocalVideoCount`
- `repo.Creator.StorageBytes`
- `repo.LibraryVideo`
- `repo.CleanupCandidate`

这些结构的作用分别是：

- 支撑 Dashboard / 列表聚合查询
- 支撑 `library/` 导出
- 支撑 cleanup 候选排序

因此，阅读 Go 模型时不能直接假设每个字段都在表里有同名列。

## 5. 当前仍在 schema 中，但未接入主流程的表

### 5.1 check_history

表结构仍存在，但当前主业务代码未围绕它建立持续写入和查询链路。

当前状态应理解为：

- schema 预留或历史残留表
- 不是当前检查链路的主事实源

### 5.2 storage_reports

表结构仍存在，但当前 cleanup 主流程没有把它作为主要结果记录表来使用。

当前状态应理解为：

- schema 预留或历史残留表
- 不是当前存储统计的主要来源

当前存储观察主要来自：

- 实时扫描 `store/`
- `videos` / `video_files`
- worker 发布的 `storage.changed`

## 6. 关键约束

### 6.1 真源约束

当前业务真源是：

- MySQL 表中的实体数据
- `store/` 下真实存在的文件

以下内容都不是业务真源：

- `library/` 浏览目录
- `_meta/*.json`
- 前端本地缓存
- SSE 事件本身

### 6.2 删除语义

- `creators.removed` 是逻辑删除态，不是物理删除
- `videos.DELETED` 表示本地文件被清理后的业务状态
- `video_files` 删除后，若该视频已无剩余文件，视频状态才会推进为 `DELETED`

### 6.3 一致性约束

- `video_files.path` 必须指向真实文件，而不是 `library/` 符号链接
- `videos.state = DOWNLOADED` 时，应有对应真实文件和 `video_files` 记录支撑
- 启动恢复会修复“状态存在但文件不存在”的残留异常

## 7. 索引与查询关注点

当前较关键的索引和查询方向包括：

- `creators(platform, uid)` 唯一约束
- `videos(platform, video_id)` 唯一约束
- `videos(creator_id, publish_time)` 支撑按博主时间查询
- `videos(state)` 支撑检查 / 清理 / 状态筛选
- `jobs(type, status)`、`jobs(status)`、`jobs(not_before)` 支撑任务拉取
- `candidate_creators(status, score)`、`last_discovered_at` 支撑候选池筛选与排
  序

## 8. 当前迁移注意事项

当前数据模型的一个重要注意点是：

- `00001_init.sql` 只定义了最初版本
- `00002_creator_removed_status.sql` 扩展了 `creators.status`
- `00003_candidate_discovery.sql` 扩展了 `jobs.type`，并新增候选池 3 张表

因此，任何需要判断“当前 schema 长什么样”的场景，都必须看完整迁移链路，
而不是只看首个初始化文件。
