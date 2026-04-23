# MySQL 表结构说明（当前有效 schema）

说明：

- 当前 MySQL schema 的事实源不是单个初始化文件，而是完整迁移链：
  - `migrations/00001_init.sql`
  - `migrations/00002_creator_removed_status.sql`
  - `migrations/00003_candidate_discovery.sql`
- 服务启动默认会自动执行内置迁移（`mysql.auto_migrate: true`）。
- 本文档描述的是“按顺序执行完当前迁移后的有效 schema”，不是某一个迁移文件
  的原始内容。

## 1. 迁移链概览

### 1.1 `00001_init.sql`

初始化以下基础表：

- `creators`
- `videos`
- `video_files`
- `jobs`
- `check_history`
- `storage_reports`

### 1.2 `00002_creator_removed_status.sql`

扩展 `creators.status`，把：

- `ENUM('active','paused')`

升级为：

- `ENUM('active','paused','removed')`

### 1.3 `00003_candidate_discovery.sql`

新增候选池相关表：

- `candidate_creators`
- `candidate_creator_sources`
- `candidate_creator_score_details`

同时扩展 `jobs.type`，把：

- `ENUM('fetch','download','check','cleanup')`

升级为：

- `ENUM('fetch','download','check','cleanup','discover')`

## 2. 当前有效表结构

## 2.1 creators（正式追踪博主）

```sql
CREATE TABLE creators (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  platform VARCHAR(32) NOT NULL DEFAULT 'bilibili',
  uid VARCHAR(64) NOT NULL,
  name VARCHAR(255) DEFAULT NULL,
  follower_count BIGINT UNSIGNED DEFAULT 0,
  status ENUM('active','paused','removed') NOT NULL DEFAULT 'active',
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uk_platform_uid (platform, uid),
  KEY idx_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

说明：

- `removed` 由 `00002` 引入。
- `(platform, uid)` 是唯一键。

## 2.2 videos（视频）

```sql
CREATE TABLE videos (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  platform VARCHAR(32) NOT NULL DEFAULT 'bilibili',
  video_id VARCHAR(64) NOT NULL,
  creator_id BIGINT UNSIGNED NOT NULL,
  title VARCHAR(512) NOT NULL,
  description MEDIUMTEXT,
  publish_time DATETIME DEFAULT NULL,
  duration INT UNSIGNED DEFAULT 0,
  cover_url VARCHAR(1024) DEFAULT NULL,
  view_count BIGINT UNSIGNED DEFAULT 0,
  favorite_count BIGINT UNSIGNED DEFAULT 0,
  state ENUM(
    'NEW','DOWNLOADING','DOWNLOADED','OUT_OF_PRINT','STABLE','DELETED'
  ) NOT NULL DEFAULT 'NEW',
  out_of_print_at DATETIME DEFAULT NULL,
  stable_at DATETIME DEFAULT NULL,
  last_check_at DATETIME DEFAULT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uk_platform_video (platform, video_id),
  KEY idx_creator_publish (creator_id, publish_time),
  KEY idx_state (state),
  CONSTRAINT fk_videos_creator_id FOREIGN KEY (creator_id) REFERENCES creators(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

说明：

- `(platform, video_id)` 是唯一键。
- `creator_id` 外键指向 `creators.id`。

## 2.3 video_files（本地文件记录）

```sql
CREATE TABLE video_files (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  video_id BIGINT UNSIGNED NOT NULL,
  path VARCHAR(1024) NOT NULL,
  size_bytes BIGINT UNSIGNED NOT NULL,
  checksum VARCHAR(128) DEFAULT NULL,
  type ENUM('video','cover','subtitle','other') NOT NULL DEFAULT 'video',
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  KEY idx_video_id (video_id),
  CONSTRAINT fk_video_files_video_id FOREIGN KEY (video_id) REFERENCES videos(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

说明：

- `path` 指向真实文件路径。
- 当前主流程主要依赖 `type = 'video'`。

## 2.4 jobs（后台任务）

```sql
CREATE TABLE jobs (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  type ENUM('fetch','download','check','cleanup','discover') NOT NULL,
  status ENUM('queued','running','success','failed') NOT NULL DEFAULT 'queued',
  payload_json JSON DEFAULT NULL,
  error_message TEXT,
  not_before DATETIME DEFAULT NULL,
  started_at DATETIME DEFAULT NULL,
  finished_at DATETIME DEFAULT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  KEY idx_type_status (type, status),
  KEY idx_status (status),
  KEY idx_not_before (not_before)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

说明：

- `discover` 由 `00003` 引入。
- `payload_json` 用于承载 `video_id`、`creator_id` 等附加参数。
- `idx_not_before` 当前已存在，但默认链路尚未形成完整重试调度体系。

## 2.5 candidate_creators（候选博主）

```sql
CREATE TABLE candidate_creators (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  platform VARCHAR(32) NOT NULL DEFAULT 'bilibili',
  uid VARCHAR(64) NOT NULL,
  name VARCHAR(255) DEFAULT NULL,
  avatar_url VARCHAR(1024) DEFAULT NULL,
  profile_url VARCHAR(1024) DEFAULT NULL,
  follower_count BIGINT UNSIGNED DEFAULT 0,
  status ENUM(
    'new','reviewing','approved','ignored','blocked'
  ) NOT NULL DEFAULT 'new',
  score INT NOT NULL DEFAULT 0,
  score_version VARCHAR(32) NOT NULL DEFAULT 'v1',
  last_discovered_at DATETIME DEFAULT NULL,
  last_scored_at DATETIME DEFAULT NULL,
  approved_at DATETIME DEFAULT NULL,
  ignored_at DATETIME DEFAULT NULL,
  blocked_at DATETIME DEFAULT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uk_candidate_platform_uid (platform, uid),
  KEY idx_candidate_status_score (status, score),
  KEY idx_candidate_last_discovered (last_discovered_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

说明：

- 候选池 3 张表都由 `00003` 引入。
- schema 允许 `new`，但当前业务主流程主要使用 `reviewing` 作为新增候选的实
  际工作态。

## 2.6 candidate_creator_sources（候选来源）

```sql
CREATE TABLE candidate_creator_sources (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  candidate_creator_id BIGINT UNSIGNED NOT NULL,
  source_type VARCHAR(32) NOT NULL,
  source_value VARCHAR(255) NOT NULL,
  source_label VARCHAR(255) DEFAULT NULL,
  weight INT NOT NULL DEFAULT 0,
  detail_json JSON DEFAULT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uk_candidate_source (candidate_creator_id, source_type, source_value),
  KEY idx_candidate_source (candidate_creator_id),
  KEY idx_candidate_source_type_value (source_type, source_value),
  CONSTRAINT fk_candidate_creator_sources_candidate_id
    FOREIGN KEY (candidate_creator_id) REFERENCES candidate_creators(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

说明：

- 用于保存 `keyword`、`related_creator` 等发现来源。
- `(candidate_creator_id, source_type, source_value)` 保证来源去重。

## 2.7 candidate_creator_score_details（候选评分明细）

```sql
CREATE TABLE candidate_creator_score_details (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  candidate_creator_id BIGINT UNSIGNED NOT NULL,
  factor_key VARCHAR(64) NOT NULL,
  factor_label VARCHAR(255) NOT NULL,
  score_delta INT NOT NULL,
  detail_json JSON DEFAULT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  KEY idx_candidate_score_detail_candidate (candidate_creator_id),
  CONSTRAINT fk_candidate_creator_score_details_candidate_id
    FOREIGN KEY (candidate_creator_id) REFERENCES candidate_creators(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

说明：

- 保存候选总分的拆解项，不直接保存总分本身。

## 2.8 check_history（检查历史）

```sql
CREATE TABLE check_history (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  video_id BIGINT UNSIGNED NOT NULL,
  checked_at DATETIME NOT NULL,
  result ENUM('available','unavailable','error') NOT NULL,
  detail_json JSON DEFAULT NULL,
  PRIMARY KEY (id),
  KEY idx_video_checked (video_id, checked_at),
  CONSTRAINT fk_check_history_video_id FOREIGN KEY (video_id) REFERENCES videos(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

说明：

- 当前表结构仍存在，但主业务代码未围绕它建立持续写入 / 查询闭环。

## 2.9 storage_reports（清理与容量记录）

```sql
CREATE TABLE storage_reports (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  total_bytes BIGINT UNSIGNED NOT NULL,
  freed_bytes BIGINT UNSIGNED NOT NULL,
  deleted_count INT UNSIGNED NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

说明：

- 当前表结构仍存在，但 cleanup 主流程未把它作为主要结果记录表使用。

## 3. 当前关键索引

- `creators.uk_platform_uid`
- `creators.idx_status`
- `videos.uk_platform_video`
- `videos.idx_creator_publish`
- `videos.idx_state`
- `video_files.idx_video_id`
- `jobs.idx_type_status`
- `jobs.idx_status`
- `jobs.idx_not_before`
- `candidate_creators.uk_candidate_platform_uid`
- `candidate_creators.idx_candidate_status_score`
- `candidate_creators.idx_candidate_last_discovered`
- `candidate_creator_sources.uk_candidate_source`
- `candidate_creator_sources.idx_candidate_source`
- `candidate_creator_sources.idx_candidate_source_type_value`
- `candidate_creator_score_details.idx_candidate_score_detail_candidate`
- `check_history.idx_video_checked`

## 4. 当前约束与注意事项

### 4.1 不要把 `00001_init.sql` 当成唯一权威来源

当前文档最容易出错的地方，就是只看初始化文件就判断“当前 schema 长什么样”。

实际判断规则应是：

- 先看 `00001` 建了什么
- 再看 `00002` 怎么改 `creators.status`
- 再看 `00003` 怎么扩 `jobs.type` 并新增候选池表

### 4.2 表存在，不等于主流程正在使用

当前仓库里：

- `check_history`
- `storage_reports`

都仍存在于 schema 中，但这不等于它们是当前业务链路的主事实源。

### 4.3 schema 枚举与业务主流程不一定完全等价

例如：

- `candidate_creators.status` 的 schema 允许 `new`
- 但当前自动发现链路默认写入的是 `reviewing`

因此，阅读 schema 时要区分：

- 数据库允许哪些值
- 当前业务主流程主要实际写哪些值

## 5. 迁移策略说明

- 当前采用 `Goose + embed SQL`。
- 启动阶段默认执行 `Up` 迁移，迁移状态由 Goose 版本表维护。
- 新增字段优先可空或带默认值，降低在线变更风险。
- 扩展状态枚举时，应优先保证旧数据兼容。
- 大表结构调整应谨慎评估，不应只依赖“直接 ALTER 一把过”的假设。
