# MySQL 表结构说明

说明：
- 当前权威 schema 来源为 `migrations/00001_init.sql`。
- 服务启动默认会自动执行内置迁移（`mysql.auto_migrate: true`）。
- 本文档用于说明当前表结构与索引，不再作为手工执行 SQL 的主入口。

## 1. creators（博主）
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

## 2. videos（视频）
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
  state ENUM('NEW','DOWNLOADING','DOWNLOADED','OUT_OF_PRINT','STABLE','DELETED') NOT NULL DEFAULT 'NEW',
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

## 3. video_files（视频文件）
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

## 4. jobs（任务）
```sql
CREATE TABLE jobs (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  type ENUM('fetch','download','check','cleanup') NOT NULL,
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

## 5. check_history（下架检查历史）
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

## 6. （可选）storage_reports（清理与容量记录）
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

## 7. 当前迁移策略
- 当前采用 `Goose + embed SQL`。
- 迁移文件目录：`migrations/`
- 启动阶段自动执行 `Up` 迁移，迁移状态由 Goose 版本表维护。
- 变更策略：
  - 新增字段：尽量可空并提供默认值。
  - 状态枚举扩展：优先兼容旧状态。
  - 大字段变更：尽量避免在线 ALTER 大表，必要时使用影子表。

## 8. 博主状态说明
- `active`：正常采集与展示。
- `paused`：已暂停采集，可通过配置文件或接口重新启用。
- `removed`：手工移除后的保留态，不在活跃列表展示，且不会被文件同步自动恢复。

## 9. 索引与性能备注
- `videos` 表的 `creator_id + publish_time` 支持按博主时间查询。
- `videos.state` 用于筛选下载/检查/清理任务。
- `jobs` 表索引支持任务调度器快速拉取待执行队列。
