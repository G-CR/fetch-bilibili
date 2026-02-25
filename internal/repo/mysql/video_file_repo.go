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
