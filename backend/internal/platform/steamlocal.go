package platform

import (
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"unicode"
)

type steamLocalUser struct {
	ID   string
	Name string
}

func LocalSteamUser() (steamLocalUser, bool) {
	for _, path := range steamLoginPaths() {
		if user, ok := readSteamUsers(path); ok {
			return user, true
		}
	}
	return steamLocalUser{}, false
}

func steamLoginPaths() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	switch runtime.GOOS {
	case "darwin":
		return []string{filepath.Join(home, "Library", "Application Support", "Steam", "config", "loginusers.vdf")}
	case "windows":
		return []string{
			filepath.Join(os.Getenv("ProgramFiles(x86)"), "Steam", "config", "loginusers.vdf"),
			filepath.Join(os.Getenv("ProgramFiles"), "Steam", "config", "loginusers.vdf"),
		}
	default:
		return []string{
			filepath.Join(home, ".steam", "steam", "config", "loginusers.vdf"),
			filepath.Join(home, ".local", "share", "Steam", "config", "loginusers.vdf"),
		}
	}
}

func readSteamUsers(path string) (steamLocalUser, bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return steamLocalUser{}, false
	}
	var current steamLocalUser
	var recent steamLocalUser
	var any steamLocalUser
	inUser := false
	isRecent := false
	for _, line := range strings.Split(string(raw), "\n") {
		key, value, ok := vdfPair(line)
		if !ok {
			if strings.TrimSpace(line) == "}" && inUser {
				if isRecent {
					recent = current
				}
				if current.ID != "" {
					any = current
				}
				current = steamLocalUser{}
				inUser = false
				isRecent = false
			}
			continue
		}
		if value == "" && len(key) >= 17 && onlyDigits(key) {
			current = steamLocalUser{ID: key}
			inUser = true
			isRecent = false
			continue
		}
		if !inUser {
			continue
		}
		switch strings.ToLower(key) {
		case "personaname":
			current.Name = value
		case "mostrecent":
			isRecent = value == "1"
		}
	}
	if recent.ID != "" {
		if recent.Name == "" {
			recent.Name = recent.ID
		}
		return recent, true
	}
	if any.ID != "" {
		if any.Name == "" {
			any.Name = any.ID
		}
		return any, true
	}
	return steamLocalUser{}, false
}

func vdfPair(line string) (key, value string, ok bool) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, `"`) {
		return "", "", false
	}
	parts := strings.Split(line, `"`)
	// "key" then optional "value"
	fields := make([]string, 0, 2)
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			fields = append(fields, part)
		}
	}
	if len(fields) == 0 {
		return "", "", false
	}
	if len(fields) == 1 {
		return fields[0], "", true
	}
	return fields[0], fields[1], true
}

func onlyDigits(s string) bool {
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return s != ""
}

func localSteamGames(steamID string) ([]Game, error) {
	root, account, ok := steamUserDir(steamID)
	if !ok {
		return nil, os.ErrNotExist
	}
	minutes := readPlaytime(filepath.Join(root, "userdata", account, "config", "localconfig.vdf"))
	if len(minutes) == 0 {
		return nil, os.ErrNotExist
	}
	facts := readAppFacts(filepath.Join(root, "appcache", "appinfo.vdf"), minutes)
	cache := filepath.Join(root, "userdata", account, "config", "librarycache")
	out := make([]Game, 0, len(minutes))
	for id, mins := range minutes {
		fact := facts[id]
		if !strings.EqualFold(fact.kind, "game") || fact.name == "" {
			continue
		}
		out = append(out, Game{
			Ref:      id,
			Title:    fact.name,
			Minutes:  mins,
			Complete: mins > 0 && steamAchieved(filepath.Join(cache, id+".json")),
		})
	}
	return out, nil
}

func steamUserDir(steamID string) (root, account string, ok bool) {
	id, err := strconv.ParseUint(steamID, 10, 64)
	if err != nil {
		return "", "", false
	}
	const base = 76561197960265728
	if id < base {
		return "", "", false
	}
	account = strconv.FormatUint(id-base, 10)
	for _, path := range steamLoginPaths() {
		root = filepath.Dir(filepath.Dir(path))
		info, err := os.Stat(filepath.Join(root, "userdata", account, "config", "localconfig.vdf"))
		if err == nil && !info.IsDir() {
			return root, account, true
		}
	}
	return "", "", false
}

func readPlaytime(path string) map[string]int {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	out := map[string]int{}
	depth := 0
	appsDepth := 0
	inApps := false
	armApps := false
	appID := ""
	minutes := 0
	for _, line := range strings.Split(string(raw), "\n") {
		trim := strings.TrimSpace(line)
		if trim == "{" {
			depth++
			if armApps {
				inApps = true
				appsDepth = depth
				armApps = false
			}
			continue
		}
		if trim == "}" {
			if inApps && appID != "" && depth == appsDepth+1 {
				if prev, ok := out[appID]; !ok || minutes > prev {
					out[appID] = minutes
				}
			}
			if inApps && depth == appsDepth {
				inApps = false
			}
			depth--
			appID = ""
			minutes = 0
			continue
		}
		key, value, ok := vdfPair(line)
		if !ok {
			continue
		}
		if key == "apps" && value == "" && !inApps {
			armApps = true
			continue
		}
		if inApps && depth == appsDepth && value == "" && onlyDigits(key) {
			appID = key
			minutes = 0
			continue
		}
		if inApps && appID != "" && strings.EqualFold(key, "Playtime") {
			n, _ := strconv.Atoi(value)
			minutes = n
		}
	}
	return out
}

func steamAchieved(path string) bool {
	raw, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var rows []any
	if json.Unmarshal(raw, &rows) != nil {
		return false
	}
	for _, row := range rows {
		pair, ok := row.([]any)
		if !ok || len(pair) < 2 {
			continue
		}
		kind, _ := pair[0].(string)
		if kind != "achievements" {
			continue
		}
		obj, _ := pair[1].(map[string]any)
		data, _ := obj["data"].(map[string]any)
		if data == nil {
			continue
		}
		total := firstInt(data, "nTotal")
		achieved := firstInt(data, "nAchieved")
		un := 0
		if list, ok := data["vecUnachieved"].([]any); ok {
			un = len(list)
		}
		if total > 0 && achieved >= total && un == 0 {
			return true
		}
	}
	return false
}

type kvNode struct {
	key      string
	str      string
	children []kvNode
}

type appFact struct {
	name string
	kind string
}

func readAppFacts(path string, wanted map[string]int) map[string]appFact {
	data, err := os.ReadFile(path)
	if err != nil || len(data) < 16 {
		return nil
	}
	if binary.LittleEndian.Uint32(data) != 0x07564429 {
		return nil
	}
	tableAt := binary.LittleEndian.Uint64(data[8:])
	if tableAt == 0 || tableAt >= uint64(len(data)) {
		return nil
	}
	table, _ := readStringTable(data[tableAt:])
	if len(table) == 0 {
		return nil
	}
	facts := map[string]appFact{}
	pos := 16
	limit := int(tableAt)
	for pos+8 < limit && len(facts) < len(wanted) {
		appID := binary.LittleEndian.Uint32(data[pos:])
		size := binary.LittleEndian.Uint32(data[pos+4:])
		if appID == 0 || size < 60 || pos+8+int(size) > len(data) {
			break
		}
		id := strconv.FormatUint(uint64(appID), 10)
		if _, ok := wanted[id]; ok {
			body := data[pos+8+60 : pos+8+int(size)]
			if node, _, err := parseKV(body, 0, table); err == nil {
				facts[id] = describeApp(node)
			} else {
				facts[id] = appFact{}
			}
		}
		pos += 8 + int(size)
	}
	return facts
}

func readStringTable(data []byte) ([]string, int) {
	if len(data) < 4 {
		return nil, 0
	}
	count := int(binary.LittleEndian.Uint32(data))
	pos := 4
	out := make([]string, 0, count)
	for i := 0; i < count && pos < len(data); i++ {
		end := pos
		for end < len(data) && data[end] != 0 {
			end++
		}
		out = append(out, string(data[pos:end]))
		pos = end + 1
	}
	return out, pos
}

func parseKV(buf []byte, i int, table []string) ([]kvNode, int, error) {
	var out []kvNode
	for i < len(buf) {
		kind := buf[i]
		i++
		if kind == 0x08 {
			return out, i, nil
		}
		if i+4 > len(buf) {
			return out, i, os.ErrInvalid
		}
		keyN := int(binary.LittleEndian.Uint32(buf[i:]))
		i += 4
		if keyN < 0 || keyN >= len(table) {
			return out, i, os.ErrInvalid
		}
		node := kvNode{key: table[keyN]}
		switch kind {
		case 0x00:
			children, next, err := parseKV(buf, i, table)
			if err != nil {
				return out, next, err
			}
			node.children = children
			i = next
		case 0x01:
			end := i
			for end < len(buf) && buf[end] != 0 {
				end++
			}
			node.str = string(buf[i:end])
			i = end + 1
		case 0x02, 0x03, 0x04, 0x06:
			if i+4 > len(buf) {
				return out, i, os.ErrInvalid
			}
			i += 4
		case 0x07, 0x0A:
			if i+8 > len(buf) {
				return out, i, os.ErrInvalid
			}
			i += 8
		case 0x05:
			for i+1 < len(buf) && !(buf[i] == 0 && buf[i+1] == 0) {
				i += 2
			}
			i += 2
		default:
			return out, i, os.ErrInvalid
		}
		out = append(out, node)
	}
	return out, i, nil
}

func describeApp(nodes []kvNode) appFact {
	en, zh, kind := "", "", ""
	var walk func([]kvNode, string)
	walk = func(items []kvNode, parent string) {
		for _, item := range items {
			if parent == "common" && item.key == "type" && item.str != "" && kind == "" {
				kind = item.str
			}
			if item.key == "name" && item.str != "" {
				if parent == "schinese" {
					zh = item.str
				}
				if parent == "common" && en == "" {
					en = item.str
				}
			}
			if len(item.children) > 0 {
				walk(item.children, item.key)
			}
		}
	}
	walk(nodes, "")
	name := en
	if zh != "" {
		name = zh
	}
	return appFact{name: name, kind: kind}
}
