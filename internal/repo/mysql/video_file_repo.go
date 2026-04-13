package mysqlrepo

import (
	"context"

	"fetch-bilibili/internal/repo"
)

func (r *videoFileRepo) Create(ctx context.Context, f repo.VideoFile) (int64, error) {
	fileType := f.Type
	if fileType == "" {
		fileType = "video"
	}

	res, err := r.db.ExecContext(ctx, `
		INSERT INTO video_files (video_id, path, size_bytes, checksum, type, created_at)
		VALUES (?, ?, ?, ?, ?, NOW())
	`, f.VideoID, f.Path, f.SizeBytes, f.Checksum, fileType)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (r *videoFileRepo) DeleteByID(ctx context.Context, id int64) (int64, error) {
	res, err := r.db.ExecContext(ctx, `
		DELETE FROM video_files
		WHERE id = ?
	`, id)
	if err != nil {
		return 0, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return affected, nil
}

func (r *videoFileRepo) DeleteByVideoID(ctx context.Context, videoID int64) (int64, error) {
	res, err := r.db.ExecContext(ctx, `
		DELETE FROM video_files
		WHERE video_id = ?
	`, videoID)
	if err != nil {
		return 0, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return affected, nil
}

func (r *videoFileRepo) CountByVideoID(ctx context.Context, videoID int64) (int64, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM video_files WHERE video_id = ?
	`, videoID)

	var count int64
	if err := row.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}
