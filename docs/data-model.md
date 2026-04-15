# 数据模型设计

## 1. 实体列表
### 1.1 creators（博主）
- id (pk)
- platform (default: bilibili)
- uid (唯一)
- name
- follower_count
- status (active/paused)
- created_at, updated_at

### 1.2 videos（视频）
- id (pk)
- platform
- video_id (平台视频唯一标识)
- creator_id (fk)
- title
- description
- publish_time
- duration
- cover_url
- view_count
- favorite_count
- state (NEW/DOWNLOADING/DOWNLOADED/OUT_OF_PRINT/STABLE/DELETED)
- out_of_print_at
- stable_at
- last_check_at
- created_at, updated_at

### 1.3 video_files（视频文件）
- id (pk)
- video_id (fk)
- path（真实文件路径，指向 `storage.root_dir/store/...`）
- size_bytes
- checksum
- type (video/cover/subtitle/other)
- created_at

### 1.4 jobs（任务）
- id (pk)
- type (fetch/download/check/cleanup)
- status (queued/running/success/failed)
- payload_json
- error_message
- not_before
- started_at, finished_at
- created_at, updated_at

### 1.5 check_history（下架检查历史）
- id (pk)
- video_id (fk)
- checked_at
- result (available/unavailable/error)
- detail_json

### 1.6 storage_reports（清理报告）
- id (pk)
- total_bytes
- freed_bytes
- deleted_count
- created_at

## 2. 状态机与规则
- NEW：发现但未下载。
- DOWNLOADING：正在下载。
- DOWNLOADED：已下载且可访问。
- OUT_OF_PRINT：检测到不可访问。
- STABLE：超过阈值仍可访问。
- DELETED：因清理被删除。

状态转移应具备幂等性，避免重复写入导致异常。

## 3. 索引建议
- creators: (platform, uid) unique
- videos: (platform, video_id) unique
- videos: (creator_id, publish_time)
- videos: (state)
- check_history: (video_id, checked_at)

## 4. 存储与投影约束
- 数据库和 `store/` 主存储是真实业务状态。
- `video_files.path` 必须指向 `store/` 中的真实文件，而不是 `library/` 中的符号链接。
- `library/` 浏览目录不单独建表，它完全由数据库和 `store/` 派生出来。
- `library/` 中每个博主目录会实时生成：
  - `_meta/creator.json`
  - `_meta/index.json`
  - `videos/` 普通库存链接
  - `rare/` 绝版库存链接
- cleanup 删除真实文件后，投影层会同步移除对应符号链接和索引记录。
