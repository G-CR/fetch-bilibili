-- +goose Up
ALTER TABLE creators
  MODIFY COLUMN status ENUM('active','paused','removed') NOT NULL DEFAULT 'active';

-- +goose Down
UPDATE creators SET status = 'paused' WHERE status = 'removed';
ALTER TABLE creators
  MODIFY COLUMN status ENUM('active','paused') NOT NULL DEFAULT 'active';
