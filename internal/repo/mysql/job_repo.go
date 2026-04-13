package mysqlrepo

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"fetch-bilibili/internal/jobs"
	"fetch-bilibili/internal/repo"
)

func (r *jobRepo) Enqueue(ctx context.Context, job repo.Job) (int64, error) {
	status := job.Status
	if status == "" {
		status = jobs.StatusQueued
	}

	var payload []byte
	var err error
	if job.Payload != nil {
		payload, err = json.Marshal(job.Payload)
		if err != nil {
			return 0, err
		}
	}
	if status == jobs.StatusQueued || status == jobs.StatusRunning {
		exists, err := r.hasActiveJob(ctx, job.Type, job.Payload)
		if err != nil {
			return 0, err
		}
		if exists {
			return 0, jobs.ErrJobAlreadyActive
		}
	}

	var notBefore any
	if !job.NotBefore.IsZero() {
		notBefore = job.NotBefore
	}

	res, err := r.db.ExecContext(ctx, `
		INSERT INTO jobs (type, status, payload_json, not_before, created_at, updated_at)
		VALUES (?, ?, ?, ?, NOW(), NOW())
	`, job.Type, status, payload, notBefore)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (r *jobRepo) hasActiveJob(ctx context.Context, jobType string, payload map[string]any) (bool, error) {
	query := "SELECT COUNT(*) FROM jobs WHERE type = ? AND status IN (?,?)"
	args := []any{jobType, jobs.StatusQueued, jobs.StatusRunning}
	if jobType == jobs.TypeDownload {
		if videoID, ok := payloadInt64(payload, "video_id"); ok && videoID > 0 {
			query += " AND CAST(JSON_UNQUOTE(JSON_EXTRACT(payload_json, '$.video_id')) AS UNSIGNED) = ?"
			args = append(args, videoID)
		}
	}

	row := r.db.QueryRowContext(ctx, query, args...)
	var count int64
	if err := row.Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *jobRepo) FetchQueued(ctx context.Context, limit int) ([]repo.Job, error) {
	if limit <= 0 {
		limit = 10
	}

	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, err
	}

	rows, err := tx.QueryContext(ctx, `
		SELECT id, type, status, payload_json, error_message, not_before, started_at, finished_at, created_at, updated_at
		FROM jobs
		WHERE status = ? AND (not_before IS NULL OR not_before <= NOW())
		ORDER BY id ASC
		LIMIT ?
		FOR UPDATE SKIP LOCKED
	`, jobs.StatusQueued, limit)
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	defer rows.Close()

	var out []repo.Job
	for rows.Next() {
		var (
			job                              repo.Job
			payload                          []byte
			notBefore, startedAt, finishedAt sql.NullTime
			createdAt, updatedAt             time.Time
			errMsg                           sql.NullString
		)
		if err := rows.Scan(&job.ID, &job.Type, &job.Status, &payload, &errMsg, &notBefore, &startedAt, &finishedAt, &createdAt, &updatedAt); err != nil {
			_ = tx.Rollback()
			return nil, err
		}
		job.CreatedAt = createdAt
		job.UpdatedAt = updatedAt
		if errMsg.Valid {
			job.ErrorMsg = errMsg.String
		}
		if notBefore.Valid {
			job.NotBefore = notBefore.Time
		}
		if startedAt.Valid {
			job.StartedAt = startedAt.Time
		}
		if finishedAt.Valid {
			job.FinishedAt = finishedAt.Time
		}
		if len(payload) > 0 {
			var m map[string]any
			if err := json.Unmarshal(payload, &m); err != nil {
				_ = tx.Rollback()
				return nil, err
			}
			job.Payload = m
		}
		out = append(out, job)
	}
	if err := rows.Err(); err != nil {
		_ = tx.Rollback()
		return nil, err
	}

	if len(out) == 0 {
		return out, tx.Commit()
	}

	ids := make([]string, 0, len(out))
	args := make([]any, 0, len(out))
	for _, job := range out {
		ids = append(ids, "?")
		args = append(args, job.ID)
	}
	query := fmt.Sprintf(
		"UPDATE jobs SET status = '%s', started_at = NOW(), updated_at = NOW() WHERE id IN (%s)",
		jobs.StatusRunning,
		strings.Join(ids, ","),
	)
	if _, err := tx.ExecContext(ctx, query, args...); err != nil {
		_ = tx.Rollback()
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	for i := range out {
		out[i].Status = jobs.StatusRunning
		out[i].StartedAt = time.Now()
	}
	return out, nil
}

func (r *jobRepo) UpdateStatus(ctx context.Context, id int64, status string, errMsg string) error {
	var msg any
	if errMsg != "" {
		msg = errMsg
	}

	_, err := r.db.ExecContext(ctx, `
		UPDATE jobs
		SET status = ?,
			error_message = ?,
			started_at = CASE WHEN ? = 'queued' THEN NULL ELSE started_at END,
			finished_at = CASE
				WHEN ? = 'queued' THEN NULL
				WHEN ? IN ('success','failed') THEN NOW()
				ELSE finished_at
			END,
			updated_at = NOW()
		WHERE id = ?
	`, status, msg, status, status, status, id)
	return err
}

func (r *jobRepo) ListRecent(ctx context.Context, filter repo.JobListFilter) ([]repo.Job, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}

	conditions := make([]string, 0, 2)
	args := make([]any, 0, 3)
	if filter.Status != "" {
		conditions = append(conditions, "status = ?")
		args = append(args, filter.Status)
	}
	if filter.Type != "" {
		conditions = append(conditions, "type = ?")
		args = append(args, filter.Type)
	}

	query := `
		SELECT id, type, status, payload_json, error_message, not_before, started_at, finished_at, created_at, updated_at
		FROM jobs
	`
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY id DESC LIMIT ?"
	args = append(args, limit)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []repo.Job
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, job)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *jobRepo) CountByStatuses(ctx context.Context, statuses []string) (int64, error) {
	if len(statuses) == 0 {
		return 0, nil
	}

	placeholders := make([]string, 0, len(statuses))
	args := make([]any, 0, len(statuses))
	for _, status := range statuses {
		placeholders = append(placeholders, "?")
		args = append(args, status)
	}

	query := fmt.Sprintf(
		"SELECT COUNT(*) FROM jobs WHERE status IN (%s)",
		strings.Join(placeholders, ","),
	)
	row := r.db.QueryRowContext(ctx, query, args...)

	var count int64
	if err := row.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func scanJob(scanner interface {
	Scan(dest ...any) error
}) (repo.Job, error) {
	var (
		job                              repo.Job
		payload                          []byte
		notBefore, startedAt, finishedAt sql.NullTime
		createdAt, updatedAt             time.Time
		errMsg                           sql.NullString
	)
	if err := scanner.Scan(&job.ID, &job.Type, &job.Status, &payload, &errMsg, &notBefore, &startedAt, &finishedAt, &createdAt, &updatedAt); err != nil {
		return repo.Job{}, err
	}
	job.CreatedAt = createdAt
	job.UpdatedAt = updatedAt
	if errMsg.Valid {
		job.ErrorMsg = errMsg.String
	}
	if notBefore.Valid {
		job.NotBefore = notBefore.Time
	}
	if startedAt.Valid {
		job.StartedAt = startedAt.Time
	}
	if finishedAt.Valid {
		job.FinishedAt = finishedAt.Time
	}
	if len(payload) > 0 {
		var m map[string]any
		if err := json.Unmarshal(payload, &m); err != nil {
			return repo.Job{}, err
		}
		job.Payload = m
	}
	return job, nil
}

func payloadInt64(payload map[string]any, key string) (int64, bool) {
	if payload == nil {
		return 0, false
	}
	raw, ok := payload[key]
	if !ok {
		return 0, false
	}
	switch value := raw.(type) {
	case int64:
		return value, true
	case int:
		return int64(value), true
	case float64:
		return int64(value), true
	case json.Number:
		n, err := value.Int64()
		return n, err == nil
	case string:
		n, err := strconv.ParseInt(value, 10, 64)
		return n, err == nil
	default:
		return 0, false
	}
}
