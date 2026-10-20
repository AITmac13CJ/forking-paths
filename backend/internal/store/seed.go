package store

import (
	"context"
	"fmt"
)

func (s *Store) Seed(ctx context.Context) (int, error) {
	if err := s.EnsureCatalog(ctx); err != nil {
		return 0, err
	}
	groups := []struct {
		slug  string
		items []demo
	}{
		{"games", stamp("games", demoGames())},
		{"reading", readingDemos()},
		{"film", filmDemos()},
		{"music", musicDemos()},
	}
	total := 0
	for _, group := range groups {
		n, err := s.seedPath(ctx, group.slug, group.items)
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

func stamp(path string, items []demo) []demo {
	for i := range items {
		items[i].path = path
	}
	return items
}

func (s *Store) seedPath(ctx context.Context, slug string, items []demo) (int, error) {
	var n int
	if err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*)::int FROM entries e
		JOIN paths p ON p.id = e.path_id
		WHERE p.slug = $1
	`, slug).Scan(&n); err != nil {
		return 0, err
	}
	if n > 0 {
		return 0, nil
	}
	paths, err := s.nameIDs(ctx, "paths")
	if err != nil {
		return 0, err
	}
	media, err := s.keyedIDs(ctx, "media")
	if err != nil {
		return 0, err
	}
	vias, err := s.keyedIDs(ctx, "vias")
	if err != nil {
		return 0, err
	}
	tags, err := s.nameIDs(ctx, "tags")
	if err != nil {
		return 0, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	count := 0
	for _, item := range items {
		in, err := item.toInput(paths, media, vias, tags)
		if err != nil {
			return 0, err
		}
		in, err = normalize(in)
		if err != nil {
			return 0, err
		}
		if _, err := insertEntry(ctx, tx, in); err != nil {
			return 0, err
		}
		count++
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *Store) nameIDs(ctx context.Context, table string) (map[string]int, error) {
	var q string
	switch table {
	case "paths":
		q = `SELECT id, slug FROM paths`
	case "tags":
		q = `SELECT id, name FROM tags`
	default:
		return nil, fmt.Errorf("未知目录 %s", table)
	}
	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var id int
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		out[name] = id
	}
	return out, rows.Err()
}

func (s *Store) keyedIDs(ctx context.Context, table string) (map[string]int, error) {
	var q string
	switch table {
	case "media":
		q = `SELECT p.slug || '/' || m.name, m.id FROM media m JOIN paths p ON p.id = m.path_id`
	case "vias":
		q = `SELECT p.slug || '/' || v.name, v.id FROM vias v JOIN paths p ON p.id = v.path_id`
	default:
		return nil, fmt.Errorf("未知目录 %s", table)
	}
	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var id int
		var key string
		if err := rows.Scan(&key, &id); err != nil {
			return nil, err
		}
		out[key] = id
	}
	return out, rows.Err()
}

type demo struct {
	path     string
	title    string
	platform string
	device   string
	tags     []string
	release  string
	status   string
	playH    int
	playM    int
	clearH   int
	clearM   int
	clearOn  string
	rating   float64
	badges   []string
	cover    Cover
	notes    string
}

func (d demo) toInput(paths, media, vias, tags map[string]int) (EntryInput, error) {
	pathID, ok := paths[d.path]
	if !ok {
		return EntryInput{}, fmt.Errorf("缺少小径：%s", d.path)
	}
	mediumID, ok := media[d.path+"/"+d.platform]
	if !ok {
		return EntryInput{}, fmt.Errorf("缺少载体：%s/%s", d.path, d.platform)
	}
	var viaID *int
	if d.device != "" {
		id, ok := vias[d.path+"/"+d.device]
		if !ok {
			return EntryInput{}, fmt.Errorf("缺少经由：%s/%s", d.path, d.device)
		}
		viaID = &id
	}
	tagIDs := make([]int, 0, len(d.tags))
	for _, name := range d.tags {
		id, ok := tags[name]
		if !ok {
			return EntryInput{}, fmt.Errorf("缺少标签：%s", name)
		}
		tagIDs = append(tagIDs, id)
	}
	play := d.playH*60 + d.playM
	var clear *int
	if d.clearOn != "" {
		v := d.clearH*60 + d.clearM
		clear = &v
	}
	var rating *float64
	if d.rating >= 0 {
		r := d.rating
		rating = &r
	}
	return EntryInput{
		Title:         d.title,
		PathID:        pathID,
		MediumID:      mediumID,
		ViaID:         viaID,
		TagIDs:        tagIDs,
		ReleaseDate:   d.release,
		Status:        d.status,
		SpendMinutes:  play,
		FinishMinutes: clear,
		FinishDate:    d.clearOn,
		Rating:        rating,
		Badges:        d.badges,
		Cover:         d.cover,
		Notes:         d.notes,
	}, nil
}

func done(title, platform, device, release, clearOn string, playH, playM, clearH, clearM int, rating float64, badges, tags []string, cover Cover, notes string) demo {
	return demo{
		title: title, platform: platform, device: device, tags: tags,
		release: release, status: "done",
		playH: playH, playM: playM, clearH: clearH, clearM: clearM, clearOn: clearOn,
		rating: rating, badges: badges, cover: cover, notes: notes,
	}
}

func open(status, title, platform, device, release string, playH, playM int, rating float64, tags []string, cover Cover, notes string) demo {
	return demo{
		title: title, platform: platform, device: device, tags: tags,
		release: release, status: status,
		playH: playH, playM: playM, rating: rating, cover: cover, notes: notes,
	}
}

func demoGames() []demo {
	return []demo{
		done("银河战士 究极", "Switch", "Switch 2", "2025-03-18", "2026-10-02", 12, 40, 11, 5, 5, nil, []string{"动作冒险"}, Cover{A: "#14532d", B: "#86efac", Motif: "leaf"}, ""),
		done("双截龙 外传", "Switch", "Switch 2", "2025-06-12", "2026-10-09", 9, 20, 9, 20, 3.5, nil, []string{"动作"}, Cover{A: "#9a3412", B: "#fdba74", Motif: "fox"}, ""),
		done("地平线 西之绝境", "PS5", "PS5", "2022-02-18", "2026-10-12", 48, 10, 36, 0, 4.5, nil, []string{"动作冒险"}, Cover{A: "#0f172a", B: "#818cf8", Motif: "galaxy"}, ""),
		done("咚奇刚 热带冻结", "Switch", "Switch 2", "2018-05-03", "2026-08-30", 23, 34, 23, 34, 4, nil, []string{"2D平台"}, Cover{A: "#f4a261", B: "#1d3557", Motif: "sun"}, ""),
		done("火焰之纹章无双 风花雪月", "Switch", "Switch 2", "2022-06-24", "2026-08-29", 41, 1, 41, 1, 5, nil, []string{"砍杀"}, Cover{A: "#6d28d9", B: "#e9d5ff", Motif: "crest"}, ""),
		done("斯普拉遁 涂击队", "Switch 2", "Switch 2", "2026-07-23", "2026-08-21", 14, 43, 13, 54, 4, nil, []string{"动作冒险"}, Cover{A: "#06d6a0", B: "#118ab2", Motif: "ink"}, ""),
		done("沙罗周期", "PS5", "PS Portal", "2026-04-30", "2026-08-13", 64, 27, 28, 39, 4, []string{"白金"}, []string{"弹幕射击", "动作", "肉鸽"}, Cover{A: "#111827", B: "#ea580c", Motif: "eclipse"}, "弹幕周目，顺手拿了白金。"),
		done("重力异想世界", "PS Vita", "PS Vita 2000", "2012-02-09", "2026-08-05", 16, 20, 21, 2, 4, []string{"白金"}, []string{"动作冒险"}, Cover{A: "#4c1d95", B: "#38bdf8", Motif: "galaxy"}, ""),
		done("龙之皇冠", "PS Vita", "PS Vita 2000", "2013-07-25", "2026-07-29", 39, 59, 15, 46, 5, []string{"两周目"}, []string{"动作冒险"}, Cover{A: "#b45309", B: "#fde68a", Motif: "crown"}, ""),
		done("洛克人X传奇合集 2", "Steam", "Rog Xbox Ally X", "2018-07-24", "2026-07-21", 27, 55, 27, 55, 4, nil, []string{"2D平台", "动作"}, Cover{A: "#1e3a8a", B: "#60a5fa", Motif: "mech"}, ""),
		done("星际火狐", "Switch 2", "Switch 2", "2026-06-25", "2026-07-16", 23, 37, 2, 16, 5, []string{"全收集", "五周目"}, []string{"轨道射击"}, Cover{A: "#fb7185", B: "#1f2937", Motif: "fox"}, ""),
		done("超级马力欧银河 2", "Switch", "Switch 2", "2026-09-30", "2026-07-14", 31, 28, 13, 13, 5, []string{"全收集"}, []string{"3D平台"}, Cover{A: "#0284c7", B: "#bae6fd", Motif: "sky"}, ""),
		done("空之轨迹 the 1st", "Switch 2", "Switch 2", "2025-09-19", "2026-07-12", 87, 1, 44, 13, 5, []string{"全收集", "三周目"}, []string{"日式角色扮演"}, Cover{A: "#312e81", B: "#f9a8d4", Motif: "castle"}, "三周目全收集。"),
		done("洛克人X传奇合集", "Steam", "Rog Xbox Ally X", "2018-07-24", "2026-07-06", 42, 51, 42, 51, 4.5, nil, []string{"2D平台", "动作"}, Cover{A: "#7c3aed", B: "#22d3ee", Motif: "mech"}, ""),
		done("Windswept", "Switch", "Switch 2", "2025-11-11", "2026-07-05", 8, 57, 8, 57, 4, nil, []string{"2D平台", "类DKC"}, Cover{A: "#3f6212", B: "#d9f99d", Motif: "wind"}, ""),
		done("哈迪斯 2", "Steam", "Rog Xbox Ally X", "2025-09-25", "2026-06-28", 32, 10, 28, 0, 5, []string{"全收集"}, []string{"动作", "肉鸽"}, Cover{A: "#7f1d1d", B: "#fecaca", Motif: "crown"}, ""),
		done("星之卡比 探索发现", "Switch", "Switch 2", "2022-03-25", "2026-06-18", 14, 22, 12, 5, 4, nil, []string{"3D平台"}, Cover{A: "#f472b6", B: "#fdf2f8", Motif: "sun"}, ""),
		done("女神异闻录 5 皇家版", "PS5", "PS Portal", "2022-10-21", "2026-05-30", 110, 40, 98, 20, 5, []string{"白金"}, []string{"日式角色扮演"}, Cover{A: "#1e1b4b", B: "#fbbf24", Motif: "crest"}, ""),
		done("蔚蓝", "Steam", "Steam Deck", "2018-01-25", "2026-05-11", 18, 45, 7, 30, 5, []string{"全收集"}, []string{"2D平台"}, Cover{A: "#0369a1", B: "#ffffff", Motif: "sky"}, ""),
		done("怪物猎人 荒野", "PS5", "PS5", "2025-02-28", "2026-04-19", 76, 15, 54, 0, 4.5, nil, []string{"动作"}, Cover{A: "#3f6212", B: "#a3e635", Motif: "mech"}, ""),
		done("匹诺曹的谎言", "PS5", "PS5", "2023-09-19", "2026-03-22", 36, 50, 32, 10, 4, nil, []string{"动作冒险"}, Cover{A: "#3f1d2e", B: "#fb7185", Motif: "leaf"}, ""),
		done("塞尔达传说 王国之泪", "Switch", "Switch", "2023-05-12", "2025-11-20", 95, 0, 80, 0, 5, nil, []string{"动作冒险"}, Cover{A: "#0369a1", B: "#fde047", Motif: "sky"}, ""),
		done("只狼", "PS5", "PS5", "2019-03-22", "2025-08-14", 38, 20, 28, 40, 5, nil, []string{"动作"}, Cover{A: "#334155", B: "#94a3b8", Motif: "ink"}, ""),
		open("doing", "艾尔登法环", "PS5", "PS5", "2022-02-25", 62, 15, 5, []string{"动作冒险", "角色扮演"}, Cover{A: "#3f0d12", B: "#f43f5e", Motif: "eclipse"}, "还在爬树。"),
		open("doing", "博德之门 3", "Steam", "Steam Deck", "2023-08-03", 40, 5, -1, []string{"角色扮演"}, Cover{A: "#4a044e", B: "#e879f9", Motif: "crest"}, ""),
		open("doing", "集合啦 动物森友会", "Switch", "Switch", "2020-03-20", 120, 0, 4, []string{"生活模拟"}, Cover{A: "#16a34a", B: "#bbf7d0", Motif: "sun"}, ""),
		open("want", "对马岛之魂", "PS5", "", "2020-07-17", 0, 0, -1, []string{"动作冒险"}, Cover{A: "#44403c", B: "#d6d3d1", Motif: "castle"}, ""),
		open("hold", "死亡搁浅 2", "PS5", "PS5", "2025-06-26", 6, 10, -1, []string{"动作冒险"}, Cover{A: "#52525b", B: "#e4e4e7", Motif: "wind"}, "先放到一边。"),
	}
}

func readingDemos() []demo {
	items := []demo{
		done("小径分岔的花园", "纸书", "枕边", "1941-01-01", "2026-09-02", 4, 30, 4, 30, 5, nil, []string{"短篇", "哲学"}, Cover{A: "#3f6212", B: "#ecfccb", Motif: "leaf"}, "博尔赫斯。这座花园的名字从这里来。"),
		done("看不见的城市", "纸书", "书桌", "1972-01-01", "2026-04-18", 8, 10, 8, 10, 5, []string{"摘抄"}, []string{"小说"}, Cover{A: "#0f766e", B: "#99f6e4", Motif: "sky"}, "卡尔维诺。一座城说出许多座城。"),
		done("金阁寺", "纸书", "枕边", "1956-01-01", "2025-12-02", 9, 40, 9, 40, 4.5, nil, []string{"小说"}, Cover{A: "#7c2d12", B: "#fed7aa", Motif: "castle"}, ""),
		open("doing", "小说的艺术", "纸书", "书桌", "1986-01-01", 3, 20, -1, []string{"散文"}, Cover{A: "#1e3a8a", B: "#bfdbfe", Motif: "crest"}, "昆德拉。读到一半，先放下。"),
		open("want", "爱达或爱欲", "纸书", "", "1969-01-01", 0, 0, -1, []string{"小说"}, Cover{A: "#4c1d95", B: "#ddd6fe", Motif: "galaxy"}, ""),
	}
	return stamp("reading", items)
}

func filmDemos() []demo {
	items := []demo{
		done("花样年华", "流媒体", "家里", "2000-09-29", "2026-02-14", 1, 38, 1, 38, 5, []string{"二刷"}, []string{"电影", "华语", "剧情"}, Cover{A: "#9f1239", B: "#fecdd3", Motif: "leaf"}, "王家卫。走廊里的余响。"),
		done("一一", "流媒体", "家里", "2000-05-15", "2026-01-20", 2, 53, 2, 53, 5, nil, []string{"电影", "华语", "剧情"}, Cover{A: "#334155", B: "#e2e8f0", Motif: "sky"}, "杨德昌。"),
		done("星际穿越", "院线", "电影院", "2014-11-07", "2025-08-03", 2, 49, 2, 49, 4.5, []string{"影院"}, []string{"电影", "科幻"}, Cover{A: "#0f172a", B: "#38bdf8", Motif: "galaxy"}, ""),
		open("doing", "重庆森林", "蓝光", "家里", "1994-07-14", 0, 36, -1, []string{"电影", "华语"}, Cover{A: "#7f1d1d", B: "#fda4af", Motif: "eclipse"}, "王家卫。这次看蓝光。"),
	}
	return stamp("film", items)
}

func musicDemos() []demo {
	items := []demo{
		done("In Rainbows", "流媒体", "耳机", "2007-10-10", "2026-03-08", 6, 20, 0, 43, 5, []string{"循环"}, []string{"摇滚"}, Cover{A: "#14532d", B: "#bbf7d0", Motif: "wind"}, "听完之后又循环了很久。"),
		done("范特西", "流媒体", "家里", "2001-09-14", "2026-01-11", 4, 10, 0, 58, 5, nil, []string{"流行"}, Cover{A: "#166534", B: "#fef9c3", Motif: "sun"}, ""),
		open("doing", "Kind of Blue", "黑胶", "家里", "1959-08-17", 1, 10, -1, []string{"爵士"}, Cover{A: "#1e3a8a", B: "#bfdbfe", Motif: "ink"}, ""),
	}
	return stamp("music", items)
}
