package mysqlrepo

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"fetch-bilibili/internal/repo"
)

func (r *candidateRepo) Upsert(ctx context.Context, candidate repo.CandidateCreator) (repo.CandidateCreator, error) {
	platform := strings.TrimSpace(candidate.Platform)
	if platform == "" {
		platform = "bilibili"
	}
	status := strings.TrimSpace(candidate.Status)
	if status == "" {
		status = "new"
	}
	scoreVersion := strings.TrimSpace(candidate.ScoreVersion)
	if scoreVersion == "" {
		scoreVersion = "v1"
	}

	res, err := r.db.ExecContext(ctx, `
		INSERT INTO candidate_creators (
			platform, uid, name, avatar_url, profile_url, follower_count, status, score, score_version,
			last_discovered_at, last_scored_at, approved_at, ignored_at, blocked_at, created_at, updated_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NOW(), NOW())
		ON DUPLICATE KEY UPDATE
			name = IF(VALUES(name) = '', name, VALUES(name)),
			avatar_url = IF(VALUES(avatar_url) = '', avatar_url, VALUES(avatar_url)),
			profile_url = IF(VALUES(profile_url) = '', profile_url, VALUES(profile_url)),
			follower_count = VALUES(follower_count),
			status = VALUES(status),
			score = VALUES(score),
			score_version = VALUES(score_version),
			last_discovered_at = COALESCE(VALUES(last_discovered_at), last_discovered_at),
			last_scored_at = COALESCE(VALUES(last_scored_at), last_scored_at),
			approved_at = COALESCE(VALUES(approved_at), approved_at),
			ignored_at = COALESCE(VALUES(ignored_at), ignored_at),
			blocked_at = COALESCE(VALUES(blocked_at), blocked_at),
			updated_at = NOW(),
			id = LAST_INSERT_ID(id)
	`, platform, candidate.UID, candidate.Name, candidate.AvatarURL, candidate.ProfileURL, candidate.FollowerCount, status, candidate.Score, scoreVersion,
		nullTime(candidate.LastDiscoveredAt), nullTime(candidate.LastScoredAt), nullTime(candidate.ApprovedAt), nullTime(candidate.IgnoredAt), nullTime(candidate.BlockedAt))
	if err != nil {
		return repo.CandidateCreator{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return repo.CandidateCreator{}, err
	}
	return r.FindByID(ctx, id)
}

func (r *candidateRepo) FindByID(ctx context.Context, id int64) (repo.CandidateCreator, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, platform, uid, name, avatar_url, profile_url, follower_count, status, score, score_version,
			last_discovered_at, last_scored_at, approved_at, ignored_at, blocked_at, created_at, updated_at
		FROM candidate_creators
		WHERE id = ?
	`, id)
	return scanCandidateCreator(row)
}

func (r *candidateRepo) FindByPlatformUID(ctx context.Context, platform, uid string) (repo.CandidateCreator, error) {
	if strings.TrimSpace(platform) == "" {
		platform = "bilibili"
	}
	row := r.db.QueryRowContext(ctx, `
		SELECT id, platform, uid, name, avatar_url, profile_url, follower_count, status, score, score_version,
			last_discovered_at, last_scored_at, approved_at, ignored_at, blocked_at, created_at, updated_at
		FROM candidate_creators
		WHERE platform = ? AND uid = ?
	`, platform, uid)
	return scanCandidateCreator(row)
}

func (r *candidateRepo) List(ctx context.Context, filter repo.CandidateListFilter) ([]repo.CandidateCreator, int64, error) {
	var (
		conditions []string
		args       []any
	)
	if status := strings.TrimSpace(filter.Status); status != "" {
		conditions = append(conditions, "c.status = ?")
		args = append(args, status)
	}
	if filter.MinScore > 0 {
		conditions = append(conditions, "c.score >= ?")
		args = append(args, filter.MinScore)
	}
	if keyword := strings.TrimSpace(filter.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		conditions = append(conditions, `(c.name LIKE ? OR c.uid LIKE ? OR EXISTS (
			SELECT 1 FROM candidate_creator_sources s
			WHERE s.candidate_creator_id = c.id AND s.source_label LIKE ?
		))`)
		args = append(args, like, like, like)
	}

	where := ""
	if len(conditions) > 0 {
		where = " WHERE " + strings.Join(conditions, " AND ")
	}

	countQuery := "SELECT COUNT(*) FROM candidate_creators c" + where
	var total int64
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	pageSize := filter.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	page := filter.Page
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * pageSize

	listArgs := append(append([]any{}, args...), pageSize, offset)
	rows, err := r.db.QueryContext(ctx, `
		SELECT c.id, c.platform, c.uid, c.name, c.avatar_url, c.profile_url, c.follower_count, c.status, c.score, c.score_version,
			c.last_discovered_at, c.last_scored_at, c.approved_at, c.ignored_at, c.blocked_at, c.created_at, c.updated_at
		FROM candidate_creators c`+where+`
		ORDER BY c.score DESC, c.last_discovered_at DESC, c.id DESC
		LIMIT ? OFFSET ?
	`, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []repo.CandidateCreator
	for rows.Next() {
		item, err := scanCandidateCreator(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *candidateRepo) ListSources(ctx context.Context, candidateID int64) ([]repo.CandidateCreatorSource, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, candidate_creator_id, source_type, source_value, source_label, weight, detail_json, created_at
		FROM candidate_creator_sources
		WHERE candidate_creator_id = ?
		ORDER BY weight DESC, id ASC
	`, candidateID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []repo.CandidateCreatorSource
	for rows.Next() {
		item, err := scanCandidateCreatorSource(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *candidateRepo) ListScoreDetails(ctx context.Context, candidateID int64) ([]repo.CandidateCreatorScoreDetail, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, candidate_creator_id, factor_key, factor_label, score_delta, detail_json, created_at
		FROM candidate_creator_score_details
		WHERE candidate_creator_id = ?
		ORDER BY id ASC
	`, candidateID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []repo.CandidateCreatorScoreDetail
	for rows.Next() {
		item, err := scanCandidateCreatorScoreDetail(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *candidateRepo) ReplaceSources(ctx context.Context, candidateID int64, sources []repo.CandidateCreatorSource) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err = tx.ExecContext(ctx, `DELETE FROM candidate_creator_sources WHERE candidate_creator_id = ?`, candidateID); err != nil {
		return err
	}

	for _, source := range dedupeCandidateSources(candidateID, sources) {
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO candidate_creator_sources (
				candidate_creator_id, source_type, source_value, source_label, weight, detail_json, created_at
			)
			VALUES (?, ?, ?, ?, ?, ?, NOW())
		`, candidateID, source.SourceType, source.SourceValue, source.SourceLabel, source.Weight, nullJSON(source.DetailJSON)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *candidateRepo) ReplaceScoreDetails(ctx context.Context, candidateID int64, details []repo.CandidateCreatorScoreDetail) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err = tx.ExecContext(ctx, `DELETE FROM candidate_creator_score_details WHERE candidate_creator_id = ?`, candidateID); err != nil {
		return err
	}

	for _, detail := range details {
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO candidate_creator_score_details (
				candidate_creator_id, factor_key, factor_label, score_delta, detail_json, created_at
			)
			VALUES (?, ?, ?, ?, ?, NOW())
		`, candidateID, detail.FactorKey, detail.FactorLabel, detail.ScoreDelta, nullJSON(detail.DetailJSON)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *candidateRepo) UpdateReviewStatus(ctx context.Context, id int64, from []string, to string, at time.Time) error {
	if len(from) == 0 {
		from = []string{to}
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(from)), ",")
	query := `
		UPDATE candidate_creators
		SET status = ?,
			updated_at = NOW()
		WHERE id = ?
			AND status IN (` + placeholders + `)
	`
	args := []any{to, id}
	if changedAt := nullTime(statusChangedAt(to, at)); changedAt != nil {
		column := statusTimestampColumn(to)
		query = `
			UPDATE candidate_creators
			SET status = ?,
				` + column + ` = ?,
				updated_at = NOW()
			WHERE id = ?
				AND status IN (` + placeholders + `)
		`
		args = []any{to, changedAt, id}
	}
	for _, status := range from {
		args = append(args, status)
	}

	res, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected > 0 {
		return nil
	}

	var current string
	row := r.db.QueryRowContext(ctx, `SELECT status FROM candidate_creators WHERE id = ?`, id)
	if err := row.Scan(&current); err != nil {
		if err == sql.ErrNoRows {
			return repo.ErrNotFound
		}
		return err
	}
	return fmt.Errorf("非法状态流转: %s -> %s", current, to)
}

type candidateScanner interface {
	Scan(dest ...any) error
}

func scanCandidateCreator(scanner candidateScanner) (repo.CandidateCreator, error) {
	var (
		item           repo.CandidateCreator
		name           sql.NullString
		avatarURL      sql.NullString
		profileURL     sql.NullString
		lastDiscovered sql.NullTime
		lastScored     sql.NullTime
		approvedAt     sql.NullTime
		ignoredAt      sql.NullTime
		blockedAt      sql.NullTime
		createdAt      time.Time
		updatedAt      time.Time
	)
	if err := scanner.Scan(
		&item.ID, &item.Platform, &item.UID, &name, &avatarURL, &profileURL, &item.FollowerCount, &item.Status, &item.Score, &item.ScoreVersion,
		&lastDiscovered, &lastScored, &approvedAt, &ignoredAt, &blockedAt, &createdAt, &updatedAt,
	); err != nil {
		return repo.CandidateCreator{}, err
	}
	item.Name = nullString(name)
	item.AvatarURL = nullString(avatarURL)
	item.ProfileURL = nullString(profileURL)
	item.LastDiscoveredAt = nullTimeValue(lastDiscovered)
	item.LastScoredAt = nullTimeValue(lastScored)
	item.ApprovedAt = nullTimeValue(approvedAt)
	item.IgnoredAt = nullTimeValue(ignoredAt)
	item.BlockedAt = nullTimeValue(blockedAt)
	item.CreatedAt = createdAt
	item.UpdatedAt = updatedAt
	return item, nil
}

func scanCandidateCreatorSource(scanner candidateScanner) (repo.CandidateCreatorSource, error) {
	var (
		item      repo.CandidateCreatorSource
		label     sql.NullString
		detail    []byte
		createdAt time.Time
	)
	if err := scanner.Scan(&item.ID, &item.CandidateCreatorID, &item.SourceType, &item.SourceValue, &label, &item.Weight, &detail, &createdAt); err != nil {
		return repo.CandidateCreatorSource{}, err
	}
	item.SourceLabel = nullString(label)
	item.DetailJSON = copyJSON(detail)
	item.CreatedAt = createdAt
	return item, nil
}

func scanCandidateCreatorScoreDetail(scanner candidateScanner) (repo.CandidateCreatorScoreDetail, error) {
	var (
		item      repo.CandidateCreatorScoreDetail
		detail    []byte
		createdAt time.Time
	)
	if err := scanner.Scan(&item.ID, &item.CandidateCreatorID, &item.FactorKey, &item.FactorLabel, &item.ScoreDelta, &detail, &createdAt); err != nil {
		return repo.CandidateCreatorScoreDetail{}, err
	}
	item.DetailJSON = copyJSON(detail)
	item.CreatedAt = createdAt
	return item, nil
}

func nullString(v sql.NullString) string {
	if v.Valid {
		return v.String
	}
	return ""
}

func nullTime(v time.Time) any {
	if v.IsZero() {
		return nil
	}
	return v
}

func nullTimeValue(v sql.NullTime) time.Time {
	if v.Valid {
		return v.Time
	}
	return time.Time{}
}

func nullJSON(v []byte) any {
	if len(v) == 0 {
		return nil
	}
	return v
}

func copyJSON(v []byte) []byte {
	if len(v) == 0 {
		return nil
	}
	out := make([]byte, len(v))
	copy(out, v)
	return out
}

func dedupeCandidateSources(candidateID int64, sources []repo.CandidateCreatorSource) []repo.CandidateCreatorSource {
	if len(sources) == 0 {
		return nil
	}
	ordered := make([]string, 0, len(sources))
	items := make(map[string]repo.CandidateCreatorSource, len(sources))
	for _, source := range sources {
		key := source.SourceType + "\x00" + source.SourceValue
		if _, exists := items[key]; !exists {
			ordered = append(ordered, key)
		}
		source.CandidateCreatorID = candidateID
		items[key] = source
	}
	out := make([]repo.CandidateCreatorSource, 0, len(items))
	for _, key := range ordered {
		out = append(out, items[key])
	}
	return out
}

func statusChangedAt(to string, at time.Time) time.Time {
	if at.IsZero() {
		return time.Time{}
	}
	switch to {
	case "approved", "ignored", "blocked":
		return at
	default:
		return time.Time{}
	}
}

func statusTimestampColumn(status string) string {
	switch status {
	case "approved":
		return "approved_at"
	case "ignored":
		return "ignored_at"
	case "blocked":
		return "blocked_at"
	default:
		return ""
	}
}
