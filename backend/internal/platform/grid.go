package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type GridSide struct {
	Portrait  string
	Landscape string
}

type gridAsset struct {
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Style  string `json:"style"`
}

func (c *Client) CheckGridKey(ctx context.Context, key string) error {
	key = strings.TrimSpace(key)
	if !plausibleGridKey(key) {
		return fmt.Errorf("密钥格式不对")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.steamgriddb.com/api/v2/search/autocomplete/garden", nil)
	if err != nil {
		return fmt.Errorf("SteamGridDB 没有响应")
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("User-Agent", "forking-paths")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("SteamGridDB 没有响应")
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 1<<20))
	if res.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("SteamGridDB 密钥不正确")
	}
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("SteamGridDB 没有响应")
	}
	return nil
}

func (c *Client) GridArts(ctx context.Context, key string, portraitIDs, landscapeIDs []string) (map[string]GridSide, error) {
	out := map[string]GridSide{}
	if err := c.fillArts(ctx, key, "grids/steam", "600x900,660x930,342x482", "alternate,material,white_logo,no_logo", 600, 900, portraitIDs, func(side *GridSide, raw string) {
		side.Portrait = raw
	}, out); err != nil {
		return out, err
	}
	if err := c.fillArts(ctx, key, "grids/steam", "920x430,460x215", "alternate,material,white_logo,no_logo", 920, 430, landscapeIDs, func(side *GridSide, raw string) {
		side.Landscape = raw
	}, out); err != nil {
		return out, err
	}
	missing := make([]string, 0)
	for _, id := range landscapeIDs {
		if out[id].Landscape == "" {
			missing = append(missing, id)
		}
	}
	if err := c.fillArts(ctx, key, "heroes/steam", "1920x620,3840x1240,1600x650", "alternate,material", 1920, 620, missing, func(side *GridSide, raw string) {
		side.Landscape = raw
	}, out); err != nil {
		return out, err
	}
	return out, nil
}

func (c *Client) GridGameArt(ctx context.Context, key string, gameID int, portrait, landscape bool) (GridSide, error) {
	var side GridSide
	if gameID <= 0 {
		return side, nil
	}
	id := []string{fmt.Sprintf("%d", gameID)}
	out := map[string]GridSide{}
	if portrait {
		if err := c.fillArts(ctx, key, "grids/game", "600x900,660x930,342x482", "alternate,material,white_logo,no_logo", 600, 900, id, func(item *GridSide, raw string) {
			item.Portrait = raw
		}, out); err != nil {
			return side, err
		}
	}
	if landscape {
		if err := c.fillArts(ctx, key, "grids/game", "920x430,460x215", "alternate,material,white_logo,no_logo", 920, 430, id, func(item *GridSide, raw string) {
			item.Landscape = raw
		}, out); err != nil {
			return side, err
		}
		if out[id[0]].Landscape == "" {
			if err := c.fillArts(ctx, key, "heroes/game", "1920x620,3840x1240,1600x650", "alternate,material", 1920, 620, id, func(item *GridSide, raw string) {
				item.Landscape = raw
			}, out); err != nil {
				return side, err
			}
		}
	}
	return out[id[0]], nil
}

func (c *Client) FindGridGame(ctx context.Context, key, name string) (int, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return 0, nil
	}
	endpoint := "https://www.steamgriddb.com/api/v2/search/autocomplete/" + url.PathEscape(name)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, fmt.Errorf("SteamGridDB 没有响应")
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("User-Agent", "forking-paths")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return 0, fmt.Errorf("SteamGridDB 没有响应")
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return 0, fmt.Errorf("SteamGridDB 没有响应")
	}
	if res.StatusCode == http.StatusUnauthorized {
		return 0, fmt.Errorf("SteamGridDB 密钥不正确")
	}
	if res.StatusCode != http.StatusOK {
		return 0, nil
	}
	var payload struct {
		Data []struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &payload) != nil || len(payload.Data) == 0 {
		return 0, nil
	}
	want := strings.ToLower(name)
	for _, item := range payload.Data {
		if item.ID > 0 && strings.ToLower(strings.TrimSpace(item.Name)) == want {
			return item.ID, nil
		}
	}
	if payload.Data[0].ID > 0 {
		return payload.Data[0].ID, nil
	}
	return 0, nil
}

func (c *Client) fillArts(ctx context.Context, key, route, dimensions, styles string, preferW, preferH int, ids []string, assign func(*GridSide, string), out map[string]GridSide) error {
	for start := 0; start < len(ids); start += 20 {
		end := start + 20
		if end > len(ids) {
			end = len(ids)
		}
		batch := ids[start:end]
		found, err := c.artBatch(ctx, key, route, dimensions, styles, preferW, preferH, batch)
		if err != nil {
			return err
		}
		for id, raw := range found {
			side := out[id]
			assign(&side, raw)
			out[id] = side
		}
	}
	return nil
}

func (c *Client) artBatch(ctx context.Context, key, route, dimensions, styles string, preferW, preferH int, ids []string) (map[string]string, error) {
	if len(ids) == 0 {
		return map[string]string{}, nil
	}
	q := url.Values{}
	q.Set("dimensions", dimensions)
	q.Set("types", "static")
	q.Set("mimes", "image/png,image/jpeg,image/webp")
	q.Set("styles", styles)
	q.Set("nsfw", "false")
	q.Set("humor", "false")
	q.Set("epilepsy", "false")
	q.Set("limit", "8")
	endpoint := "https://www.steamgriddb.com/api/v2/" + route + "/" + strings.Join(ids, ",") + "?" + q.Encode()
	var lastStatus int
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("SteamGridDB 没有响应")
			case <-time.After(time.Second):
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, fmt.Errorf("SteamGridDB 没有响应")
		}
		req.Header.Set("Authorization", "Bearer "+key)
		req.Header.Set("User-Agent", "forking-paths")
		res, err := c.HTTP.Do(req)
		if err != nil {
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(res.Body, 4<<20))
		res.Body.Close()
		if readErr != nil {
			continue
		}
		lastStatus = res.StatusCode
		if res.StatusCode == http.StatusUnauthorized {
			return nil, fmt.Errorf("SteamGridDB 密钥不正确")
		}
		if res.StatusCode == http.StatusOK || res.StatusCode == http.StatusMultiStatus || res.StatusCode == http.StatusNotFound {
			return parseArtMap(body, ids, preferW, preferH), nil
		}
	}
	if lastStatus != 0 {
		log.Printf("steamgriddb %s status %d", route, lastStatus)
	}
	return map[string]string{}, nil
}

func GridURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || parsed.User != nil || !gridHost(parsed.Hostname()) {
		return ""
	}
	return parsed.String()
}

func (c *Client) DownloadGrid(ctx context.Context, raw string) ([]byte, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || !gridHost(parsed.Host) {
		return nil, fmt.Errorf("封面地址不正确")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "forking-paths")
	client := *c.HTTP
	client.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if len(via) >= 3 || next.URL.Scheme != "https" || !gridHost(next.URL.Host) {
			return fmt.Errorf("封面地址不正确")
		}
		return nil
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("封面下载失败")
	}
	return io.ReadAll(io.LimitReader(res.Body, 8<<20))
}

func parseArtMap(body []byte, ids []string, preferW, preferH int) map[string]string {
	body = bytes.TrimSpace(body)
	out := map[string]string{}
	if len(body) == 0 || len(ids) == 0 {
		return out
	}
	groups := artGroups(body)
	for i, id := range ids {
		if i >= len(groups) {
			break
		}
		if raw := pickAsset(groups[i], preferW, preferH); raw != "" {
			out[id] = raw
		}
	}
	return out
}

func artGroups(body []byte) [][]gridAsset {
	if body[0] == '[' {
		return groupsFromList(body)
	}
	var root struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &root); err != nil {
		return nil
	}
	return groupsFromList(root.Data)
}

func groupsFromList(raw json.RawMessage) [][]gridAsset {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '[' {
		return nil
	}
	var probe []json.RawMessage
	if err := json.Unmarshal(raw, &probe); err != nil || len(probe) == 0 {
		return nil
	}
	var head map[string]json.RawMessage
	if err := json.Unmarshal(probe[0], &head); err != nil {
		return nil
	}
	if _, ok := head["url"]; ok {
		var assets []gridAsset
		if err := json.Unmarshal(raw, &assets); err != nil {
			return nil
		}
		return [][]gridAsset{assets}
	}
	var nested []struct {
		Data []gridAsset `json:"data"`
	}
	if err := json.Unmarshal(raw, &nested); err != nil {
		return nil
	}
	groups := make([][]gridAsset, len(nested))
	for i, item := range nested {
		groups[i] = item.Data
	}
	return groups
}

func pickAsset(assets []gridAsset, preferW, preferH int) string {
	best := ""
	bestRank := 9
	for _, asset := range assets {
		if !strings.HasPrefix(asset.URL, "https://") {
			continue
		}
		rank := 4
		fitted := asset.Width == preferW && asset.Height == preferH
		switch {
		case asset.Style == "alternate" && fitted:
			rank = 0
		case asset.Style == "alternate":
			rank = 1
		case fitted && asset.Style != "material":
			rank = 2
		case fitted:
			rank = 3
		}
		if rank < bestRank {
			bestRank = rank
			best = asset.URL
		}
	}
	return best
}

func gridHost(host string) bool {
	host = strings.ToLower(host)
	return host == "steamgriddb.com" || strings.HasSuffix(host, ".steamgriddb.com")
}

func plausibleGridKey(key string) bool {
	if len(key) < 16 || len(key) > 128 {
		return false
	}
	for _, r := range key {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r == '-' || r == '_':
		default:
			return false
		}
	}
	return true
}
