package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

type querier interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

var motifList = []string{
	"sun", "crest", "ink", "eclipse", "galaxy", "crown",
	"mech", "fox", "sky", "wind", "castle", "leaf",
}

var statusSet = map[string]struct{}{
	"want": {}, "doing": {}, "done": {}, "hold": {},
}

var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

var fallbackCovers = []Cover{
	{A: "#143d59", B: "#f4b41a", Motif: "sun"},
	{A: "#5b2a86", B: "#c77dff", Motif: "crest"},
	{A: "#005f73", B: "#0a9396", Motif: "ink"},
	{A: "#111827", B: "#ea580c", Motif: "eclipse"},
}

func (s *Store) Meta(ctx context.Context) (Meta, error) {
	meta := Meta{
		Paths:  []Path{},
		Media:  []Medium{},
		Vias:   []Via{},
		Tags:   []Tag{},
		Motifs: append([]string{}, motifList...),
	}
	prows, err := s.pool.Query(ctx, `
		SELECT id, slug, name, medium_label, via_label, release_label, spend_label,
			finish_time_label, finish_date_label, done_word, doing_title,
			status_want, status_doing, status_done, status_hold, badges
		FROM paths ORDER BY sort_order, id
	`)
	if err != nil {
		return meta, err
	}
	defer prows.Close()
	for prows.Next() {
		var p Path
		if err := prows.Scan(
			&p.ID, &p.Slug, &p.Name, &p.MediumLabel, &p.ViaLabel, &p.ReleaseLabel, &p.SpendLabel,
			&p.FinishTimeLabel, &p.FinishDateLabel, &p.DoneWord, &p.DoingTitle,
			&p.StatusWant, &p.StatusDoing, &p.StatusDone, &p.StatusHold, &p.Badges,
		); err != nil {
			return meta, err
		}
		if p.Badges == nil {
			p.Badges = []string{}
		}
		meta.Paths = append(meta.Paths, p)
	}
	if err := prows.Err(); err != nil {
		return meta, err
	}

	mrows, err := s.pool.Query(ctx, `SELECT id, path_id, name, color FROM media ORDER BY path_id, sort_order, name`)
	if err != nil {
		return meta, err
	}
	defer mrows.Close()
	for mrows.Next() {
		var m Medium
		if err := mrows.Scan(&m.ID, &m.PathID, &m.Name, &m.Color); err != nil {
			return meta, err
		}
		meta.Media = append(meta.Media, m)
	}
	if err := mrows.Err(); err != nil {
		return meta, err
	}

	vrows, err := s.pool.Query(ctx, `SELECT id, path_id, name FROM vias ORDER BY path_id, name`)
	if err != nil {
		return meta, err
	}
	defer vrows.Close()
	for vrows.Next() {
		var v Via
		if err := vrows.Scan(&v.ID, &v.PathID, &v.Name); err != nil {
			return meta, err
		}
		meta.Vias = append(meta.Vias, v)
	}
	if err := vrows.Err(); err != nil {
		return meta, err
	}

	trows, err := s.pool.Query(ctx, `SELECT id, name FROM tags ORDER BY name`)
	if err != nil {
		return meta, err
	}
	defer trows.Close()
	for trows.Next() {
		var t Tag
		if err := trows.Scan(&t.ID, &t.Name); err != nil {
			return meta, err
		}
		meta.Tags = append(meta.Tags, t)
	}
	return meta, trows.Err()
}

func (s *Store) CreateTag(ctx context.Context, name string) (Tag, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 16 {
		return Tag{}, &ValidationError{Msg: "标签名称需要在 1 到 16 个字之间"}
	}
	var t Tag
	err := s.pool.QueryRow(ctx, `
		INSERT INTO tags (name) VALUES ($1)
		ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name
		RETURNING id, name
	`, name).Scan(&t.ID, &t.Name)
	return t, err
}

func (s *Store) CreateMedium(ctx context.Context, pathID int, name, color string) (Medium, error) {
	name = strings.TrimSpace(name)
	color = strings.TrimSpace(color)
	if pathID <= 0 {
		return Medium{}, &ValidationError{Msg: "请选择一条小径"}
	}
	if name == "" || utf8.RuneCountInString(name) > 24 {
		return Medium{}, &ValidationError{Msg: "载体名称需要在 1 到 24 个字之间"}
	}
	if color == "" {
		color = "#57534e"
	}
	if !hexColor.MatchString(color) {
		return Medium{}, &ValidationError{Msg: "颜色格式应为 #rrggbb"}
	}
	var m Medium
	err := s.pool.QueryRow(ctx, `
		INSERT INTO media (path_id, name, color) VALUES ($1, $2, $3)
		ON CONFLICT (path_id, name) DO UPDATE SET color = EXCLUDED.color
		RETURNING id, path_id, name, color
	`, pathID, name, color).Scan(&m.ID, &m.PathID, &m.Name, &m.Color)
	return m, mapDBErr(err)
}

func (s *Store) CreateVia(ctx context.Context, pathID int, name string) (Via, error) {
	name = strings.TrimSpace(name)
	if pathID <= 0 {
		return Via{}, &ValidationError{Msg: "请选择一条小径"}
	}
	if name == "" || utf8.RuneCountInString(name) > 24 {
		return Via{}, &ValidationError{Msg: "经由名称需要在 1 到 24 个字之间"}
	}
	var v Via
	err := s.pool.QueryRow(ctx, `
		INSERT INTO vias (path_id, name) VALUES ($1, $2)
		ON CONFLICT (path_id, name) DO UPDATE SET name = EXCLUDED.name
		RETURNING id, path_id, name
	`, pathID, name).Scan(&v.ID, &v.PathID, &v.Name)
	return v, mapDBErr(err)
}

func (s *Store) List(ctx context.Context, f Filter) ([]Entry, error) {
	uid, err := UserID(ctx)
	if err != nil {
		return nil, err
	}
	where, args, err := buildWhere(uid, f)
	if err != nil {
		return nil, err
	}
	sql := entrySelect + " WHERE " + where + " GROUP BY e.id, path.id, m.id, v.id " + orderBy(f.Sorts) + " LIMIT 5000"
	return s.queryEntries(ctx, sql, args)
}

func (s *Store) Get(ctx context.Context, id int) (Entry, error) {
	uid, err := UserID(ctx)
	if err != nil {
		return Entry{}, err
	}
	entries, err := s.queryEntries(ctx, entrySelect+` WHERE e.id = $1 AND e.user_id = $2 GROUP BY e.id, path.id, m.id, v.id`, []any{id, uid})
	if err != nil {
		return Entry{}, err
	}
	if len(entries) == 0 {
		return Entry{}, ErrNotFound
	}
	return entries[0], nil
}

func (s *Store) Create(ctx context.Context, in EntryInput) (Entry, error) {
	in, err := normalize(in)
	if err != nil {
		return Entry{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Entry{}, err
	}
	defer tx.Rollback(ctx)
	id, err := insertEntry(ctx, tx, in)
	if err != nil {
		return Entry{}, mapDBErr(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Entry{}, err
	}
	return s.Get(ctx, id)
}

func (s *Store) Update(ctx context.Context, id int, in EntryInput) (Entry, error) {
	in, err := normalize(in)
	if err != nil {
		return Entry{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Entry{}, err
	}
	defer tx.Rollback(ctx)
	if err := updateEntry(ctx, tx, id, in); err != nil {
		return Entry{}, mapDBErr(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Entry{}, err
	}
	return s.Get(ctx, id)
}

func (s *Store) Delete(ctx context.Context, id int) error {
	uid, err := UserID(ctx)
	if err != nil {
		return err
	}
	tag, err := s.pool.Exec(ctx, `DELETE FROM entries WHERE id = $1 AND user_id = $2`, id, uid)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) Stats(ctx context.Context, year, pathID int) (Stats, error) {
	start := fmt.Sprintf("%04d-01-01", year)
	end := fmt.Sprintf("%04d-01-01", year+1)
	out := Stats{
		Year:     year,
		ByMedium: []MediumCount{},
		ByMonth:  make([]MonthCount, 12),
		TopTags:  []NameCount{},
		Recent:   []Recent{},
		Days:     []DayMark{},
		Ranking:  []RankItem{},
		ByPath:   []PathShare{},
		ByBadge:  []NameCount{},
		ByVia:    []NameCount{},
	}
	for i := 0; i < 12; i++ {
		out.ByMonth[i] = MonthCount{Month: i + 1}
	}
	uid, err := UserID(ctx)
	if err != nil {
		return out, err
	}

	err = s.pool.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE status_kind = 'done')::int,
			COALESCE(SUM(spend_minutes) FILTER (WHERE status_kind = 'done'), 0)::int,
			COALESCE(SUM(finish_minutes) FILTER (WHERE status_kind = 'done'), 0)::int,
			AVG(rating) FILTER (WHERE status_kind = 'done' AND rating IS NOT NULL),
			COUNT(*) FILTER (WHERE status_kind = 'done' AND '白金' = ANY(badges))::int,
			COUNT(*) FILTER (WHERE status_kind = 'done' AND cardinality(badges) > 0)::int
		FROM entries
		WHERE finish_date >= $1::date AND finish_date < $2::date
			AND ($3 = 0 OR path_id = $3) AND user_id = $4
	`, start, end, pathID, uid).Scan(&out.Done, &out.SpendMinutes, &out.FinishMinutes, &out.AvgRating, &out.Platinum, &out.Marked)
	if err != nil {
		return out, err
	}
	if out.AvgRating != nil {
		v := math.Round(*out.AvgRating*10) / 10
		out.AvgRating = &v
	}
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*)::int FROM entries WHERE status_kind = 'doing' AND ($1 = 0 OR path_id = $1) AND user_id = $2`, pathID, uid).Scan(&out.Doing); err != nil {
		return out, err
	}
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*)::int FROM entries WHERE status_kind = 'want' AND ($1 = 0 OR path_id = $1) AND user_id = $2`, pathID, uid).Scan(&out.Want); err != nil {
		return out, err
	}

	mrows, err := s.pool.Query(ctx, `
		SELECT m.name, m.color, COUNT(*)::int
		FROM entries e
		JOIN media m ON m.id = e.medium_id
		WHERE e.status_kind = 'done' AND e.finish_date >= $1::date AND e.finish_date < $2::date
			AND ($3 = 0 OR e.path_id = $3) AND e.user_id = $4
		GROUP BY m.id
		ORDER BY COUNT(*) DESC, m.name
	`, start, end, pathID, uid)
	if err != nil {
		return out, err
	}
	defer mrows.Close()
	for mrows.Next() {
		var row MediumCount
		if err := mrows.Scan(&row.Name, &row.Color, &row.Count); err != nil {
			return out, err
		}
		out.ByMedium = append(out.ByMedium, row)
	}
	if err := mrows.Err(); err != nil {
		return out, err
	}

	monthRows, err := s.pool.Query(ctx, `
		SELECT EXTRACT(MONTH FROM finish_date)::int, COUNT(*)::int, COALESCE(SUM(spend_minutes), 0)::int
		FROM entries
		WHERE status_kind = 'done' AND finish_date >= $1::date AND finish_date < $2::date
			AND ($3 = 0 OR path_id = $3) AND user_id = $4
		GROUP BY 1
	`, start, end, pathID, uid)
	if err != nil {
		return out, err
	}
	defer monthRows.Close()
	for monthRows.Next() {
		var month, count, minutes int
		if err := monthRows.Scan(&month, &count, &minutes); err != nil {
			return out, err
		}
		if month >= 1 && month <= 12 {
			out.ByMonth[month-1].Count = count
			out.ByMonth[month-1].Minutes = minutes
		}
	}
	if err := monthRows.Err(); err != nil {
		return out, err
	}

	trows, err := s.pool.Query(ctx, `
		SELECT t.name, COUNT(*)::int
		FROM entries e
		JOIN entry_tags et ON et.entry_id = e.id
		JOIN tags t ON t.id = et.tag_id
		WHERE e.status_kind = 'done' AND e.finish_date >= $1::date AND e.finish_date < $2::date
			AND ($3 = 0 OR e.path_id = $3) AND e.user_id = $4
		GROUP BY t.id
		ORDER BY COUNT(*) DESC, t.name
		LIMIT 24
	`, start, end, pathID, uid)
	if err != nil {
		return out, err
	}
	defer trows.Close()
	for trows.Next() {
		var row NameCount
		if err := trows.Scan(&row.Name, &row.Count); err != nil {
			return out, err
		}
		out.TopTags = append(out.TopTags, row)
	}
	if err := trows.Err(); err != nil {
		return out, err
	}

	rrows, err := s.pool.Query(ctx, `
		SELECT e.title, to_char(e.finish_date, 'YYYY-MM-DD'), COALESCE(m.name, ''), p.name, e.rating
		FROM entries e
		JOIN paths p ON p.id = e.path_id
		LEFT JOIN media m ON m.id = e.medium_id
		WHERE e.status_kind = 'done' AND e.finish_date >= $1::date AND e.finish_date < $2::date
			AND ($3 = 0 OR e.path_id = $3) AND e.user_id = $4
		ORDER BY e.finish_date DESC, e.id DESC
		LIMIT 6
	`, start, end, pathID, uid)
	if err != nil {
		return out, err
	}
	defer rrows.Close()
	for rrows.Next() {
		var row Recent
		if err := rrows.Scan(&row.Title, &row.FinishDate, &row.Medium, &row.Path, &row.Rating); err != nil {
			return out, err
		}
		out.Recent = append(out.Recent, row)
	}
	if err := rrows.Err(); err != nil {
		return out, err
	}
	return out, s.enrichStats(ctx, &out, year, pathID, uid, start, end)
}

const entrySelect = `
SELECT
	e.id,
	e.title,
	e.notes,
	e.status_kind,
	e.spend_minutes,
	e.finish_minutes,
	COALESCE(to_char(e.release_date, 'YYYY-MM-DD'), ''),
	COALESCE(to_char(e.finish_date, 'YYYY-MM-DD'), ''),
	e.rating,
	e.badges,
	e.cover,
	e.cover_url,
	e.cover_landscape_url,
	e.progress_earned,
	e.progress_total,
	jsonb_build_object(
		'id', path.id,
		'slug', path.slug,
		'name', path.name,
		'mediumLabel', path.medium_label,
		'viaLabel', path.via_label,
		'releaseLabel', path.release_label,
		'spendLabel', path.spend_label,
		'finishTimeLabel', path.finish_time_label,
		'finishDateLabel', path.finish_date_label,
		'doneWord', path.done_word,
		'doingTitle', path.doing_title,
		'statusWant', path.status_want,
		'statusDoing', path.status_doing,
		'statusDone', path.status_done,
		'statusHold', path.status_hold,
		'badges', to_jsonb(path.badges)
	),
	m.id, m.path_id, m.name, m.color,
	v.id, v.path_id, v.name,
	COALESCE(
		jsonb_agg(jsonb_build_object('id', t.id, 'name', t.name) ORDER BY t.name)
		FILTER (WHERE t.id IS NOT NULL),
		'[]'::jsonb
	)
FROM entries e
JOIN paths path ON path.id = e.path_id
LEFT JOIN media m ON m.id = e.medium_id
LEFT JOIN vias v ON v.id = e.via_id
LEFT JOIN entry_tags et ON et.entry_id = e.id
LEFT JOIN tags t ON t.id = et.tag_id
`

func (s *Store) queryEntries(ctx context.Context, sql string, args []any) ([]Entry, error) {
	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Entry{}
	for rows.Next() {
		entry, err := scanEntry(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, entry)
	}
	return out, rows.Err()
}

func scanEntry(rows pgx.Rows) (Entry, error) {
	var entry Entry
	var finishMinutes *int32
	var badges []string
	var coverRaw []byte
	var pathRaw []byte
	var tagsRaw []byte
	var progressEarned, progressTotal *int32
	var mediumID, mediumPath *int32
	var mediumName, mediumColor *string
	var viaID, viaPath *int32
	var viaName *string
	err := rows.Scan(
		&entry.ID, &entry.Title, &entry.Notes, &entry.Status, &entry.SpendMinutes, &finishMinutes,
		&entry.ReleaseDate, &entry.FinishDate, &entry.Rating, &badges, &coverRaw, &entry.CoverURL, &entry.CoverLandscapeURL,
		&progressEarned, &progressTotal,
		&pathRaw,
		&mediumID, &mediumPath, &mediumName, &mediumColor,
		&viaID, &viaPath, &viaName,
		&tagsRaw,
	)
	if err != nil {
		return entry, err
	}
	if finishMinutes != nil {
		v := int(*finishMinutes)
		entry.FinishMinutes = &v
	}
	if badges == nil {
		badges = []string{}
	}
	entry.Badges = badges
	if progressEarned != nil {
		v := int(*progressEarned)
		entry.ProgressEarned = &v
	}
	if progressTotal != nil {
		v := int(*progressTotal)
		entry.ProgressTotal = &v
	}
	if len(coverRaw) > 0 {
		_ = json.Unmarshal(coverRaw, &entry.Cover)
	}
	if len(pathRaw) > 0 {
		if err := json.Unmarshal(pathRaw, &entry.Path); err != nil {
			return entry, err
		}
	}
	if entry.Path.Badges == nil {
		entry.Path.Badges = []string{}
	}
	entry.Tags = []Tag{}
	if len(tagsRaw) > 0 {
		if err := json.Unmarshal(tagsRaw, &entry.Tags); err != nil {
			return entry, err
		}
	}
	if entry.Tags == nil {
		entry.Tags = []Tag{}
	}
	if mediumID != nil && mediumPath != nil && mediumName != nil && mediumColor != nil {
		entry.Medium = &Medium{ID: int(*mediumID), PathID: int(*mediumPath), Name: *mediumName, Color: *mediumColor}
	}
	if viaID != nil && viaPath != nil && viaName != nil {
		entry.Via = &Via{ID: int(*viaID), PathID: int(*viaPath), Name: *viaName}
	}
	return entry, nil
}

func buildWhere(userID int, f Filter) (string, []any, error) {
	if f.Status != "" {
		if _, ok := statusSet[f.Status]; !ok {
			return "", nil, &ValidationError{Msg: "状态不正确"}
		}
	}
	switch f.Release {
	case "", "not_empty", "empty":
	default:
		return "", nil, &ValidationError{Msg: "面世日期筛选不正确"}
	}
	switch f.Finish {
	case "", "year", "month", "empty":
	default:
		return "", nil, &ValidationError{Msg: "抵达日期筛选不正确"}
	}

	parts := []string{}
	args := []any{}
	add := func(cond string, vals ...any) {
		for _, v := range vals {
			args = append(args, v)
			cond = strings.Replace(cond, "?", fmt.Sprintf("$%d", len(args)), 1)
		}
		parts = append(parts, cond)
	}
	add(`e.user_id = ?`, userID)

	if f.PathID > 0 {
		add(`e.path_id = ?`, f.PathID)
	}
	if q := strings.TrimSpace(f.Q); q != "" {
		add(`e.title ILIKE ? ESCAPE '\'`, likePattern(q))
	}
	if f.Status != "" {
		add(`e.status_kind = ?`, f.Status)
	}
	if f.Release == "not_empty" {
		parts = append(parts, "e.release_date IS NOT NULL")
	}
	if f.Release == "empty" {
		parts = append(parts, "e.release_date IS NULL")
	}
	now := time.Now()
	switch f.Finish {
	case "year":
		y := now.Year()
		add(`e.finish_date >= ?::date AND e.finish_date < ?::date`, fmt.Sprintf("%04d-01-01", y), fmt.Sprintf("%04d-01-01", y+1))
	case "month":
		start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local)
		end := start.AddDate(0, 1, 0)
		add(`e.finish_date >= ?::date AND e.finish_date < ?::date`, start.Format("2006-01-02"), end.Format("2006-01-02"))
	case "empty":
		parts = append(parts, "e.finish_date IS NULL")
	}
	if len(f.MediumIDs) > 0 {
		holders := make([]string, 0, len(f.MediumIDs))
		vals := make([]any, 0, len(f.MediumIDs))
		for _, id := range f.MediumIDs {
			if id > 0 {
				holders = append(holders, "?")
				vals = append(vals, id)
			}
		}
		if len(vals) > 0 {
			add("e.medium_id IN ("+strings.Join(holders, ",")+")", vals...)
		}
	}
	if len(f.TagIDs) > 0 {
		holders := make([]string, 0, len(f.TagIDs))
		vals := make([]any, 0, len(f.TagIDs))
		for _, id := range f.TagIDs {
			if id > 0 {
				holders = append(holders, "?")
				vals = append(vals, id)
			}
		}
		if len(vals) > 0 {
			add(`EXISTS (
				SELECT 1 FROM entry_tags et2
				WHERE et2.entry_id = e.id AND et2.tag_id IN (`+strings.Join(holders, ",")+`)
			)`, vals...)
		}
	}
	return strings.Join(parts, " AND "), args, nil
}

func likePattern(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return "%" + s + "%"
}

var sortColumns = map[string]string{
	"finish_date":    "e.finish_date",
	"release_date":   "e.release_date",
	"rating":         "e.rating",
	"spend_minutes":  "e.spend_minutes",
	"finish_minutes": "e.finish_minutes",
	"title":          "e.title",
}

func orderBy(sorts []Sort) string {
	parts := []string{}
	seen := map[string]struct{}{}
	for _, srt := range sorts {
		col, ok := sortColumns[srt.Field]
		if !ok {
			continue
		}
		if _, dup := seen[srt.Field]; dup {
			continue
		}
		seen[srt.Field] = struct{}{}
		dir := "ASC"
		if strings.EqualFold(srt.Dir, "desc") {
			dir = "DESC"
		}
		parts = append(parts, col+" "+dir+" NULLS LAST")
		if len(parts) == 3 {
			break
		}
	}
	if len(parts) == 0 {
		parts = append(parts, "e.finish_date DESC NULLS LAST")
	}
	parts = append(parts, "e.id DESC")
	return "ORDER BY " + strings.Join(parts, ", ")
}

func insertEntry(ctx context.Context, q querier, in EntryInput) (int, error) {
	cover, err := json.Marshal(in.Cover)
	if err != nil {
		return 0, err
	}
	release, err := parseDate(in.ReleaseDate)
	if err != nil {
		return 0, err
	}
	finished, err := parseDate(in.FinishDate)
	if err != nil {
		return 0, err
	}
	uid, err := UserID(ctx)
	if err != nil {
		return 0, err
	}
	var id int
	err = q.QueryRow(ctx, `
		INSERT INTO entries (
			path_id, title, medium_id, via_id, release_date, status_kind,
			spend_minutes, finish_minutes, finish_date, rating, badges, cover, cover_url, cover_landscape_url, notes, progress_earned, progress_total, user_id
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, $10, $11, $12::jsonb, $13, $14, $15, $16, $17, $18
		) RETURNING id
	`, in.PathID, in.Title, in.MediumID, in.ViaID, release, in.Status,
		in.SpendMinutes, in.FinishMinutes, finished, in.Rating, in.Badges, string(cover), in.CoverURL, in.CoverLandscapeURL, in.Notes, in.ProgressEarned, in.ProgressTotal, uid,
	).Scan(&id)
	if err != nil {
		return 0, err
	}
	if err := writeTags(ctx, q, id, in.TagIDs); err != nil {
		return 0, err
	}
	return id, nil
}

func updateEntry(ctx context.Context, q querier, id int, in EntryInput) error {
	cover, err := json.Marshal(in.Cover)
	if err != nil {
		return err
	}
	release, err := parseDate(in.ReleaseDate)
	if err != nil {
		return err
	}
	finished, err := parseDate(in.FinishDate)
	if err != nil {
		return err
	}
	uid, err := UserID(ctx)
	if err != nil {
		return err
	}
	tag, err := q.Exec(ctx, `
		UPDATE entries SET
			path_id = $1,
			title = $2,
			medium_id = $3,
			via_id = $4,
			release_date = $5,
			status_kind = $6,
			spend_minutes = $7,
			finish_minutes = $8,
			finish_date = $9,
			rating = $10,
			badges = $11,
			cover = $12::jsonb,
			cover_url = $13,
			cover_landscape_url = $14,
			notes = $15,
			progress_earned = $16,
			progress_total = $17,
			updated_at = now()
		WHERE id = $18 AND user_id = $19
	`, in.PathID, in.Title, in.MediumID, in.ViaID, release, in.Status,
		in.SpendMinutes, in.FinishMinutes, finished, in.Rating, in.Badges, string(cover), in.CoverURL, in.CoverLandscapeURL, in.Notes, in.ProgressEarned, in.ProgressTotal, id, uid,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	if _, err := q.Exec(ctx, `DELETE FROM entry_tags WHERE entry_id = $1`, id); err != nil {
		return err
	}
	return writeTags(ctx, q, id, in.TagIDs)
}

func writeTags(ctx context.Context, q querier, entryID int, tagIDs []int) error {
	for _, tagID := range tagIDs {
		if _, err := q.Exec(ctx, `INSERT INTO entry_tags (entry_id, tag_id) VALUES ($1, $2)`, entryID, tagID); err != nil {
			return err
		}
	}
	return nil
}

func parseDate(s string) (*time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return nil, &ValidationError{Msg: "日期格式应为 YYYY-MM-DD"}
	}
	return &t, nil
}

func normalize(in EntryInput) (EntryInput, error) {
	in.Title = strings.TrimSpace(in.Title)
	in.Notes = strings.TrimSpace(in.Notes)
	in.CoverURL = strings.TrimSpace(in.CoverURL)
	in.CoverLandscapeURL = strings.TrimSpace(in.CoverLandscapeURL)
	in.Status = strings.TrimSpace(in.Status)
	if in.Status == "" {
		in.Status = "want"
	}
	if utf8.RuneCountInString(in.Title) < 1 || utf8.RuneCountInString(in.Title) > 80 {
		return in, &ValidationError{Msg: "请填写名称（不超过 80 个字）"}
	}
	if _, ok := statusSet[in.Status]; !ok {
		return in, &ValidationError{Msg: "状态不正确"}
	}
	if in.PathID <= 0 {
		return in, &ValidationError{Msg: "请选择一条小径"}
	}
	if in.MediumID <= 0 {
		return in, &ValidationError{Msg: "请选择载体"}
	}
	if in.ViaID != nil && *in.ViaID <= 0 {
		in.ViaID = nil
	}
	if in.SpendMinutes < 0 || in.SpendMinutes > 200000 {
		return in, &ValidationError{Msg: "耗时不正确"}
	}
	if in.FinishMinutes != nil && (*in.FinishMinutes < 0 || *in.FinishMinutes > 200000) {
		return in, &ValidationError{Msg: "抵达用时不正确"}
	}
	if utf8.RuneCountInString(in.Notes) > 4000 {
		return in, &ValidationError{Msg: "笔记过长"}
	}
	if in.Rating != nil {
		r := *in.Rating
		if r < 0 || r > 5 {
			return in, &ValidationError{Msg: "评分需要在 0 到 5 之间"}
		}
		rounded := math.Round(r*2) / 2
		in.Rating = &rounded
	}
	in.CoverURL = strings.TrimSpace(in.CoverURL)
	in.CoverLandscapeURL = strings.TrimSpace(in.CoverLandscapeURL)
	if len(in.CoverURL) > 500 || len(in.CoverLandscapeURL) > 500 {
		return in, &ValidationError{Msg: "封面地址过长"}
	}
	if !hexColor.MatchString(in.Cover.A) || !hexColor.MatchString(in.Cover.B) || !validMotif(in.Cover.Motif) {
		in.Cover = fallbackCover(in.Title)
	}
	in.TagIDs = uniquePositive(in.TagIDs)
	in.Badges = cleanBadges(in.Badges)
	if in.Badges == nil {
		in.Badges = []string{}
	}
	return in, nil
}

func uniquePositive(ids []int) []int {
	seen := map[int]struct{}{}
	out := make([]int, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func cleanBadges(badges []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(badges))
	for _, b := range badges {
		b = strings.TrimSpace(b)
		if b == "" || utf8.RuneCountInString(b) > 12 {
			continue
		}
		if _, ok := seen[b]; ok {
			continue
		}
		seen[b] = struct{}{}
		out = append(out, b)
		if len(out) == 6 {
			break
		}
	}
	return out
}

func validMotif(s string) bool {
	for _, m := range motifList {
		if m == s {
			return true
		}
	}
	return false
}

func fallbackCover(title string) Cover {
	h := 0
	for _, r := range title {
		h = h*31 + int(r)
	}
	if h < 0 {
		h = -h
	}
	return fallbackCovers[h%len(fallbackCovers)]
}

func mapDBErr(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23503" {
		return &ValidationError{Msg: "小径、载体或标签不存在"}
	}
	return err
}
