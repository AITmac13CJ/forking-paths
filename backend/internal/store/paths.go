package store

import "context"

type mediumDef struct {
	Name  string
	Color string
}

type pathDef struct {
	Slug            string
	Name            string
	MediumLabel     string
	ViaLabel        string
	ReleaseLabel    string
	SpendLabel      string
	FinishTimeLabel string
	FinishDateLabel string
	DoneWord        string
	DoingTitle      string
	StatusWant      string
	StatusDoing     string
	StatusDone      string
	StatusHold      string
	Badges          []string
	Media           []mediumDef
	Vias            []string
	Tags            []string
}

func catalog() []pathDef {
	return []pathDef{
		{
			Slug: "games", Name: "游戏",
			MediumLabel: "游玩平台", ViaLabel: "游玩设备", ReleaseLabel: "发售日期",
			SpendLabel: "累计时长", FinishTimeLabel: "通关时长", FinishDateLabel: "通关日期",
			DoneWord: "通关", DoingTitle: "持续游玩",
			StatusWant: "想玩", StatusDoing: "游玩中", StatusDone: "已通关", StatusHold: "搁置",
			Badges: []string{"白金", "全收集", "全成就", "一周目", "两周目", "三周目", "五周目"},
			Media: []mediumDef{
				{"Switch", "#e11d48"},
				{"Switch 2", "#be123c"},
				{"PS5", "#2563eb"},
				{"PS Vita", "#4f46e5"},
				{"Steam", "#475569"},
				{"PS4", "#1d4ed8"},
				{"Xbox", "#107C10"},
			},
			Vias: []string{"Switch", "Switch 2", "PS5", "PS Portal", "PS Vita 2000", "Rog Xbox Ally X", "Steam Deck"},
			Tags: []string{"2D平台", "3D平台", "砍杀", "动作冒险", "弹幕射击", "动作", "肉鸽", "轨道射击", "日式角色扮演", "类DKC", "角色扮演", "生活模拟"},
		},
		{
			Slug: "reading", Name: "书籍",
			MediumLabel: "版本", ViaLabel: "读于", ReleaseLabel: "出版日期",
			SpendLabel: "阅读时长", FinishTimeLabel: "读完用时", FinishDateLabel: "读完日期",
			DoneWord: "读完", DoingTitle: "仍在读",
			StatusWant: "想读", StatusDoing: "在读", StatusDone: "已读完", StatusHold: "搁下",
			Badges: []string{"重读", "摘抄", "藏书"},
			Media: []mediumDef{
				{"纸书", "#9a3412"},
				{"电子书", "#0369a1"},
				{"有声书", "#65a30d"},
			},
			Vias: []string{"枕边", "通勤", "书桌"},
			Tags: []string{"小说", "散文", "诗", "哲学", "短篇"},
		},
		{
			Slug: "film", Name: "电影",
			MediumLabel: "观看渠道", ViaLabel: "看于", ReleaseLabel: "上映日期",
			SpendLabel: "观看时长", FinishTimeLabel: "看完用时", FinishDateLabel: "看完日期",
			DoneWord: "看完", DoingTitle: "仍在看",
			StatusWant: "想看", StatusDoing: "在看", StatusDone: "已看完", StatusHold: "搁下",
			Badges: []string{"影院", "二刷", "导演剪辑"},
			Media: []mediumDef{
				{"院线", "#b45309"},
				{"流媒体", "#1d4ed8"},
				{"蓝光", "#334155"},
			},
			Vias: []string{"电影院", "家里", "旅途"},
			Tags: []string{"电影", "华语", "剧情", "科幻"},
		},
		{
			Slug: "music", Name: "音乐",
			MediumLabel: "载体", ViaLabel: "听于", ReleaseLabel: "发行日期",
			SpendLabel: "累计时长", FinishTimeLabel: "听完用时", FinishDateLabel: "听完日期",
			DoneWord: "听完", DoingTitle: "仍在听",
			StatusWant: "想听", StatusDoing: "在听", StatusDone: "已听完", StatusHold: "搁下",
			Badges: []string{"循环", "现场", "黑胶"},
			Media: []mediumDef{
				{"流媒体", "#166534"},
				{"黑胶", "#14532d"},
				{"CD", "#4d7c0f"},
			},
			Vias: []string{"耳机", "家里", "通勤"},
			Tags: []string{"流行", "摇滚", "爵士", "古典", "原声"},
		},
	}
}

func (s *Store) EnsureCatalog(ctx context.Context) error {
	for i, def := range catalog() {
		var id int
		err := s.pool.QueryRow(ctx, `
			INSERT INTO paths (
				slug, name, sort_order,
				medium_label, via_label, release_label, spend_label,
				finish_time_label, finish_date_label, done_word, doing_title,
				status_want, status_doing, status_done, status_hold, badges
			) VALUES (
				$1,$2,$3,
				$4,$5,$6,$7,
				$8,$9,$10,$11,
				$12,$13,$14,$15,$16
			)
			ON CONFLICT (slug) DO UPDATE SET
				name = EXCLUDED.name,
				sort_order = EXCLUDED.sort_order,
				medium_label = EXCLUDED.medium_label,
				via_label = EXCLUDED.via_label,
				release_label = EXCLUDED.release_label,
				spend_label = EXCLUDED.spend_label,
				finish_time_label = EXCLUDED.finish_time_label,
				finish_date_label = EXCLUDED.finish_date_label,
				done_word = EXCLUDED.done_word,
				doing_title = EXCLUDED.doing_title,
				status_want = EXCLUDED.status_want,
				status_doing = EXCLUDED.status_doing,
				status_done = EXCLUDED.status_done,
				status_hold = EXCLUDED.status_hold,
				badges = EXCLUDED.badges
			RETURNING id
		`, def.Slug, def.Name, i+1,
			def.MediumLabel, def.ViaLabel, def.ReleaseLabel, def.SpendLabel,
			def.FinishTimeLabel, def.FinishDateLabel, def.DoneWord, def.DoingTitle,
			def.StatusWant, def.StatusDoing, def.StatusDone, def.StatusHold, def.Badges,
		).Scan(&id)
		if err != nil {
			return err
		}
		for j, medium := range def.Media {
			if _, err := s.pool.Exec(ctx, `
				INSERT INTO media (path_id, name, color, sort_order) VALUES ($1,$2,$3,$4)
				ON CONFLICT (path_id, name) DO UPDATE SET color = EXCLUDED.color, sort_order = EXCLUDED.sort_order
			`, id, medium.Name, medium.Color, j+1); err != nil {
				return err
			}
		}
		for _, via := range def.Vias {
			if _, err := s.pool.Exec(ctx, `
				INSERT INTO vias (path_id, name) VALUES ($1,$2) ON CONFLICT (path_id, name) DO NOTHING
			`, id, via); err != nil {
				return err
			}
		}
		for _, tag := range def.Tags {
			if _, err := s.pool.Exec(ctx, `
				INSERT INTO tags (name) VALUES ($1) ON CONFLICT (name) DO NOTHING
			`, tag); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Store) ImportLegacy(ctx context.Context) error {
	if err := s.EnsureCatalog(ctx); err != nil {
		return err
	}
	var reg *string
	if err := s.pool.QueryRow(ctx, `SELECT to_regclass('public.games')::text`).Scan(&reg); err != nil {
		return err
	}
	if reg == nil || *reg == "" {
		return nil
	}

	var pathID int
	if err := s.pool.QueryRow(ctx, `SELECT id FROM paths WHERE slug = 'games'`).Scan(&pathID); err != nil {
		return err
	}
	var existing int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*)::int FROM entries WHERE path_id = $1`, pathID).Scan(&existing); err != nil {
		return err
	}
	if existing == 0 {
		tx, err := s.pool.Begin(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback(ctx)
		if _, err := tx.Exec(ctx, `
			INSERT INTO entries (
				id, path_id, title, medium_id, via_id, release_date, status_kind,
				spend_minutes, finish_minutes, finish_date, rating, badges, cover, cover_url, notes,
				created_at, updated_at
			)
			SELECT
				g.id, $1, g.title, m.id, v.id, g.release_date,
				CASE g.status
					WHEN '已通关' THEN 'done'
					WHEN '游玩中' THEN 'doing'
					WHEN '搁置' THEN 'hold'
					ELSE 'want'
				END,
				g.play_minutes, g.clear_minutes, g.clear_date, g.rating, g.badges, g.cover, g.cover_url, g.notes,
				g.created_at, g.updated_at
			FROM games g
			LEFT JOIN platforms p ON p.id = g.platform_id
			LEFT JOIN media m ON m.path_id = $1 AND m.name = p.name
			LEFT JOIN devices d ON d.id = g.device_id
			LEFT JOIN vias v ON v.path_id = $1 AND v.name = d.name
		`, pathID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO entry_tags (entry_id, tag_id)
			SELECT game_id, tag_id FROM game_tags
			ON CONFLICT DO NOTHING
		`); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			SELECT setval(pg_get_serial_sequence('entries', 'id'), GREATEST((SELECT COALESCE(MAX(id), 1) FROM entries), 1))
		`); err != nil {
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
	}
	for _, table := range []string{"game_tags", "games", "platforms", "devices"} {
		if _, err := s.pool.Exec(ctx, `DROP TABLE IF EXISTS `+table); err != nil {
			return err
		}
	}
	return nil
}
