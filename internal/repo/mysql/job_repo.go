package mysqlrepo

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
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
			finished_at = CASE WHEN ? IN ('success','failed') THEN NOW() ELSE finished_at END,
			updated_at = NOW()
		WHERE id = ?
	`, status, msg, status, id)
	return err
}
