package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

type PlazaPerson struct {
	ID       int     `json:"id"`
	Username string  `json:"username"`
	Total    int     `json:"total"`
	Minutes  int     `json:"minutes"`
	Entries  []Entry `json:"entries"`
}

func (s *Store) GardenPublic(ctx context.Context) (bool, error) {
	uid, err := UserID(ctx)
	if err != nil {
		return false, err
	}
	var on bool
	err = s.pool.QueryRow(ctx, `SELECT public_garden FROM users WHERE id = $1`, uid).Scan(&on)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrUnauthorized
	}
	return on, err
}

func (s *Store) SetGardenPublic(ctx context.Context, on bool) error {
	uid, err := UserID(ctx)
	if err != nil {
		return err
	}
	tag, err := s.pool.Exec(ctx, `UPDATE users SET public_garden = $1 WHERE id = $2`, on, uid)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrUnauthorized
	}
	return nil
}

func (s *Store) Plaza(ctx context.Context) ([]PlazaPerson, error) {
	if _, err := UserID(ctx); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
		SELECT u.id, u.username,
			COUNT(e.id)::int,
			COALESCE(SUM(e.spend_minutes), 0)::int
		FROM users u
		LEFT JOIN entries e ON e.user_id = u.id
			AND e.path_id = (SELECT id FROM paths WHERE slug = 'games')
		WHERE u.public_garden
		GROUP BY u.id, u.username
		ORDER BY COALESCE(SUM(e.spend_minutes), 0) DESC, u.username
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PlazaPerson{}
	for rows.Next() {
		var person PlazaPerson
		if err := rows.Scan(&person.ID, &person.Username, &person.Total, &person.Minutes); err != nil {
			return nil, err
		}
		out = append(out, person)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		entries, err := s.publicGames(ctx, out[i].ID, 4)
		if err != nil {
			return nil, err
		}
		out[i].Entries = entries
	}
	return out, nil
}

func (s *Store) PlazaOne(ctx context.Context, id int) (PlazaPerson, error) {
	if _, err := UserID(ctx); err != nil {
		return PlazaPerson{}, err
	}
	var person PlazaPerson
	var on bool
	err := s.pool.QueryRow(ctx, `
		SELECT id, username, public_garden FROM users WHERE id = $1
	`, id).Scan(&person.ID, &person.Username, &on)
	if errors.Is(err, pgx.ErrNoRows) || !on {
		return PlazaPerson{}, ErrNotFound
	}
	if err != nil {
		return PlazaPerson{}, err
	}
	err = s.pool.QueryRow(ctx, `
		SELECT COUNT(*)::int, COALESCE(SUM(e.spend_minutes), 0)::int
		FROM entries e
		WHERE e.user_id = $1 AND e.path_id = (SELECT id FROM paths WHERE slug = 'games')
	`, id).Scan(&person.Total, &person.Minutes)
	if err != nil {
		return PlazaPerson{}, err
	}
	person.Entries, err = s.publicGames(ctx, id, 0)
	if err != nil {
		return PlazaPerson{}, err
	}
	return person, nil
}

func (s *Store) publicGames(ctx context.Context, userID, limit int) ([]Entry, error) {
	sql := entrySelect + `
WHERE e.user_id = $1 AND path.slug = 'games'
GROUP BY e.id, path.id, m.id, v.id
ORDER BY e.spend_minutes DESC, e.title ASC`
	args := []any{userID}
	if limit > 0 {
		sql += ` LIMIT $2`
		args = append(args, limit)
	}
	entries, err := s.queryEntries(ctx, sql, args)
	if err != nil {
		return nil, err
	}
	for i := range entries {
		entries[i].Notes = ""
	}
	return entries, nil
}
