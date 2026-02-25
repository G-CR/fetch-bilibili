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
- stats_json (其他统计，JSON)
- state (NEW/DOWNLOADING/DOWNLOADED/OUT_OF_PRINT/STABLE/DELETED)
- out_of_print_at
- stable_at
- last_check_at
- created_at, updated_at

### 1.3 video_files（视频文件）
- id (pk)
- video_id (fk)
- path
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

### 1.5 check_history（下架检查历史）
- id (pk)
- video_id (fk)
- checked_at
- result (available/unavailable/error)
- detail_json

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
