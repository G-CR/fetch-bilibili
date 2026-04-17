-- +goose Up
CREATE TABLE IF NOT EXISTS candidate_creators (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  platform VARCHAR(32) NOT NULL DEFAULT 'bilibili',
  uid VARCHAR(64) NOT NULL,
  name VARCHAR(255) DEFAULT NULL,
  avatar_url VARCHAR(1024) DEFAULT NULL,
  profile_url VARCHAR(1024) DEFAULT NULL,
  follower_count BIGINT UNSIGNED DEFAULT 0,
  status ENUM('new','reviewing','approved','ignored','blocked') NOT NULL DEFAULT 'new',
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

CREATE TABLE IF NOT EXISTS candidate_creator_sources (
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
  CONSTRAINT fk_candidate_creator_sources_candidate_id FOREIGN KEY (candidate_creator_id) REFERENCES candidate_creators(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS candidate_creator_score_details (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  candidate_creator_id BIGINT UNSIGNED NOT NULL,
  factor_key VARCHAR(64) NOT NULL,
  factor_label VARCHAR(255) NOT NULL,
  score_delta INT NOT NULL,
  detail_json JSON DEFAULT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  KEY idx_candidate_score_detail_candidate (candidate_creator_id),
  CONSTRAINT fk_candidate_creator_score_details_candidate_id FOREIGN KEY (candidate_creator_id) REFERENCES candidate_creators(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

ALTER TABLE jobs
  MODIFY COLUMN type ENUM('fetch','download','check','cleanup','discover') NOT NULL;

-- +goose Down
DELETE FROM jobs WHERE type = 'discover';

ALTER TABLE jobs
  MODIFY COLUMN type ENUM('fetch','download','check','cleanup') NOT NULL;

DROP TABLE IF EXISTS candidate_creator_score_details;
DROP TABLE IF EXISTS candidate_creator_sources;
DROP TABLE IF EXISTS candidate_creators;
