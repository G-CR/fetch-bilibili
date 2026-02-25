package mysqlrepo

import (
	"context"
	"database/sql"
	"time"

	"fetch-bilibili/internal/repo"
)

func (r *videoRepo) Upsert(ctx context.Context, v repo.Video) (int64, bool, error) {
	platform := v.Platform
	if platform == "" {
		platform = "bilibili"
	}

	var publishTime any
	if !v.PublishTime.IsZero() {
		publishTime = v.PublishTime
	}

	state := v.State
	if state == "" {
		state = "NEW"
	}

	res, err := r.db.ExecContext(ctx, `
		INSERT INTO videos (
			platform, video_id, creator_id, title, description, publish_time, duration, cover_url,
			view_count, favorite_count, state, created_at, updated_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NOW(), NOW())
		ON DUPLICATE KEY UPDATE
			id = LAST_INSERT_ID(id),
			title = VALUES(title),
			description = VALUES(description),
			publish_time = VALUES(publish_time),
			duration = VALUES(duration),
			cover_url = VALUES(cover_url),
			view_count = VALUES(view_count),
			favorite_count = VALUES(favorite_count),
			updated_at = NOW()
	`, platform, v.VideoID, v.CreatorID, v.Title, v.Description, publishTime, v.Duration, v.CoverURL, v.ViewCount, v.FavoriteCount, state)
	if err != nil {
		return 0, false, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, false, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return id, false, nil
	}
	created := affected == 1
	return id, created, nil
}

func (r *videoRepo) UpdateState(ctx context.Context, id int64, state string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE videos
		SET state = ?, updated_at = NOW()
		WHERE id = ?
	`, state, id)
	return err
}

func (r *videoRepo) FindByID(ctx context.Context, id int64) (repo.Video, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, platform, video_id, creator_id, title, description, publish_time, duration, cover_url,
			view_count, favorite_count, state, out_of_print_at, stable_at, last_check_at, created_at, updated_at
		FROM videos
		WHERE id = ?
	`, id)

	var (
		v                         repo.Video
		description               sql.NullString
		publishTime, outOfPrintAt sql.NullTime
		stableAt, lastCheckAt     sql.NullTime
		createdAt, updatedAt      time.Time
	)
	if err := row.Scan(
		&v.ID,
		&v.Platform,
		&v.VideoID,
		&v.CreatorID,
		&v.Title,
		&description,
		&publishTime,
		&v.Duration,
		&v.CoverURL,
		&v.ViewCount,
		&v.FavoriteCount,
		&v.State,
		&outOfPrintAt,
		&stableAt,
		&lastCheckAt,
		&createdAt,
		&updatedAt,
	); err != nil {
		return repo.Video{}, err
	}
	if description.Valid {
		v.Description = description.String
	}
	if publishTime.Valid {
		v.PublishTime = publishTime.Time
	}
	if outOfPrintAt.Valid {
		v.OutOfPrintAt = outOfPrintAt.Time
	}
	if stableAt.Valid {
		v.StableAt = stableAt.Time
	}
	if lastCheckAt.Valid {
		v.LastCheckAt = lastCheckAt.Time
	}
	v.CreatedAt = createdAt
	v.UpdatedAt = updatedAt
	return v, nil
}

func (r *videoRepo) ListForCheck(ctx context.Context, limit int) ([]repo.Video, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT id, platform, video_id, creator_id, title, description, publish_time, duration, cover_url,
			view_count, favorite_count, state, out_of_print_at, stable_at, last_check_at, created_at, updated_at
		FROM videos
		WHERE state IN ('DOWNLOADED', 'STABLE')
		ORDER BY (last_check_at IS NULL) DESC, last_check_at ASC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []repo.Video
	for rows.Next() {
		var (
			v                         repo.Video
			description               sql.NullString
			publishTime, outOfPrintAt sql.NullTime
			stableAt, lastCheckAt     sql.NullTime
			createdAt, updatedAt      time.Time
		)
		if err := rows.Scan(
			&v.ID,
			&v.Platform,
			&v.VideoID,
			&v.CreatorID,
			&v.Title,
			&description,
			&publishTime,
			&v.Duration,
			&v.CoverURL,
			&v.ViewCount,
			&v.FavoriteCount,
			&v.State,
			&outOfPrintAt,
			&stableAt,
			&lastCheckAt,
			&createdAt,
			&updatedAt,
		); err != nil {
			return nil, err
		}
		if description.Valid {
			v.Description = description.String
		}
		if publishTime.Valid {
			v.PublishTime = publishTime.Time
		}
		if outOfPrintAt.Valid {
			v.OutOfPrintAt = outOfPrintAt.Time
		}
		if stableAt.Valid {
			v.StableAt = stableAt.Time
		}
		if lastCheckAt.Valid {
			v.LastCheckAt = lastCheckAt.Time
		}
		v.CreatedAt = createdAt
		v.UpdatedAt = updatedAt
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return out, nil
}

func (r *videoRepo) UpdateCheckStatus(ctx context.Context, id int64, state string, outOfPrintAt *time.Time, stableAt *time.Time, lastCheckAt time.Time) error {
	var out any
	var stable any
	if outOfPrintAt != nil {
		out = *outOfPrintAt
	}
	if stableAt != nil {
		stable = *stableAt
	}

	_, err := r.db.ExecContext(ctx, `
		UPDATE videos
		SET state = ?,
			out_of_print_at = COALESCE(?, out_of_print_at),
			stable_at = COALESCE(?, stable_at),
			last_check_at = ?,
			updated_at = NOW()
		WHERE id = ?
	`, state, out, stable, lastCheckAt, id)
	return err
}
