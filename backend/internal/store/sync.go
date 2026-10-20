package store

import (
	"context"
	"encoding/json"
	"strings"
	"unicode"
	"unicode/utf8"

	"forking-paths/internal/platform"
)

var platformFamilies = map[string][]string{
	"steam":  {"Steam"},
	"psn":    {"PS5", "PS4", "PS Vita"},
	"xbox":   {"Xbox"},
	"switch": {"Switch", "Switch 2"},
}

var platformMedium = map[string]string{
	"steam":  "Steam",
	"psn":    "PS5",
	"xbox":   "Xbox",
	"switch": "Switch",
}

type Account struct {
	Provider string `json:"provider"`
	Linked   bool   `json:"linked"`
	Account  string `json:"account"`
}

type SyncItem struct {
	Ref      string `json:"ref"`
	Title    string `json:"title"`
	Minutes  int    `json:"minutes"`
	Earned   int    `json:"earned"`
	Total    int    `json:"total"`
	Complete bool   `json:"complete"`
	Platinum bool   `json:"platinum"`
	Action   string `json:"action"`
}

type SyncResult struct {
	Updated int        `json:"updated"`
	Created int        `json:"created"`
	Items   []SyncItem `json:"items"`
}

func KnownProvider(name string) bool {
	_, ok := platformFamilies[name]
	return ok
}

func (s *Store) Accounts(ctx context.Context) ([]Account, error) {
	uid, err := UserID(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT provider, account, secret <> '' FROM platform_accounts WHERE user_id = $1`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	found := map[string]Account{}
	for rows.Next() {
		var item Account
		if err := rows.Scan(&item.Provider, &item.Account, &item.Linked); err != nil {
			return nil, err
		}
		found[item.Provider] = item
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]Account, 0, len(platformFamilies))
	for _, name := range []string{"steam", "psn", "xbox", "switch"} {
		item := found[name]
		item.Provider = name
		if name == "steam" {
			if user, ok := platform.LocalSteamUser(); ok && item.Account == user.ID {
				item.Account = user.Name
			}
		}
		out = append(out, item)
	}
	grid := found["steamgriddb"]
	grid.Provider = "steamgriddb"
	grid.Account = ""
	out = append(out, grid)
	return out, nil
}

func (s *Store) SaveAccount(ctx context.Context, provider, account, secret string) error {
	if !KnownProvider(provider) {
		return &ValidationError{Msg: "未知的平台"}
	}
	account = strings.TrimSpace(account)
	secret = strings.TrimSpace(secret)
	if len(account) > 200 || len(secret) > 20000 {
		return &ValidationError{Msg: "凭证太长了"}
	}
	uid, err := UserID(ctx)
	if err != nil {
		return err
	}
	if secret == "" {
		var existing string
		err := s.pool.QueryRow(ctx, `SELECT secret FROM platform_accounts WHERE provider = $1 AND user_id = $2`, provider, uid).Scan(&existing)
		if err != nil || existing == "" {
			return &ValidationError{Msg: "请先登录这个平台"}
		}
		secret = existing
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO platform_accounts (provider, account, secret, user_id)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id, provider) DO UPDATE SET account = EXCLUDED.account, secret = EXCLUDED.secret, updated_at = now()
	`, provider, account, secret, uid)
	return err
}

func (s *Store) UpdateSecret(ctx context.Context, provider, secret string) error {
	uid, err := UserID(ctx)
	if err != nil {
		return err
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE platform_accounts SET secret = $3, updated_at = now()
		WHERE user_id = $1 AND provider = $2
	`, uid, provider, secret)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

type StoredAccount struct {
	UserID   int
	Provider string
	Secret   string
}

func (s *Store) StoredAccounts(ctx context.Context) ([]StoredAccount, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT user_id, provider, secret FROM platform_accounts
		WHERE provider IN ('steam', 'psn', 'xbox') AND secret <> '' AND user_id IS NOT NULL
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []StoredAccount{}
	for rows.Next() {
		var item StoredAccount
		if err := rows.Scan(&item.UserID, &item.Provider, &item.Secret); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) PSNSecrets(ctx context.Context) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT secret FROM platform_accounts
		WHERE provider = 'psn' AND secret <> ''
		ORDER BY updated_at DESC
		LIMIT 8
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var secret string
		if err := rows.Scan(&secret); err != nil {
			return nil, err
		}
		out = append(out, secret)
	}
	return out, rows.Err()
}

func (s *Store) SteamAccessTokens(ctx context.Context) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT secret FROM platform_accounts
		WHERE provider = 'steam' AND left(secret, 1) = '{'
		  AND coalesce(secret::jsonb->>'access_token', '') <> ''
		ORDER BY updated_at DESC
		LIMIT 5
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var secret string
		if err := rows.Scan(&secret); err != nil {
			return nil, err
		}
		var cred struct {
			Access string `json:"access_token"`
		}
		if json.Unmarshal([]byte(secret), &cred) == nil && cred.Access != "" {
			out = append(out, cred.Access)
		}
	}
	return out, rows.Err()
}

// SteamWebSession returns a still-usable Steam access token already stored for this Steam ID.
func (s *Store) SteamWebSession(ctx context.Context, steamID string) (access, refresh string, ok bool) {
	if len(steamID) < 17 {
		return "", "", false
	}
	var secret string
	err := s.pool.QueryRow(ctx, `
		SELECT secret FROM platform_accounts
		WHERE provider = 'steam' AND left(secret, 1) = '{'
		  AND secret::jsonb->>'steam_id' = $1
		  AND coalesce(secret::jsonb->>'access_token', '') <> ''
		ORDER BY updated_at DESC
		LIMIT 1
	`, steamID).Scan(&secret)
	if err != nil {
		return "", "", false
	}
	var cred struct {
		Access  string `json:"access_token"`
		Refresh string `json:"refresh_token"`
	}
	if json.Unmarshal([]byte(secret), &cred) != nil || cred.Access == "" {
		return "", "", false
	}
	return cred.Access, cred.Refresh, true
}

func (s *Store) AccountSecret(ctx context.Context, provider string) (account, secret string, err error) {
	uid, err := UserID(ctx)
	if err != nil {
		return "", "", err
	}
	err = s.pool.QueryRow(ctx, `SELECT account, secret FROM platform_accounts WHERE provider = $1 AND user_id = $2`, provider, uid).Scan(&account, &secret)
	if err != nil || secret == "" {
		return "", "", &ValidationError{Msg: "请先登录这个平台"}
	}
	return account, secret, nil
}

type localGame struct {
	id     int
	title  string
	medium string
	status string
	spend  int
	badges []string
	ref    string
	earned int
	total  int
}

func (s *Store) ApplySync(ctx context.Context, provider string, games []platform.Game, report func(done, total int)) (SyncResult, error) {
	family := platformFamilies[provider]
	if family == nil {
		return SyncResult{}, &ValidationError{Msg: "未知的平台"}
	}
	locals, err := s.gameLocals(ctx)
	if err != nil {
		return SyncResult{}, err
	}
	byRef := map[string]int{}
	byTitle := map[string][]int{}
	for i, item := range locals {
		if item.ref != "" {
			byRef[item.ref] = i
		}
		key := foldTitle(item.title)
		if key != "" && inFamily(item.medium, family) {
			byTitle[key] = append(byTitle[key], i)
		}
	}
	result := SyncResult{Items: []SyncItem{}}
	for _, game := range games {
		ref := ""
		if game.Ref != "" {
			ref = provider + ":" + game.Ref
		}
		idx := -1
		if ref != "" {
			if i, ok := byRef[ref]; ok {
				idx = i
			}
		}
		if idx < 0 {
			hits := byTitle[foldTitle(game.Title)]
			if len(hits) == 1 {
				idx = hits[0]
			}
		}
		if idx < 0 {
			result.Items = append(result.Items, SyncItem{
				Ref: game.Ref, Title: game.Title, Minutes: game.Minutes,
				Earned: game.Earned, Total: game.Total, Complete: game.Complete, Platinum: game.Platinum,
				Action: "new",
			})
			continue
		}
		changed := game.Minutes > locals[idx].spend || (ref != "" && locals[idx].ref == "")
		if game.Platinum && !hasBadge(locals[idx].badges, "白金") {
			changed = true
		}
		if game.Complete && !hasBadge(locals[idx].badges, "全成就") && !hasBadge(locals[idx].badges, "白金") {
			changed = true
		}
		if game.Total > 0 && (game.Earned != locals[idx].earned || game.Total != locals[idx].total) {
			changed = true
		}
		if game.Minutes > 0 && (locals[idx].status == "want" || locals[idx].status == "hold") {
			changed = true
		}
		if !changed {
			continue
		}
		if err := s.touchGame(ctx, &locals[idx], ref, game); err != nil {
			return SyncResult{}, err
		}
		if ref != "" {
			locals[idx].ref = ref
			byRef[ref] = idx
		}
		result.Updated++
	}
	fresh := make([]platform.Game, 0)
	for _, item := range result.Items {
		if item.Action != "new" || strings.TrimSpace(item.Title) == "" || strings.HasPrefix(item.Title, "App ") {
			continue
		}
		fresh = append(fresh, platform.Game{
			Ref: item.Ref, Title: item.Title, Minutes: item.Minutes,
			Earned: item.Earned, Total: item.Total, Complete: item.Complete, Platinum: item.Platinum,
		})
	}
	created, err := s.ImportSync(ctx, provider, fresh, report)
	if err != nil {
		return result, err
	}
	result.Created = created
	result.Items = nil
	return result, nil
}

func (s *Store) ImportSync(ctx context.Context, provider string, games []platform.Game, report func(done, total int)) (int, error) {
	mediumName := platformMedium[provider]
	if mediumName == "" {
		return 0, &ValidationError{Msg: "未知的平台"}
	}
	var pathID, mediumID int
	if err := s.pool.QueryRow(ctx, `SELECT id FROM paths WHERE slug = 'games'`).Scan(&pathID); err != nil {
		return 0, err
	}
	if err := s.pool.QueryRow(ctx, `SELECT id FROM media WHERE path_id = $1 AND name = $2`, pathID, mediumName).Scan(&mediumID); err != nil {
		return 0, &ValidationError{Msg: "游戏小径里还没有这个载体"}
	}
	uid, err := UserID(ctx)
	if err != nil {
		return 0, err
	}
	created := 0
	for _, game := range games {
		title := strings.TrimSpace(game.Title)
		if title == "" || strings.HasPrefix(title, "App ") {
			continue
		}
		if game.Minutes < 0 {
			continue
		}
		if game.Minutes > 200000 {
			game.Minutes = 200000
		}
		if utf8.RuneCountInString(title) > 80 {
			title = string([]rune(title)[:80])
		}
		badges := honorBadges(nil, game)
		status := "want"
		if game.Minutes > 0 {
			status = "doing"
		}
		id, err := s.Create(ctx, EntryInput{
			Title:          title,
			PathID:         pathID,
			MediumID:       mediumID,
			Status:         status,
			SpendMinutes:   game.Minutes,
			Badges:         badges,
			ProgressEarned: progressPtr(game),
			ProgressTotal:  progressTotalPtr(game),
			Cover:          fallbackCover(title),
			Notes:          "从账户同步",
		})
		if err != nil {
			return created, err
		}
		if game.Ref != "" {
			_, _ = s.pool.Exec(ctx, `UPDATE entries SET platform_ref = $1 WHERE id = $2 AND user_id = $3`, provider+":"+game.Ref, id.ID, uid)
		}
		created++
		if report != nil && (created == 1 || created%25 == 0) {
			report(created, len(games))
		}
	}
	if report != nil {
		report(created, len(games))
	}
	return created, nil
}

func (s *Store) gameLocals(ctx context.Context) ([]localGame, error) {
	uid, err := UserID(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
		SELECT e.id, e.title, COALESCE(m.name, ''), e.status_kind, e.spend_minutes, e.badges, e.platform_ref,
			COALESCE(e.progress_earned, 0), COALESCE(e.progress_total, 0)
		FROM entries e
		JOIN paths p ON p.id = e.path_id
		LEFT JOIN media m ON m.id = e.medium_id
		WHERE p.slug = 'games' AND e.user_id = $1
	`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []localGame{}
	for rows.Next() {
		var item localGame
		if err := rows.Scan(&item.id, &item.title, &item.medium, &item.status, &item.spend, &item.badges, &item.ref, &item.earned, &item.total); err != nil {
			return nil, err
		}
		if item.badges == nil {
			item.badges = []string{}
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) touchGame(ctx context.Context, item *localGame, ref string, game platform.Game) error {
	spend := item.spend
	if game.Minutes > spend {
		spend = game.Minutes
	}
	if spend > 200000 {
		spend = 200000
	}
	badges := honorBadges(item.badges, game)
	earned, total := item.earned, item.total
	if game.Total > 0 {
		earned, total = game.Earned, game.Total
	}
	status := item.status
	if game.Minutes > 0 && (status == "want" || status == "hold") {
		status = "doing"
	}
	nextRef := item.ref
	if ref != "" {
		nextRef = ref
	}
	uid, err := UserID(ctx)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		UPDATE entries SET spend_minutes = $1, badges = $2, platform_ref = $3, status_kind = $4,
			progress_earned = $5, progress_total = $6, updated_at = now()
		WHERE id = $7 AND user_id = $8
	`, spend, badges, nextRef, status, earned, total, item.id, uid)
	if err != nil {
		return err
	}
	item.spend = spend
	item.badges = badges
	item.earned = earned
	item.total = total
	item.status = status
	return nil
}

func inFamily(medium string, family []string) bool {
	for _, name := range family {
		if medium == name {
			return true
		}
	}
	return false
}

func honorBadges(badges []string, game platform.Game) []string {
	next := append([]string{}, badges...)
	if game.Platinum && !hasBadge(next, "白金") {
		next = append(next, "白金")
	}
	if game.Complete && !hasBadge(next, "全成就") && !hasBadge(next, "白金") {
		next = append(next, "全成就")
	}
	return next
}

func progressPtr(game platform.Game) *int {
	if game.Total <= 0 {
		return nil
	}
	v := game.Earned
	return &v
}

func progressTotalPtr(game platform.Game) *int {
	if game.Total <= 0 {
		return nil
	}
	v := game.Total
	return &v
}

func hasBadge(badges []string, name string) bool {
	for _, badge := range badges {
		if badge == name {
			return true
		}
	}
	return false
}

func foldTitle(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}
