package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

type SyncJob struct {
	ID      int    `json:"id"`
	Kind    string `json:"kind"`
	Status  string `json:"status"`
	Message string `json:"message"`
	Done    int    `json:"done"`
	Total   int    `json:"total"`
	Updated int    `json:"updated"`
	Created int    `json:"created"`
}

func (s *Store) StartJob(ctx context.Context, kind string) (SyncJob, error) {
	uid, err := UserID(ctx)
	if err != nil {
		return SyncJob{}, err
	}
	var job SyncJob
	err = s.pool.QueryRow(ctx, `
		INSERT INTO sync_jobs (user_id, kind, status, message)
		VALUES ($1, $2, 'running', '正在开始')
		RETURNING id, kind, status, message, done_count, total_count, updated_count, created_count
	`, uid, kind).Scan(&job.ID, &job.Kind, &job.Status, &job.Message, &job.Done, &job.Total, &job.Updated, &job.Created)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return SyncJob{}, &ValidationError{Msg: "这一项正在同步"}
		}
		return SyncJob{}, err
	}
	return job, nil
}

func (s *Store) TouchJob(ctx context.Context, id, done, total int, message string) error {
	uid, err := UserID(ctx)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		UPDATE sync_jobs
		SET done_count = $3, total_count = $4, message = $5
		WHERE id = $1 AND user_id = $2 AND status = 'running'
	`, id, uid, done, total, message)
	return err
}

func (s *Store) FinishJob(ctx context.Context, id int, status, message string, updated, created int) error {
	uid, err := UserID(ctx)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		UPDATE sync_jobs
		SET status = $3, message = $4, updated_count = $5, created_count = $6, finished_at = now()
		WHERE id = $1 AND user_id = $2 AND status = 'running'
	`, id, uid, status, message, updated, created)
	return err
}

func (s *Store) ListJobs(ctx context.Context) ([]SyncJob, error) {
	uid, err := UserID(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, kind, status, message, done_count, total_count, updated_count, created_count
		FROM sync_jobs
		WHERE user_id = $1 AND acked = FALSE
		ORDER BY CASE WHEN status = 'running' THEN 0 ELSE 1 END, id DESC
		LIMIT 12
	`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SyncJob{}
	for rows.Next() {
		var job SyncJob
		if err := rows.Scan(&job.ID, &job.Kind, &job.Status, &job.Message, &job.Done, &job.Total, &job.Updated, &job.Created); err != nil {
			return nil, err
		}
		out = append(out, job)
	}
	return out, rows.Err()
}

func (s *Store) AckJob(ctx context.Context, id int) error {
	uid, err := UserID(ctx)
	if err != nil {
		return err
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE sync_jobs SET acked = TRUE
		WHERE id = $1 AND user_id = $2 AND status <> 'running' AND acked = FALSE
	`, id, uid)
	if err != nil {
		return err
	}
	if tag.RowsAffected() > 0 {
		return nil
	}
	var status string
	err = s.pool.QueryRow(ctx, `SELECT status FROM sync_jobs WHERE id = $1 AND user_id = $2`, id, uid).Scan(&status)
	if err != nil {
		return ErrNotFound
	}
	if status == "running" {
		return &ValidationError{Msg: "还在同步"}
	}
	return nil
}

func (s *Store) FailInterruptedJobs(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE sync_jobs
		SET status = 'error', message = '同步中断了，请再试一次', finished_at = now()
		WHERE status = 'running'
	`)
	return err
}
