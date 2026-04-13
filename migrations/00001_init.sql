-- +goose Up
CREATE TABLE IF NOT EXISTS creators (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  platform VARCHAR(32) NOT NULL DEFAULT 'bilibili',
  uid VARCHAR(64) NOT NULL,
  name VARCHAR(255) DEFAULT NULL,
  follower_count BIGINT UNSIGNED DEFAULT 0,
  status ENUM('active','paused') NOT NULL DEFAULT 'active',
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uk_platform_uid (platform, uid),
  KEY idx_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS videos (
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

CREATE TABLE IF NOT EXISTS video_files (
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

CREATE TABLE IF NOT EXISTS jobs (
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

CREATE TABLE IF NOT EXISTS check_history (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  video_id BIGINT UNSIGNED NOT NULL,
  checked_at DATETIME NOT NULL,
  result ENUM('available','unavailable','error') NOT NULL,
  detail_json JSON DEFAULT NULL,
  PRIMARY KEY (id),
  KEY idx_video_checked (video_id, checked_at),
  CONSTRAINT fk_check_history_video_id FOREIGN KEY (video_id) REFERENCES videos(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS storage_reports (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  total_bytes BIGINT UNSIGNED NOT NULL,
  freed_bytes BIGINT UNSIGNED NOT NULL,
  deleted_count INT UNSIGNED NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- +goose Down
DROP TABLE IF EXISTS storage_reports;
DROP TABLE IF EXISTS check_history;
DROP TABLE IF EXISTS jobs;
DROP TABLE IF EXISTS video_files;
DROP TABLE IF EXISTS videos;
DROP TABLE IF EXISTS creators;
