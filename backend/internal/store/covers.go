package store

import (
	"context"
	"strings"
	"unicode"
)

type SteamCover struct {
	ID        int
	Title     string
	AppID     string
	Portrait  bool
	Landscape bool
}

func (s *Store) SaveGridKey(ctx context.Context, key string) error {
	key = strings.TrimSpace(key)
	if len(key) < 16 || len(key) > 128 {
		return &ValidationError{Msg: "密钥格式不对"}
	}
	for _, r := range key {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' && r != '_' {
			return &ValidationError{Msg: "密钥格式不对"}
		}
	}
	uid, err := UserID(ctx)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO platform_accounts (user_id, provider, account, secret)
		VALUES ($2, 'steamgriddb', '', $1)
		ON CONFLICT (user_id, provider) DO UPDATE SET secret = EXCLUDED.secret, updated_at = now()
	`, key, uid)
	return err
}

func (s *Store) GridKey(ctx context.Context) (string, error) {
	var secret string
	uid, err := UserID(ctx)
	if err != nil {
		return "", err
	}
	err = s.pool.QueryRow(ctx, `SELECT secret FROM platform_accounts WHERE provider = 'steamgriddb' AND user_id = $1`, uid).Scan(&secret)
	if err != nil || strings.TrimSpace(secret) == "" {
		return "", &ValidationError{Msg: "请先填写 SteamGridDB 密钥"}
	}
	return secret, nil
}

func (s *Store) PendingSteamCovers(ctx context.Context) ([]SteamCover, error) {
	uid, err := UserID(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
		SELECT e.id, e.title, e.platform_ref, e.cover_url, e.cover_landscape_url
		FROM entries e
		JOIN paths p ON p.id = e.path_id
		WHERE e.user_id = $1 AND p.slug = 'games'
		  AND (
		    e.cover_url = ''
		    OR e.cover_landscape_url = ''
		    OR e.cover_landscape_url LIKE '/covers/steam-%-l.%'
		    OR e.cover_landscape_url LIKE '/covers/steam-%-c.%'
		  )
		ORDER BY e.id
	`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SteamCover{}
	for rows.Next() {
		var item SteamCover
		var ref, portrait, landscape string
		if err := rows.Scan(&item.ID, &item.Title, &ref, &portrait, &landscape); err != nil {
			return nil, err
		}
		item.Title = strings.TrimSpace(item.Title)
		if strings.HasPrefix(ref, "steam:") {
			appID := strings.TrimPrefix(ref, "steam:")
			if appID != "" && strings.Trim(appID, "0123456789") == "" {
				item.AppID = appID
			}
		}
		if item.AppID == "" && item.Title == "" {
			continue
		}
		item.Portrait = portrait == ""
		item.Landscape = landscape == "" || generatedLandscape(landscape)
		out = append(out, item)
	}
	return out, rows.Err()
}

func generatedLandscape(url string) bool {
	return strings.Contains(url, "/covers/steam-") && (strings.Contains(url, "-l.") || strings.Contains(url, "-c."))
}

func (s *Store) FillSteamCover(ctx context.Context, id int, portrait, landscape string) error {
	uid, err := UserID(ctx)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		UPDATE entries SET
			cover_url = CASE WHEN cover_url = '' AND $1 <> '' THEN $1 ELSE cover_url END,
			cover_landscape_url = CASE
				WHEN $2 <> '' AND (cover_landscape_url = '' OR cover_landscape_url LIKE '/covers/steam-%-l.%' OR cover_landscape_url LIKE '/covers/steam-%-c.%') THEN $2
				ELSE cover_landscape_url
			END,
			updated_at = now()
		WHERE id = $3 AND user_id = $4
	`, portrait, landscape, id, uid)
	return err
}
