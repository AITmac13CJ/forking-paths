package platform

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Game struct {
	Ref      string `json:"ref"`
	Title    string `json:"title"`
	Minutes  int    `json:"minutes"`
	Earned   int    `json:"earned"`
	Total    int    `json:"total"`
	Complete bool   `json:"complete"`
	Platinum bool   `json:"platinum"`
	SCID     string `json:"-"`
}

type Client struct {
	HTTP *http.Client
}

func NewClient() *Client {
	return &Client{HTTP: newSteamHTTP()}
}

type Report func(done, total int, message string)

func (c *Client) Fetch(ctx context.Context, provider, account, secret string, report Report) ([]Game, error) {
	switch provider {
	case "steam":
		return c.steam(ctx, account, secret, report)
	case "psn":
		if report != nil {
			report(0, 0, "正在读取奖杯")
		}
		return c.psn(ctx, secret)
	case "xbox":
		if report != nil {
			report(0, 0, "正在读取成就")
		}
		return c.xbox(ctx, secret, report)
	case "switch":
		if report != nil {
			report(0, 0, "正在读取游玩记录")
		}
		return c.nintendo(ctx, secret)
	default:
		return nil, fmt.Errorf("未知的平台")
	}
}

func (c *Client) steam(ctx context.Context, account, secret string, report Report) ([]Game, error) {
	var cred struct {
		SteamID string `json:"steam_id"`
		Access  string `json:"access_token"`
	}
	_ = json.Unmarshal([]byte(secret), &cred)
	if cred.SteamID != "" && cred.Access != "" {
		return c.steamWeb(ctx, cred.SteamID, cred.Access, report)
	}
	if cred.SteamID != "" {
		if key := strings.TrimSpace(os.Getenv("STEAM_WEB_API_KEY")); looksLikeAPIKey(key) {
			return c.steamWeb(ctx, cred.SteamID, key, report)
		}
		return nil, fmt.Errorf("Steam 已经登录，网站还读不到这个账号的游戏库")
	}
	return nil, fmt.Errorf("请重新登录 Steam")
}

func (c *Client) steamPublic(ctx context.Context, steamID string, report Report) ([]Game, error) {
	if report != nil {
		report(0, 0, "正在读取 Steam 游戏库")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://steamcommunity.com/profiles/"+url.PathEscape(steamID)+"/games?tab=all&xml=1", nil)
	if err != nil {
		return nil, fmt.Errorf("Steam 没有返回游戏库，请再试一次")
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Steam 没有返回游戏库，请再试一次")
	}
	defer res.Body.Close()
	var doc struct {
		XMLName xml.Name `xml:"gamesList"`
		Error   string   `xml:"error"`
		Games   []struct {
			AppID int     `xml:"appID"`
			Name  string  `xml:"name"`
			Hours float64 `xml:"hoursOnRecord"`
		} `xml:"games>game"`
	}
	if xml.NewDecoder(io.LimitReader(res.Body, 20<<20)).Decode(&doc) != nil {
		return nil, fmt.Errorf("Steam 没有返回游戏库，请再试一次")
	}
	if strings.TrimSpace(doc.Error) != "" || len(doc.Games) == 0 {
		return nil, fmt.Errorf("Steam 游戏详情没有公开，网站登录读不到游戏库")
	}
	out := make([]Game, 0, len(doc.Games))
	for _, game := range doc.Games {
		if game.AppID <= 0 || strings.TrimSpace(game.Name) == "" {
			continue
		}
		minutes := int(game.Hours * 60)
		out = append(out, Game{Ref: strconv.Itoa(game.AppID), Title: strings.TrimSpace(game.Name), Minutes: minutes})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Minutes > out[j].Minutes })
	c.steamPublicAchievements(ctx, steamID, out, report)
	return out, nil
}

func (c *Client) steamPublicAchievements(ctx context.Context, steamID string, games []Game, report Report) {
	indexes := make([]int, 0)
	for i := range games {
		if games[i].Minutes > 0 {
			indexes = append(indexes, i)
		}
	}
	if len(indexes) == 0 {
		return
	}
	workers := 4
	if len(indexes) < workers {
		workers = len(indexes)
	}
	jobs := make(chan int)
	var wg sync.WaitGroup
	var mu sync.Mutex
	done := 0
	for n := 0; n < workers; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				earned, total := c.steamPublicAchievement(ctx, steamID, games[i].Ref)
				mu.Lock()
				done++
				if total > 0 {
					games[i].Earned = earned
					games[i].Total = total
					games[i].Complete = earned >= total
				}
				if report != nil {
					report(done, len(indexes), "正在读取成就")
				}
				mu.Unlock()
			}
		}()
	}
	for _, i := range indexes {
		select {
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return
		case jobs <- i:
		}
	}
	close(jobs)
	wg.Wait()
}

func (c *Client) steamPublicAchievement(ctx context.Context, steamID, appID string) (int, int) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://steamcommunity.com/profiles/"+url.PathEscape(steamID)+"/stats/"+url.PathEscape(appID)+"/?xml=1", nil)
	if err != nil {
		return 0, 0
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return 0, 0
	}
	defer res.Body.Close()
	var doc struct {
		Items []struct {
			Closed int `xml:"closed,attr"`
		} `xml:"achievements>achievement"`
	}
	if xml.NewDecoder(io.LimitReader(res.Body, 2<<20)).Decode(&doc) != nil || len(doc.Items) == 0 {
		return 0, 0
	}
	earned := 0
	for _, item := range doc.Items {
		if item.Closed == 1 {
			earned++
		}
	}
	return earned, len(doc.Items)
}

func (c *Client) steamWeb(ctx context.Context, steamID, token string, report Report) ([]Game, error) {
	if report != nil {
		report(0, 0, "正在读取 Steam 游戏库")
	}
	q := url.Values{}
	q.Set(steamAuthField(token), token)
	q.Set("steamid", steamID)
	q.Set("include_appinfo", "true")
	q.Set("include_played_free_games", "true")
	q.Set("language", "schinese")
	q.Set("format", "json")
	var body struct {
		Response struct {
			Games []struct {
				AppID int    `json:"appid"`
				Name  string `json:"name"`
				Mins  int    `json:"playtime_forever"`
			} `json:"games"`
		} `json:"response"`
	}
	var last error
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			if report != nil {
				report(0, 0, "正在重试读取游戏库")
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt) * 2 * time.Second):
			}
		}
		last = c.getJSON(ctx, "https://api.steampowered.com/IPlayerService/GetOwnedGames/v1/?"+q.Encode(), nil, &body)
		if last == nil {
			break
		}
	}
	if last != nil {
		if steamAuthField(token) == "access_token" && steamAccessExpired(token) {
			return nil, fmt.Errorf("Steam 登录已过期，请重新登录")
		}
		return nil, fmt.Errorf("Steam 没有返回游戏库，请再试一次")
	}
	if len(body.Response.Games) == 0 {
		return nil, fmt.Errorf("Steam 游戏详情没有公开，网站读不到游戏库")
	}
	out := make([]Game, 0, len(body.Response.Games))
	for _, game := range body.Response.Games {
		if game.AppID <= 0 || strings.TrimSpace(game.Name) == "" {
			continue
		}
		out = append(out, Game{Ref: strconv.Itoa(game.AppID), Title: game.Name, Minutes: game.Mins})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Minutes > out[j].Minutes })
	ids := make([]int, 0)
	for _, game := range out {
		if game.Minutes <= 0 {
			break
		}
		id, _ := strconv.Atoi(game.Ref)
		ids = append(ids, id)
	}
	if report != nil {
		report(0, len(ids), "正在读取成就")
	}
	progress, err := c.steamProgress(ctx, token, steamID, ids, report)
	if err != nil {
		return nil, err
	}
	for i := range out {
		id, _ := strconv.Atoi(out[i].Ref)
		if row, ok := progress[id]; ok && row.Total > 0 {
			out[i].Earned = row.Unlocked
			out[i].Total = row.Total
			out[i].Complete = row.All || row.Unlocked >= row.Total
		}
	}
	return out, nil
}

type steamProgressRow struct {
	Unlocked int
	Total    int
	All      bool
}

func (c *Client) steamProgress(ctx context.Context, token, steamID string, appIDs []int, report Report) (map[int]steamProgressRow, error) {
	out := map[int]steamProgressRow{}
	if len(appIDs) == 0 {
		return out, nil
	}
	for i := 0; i < len(appIDs); i += 40 {
		end := i + 40
		if end > len(appIDs) {
			end = len(appIDs)
		}
		rows, err := c.steamProgressChunk(ctx, token, steamID, appIDs[i:end])
		if err != nil {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(800 * time.Millisecond):
			}
			rows, err = c.steamProgressChunk(ctx, token, steamID, appIDs[i:end])
		}
		if err != nil {
			return nil, fmt.Errorf("Steam 没有返回成就进度")
		}
		if report != nil {
			report(end, len(appIDs), "正在读取成就")
		}
		for id, row := range rows {
			out[id] = row
		}
	}
	return out, nil
}

func (c *Client) steamProgressChunk(ctx context.Context, token, steamID string, appIDs []int) (map[int]steamProgressRow, error) {
	payload, err := json.Marshal(map[string]any{
		"steamid":  steamID,
		"language": "schinese",
		"appids":   appIDs,
	})
	if err != nil {
		return nil, err
	}
	form := url.Values{}
	form.Set(steamAuthField(token), token)
	form.Set("input_json", string(payload))
	rows, err := c.readSteamProgress(ctx, form)
	if err == nil && len(rows) > 0 {
		return rows, nil
	}
	form = url.Values{}
	form.Set(steamAuthField(token), token)
	form.Set("steamid", steamID)
	form.Set("language", "schinese")
	for i, id := range appIDs {
		form.Set(fmt.Sprintf("appids[%d]", i), strconv.Itoa(id))
	}
	return c.readSteamProgress(ctx, form)
}

func (c *Client) readSteamProgress(ctx context.Context, form url.Values) (map[int]steamProgressRow, error) {
	var body struct {
		Response struct {
			Progress []struct {
				AppID    int  `json:"appid"`
				Unlocked int  `json:"unlocked"`
				Total    int  `json:"total"`
				All      bool `json:"all_unlocked"`
			} `json:"achievement_progress"`
		} `json:"response"`
	}
	if err := c.postForm(ctx, "https://api.steampowered.com/IPlayerService/GetAchievementsProgress/v1/", form, &body); err != nil {
		return nil, err
	}
	out := map[int]steamProgressRow{}
	for _, row := range body.Response.Progress {
		out[row.AppID] = steamProgressRow{Unlocked: row.Unlocked, Total: row.Total, All: row.All}
	}
	return out, nil
}

func steamAccessExpired(token string) bool {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return false
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if json.Unmarshal(raw, &claims) != nil || claims.Exp == 0 {
		return false
	}
	return time.Now().Unix() >= claims.Exp
}

func steamAuthField(secret string) string {
	if looksLikeAPIKey(secret) {
		return "key"
	}
	return "access_token"
}

func looksLikeAPIKey(key string) bool {
	if len(key) < 16 {
		return false
	}
	for _, r := range key {
		if (r < '0' || r > '9') && (r < 'A' || r > 'F') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

func (c *Client) steamCommunity(ctx context.Context, steamID string) ([]Game, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://steamcommunity.com/profiles/"+url.PathEscape(steamID)+"/games?tab=all&xml=1&l=schinese", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Steam 没有返回游戏库")
	}
	defer res.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	var doc struct {
		Error string `xml:"error"`
		Games []struct {
			AppID string `xml:"appID"`
			Name  string `xml:"name"`
			Hours string `xml:"hoursOnRecord"`
		} `xml:"games>game"`
	}
	if err := xml.Unmarshal(payload, &doc); err != nil || doc.Error != "" {
		return nil, fmt.Errorf("Steam 资料需要公开游戏详情，才能在登录后读到时长")
	}
	out := make([]Game, 0, len(doc.Games))
	for _, game := range doc.Games {
		hours, _ := strconv.ParseFloat(strings.TrimSpace(game.Hours), 64)
		mins := int(hours*60 + 0.5)
		if mins <= 0 || strings.TrimSpace(game.Name) == "" {
			continue
		}
		out = append(out, Game{Ref: game.AppID, Title: game.Name, Minutes: mins})
	}
	return out, nil
}

func (c *Client) steamComplete(ctx context.Context, key, steamID string, appID int) (bool, error) {
	q := url.Values{}
	q.Set("key", key)
	q.Set("steamid", steamID)
	q.Set("appid", strconv.Itoa(appID))
	var body struct {
		PlayerStats struct {
			Success      bool `json:"success"`
			Achievements []struct {
				Achieved int `json:"achieved"`
			} `json:"achievements"`
		} `json:"playerstats"`
	}
	if err := c.getJSON(ctx, "https://api.steampowered.com/ISteamUserStats/GetPlayerAchievements/v1/?"+q.Encode(), nil, &body); err != nil {
		return false, err
	}
	if !body.PlayerStats.Success || len(body.PlayerStats.Achievements) == 0 {
		return false, nil
	}
	for _, ach := range body.PlayerStats.Achievements {
		if ach.Achieved == 0 {
			return false, nil
		}
	}
	return true, nil
}

func (c *Client) psn(ctx context.Context, secret string) ([]Game, error) {
	npsso, access, onlineID := secret, "", ""
	if strings.HasPrefix(strings.TrimSpace(secret), "{") {
		var cred psnCred
		if json.Unmarshal([]byte(secret), &cred) != nil || (cred.NPSSO == "" && cred.Access == "" && cred.OnlineID == "") {
			return nil, fmt.Errorf("请重新登录 PSN")
		}
		npsso, access, onlineID = cred.NPSSO, cred.Access, cred.OnlineID
	}
	if onlineID != "" && npsso == "" {
		if access == "" || jwtExpiring(access, time.Minute) {
			return nil, fmt.Errorf("网站还读不到公开的 PlayStation 记录")
		}
		return c.psnPublic(ctx, onlineID, access)
	}
	token := access
	if token == "" || jwtExpiring(token, time.Minute) {
		if npsso == "" {
			return nil, fmt.Errorf("请重新登录 PSN")
		}
		var err error
		token, err = c.psnToken(ctx, npsso)
		if err != nil {
			return nil, err
		}
	}
	return c.psnLibrary(ctx, token, "me")
}

func (c *Client) psnPublic(ctx context.Context, onlineID, token string) ([]Game, error) {
	auth := map[string]string{"Authorization": "Bearer " + token}
	var profile struct {
		AccountID string `json:"accountId"`
		Profile   struct {
			AccountID string `json:"accountId"`
		} `json:"profile"`
	}
	raw := "https://us-prof.np.community.playstation.net/userProfile/v1/users/" + url.PathEscape(onlineID) + "/profile2?fields=onlineId,accountId"
	if err := c.getJSON(ctx, raw, auth, &profile); err != nil {
		if strings.Contains(err.Error(), "404") {
			return nil, fmt.Errorf("没有找到这个 PSN 用户名")
		}
		return nil, fmt.Errorf("没有读到这个 PSN 的公开资料")
	}
	accountID := profile.AccountID
	if accountID == "" {
		accountID = profile.Profile.AccountID
	}
	if accountID == "" {
		return nil, fmt.Errorf("没有找到这个 PSN 用户名")
	}
	games, err := c.psnLibrary(ctx, token, accountID)
	if err != nil && (strings.Contains(err.Error(), "401") || strings.Contains(err.Error(), "403")) {
		return nil, fmt.Errorf("这个账号的游戏记录没有公开")
	}
	return games, err
}

func (c *Client) psnLibrary(ctx context.Context, token, user string) ([]Game, error) {
	auth := map[string]string{"Authorization": "Bearer " + token}
	user = url.PathEscape(user)
	var titles struct {
		Titles []struct {
			Name     string `json:"name"`
			Duration any    `json:"playDuration"`
			TitleID  string `json:"titleId"`
		} `json:"titles"`
	}
	_ = c.getJSON(ctx, "https://m.np.playstation.com/api/gamelist/v2/users/"+user+"/titles?limit=200&categories=ps4_game,ps5_native_game", auth, &titles)
	var trophies struct {
		TrophyTitles []struct {
			Name     string       `json:"trophyTitleName"`
			NPID     string       `json:"npCommunicationId"`
			Progress int          `json:"progress"`
			Earned   trophyCounts `json:"earnedTrophies"`
			Defined  trophyCounts `json:"definedTrophies"`
		} `json:"trophyTitles"`
	}
	if err := c.getJSON(ctx, "https://m.np.playstation.com/api/trophy/v1/users/"+user+"/trophyTitles?limit=800", auth, &trophies); err != nil && len(titles.Titles) == 0 {
		if user == "me" {
			return nil, fmt.Errorf("PSN 登录已失效，请重新粘贴 NPSSO")
		}
		return nil, err
	}
	byName := map[string]Game{}
	for _, title := range titles.Titles {
		name := strings.TrimSpace(title.Name)
		if name == "" {
			continue
		}
		byName[strings.ToLower(name)] = Game{Ref: title.TitleID, Title: name, Minutes: durationMinutes(title.Duration)}
	}
	for _, trophy := range trophies.TrophyTitles {
		name := strings.TrimSpace(trophy.Name)
		if name == "" {
			continue
		}
		item := byName[strings.ToLower(name)]
		item.Title = name
		if item.Ref == "" {
			item.Ref = trophy.NPID
		}
		applyTrophies(&item, trophy.Progress, trophy.Earned, trophy.Defined)
		byName[strings.ToLower(name)] = item
	}
	out := make([]Game, 0, len(byName))
	for _, item := range byName {
		if item.Minutes <= 0 && !item.Complete {
			continue
		}
		out = append(out, item)
	}
	return out, nil
}

type trophyCounts struct {
	Bronze   int `json:"bronze"`
	Silver   int `json:"silver"`
	Gold     int `json:"gold"`
	Platinum int `json:"platinum"`
}

func (t trophyCounts) sum() int {
	return t.Bronze + t.Silver + t.Gold + t.Platinum
}

func applyTrophies(item *Game, progress int, earned, defined trophyCounts) {
	total := defined.sum()
	got := earned.sum()
	if total > 0 {
		item.Earned = got
		item.Total = total
		item.Complete = got >= total || progress >= 100
		item.Platinum = defined.Platinum > 0 && earned.Platinum > 0
		return
	}
	if progress > 0 {
		item.Earned = progress
		item.Total = 100
		item.Complete = progress >= 100
	}
}

func (c *Client) psnToken(ctx context.Context, npsso string) (string, error) {
	q := url.Values{}
	q.Set("access_type", "offline")
	q.Set("client_id", "09515159-7237-4370-9b40-3806e67c0891")
	q.Set("redirect_uri", "com.scee.psxandroid.scecompcall://redirect")
	q.Set("response_type", "code")
	q.Set("scope", "psn:mobile.v2.core psn:clientapp")
	noRedirect := &http.Client{
		Timeout:       25 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://ca.account.sony.com/api/authz/v3/oauth/authorize?"+q.Encode(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Cookie", "npsso="+npsso)
	res, err := noRedirect.Do(req)
	if err != nil {
		return "", fmt.Errorf("PSN 登录没有完成")
	}
	defer res.Body.Close()
	loc := res.Header.Get("Location")
	code := ""
	if u, err := url.Parse(loc); err == nil {
		code = u.Query().Get("code")
	}
	if code == "" {
		return "", fmt.Errorf("NPSSO 无效或已过期")
	}
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", "com.scee.psxandroid.scecompcall://redirect")
	form.Set("token_format", "jwt")
	tokenReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://ca.account.sony.com/api/authz/v3/oauth/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	tokenReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	tokenReq.Header.Set("Authorization", "Basic MDk1MTUxNTktNzIzNy00MzcwLTliNDAtMzgwNmU2N2MwODkxOnVjUGprYTV0bnRCMktxc1A=")
	tokenRes, err := c.HTTP.Do(tokenReq)
	if err != nil {
		return "", fmt.Errorf("PSN 令牌交换失败")
	}
	defer tokenRes.Body.Close()
	access, _, err := readPSNToken(tokenRes)
	if err != nil || access == "" {
		return "", fmt.Errorf("PSN 没有返回访问令牌")
	}
	return access, nil
}

func (c *Client) xbox(ctx context.Context, key string, report Report) ([]Game, error) {
	if strings.HasPrefix(strings.TrimSpace(key), "{") {
		return c.xboxLive(ctx, key, report)
	}
	if key == "" {
		return nil, fmt.Errorf("请先登录 Xbox")
	}
	var body struct {
		Titles []struct {
			Name    string `json:"name"`
			TitleID string `json:"titleId"`
			History struct {
				Minutes int `json:"minutesPlayed"`
			} `json:"titleHistory"`
		} `json:"titles"`
	}
	if err := c.getJSON(ctx, "https://xbl.io/api/v2/player/titleHistory", map[string]string{"X-Authorization": key, "Accept": "application/json"}, &body); err != nil {
		return nil, fmt.Errorf("Xbox 没有返回游戏记录，请确认 xbl.io 钥匙")
	}
	out := make([]Game, 0, len(body.Titles))
	for _, title := range body.Titles {
		if title.History.Minutes <= 0 || strings.TrimSpace(title.Name) == "" {
			continue
		}
		out = append(out, Game{Ref: title.TitleID, Title: title.Name, Minutes: title.History.Minutes})
	}
	return out, nil
}

func (c *Client) xboxLive(ctx context.Context, raw string, report Report) ([]Game, error) {
	var cred struct {
		UHS  string `json:"uhs"`
		XSTS string `json:"xsts"`
		XUID string `json:"xuid"`
	}
	if json.Unmarshal([]byte(raw), &cred) != nil || cred.XSTS == "" || cred.XUID == "" {
		return nil, fmt.Errorf("请重新登录 Xbox")
	}
	var body any
	err := c.getJSON(ctx, "https://titlehub.xboxlive.com/users/xuid("+url.PathEscape(cred.XUID)+")/titles/titlehistory/decoration/achievement,image,detail,scid", map[string]string{
		"Authorization":          "XBL3.0 x=" + cred.UHS + ";" + cred.XSTS,
		"x-xbl-contract-version": "2",
		"Accept":                 "application/json",
		"Accept-Language":        "zh-CN",
	}, &body)
	if err != nil {
		return nil, fmt.Errorf("Xbox 登录已过期，请重新登录")
	}
	games := xboxGames(body)
	missing := make([]int, 0)
	for i := range games {
		if games[i].Total == 0 && games[i].Ref != "" {
			missing = append(missing, i)
		}
	}
	for n, i := range missing {
		if report != nil {
			report(n, len(missing), "正在读取成就")
		}
		earned, total, err := c.xboxAchievementCount(ctx, cred.XUID, cred.UHS, cred.XSTS, games[i].Ref)
		if err != nil || total <= 0 {
			continue
		}
		games[i].Earned = earned
		games[i].Total = total
		games[i].Complete = earned >= total
	}
	if report != nil && len(missing) > 0 {
		report(len(missing), len(missing), "正在读取成就")
	}
	c.xboxPlaytime(ctx, cred.XUID, cred.UHS, cred.XSTS, games, report)
	return games, nil
}

func (c *Client) xboxPlaytime(ctx context.Context, xuid, uhs, xsts string, games []Game, report Report) {
	indexes := make([]int, 0, len(games))
	for i := range games {
		if games[i].SCID != "" {
			indexes = append(indexes, i)
		}
	}
	if len(indexes) == 0 {
		return
	}
	workers := 6
	if len(indexes) < workers {
		workers = len(indexes)
	}
	jobs := make(chan int)
	var wg sync.WaitGroup
	var mu sync.Mutex
	done := 0
	for n := 0; n < workers; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				mins, err := c.xboxMinutes(ctx, xuid, uhs, xsts, games[i].SCID)
				mu.Lock()
				done++
				if err == nil && mins > games[i].Minutes {
					games[i].Minutes = mins
				}
				if report != nil {
					report(done, len(indexes), "正在读取时长")
				}
				mu.Unlock()
			}
		}()
	}
	for _, i := range indexes {
		select {
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return
		case jobs <- i:
		}
	}
	close(jobs)
	wg.Wait()
}

func (c *Client) xboxMinutes(ctx context.Context, xuid, uhs, xsts, scid string) (int, error) {
	var body any
	endpoint := "https://userstats.xboxlive.com/users/xuid(" + url.PathEscape(xuid) + ")/scids/" + url.PathEscape(scid) + "/stats/MinutesPlayed"
	err := c.getJSON(ctx, endpoint, map[string]string{
		"Authorization":          "XBL3.0 x=" + uhs + ";" + xsts,
		"x-xbl-contract-version": "2",
		"Accept":                 "application/json",
	}, &body)
	if err != nil {
		return 0, err
	}
	return statValue(body, "MinutesPlayed"), nil
}

func statValue(v any, name string) int {
	switch n := v.(type) {
	case map[string]any:
		stat, _ := n["statname"].(string)
		if stat == "" {
			stat, _ = n["name"].(string)
		}
		if stat == name {
			switch val := n["value"].(type) {
			case float64:
				return int(val)
			case string:
				parsed, _ := strconv.Atoi(val)
				return parsed
			}
		}
		best := 0
		for _, child := range n {
			if got := statValue(child, name); got > best {
				best = got
			}
		}
		return best
	case []any:
		best := 0
		for _, child := range n {
			if got := statValue(child, name); got > best {
				best = got
			}
		}
		return best
	default:
		return 0
	}
}

func (c *Client) xboxAchievementCount(ctx context.Context, xuid, uhs, xsts, titleID string) (int, int, error) {
	headers := map[string]string{
		"Authorization":          "XBL3.0 x=" + uhs + ";" + xsts,
		"x-xbl-contract-version": "2",
		"Accept":                 "application/json",
	}
	earned, total := 0, 0
	token := ""
	for page := 0; page < 5; page++ {
		q := url.Values{}
		q.Set("titleId", titleID)
		q.Set("maxItems", "1000")
		if token != "" {
			q.Set("continuationToken", token)
		}
		var body struct {
			Paging struct {
				Total int    `json:"totalRecords"`
				Next  string `json:"continuationToken"`
			} `json:"pagingInfo"`
			Items []struct {
				State string `json:"progressState"`
			} `json:"achievements"`
		}
		endpoint := "https://achievements.xboxlive.com/users/xuid(" + url.PathEscape(xuid) + ")/achievements?" + q.Encode()
		if err := c.getJSON(ctx, endpoint, headers, &body); err != nil {
			return 0, 0, err
		}
		if body.Paging.Total > total {
			total = body.Paging.Total
		}
		for _, item := range body.Items {
			if item.State == "Achieved" {
				earned++
			}
		}
		if body.Paging.Next == "" || len(body.Items) == 0 {
			break
		}
		token = body.Paging.Next
	}
	return earned, total, nil
}

func xboxGames(body any) []Game {
	seen := map[string]struct{}{}
	var out []Game
	var walk func(any)
	walk = func(v any) {
		switch n := v.(type) {
		case map[string]any:
			name := firstString(n, "name", "titleName")
			ref := anyString(n["titleId"])
			if ref == "" {
				ref = anyString(n["modernTitleId"])
			}
			if name != "" && ref != "" {
				if _, ok := seen[ref]; !ok {
					seen[ref] = struct{}{}
					item := Game{Ref: ref, Title: name, Minutes: nestedMinutes(n), SCID: firstString(n, "serviceConfigId")}
					if ach, ok := n["achievement"].(map[string]any); ok {
						cur := firstInt(ach, "currentAchievements")
						tot := firstInt(ach, "totalAchievements")
						if tot == 0 {
							cur = firstInt(ach, "currentGamerscore")
							tot = firstInt(ach, "totalGamerscore")
						}
						if tot > 0 {
							item.Earned = cur
							item.Total = tot
							item.Complete = cur >= tot || firstInt(ach, "progressPercentage") >= 100
						}
					}
					out = append(out, item)
				}
			}
			for _, child := range n {
				walk(child)
			}
		case []any:
			for _, child := range n {
				walk(child)
			}
		}
	}
	walk(body)
	return out
}

func nestedMinutes(obj map[string]any) int {
	mins := firstInt(obj, "minutesPlayed", "MinutesPlayed")
	if mins > 0 {
		return mins
	}
	if hist, ok := obj["titleHistory"].(map[string]any); ok {
		return firstInt(hist, "minutesPlayed", "MinutesPlayed")
	}
	return 0
}

func anyString(v any) string {
	switch n := v.(type) {
	case string:
		return strings.TrimSpace(n)
	case float64:
		return strconv.FormatInt(int64(n), 10)
	default:
		return ""
	}
}

func (c *Client) nintendo(ctx context.Context, token string) ([]Game, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, fmt.Errorf("请先登录 Nintendo")
	}
	if strings.HasPrefix(token, "{") {
		var cred struct {
			Session    string `json:"session_token"`
			FriendCode string `json:"friend_code"`
		}
		if json.Unmarshal([]byte(token), &cred) != nil || (cred.Session == "" && cred.FriendCode == "") {
			return nil, fmt.Errorf("请重新登录 Nintendo")
		}
		if cred.Session == "" {
			return nil, fmt.Errorf("好友代码已保存。任天堂的公开页面不提供游玩时长，完整记录请用「用链接登录」")
		}
		access, err := c.nintendoAccess(ctx, cred.Session)
		if err != nil {
			return nil, err
		}
		token = access
	}
	headers := map[string]string{
		"Authorization": "Bearer " + token,
		"Accept":        "application/json",
		"User-Agent":    "com.nintendo.znej/1.13.0 (Android/7.1.2)",
	}
	var body any
	var err error
	for _, locale := range []string{"zh-TW", "ja-JP", "en-US"} {
		headers["gentry-locale"] = locale
		err = c.getJSON(ctx, "https://app-api.znej.nintendo.com/api/v2.0/users/me/play_histories", headers, &body)
		if err == nil {
			break
		}
	}
	if err != nil {
		err = c.getJSON(ctx, "https://mypage-api.entry.nintendo.co.jp/api/v1/users/me/play_histories", headers, &body)
	}
	if err != nil {
		return nil, fmt.Errorf("My Nintendo 没有返回游玩记录，请重新登录")
	}
	games := nintendoGames(body)
	if len(games) == 0 {
		return nil, fmt.Errorf("My Nintendo 的游玩记录是空的")
	}
	return games, nil
}

func (c *Client) nintendoAccess(ctx context.Context, sessionToken string) (string, error) {
	payload, _ := json.Marshal(map[string]string{
		"client_id":     nintendoClientID,
		"session_token": sessionToken,
		"grant_type":    "urn:ietf:params:oauth:grant-type:jwt-bearer-session-token",
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://accounts.nintendo.com/connect/1.0.0/api/token", strings.NewReader(string(payload)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("Nintendo 登录已过期，请重新登录")
	}
	defer res.Body.Close()
	var tok struct {
		Access string `json:"access_token"`
	}
	if json.NewDecoder(res.Body).Decode(&tok) != nil || tok.Access == "" {
		return "", fmt.Errorf("Nintendo 登录已过期，请重新登录")
	}
	return tok.Access, nil
}

func nintendoGames(body any) []Game {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil
	}
	var root map[string]any
	if json.Unmarshal(raw, &root) != nil {
		return nil
	}
	var lists []any
	for _, key := range []string{"playHistories", "play_histories", "histories", "applications", "titles"} {
		if v, ok := root[key].([]any); ok {
			lists = append(lists, v...)
		}
	}
	if len(lists) == 0 {
		if v, ok := body.([]any); ok {
			lists = v
		}
	}
	out := []Game{}
	for _, item := range lists {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		title := firstString(obj, "titleName", "applicationName", "name", "softwareName")
		if title == "" {
			continue
		}
		minutes := firstInt(obj, "totalPlayedMinutes", "totalPlayTime", "playedMinutes", "playTime")
		if minutes > 100000 {
			minutes = minutes / 60
		}
		if minutes <= 0 {
			continue
		}
		out = append(out, Game{Ref: firstString(obj, "applicationId", "titleId", "id"), Title: title, Minutes: minutes})
	}
	return out
}

func firstString(obj map[string]any, keys ...string) string {
	for _, key := range keys {
		if s, ok := obj[key].(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func firstInt(obj map[string]any, keys ...string) int {
	for _, key := range keys {
		switch v := obj[key].(type) {
		case float64:
			return int(v)
		case string:
			n, _ := strconv.Atoi(v)
			if n > 0 {
				return n
			}
		}
	}
	return 0
}

func durationMinutes(v any) int {
	switch n := v.(type) {
	case float64:
		if n > 10000 {
			return int(n / 60)
		}
		return int(n)
	case string:
		if strings.HasPrefix(n, "PT") {
			return isoMinutes(n)
		}
		f, _ := strconv.ParseFloat(n, 64)
		if f > 10000 {
			return int(f / 60)
		}
		return int(f)
	default:
		return 0
	}
}

func isoMinutes(v string) int {
	hours, mins := 0, 0
	num := ""
	for _, r := range v {
		if r >= '0' && r <= '9' {
			num += string(r)
			continue
		}
		n, _ := strconv.Atoi(num)
		num = ""
		switch r {
		case 'H':
			hours = n
		case 'M':
			mins = n
		}
	}
	return hours*60 + mins
}

func (c *Client) postForm(ctx context.Context, raw string, form url.Values, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, raw, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("status %d", res.StatusCode)
	}
	return json.Unmarshal(payload, dst)
}

func (c *Client) getJSON(ctx context.Context, raw string, headers map[string]string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("status %d", res.StatusCode)
	}
	return json.Unmarshal(payload, dst)
}
