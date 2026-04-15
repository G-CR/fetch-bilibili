package mysqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"fetch-bilibili/internal/repo"
	gomysql "github.com/go-sql-driver/mysql"
)

func (r *creatorRepo) Upsert(ctx context.Context, c repo.Creator) (int64, error) {
	platform := c.Platform
	if platform == "" {
		platform = "bilibili"
	}
	status := c.Status
	if status == "" {
		status = "active"
	}

	res, err := r.db.ExecContext(ctx, `
		INSERT INTO creators (platform, uid, name, follower_count, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, NOW(), NOW())
		ON DUPLICATE KEY UPDATE
			name = IF(VALUES(name) = '', name, VALUES(name)),
			follower_count = VALUES(follower_count),
			status = VALUES(status),
			updated_at = NOW(),
			id = LAST_INSERT_ID(id)
	`, platform, c.UID, c.Name, c.FollowerCount, status)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (r *creatorRepo) Create(ctx context.Context, c repo.Creator) (int64, error) {
	platform := c.Platform
	if platform == "" {
		platform = "bilibili"
	}
	status := c.Status
	if status == "" {
		status = "active"
	}

	res, err := r.db.ExecContext(ctx, `
		INSERT INTO creators (platform, uid, name, follower_count, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, NOW(), NOW())
	`, platform, c.UID, c.Name, c.FollowerCount, status)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (r *creatorRepo) Update(ctx context.Context, c repo.Creator) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE creators
		SET name = ?, follower_count = ?, status = ?, updated_at = NOW()
		WHERE id = ?
	`, c.Name, c.FollowerCount, c.Status, c.ID)
	return err
}

func (r *creatorRepo) UpdateStatus(ctx context.Context, id int64, status string) error {
	if status == "" {
		status = "active"
	}
	_, err := r.db.ExecContext(ctx, `
		UPDATE creators
		SET status = ?, updated_at = NOW()
		WHERE id = ?
	`, status, id)
	return err
}

func (r *creatorRepo) DeleteByID(ctx context.Context, id int64) (int64, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM creators WHERE id = ?`, id)
	if err != nil {
		var mysqlErr *gomysql.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1451 {
			return 0, repo.ErrConflict
		}
		return 0, err
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return affected, nil
}

func (r *creatorRepo) FindByID(ctx context.Context, id int64) (repo.Creator, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, platform, uid, name, follower_count, status, created_at, updated_at
		FROM creators
		WHERE id = ?
	`, id)

	var c repo.Creator
	var name sql.NullString
	var createdAt, updatedAt time.Time
	if err := row.Scan(&c.ID, &c.Platform, &c.UID, &name, &c.FollowerCount, &c.Status, &createdAt, &updatedAt); err != nil {
		return repo.Creator{}, err
	}
	if name.Valid {
		c.Name = name.String
	}
	c.CreatedAt = createdAt
	c.UpdatedAt = updatedAt
	return c, nil
}

func (r *creatorRepo) FindByPlatformUID(ctx context.Context, platform, uid string) (repo.Creator, error) {
	if platform == "" {
		platform = "bilibili"
	}

	row := r.db.QueryRowContext(ctx, `
		SELECT id, platform, uid, name, follower_count, status, created_at, updated_at
		FROM creators
		WHERE platform = ? AND uid = ?
	`, platform, uid)

	var c repo.Creator
	var name sql.NullString
	var createdAt, updatedAt time.Time
	if err := row.Scan(&c.ID, &c.Platform, &c.UID, &name, &c.FollowerCount, &c.Status, &createdAt, &updatedAt); err != nil {
		return repo.Creator{}, err
	}
	if name.Valid {
		c.Name = name.String
	}
	c.CreatedAt = createdAt
	c.UpdatedAt = updatedAt
	return c, nil
}

func (r *creatorRepo) ListActive(ctx context.Context, limit int) ([]repo.Creator, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT id, platform, uid, name, follower_count, status, created_at, updated_at
		FROM creators
		WHERE status = 'active'
		ORDER BY id ASC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []repo.Creator
	for rows.Next() {
		var c repo.Creator
		var name sql.NullString
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&c.ID, &c.Platform, &c.UID, &name, &c.FollowerCount, &c.Status, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		if name.Valid {
			c.Name = name.String
		}
		c.CreatedAt = createdAt
		c.UpdatedAt = updatedAt
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *creatorRepo) ListActiveAfter(ctx context.Context, lastID int64, limit int) ([]repo.Creator, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT id, platform, uid, name, follower_count, status, created_at, updated_at
		FROM creators
		WHERE status = 'active' AND id > ?
		ORDER BY id ASC
		LIMIT ?
	`, lastID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []repo.Creator
	for rows.Next() {
		var c repo.Creator
		var name sql.NullString
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&c.ID, &c.Platform, &c.UID, &name, &c.FollowerCount, &c.Status, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		if name.Valid {
			c.Name = name.String
		}
		c.CreatedAt = createdAt
		c.UpdatedAt = updatedAt
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *creatorRepo) ListForLibraryAfter(ctx context.Context, lastID int64, limit int) ([]repo.Creator, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT DISTINCT c.id, c.platform, c.uid, c.name, c.follower_count, c.status, c.created_at, c.updated_at
		FROM creators c
		INNER JOIN videos v ON v.creator_id = c.id
		INNER JOIN video_files vf ON vf.video_id = v.id AND vf.type = 'video'
		WHERE c.id > ?
			AND v.state IN ('DOWNLOADED', 'STABLE', 'OUT_OF_PRINT')
		ORDER BY c.id ASC
		LIMIT ?
	`, lastID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []repo.Creator
	for rows.Next() {
		var c repo.Creator
		var name sql.NullString
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&c.ID, &c.Platform, &c.UID, &name, &c.FollowerCount, &c.Status, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		if name.Valid {
			c.Name = name.String
		}
		c.CreatedAt = createdAt
		c.UpdatedAt = updatedAt
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *creatorRepo) CountActive(ctx context.Context) (int64, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM creators WHERE status = 'active'
	`)

	var count int64
	if err := row.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}
