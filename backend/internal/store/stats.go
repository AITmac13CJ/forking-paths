package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func (s *Store) enrichStats(ctx context.Context, out *Stats, year, pathID, userID int, start, end string) error {
	if out.Done > 0 {
		out.AvgSpend = out.SpendMinutes / out.Done
	}
	prevStart := fmt.Sprintf("%04d-01-01", year-1)
	prevEnd := fmt.Sprintf("%04d-01-01", year)
	if err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*)::int, COALESCE(SUM(spend_minutes), 0)::int
		FROM entries
		WHERE status_kind = 'done' AND finish_date >= $1::date AND finish_date < $2::date
			AND ($3 = 0 OR path_id = $3) AND user_id = $4
	`, prevStart, prevEnd, pathID, userID).Scan(&out.PrevDone, &out.PrevSpend); err != nil {
		return err
	}

	dayRows, err := s.pool.Query(ctx, `
		SELECT to_char(finish_date, 'YYYY-MM-DD'), COUNT(*)::int
		FROM entries
		WHERE status_kind = 'done' AND finish_date >= $1::date AND finish_date < $2::date
			AND ($3 = 0 OR path_id = $3) AND user_id = $4
		GROUP BY 1
		ORDER BY 1
	`, start, end, pathID, userID)
	if err != nil {
		return err
	}
	defer dayRows.Close()
	for dayRows.Next() {
		var row DayMark
		if err := dayRows.Scan(&row.Date, &row.Count); err != nil {
			return err
		}
		out.Days = append(out.Days, row)
	}
	if err := dayRows.Err(); err != nil {
		return err
	}
	out.ActiveDays = len(out.Days)

	longest, err := s.oneHighlight(ctx, `
		SELECT e.title, e.cover_url, COALESCE(e.cover->>'a', '#1c1b19'), COALESCE(m.name, ''), p.name,
			e.spend_minutes, e.rating, to_char(e.finish_date, 'YYYY-MM-DD')
		FROM entries e
		JOIN paths p ON p.id = e.path_id
		LEFT JOIN media m ON m.id = e.medium_id
		WHERE e.status_kind = 'done' AND e.finish_date >= $1::date AND e.finish_date < $2::date
			AND ($3 = 0 OR e.path_id = $3) AND e.user_id = $4 AND e.spend_minutes > 0
		ORDER BY e.spend_minutes DESC, e.finish_date DESC
		LIMIT 1
	`, start, end, pathID, userID)
	if err != nil {
		return err
	}
	out.Longest = longest

	favorite, err := s.oneHighlight(ctx, `
		SELECT e.title, e.cover_url, COALESCE(e.cover->>'a', '#1c1b19'), COALESCE(m.name, ''), p.name,
			e.spend_minutes, e.rating, to_char(e.finish_date, 'YYYY-MM-DD')
		FROM entries e
		JOIN paths p ON p.id = e.path_id
		LEFT JOIN media m ON m.id = e.medium_id
		WHERE e.status_kind = 'done' AND e.finish_date >= $1::date AND e.finish_date < $2::date
			AND ($3 = 0 OR e.path_id = $3) AND e.user_id = $4 AND e.rating IS NOT NULL
		ORDER BY e.rating DESC, e.spend_minutes DESC, e.finish_date DESC
		LIMIT 1
	`, start, end, pathID, userID)
	if err != nil {
		return err
	}
	out.Favorite = favorite

	rankRows, err := s.pool.Query(ctx, `
		SELECT e.title, e.cover_url, COALESCE(e.cover->>'a', '#1c1b19'), COALESCE(m.name, ''), e.spend_minutes, e.rating
		FROM entries e
		LEFT JOIN media m ON m.id = e.medium_id
		WHERE e.status_kind = 'done' AND e.finish_date >= $1::date AND e.finish_date < $2::date
			AND ($3 = 0 OR e.path_id = $3) AND e.user_id = $4 AND e.spend_minutes > 0
		ORDER BY e.spend_minutes DESC, e.finish_date DESC
		LIMIT 8
	`, start, end, pathID, userID)
	if err != nil {
		return err
	}
	defer rankRows.Close()
	for rankRows.Next() {
		var row RankItem
		if err := rankRows.Scan(&row.Title, &row.CoverURL, &row.Color, &row.Medium, &row.Minutes, &row.Rating); err != nil {
			return err
		}
		out.Ranking = append(out.Ranking, row)
	}
	if err := rankRows.Err(); err != nil {
		return err
	}

	pathRows, err := s.pool.Query(ctx, `
		SELECT p.name, p.slug,
			CASE p.slug
				WHEN 'games' THEN '#7c3aed'
				WHEN 'reading' THEN '#16a34a'
				WHEN 'film' THEN '#eab308'
				WHEN 'music' THEN '#dc2626'
				ELSE '#37352f'
			END,
			COUNT(*)::int, COALESCE(SUM(e.spend_minutes), 0)::int
		FROM entries e
		JOIN paths p ON p.id = e.path_id
		WHERE e.status_kind = 'done' AND e.finish_date >= $1::date AND e.finish_date < $2::date
			AND ($3 = 0 OR e.path_id = $3) AND e.user_id = $4
		GROUP BY p.id
		ORDER BY COUNT(*) DESC, p.sort_order
	`, start, end, pathID, userID)
	if err != nil {
		return err
	}
	defer pathRows.Close()
	for pathRows.Next() {
		var row PathShare
		if err := pathRows.Scan(&row.Name, &row.Slug, &row.Color, &row.Count, &row.Minutes); err != nil {
			return err
		}
		out.ByPath = append(out.ByPath, row)
	}
	if err := pathRows.Err(); err != nil {
		return err
	}

	badgeRows, err := s.pool.Query(ctx, `
		SELECT badge, COUNT(*)::int
		FROM entries, unnest(badges) AS badge
		WHERE status_kind = 'done' AND finish_date >= $1::date AND finish_date < $2::date
			AND ($3 = 0 OR path_id = $3) AND user_id = $4 AND badge <> ''
		GROUP BY badge
		ORDER BY COUNT(*) DESC, badge
		LIMIT 8
	`, start, end, pathID, userID)
	if err != nil {
		return err
	}
	defer badgeRows.Close()
	for badgeRows.Next() {
		var row NameCount
		if err := badgeRows.Scan(&row.Name, &row.Count); err != nil {
			return err
		}
		out.ByBadge = append(out.ByBadge, row)
	}
	if err := badgeRows.Err(); err != nil {
		return err
	}

	viaRows, err := s.pool.Query(ctx, `
		SELECT v.name, COUNT(*)::int
		FROM entries e
		JOIN vias v ON v.id = e.via_id
		WHERE e.status_kind = 'done' AND e.finish_date >= $1::date AND e.finish_date < $2::date
			AND ($3 = 0 OR e.path_id = $3) AND e.user_id = $4
		GROUP BY v.id
		ORDER BY COUNT(*) DESC, v.name
		LIMIT 6
	`, start, end, pathID, userID)
	if err != nil {
		return err
	}
	defer viaRows.Close()
	for viaRows.Next() {
		var row NameCount
		if err := viaRows.Scan(&row.Name, &row.Count); err != nil {
			return err
		}
		out.ByVia = append(out.ByVia, row)
	}
	return viaRows.Err()
}

func (s *Store) oneHighlight(ctx context.Context, query, start, end string, pathID, userID int) (*Highlight, error) {
	var row Highlight
	err := s.pool.QueryRow(ctx, query, start, end, pathID, userID).Scan(
		&row.Title, &row.CoverURL, &row.Color, &row.Medium, &row.Path, &row.Minutes, &row.Rating, &row.FinishDate,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}
