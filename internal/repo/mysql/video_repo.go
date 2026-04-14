package mysqlrepo

import (
	"context"
	"database/sql"
	"strings"
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

func (r *videoRepo) ListRecent(ctx context.Context, filter repo.VideoListFilter) ([]repo.Video, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}

	conditions := make([]string, 0, 2)
	args := make([]any, 0, 3)
	if filter.CreatorID > 0 {
		conditions = append(conditions, "creator_id = ?")
		args = append(args, filter.CreatorID)
	}
	if filter.State != "" {
		conditions = append(conditions, "state = ?")
		args = append(args, filter.State)
	}

	query := `
		SELECT id, platform, video_id, creator_id, title, description, publish_time, duration, cover_url,
			view_count, favorite_count, state, out_of_print_at, stable_at, last_check_at, created_at, updated_at
		FROM videos
	`
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY publish_time DESC, id DESC LIMIT ?"
	args = append(args, limit)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []repo.Video
	for rows.Next() {
		video, err := scanVideo(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, video)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *videoRepo) ListCleanupCandidates(ctx context.Context, filter repo.CleanupCandidateFilter) ([]repo.CleanupCandidate, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 200
	}

	query := `
		SELECT
			v.id AS video_id,
			v.video_id AS source_video_id,
			v.platform,
			v.title,
			v.state,
			c.id AS creator_id,
			c.name AS creator_name,
			c.follower_count,
			v.view_count,
			v.favorite_count,
			vf.id AS file_id,
			vf.path AS file_path,
			vf.size_bytes AS file_size_bytes,
			vf.created_at AS file_created_at
		FROM videos v
		INNER JOIN creators c ON c.id = v.creator_id
		INNER JOIN video_files vf ON vf.video_id = v.id
		WHERE vf.type = 'video'
			AND v.state IN ('DOWNLOADED', 'STABLE', 'OUT_OF_PRINT')
	`
	if !filter.IncludeOutOfPrint {
		query += " AND v.state <> 'OUT_OF_PRINT'"
	}
	query += `
		ORDER BY vf.created_at ASC, vf.id ASC
		LIMIT ?
	`

	rows, err := r.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []repo.CleanupCandidate
	for rows.Next() {
		var item repo.CleanupCandidate
		var creatorName sql.NullString
		if err := rows.Scan(
			&item.VideoID,
			&item.SourceVideoID,
			&item.Platform,
			&item.Title,
			&item.State,
			&item.CreatorID,
			&creatorName,
			&item.CreatorFollowerCount,
			&item.ViewCount,
			&item.FavoriteCount,
			&item.FileID,
			&item.FilePath,
			&item.FileSizeBytes,
			&item.FileCreatedAt,
		); err != nil {
			return nil, err
		}
		if creatorName.Valid {
			item.CreatorName = creatorName.String
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *videoRepo) CountByState(ctx context.Context, state string) (int64, error) {
	query := "SELECT COUNT(*) FROM videos"
	args := make([]any, 0, 1)
	if state != "" {
		query += " WHERE state = ?"
		args = append(args, state)
	}

	row := r.db.QueryRowContext(ctx, query, args...)
	var count int64
	if err := row.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func scanVideo(scanner interface {
	Scan(dest ...any) error
}) (repo.Video, error) {
	var (
		v                         repo.Video
		description               sql.NullString
		publishTime, outOfPrintAt sql.NullTime
		stableAt, lastCheckAt     sql.NullTime
		createdAt, updatedAt      time.Time
	)
	if err := scanner.Scan(
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
