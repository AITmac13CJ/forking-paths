package platform

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/crypto/pbkdf2"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/remote"
)

const nintendoClientID = "5c38e31cd085304b"

var loginMu sync.Mutex

type Session struct {
	Account string
	Secret  string
}

func Login(ctx context.Context, provider string) (Session, error) {
	if !loginMu.TryLock() {
		return Session{}, fmt.Errorf("正在登录另一个平台")
	}
	defer loginMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	switch provider {
	case "steam":
		return loginSteam(ctx)
	case "psn":
		clearRejectedNPSSO(ctx)
		npsso, err := readPlainCookie(ctx, "psn", "https://www.playstation.com/zh-hans-cn/", "npsso")
		if err != nil {
			return Session{}, err
		}
		return Session{Secret: npsso}, nil
	case "xbox":
		return loginXbox(ctx)
	case "switch":
		return loginNintendo(ctx)
	default:
		return Session{}, fmt.Errorf("未知的平台")
	}
}

func readCookie(ctx context.Context, provider, page, name string, cookieURLs ...string) (string, error) {
	var value string
	err := browse(ctx, provider, page, func(ctx context.Context) (bool, error) {
		cookies, err := chromedp.Run(ctx, func(ctx context.Context, t *chromedp.Target) ([]*network.Cookie, error) {
			res, err := cdp.Call(ctx, t, network.GetCookies, network.GetCookiesParams{URLs: cookieURLs})
			if err != nil {
				return nil, err
			}
			return res.Cookies, nil
		})
		if err != nil {
			return false, err
		}
		for _, cookie := range cookies {
			if cookie.Name == name && cookie.Value != "" {
				value = cookie.Value
				return true, nil
			}
		}
		return false, nil
	})
	if err != nil {
		return "", err
	}
	if value == "" {
		return "", fmt.Errorf("登录没有完成")
	}
	return value, nil
}

func loginSteam(ctx context.Context) (Session, error) {
	clearStaleSteamCookie()
	raw, err := readPlainCookie(ctx, "steam", "https://steamcommunity.com/login/home/", "steamLoginSecure")
	if err != nil {
		return Session{}, err
	}
	steamID, token, ok := parseSteamLogin(raw)
	if !ok {
		return Session{}, fmt.Errorf("Steam 登录没有完成")
	}
	refreshID, refresh := localSteamRefresh()
	if refreshID != "" && refreshID != steamID {
		refresh = ""
	}
	secret, err := json.Marshal(map[string]string{"steam_id": steamID, "access_token": token, "refresh_token": refresh})
	if err != nil {
		return Session{}, err
	}
	name := SteamPersona(ctx, steamID)
	if name == "" {
		name = steamID
	}
	return Session{Account: name, Secret: string(secret)}, nil
}

func parseSteamLogin(value string) (steamID, token string, ok bool) {
	if decoded, err := url.QueryUnescape(value); err == nil {
		value = decoded
	}
	value = strings.TrimSpace(value)
	parts := strings.SplitN(value, "||", 2)
	if len(parts) != 2 || len(parts[0]) < 17 || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func SteamPersona(ctx context.Context, steamID string) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://steamcommunity.com/profiles/"+url.PathEscape(steamID)+"/?xml=1", nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return ""
	}
	defer res.Body.Close()
	var doc struct {
		Name string `xml:"steamID"`
	}
	if xml.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&doc) != nil {
		return ""
	}
	return strings.TrimSpace(doc.Name)
}

func readPlainCookie(ctx context.Context, provider, page, name string) (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	dir = filepath.Join(dir, "forking-paths", "login-"+provider)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	stopLoginChrome(dir)
	if err := openPlainChrome(dir, page); err != nil {
		return "", err
	}
	defer stopLoginChrome(dir)
	raiseLoginChrome(dir)
	type keyResult struct {
		key []byte
		err error
	}
	keyReady := make(chan keyResult, 1)
	go func() {
		key, err := chromeSafeKey()
		keyReady <- keyResult{key: key, err: err}
	}()
	var key []byte
	rejectedNPSSO := map[string]bool{}
	deadline := time.Now().Add(3 * time.Minute)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("登录已取消")
		case got := <-keyReady:
			if got.err != nil {
				return "", got.err
			}
			key = got.key
			keyReady = nil
		case <-time.After(time.Second):
		}
		if !loginChromeAlive(dir) {
			return "", fmt.Errorf("登录窗口已关闭")
		}
		if key == nil {
			continue
		}
		value, err := profileCookie(dir, key, name, preferredCookieHost(provider))
		if err != nil || value == "" {
			continue
		}
		if provider == "psn" && name == "npsso" {
			if rejectedNPSSO[value] {
				continue
			}
			code, err := (&Client{}).psnCode(ctx, value)
			if err != nil || code == "" {
				if err != nil && strings.Contains(err.Error(), "无效或已过期") {
					rejectedNPSSO[value] = true
				}
				continue
			}
		}
		if provider == "steam" && name == "steamLoginSecure" {
			_, token, ok := parseSteamLogin(value)
			if !ok || jwtExpiring(token, time.Minute) {
				continue
			}
		}
		return value, nil
	}
	return "", fmt.Errorf("登录超时，请重试")
}

func clearRejectedNPSSO(ctx context.Context) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return
	}
	dir = filepath.Join(dir, "forking-paths", "login-psn")
	stopLoginChrome(dir)
	key, err := chromeSafeKey()
	if err != nil {
		return
	}
	value, err := profileCookie(dir, key, "npsso", "")
	if err != nil || value == "" {
		return
	}
	code, err := (&Client{}).psnCode(ctx, value)
	if err == nil && code != "" {
		return
	}
	if err != nil && !strings.Contains(err.Error(), "无效或已过期") {
		return
	}
	db := filepath.Join(dir, "Default", "Cookies")
	if _, err := os.Stat(db); err != nil {
		return
	}
	_ = exec.Command("python3", "-c", `
import sqlite3, sys
con = sqlite3.connect(sys.argv[1])
con.execute("delete from cookies")
con.commit()
`, db).Run()
}

func clearStaleSteamCookie() {
	dir, err := os.UserCacheDir()
	if err != nil {
		return
	}
	dir = filepath.Join(dir, "forking-paths", "login-steam")
	stopLoginChrome(dir)
	key, err := chromeSafeKey()
	if err != nil {
		return
	}
	value, err := profileCookie(dir, key, "steamLoginSecure", "steamcommunity.com")
	if err != nil || value == "" {
		return
	}
	_, token, ok := parseSteamLogin(value)
	if ok && !jwtExpiring(token, time.Minute) {
		return
	}
	db := filepath.Join(dir, "Default", "Cookies")
	if _, err := os.Stat(db); err != nil {
		return
	}
	_ = exec.Command("python3", "-c", `
import sqlite3, sys
con = sqlite3.connect(sys.argv[1])
con.execute("delete from cookies where name in ('steamLoginSecure', 'steamRefresh_steam')")
con.commit()
`, db).Run()
}

func openPlainChrome(dir, page string) error {
	bin := chromeBin()
	if bin == "" {
		return fmt.Errorf("没有打开登录窗口，请确认本机安装了 Google Chrome")
	}
	cmd := exec.Command(bin, chromeArgs(dir, page)...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("没有打开登录窗口，请确认本机安装了 Google Chrome")
	}
	return nil
}

func preferredCookieHost(provider string) string {
	if provider == "steam" {
		return "steamcommunity.com"
	}
	return ""
}

func profileCookie(dir string, key []byte, name, preferHost string) (string, error) {
	db, err := copyCookieDB(dir)
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(filepath.Dir(db))
	out, err := exec.Command("python3", "-c", `
import sqlite3, sys
con = sqlite3.connect(sys.argv[1])
rows = con.execute(
    "select host_key, value, hex(encrypted_value) from cookies where name = ? order by last_access_utc desc",
    (sys.argv[2],),
).fetchall()
for host, value, enc in rows:
    print(host)
    print(value or "")
    print(enc or "")
`, db, name).Output()
	if err != nil {
		return "", err
	}
	lines := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
	var fallback string
	for i := 0; i+2 < len(lines); i += 3 {
		host, plain, encHex := lines[i], lines[i+1], lines[i+2]
		value := plain
		if value == "" {
			raw, err := hex.DecodeString(encHex)
			if err != nil || len(raw) == 0 {
				continue
			}
			value, err = decryptChromeCookie(key, host, raw)
			if err != nil || value == "" {
				continue
			}
		}
		if preferHost != "" && host == preferHost {
			return value, nil
		}
		if fallback == "" {
			fallback = value
		}
	}
	return fallback, nil
}

func copyCookieDB(dir string) (string, error) {
	srcDir := filepath.Join(dir, "Default")
	if _, err := os.Stat(filepath.Join(srcDir, "Cookies")); err != nil {
		return "", err
	}
	tmp, err := os.MkdirTemp("", "fp-cookies")
	if err != nil {
		return "", err
	}
	for _, name := range []string{"Cookies", "Cookies-wal", "Cookies-shm"} {
		b, err := os.ReadFile(filepath.Join(srcDir, name))
		if err != nil {
			if name == "Cookies" {
				os.RemoveAll(tmp)
				return "", err
			}
			continue
		}
		if err := os.WriteFile(filepath.Join(tmp, name), b, 0o600); err != nil {
			os.RemoveAll(tmp)
			return "", err
		}
	}
	return filepath.Join(tmp, "Cookies"), nil
}

var (
	chromeKeyOnce sync.Once
	chromeKey     []byte
	chromeKeyErr  error
)

func chromeSafeKey() ([]byte, error) {
	chromeKeyOnce.Do(func() {
		if runtime.GOOS == "linux" {
			chromeKey = pbkdf2.Key([]byte("peanuts"), []byte("saltysalt"), 1, 16, sha1.New)
			return
		}
		out, err := exec.Command("security", "find-generic-password", "-w", "-s", "Chrome Safe Storage", "-a", "Chrome").Output()
		if err != nil {
			chromeKeyErr = fmt.Errorf("无法读取本机 Chrome 的登录状态")
			return
		}
		chromeKey = pbkdf2.Key(bytes.TrimSpace(out), []byte("saltysalt"), 1003, 16, sha1.New)
	})
	return chromeKey, chromeKeyErr
}

func decryptChromeCookie(key []byte, host string, enc []byte) (string, error) {
	if len(enc) < 3 || (string(enc[:3]) != "v10" && string(enc[:3]) != "v11") {
		return "", fmt.Errorf("cookie")
	}
	var raw []byte
	if string(enc[:3]) == "v11" {
		payload := enc[3:]
		if len(payload) < 12+16 {
			return "", fmt.Errorf("cookie")
		}
		block, err := aes.NewCipher(key)
		if err != nil {
			return "", err
		}
		gcm, err := cipher.NewGCM(block)
		if err != nil {
			return "", err
		}
		raw, err = gcm.Open(nil, payload[:12], payload[12:], nil)
		if err != nil {
			return "", err
		}
	} else {
		if len(enc) < 3+aes.BlockSize || len(enc[3:])%aes.BlockSize != 0 {
			return "", fmt.Errorf("cookie")
		}
		block, err := aes.NewCipher(key)
		if err != nil {
			return "", err
		}
		raw = make([]byte, len(enc)-3)
		cipher.NewCBCDecrypter(block, bytes.Repeat([]byte(" "), aes.BlockSize)).CryptBlocks(raw, enc[3:])
		pad := int(raw[len(raw)-1])
		if pad <= 0 || pad > aes.BlockSize || pad > len(raw) {
			return "", fmt.Errorf("cookie")
		}
		raw = raw[:len(raw)-pad]
	}
	sum := sha256.Sum256([]byte(host))
	if bytes.HasPrefix(raw, sum[:]) {
		raw = raw[len(sum):]
	}
	return string(raw), nil
}

func loginXbox(ctx context.Context) (Session, error) {
	q := url.Values{}
	q.Set("client_id", "000000004C12AE6F")
	q.Set("redirect_uri", "https://login.live.com/oauth20_desktop.srf")
	q.Set("response_type", "code")
	q.Set("scope", "service::user.auth.xboxlive.com::MBI_SSL")
	var access, refresh string
	err := browse(ctx, "xbox", "https://login.live.com/oauth20_authorize.srf?"+q.Encode(), func(ctx context.Context) (bool, error) {
		href, err := chromedp.Run(ctx, chromedp.Evaluate[string](`window.location.href`))
		if err != nil {
			return false, err
		}
		u, err := url.Parse(href)
		if err != nil {
			return false, nil
		}
		if code := u.Query().Get("code"); code != "" {
			access, refresh, err = exchangeLiveCode(ctx, code)
			return err == nil && access != "", err
		}
		frag, _ := url.ParseQuery(u.Fragment)
		if token := frag.Get("access_token"); token != "" {
			access = token
			refresh = frag.Get("refresh_token")
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		return Session{}, err
	}
	if access == "" {
		return Session{}, fmt.Errorf("微软登录没有完成")
	}
	return exchangeXbox(ctx, access, refresh)
}

func exchangeXbox(ctx context.Context, access, refresh string) (Session, error) {
	return finishXbox(ctx, access, refresh, false)
}

func finishXbox(ctx context.Context, access, refresh string, azure bool) (Session, error) {
	ticket := access
	if azure {
		ticket = "d=" + access
	}
	userBody := map[string]any{
		"RelyingParty": "http://auth.xboxlive.com",
		"TokenType":    "JWT",
		"Properties": map[string]string{
			"AuthMethod": "RPS",
			"SiteName":   "user.auth.xboxlive.com",
			"RpsTicket":  ticket,
		},
	}
	var user struct {
		Token string `json:"Token"`
	}
	if err := postJSON(ctx, "https://user.auth.xboxlive.com/user/authenticate", userBody, &user); err != nil || user.Token == "" {
		return Session{}, fmt.Errorf("Xbox 没有接受这次登录")
	}
	xstsBody := map[string]any{
		"RelyingParty": "http://xboxlive.com",
		"TokenType":    "JWT",
		"Properties": map[string]any{
			"SandboxId":  "RETAIL",
			"UserTokens": []string{user.Token},
		},
	}
	var xsts struct {
		Token  string `json:"Token"`
		Claims struct {
			XUI []struct {
				UHS string `json:"uhs"`
				XID string `json:"xid"`
				Tag string `json:"gtg"`
			} `json:"xui"`
		} `json:"DisplayClaims"`
	}
	if err := postJSON(ctx, "https://xsts.auth.xboxlive.com/xsts/authorize", xstsBody, &xsts); err != nil || xsts.Token == "" || len(xsts.Claims.XUI) == 0 {
		return Session{}, fmt.Errorf("Xbox 没有接受这次登录")
	}
	secret := map[string]string{
		"uhs":  xsts.Claims.XUI[0].UHS,
		"xsts": xsts.Token,
		"xuid": xsts.Claims.XUI[0].XID,
	}
	if azure {
		secret["ms_refresh"] = refresh
	} else if refresh != "" {
		secret["refresh_token"] = refresh
	}
	raw, err := json.Marshal(secret)
	if err != nil {
		return Session{}, err
	}
	return Session{Account: xsts.Claims.XUI[0].Tag, Secret: string(raw)}, nil
}

func loginNintendo(ctx context.Context) (Session, error) {
	verifier, challenge := pkce()
	state := randomToken()
	q := url.Values{}
	q.Set("state", state)
	q.Set("redirect_uri", "npf"+nintendoClientID+"://auth")
	q.Set("client_id", nintendoClientID)
	q.Set("scope", "openid user user.mii user.email user.links[].id")
	q.Set("response_type", "session_token_code")
	q.Set("session_token_code_challenge", challenge)
	q.Set("session_token_code_challenge_method", "S256")
	q.Set("theme", "login_form")
	script := `(() => {
		const link = document.querySelector('a[href*="session_token_code"]');
		return link ? link.href : window.location.href;
	})()`
	var code string
	err := browse(ctx, "switch", "https://accounts.nintendo.com/connect/1.0.0/authorize?"+q.Encode(), func(ctx context.Context) (bool, error) {
		href, err := chromedp.Run(ctx, chromedp.Evaluate[string](script))
		if err != nil {
			return false, err
		}
		if !strings.Contains(href, "session_token_code=") {
			return false, nil
		}
		u, err := url.Parse(href)
		if err != nil {
			return false, nil
		}
		params := u.Query()
		if u.Fragment != "" {
			frag, _ := url.ParseQuery(u.Fragment)
			for key, values := range frag {
				params[key] = values
			}
		}
		if params.Get("state") != "" && params.Get("state") != state {
			return false, nil
		}
		if got := params.Get("session_token_code"); got != "" {
			code = got
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		return Session{}, err
	}
	if code == "" {
		return Session{}, fmt.Errorf("Nintendo 登录没有完成")
	}
	return nintendoSession(ctx, code, verifier)
}

func nintendoSession(ctx context.Context, code, verifier string) (Session, error) {
	form := url.Values{}
	form.Set("client_id", nintendoClientID)
	form.Set("session_token_code", code)
	form.Set("session_token_code_verifier", verifier)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://accounts.nintendo.com/connect/1.0.0/api/session_token", strings.NewReader(form.Encode()))
	if err != nil {
		return Session{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return Session{}, fmt.Errorf("Nintendo 登录没有完成")
	}
	defer res.Body.Close()
	var body struct {
		Session string `json:"session_token"`
	}
	if json.NewDecoder(res.Body).Decode(&body) != nil || body.Session == "" {
		return Session{}, fmt.Errorf("Nintendo 登录没有完成")
	}
	raw, _ := json.Marshal(map[string]string{"session_token": body.Session})
	return Session{Secret: string(raw)}, nil
}

func browse(ctx context.Context, provider, page string, done func(context.Context) (bool, error)) error {
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	dir = filepath.Join(dir, "forking-paths", "login-"+provider)
	allocCtx, allocCancel, pageID, err := startLoginChrome(ctx, dir, page)
	if err != nil {
		return err
	}
	defer allocCancel()
	defer stopLoginChrome(dir)
	var taskOpts []chromedp.ContextOption
	if pageID != "" {
		taskOpts = append(taskOpts, chromedp.WithTargetID(pageID))
	}
	taskCtx, taskCancel := chromedp.NewContext(allocCtx, taskOpts...)
	defer taskCancel()
	if pageID == "" {
		if _, err := chromedp.Run(taskCtx, chromedp.Navigate(page)); err != nil {
			return fmt.Errorf("没有打开登录窗口，请确认本机安装了 Google Chrome")
		}
	}
	raiseLoginChrome(dir)
	deadline := time.Now().Add(3 * time.Minute)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return fmt.Errorf("登录已取消")
		case <-time.After(700 * time.Millisecond):
		}
		if !loginChromeAlive(dir) {
			return fmt.Errorf("登录窗口已关闭")
		}
		ok, err := done(taskCtx)
		if err != nil {
			if ctx.Err() != nil {
				return fmt.Errorf("登录已取消")
			}
			continue
		}
		if ok {
			return nil
		}
	}
	return fmt.Errorf("登录超时，请重试")
}

func startLoginChrome(ctx context.Context, dir, page string) (context.Context, context.CancelFunc, target.ID, error) {
	if runtime.GOOS == "darwin" {
		if allocCtx, cancel, pageID, err := startForegroundChrome(ctx, dir, page); err == nil {
			return allocCtx, cancel, pageID, nil
		}
	}
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", false),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("password-store", "basic"),
		chromedp.UserDataDir(dir),
		chromedp.WindowSize(960, 700),
	)
	allocCtx, cancel := chromedp.NewExecAllocator(ctx, opts...)
	return allocCtx, cancel, "", nil
}

func startForegroundChrome(ctx context.Context, dir, page string) (context.Context, context.CancelFunc, target.ID, error) {
	port, err := freePort()
	if err != nil {
		return nil, nil, "", err
	}
	bin := chromeBin()
	if bin == "" {
		return nil, nil, "", fmt.Errorf("没有找到 Google Chrome")
	}
	cmd := exec.Command(bin,
		"--user-data-dir="+dir,
		"--remote-debugging-port="+strconv.Itoa(port),
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-blink-features=AutomationControlled",
		"--new-window",
		page,
	)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return nil, nil, "", err
	}
	endpoint := "http://127.0.0.1:" + strconv.Itoa(port)
	waitCtx, cancelWait := context.WithTimeout(ctx, 12*time.Second)
	defer cancelWait()
	for {
		if pageID, ok := loginPageTarget(waitCtx, endpoint); ok {
			allocCtx, cancel := remote.NewAllocator(ctx, endpoint)
			return allocCtx, cancel, pageID, nil
		}
		if waitCtx.Err() != nil {
			stopLoginChrome(dir)
			return nil, nil, "", fmt.Errorf("没有打开登录窗口")
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func loginPageTarget(ctx context.Context, endpoint string) (target.ID, bool) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"/json/list", nil)
	if err != nil {
		return "", false
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", false
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", false
	}
	var pages []struct {
		ID   target.ID `json:"id"`
		Type string    `json:"type"`
		URL  string    `json:"url"`
	}
	if json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&pages) != nil {
		return "", false
	}
	var blank target.ID
	for _, page := range pages {
		if page.Type != "page" || page.ID == "" {
			continue
		}
		if page.URL != "" && page.URL != "about:blank" && page.URL != "chrome://newtab/" {
			return page.ID, true
		}
		if blank == "" {
			blank = page.ID
		}
	}
	if blank != "" {
		return blank, true
	}
	return "", false
}

func chromeArgs(dir, page string) []string {
	args := []string{
		"--user-data-dir=" + dir,
		"--no-first-run",
		"--no-default-browser-check",
		"--new-window",
		"--window-size=960,700",
		"--window-position=0,0",
	}
	if runtime.GOOS == "linux" {
		args = append(args, "--no-sandbox", "--disable-dev-shm-usage", "--disable-gpu", "--password-store=basic")
	}
	return append(args, page)
}

func chromeBin() string {
	candidates := []string{
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		filepath.Join(os.Getenv("HOME"), "Applications/Google Chrome.app/Contents/MacOS/Google Chrome"),
		"/usr/bin/chromium",
		"/usr/bin/chromium-browser",
		"/usr/bin/google-chrome",
	}
	for _, path := range candidates {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path
		}
	}
	return ""
}

func freePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	port := ln.Addr().(*net.TCPAddr).Port
	return port, ln.Close()
}

func loginChromePIDs(dir string) []int {
	out, err := exec.Command("ps", "-ax", "-o", "pid=,command=").Output()
	if err != nil {
		return nil
	}
	var pids []int
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if !strings.Contains(line, dir) || strings.Contains(line, "Helper") || strings.Contains(line, "--type=") {
			continue
		}
		if !strings.Contains(line, "Google Chrome") && !strings.Contains(line, "chromium") && !strings.Contains(line, "chrome") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err == nil && pid > 0 && pid != os.Getpid() {
			pids = append(pids, pid)
		}
	}
	return pids
}

func loginChromeAlive(dir string) bool {
	return len(loginChromePIDs(dir)) > 0
}

func raiseLoginChrome(dir string) {
	if runtime.GOOS != "darwin" {
		return
	}
	pids := loginChromePIDs(dir)
	if len(pids) == 0 {
		return
	}
	script := fmt.Sprintf(`tell application "System Events" to set frontmost of the first process whose unix id is %d to true`, pids[0])
	_ = exec.Command("osascript", "-e", script).Run()
}

func stopLoginChrome(dir string) {
	for _, pid := range loginChromePIDs(dir) {
		proc, err := os.FindProcess(pid)
		if err != nil {
			continue
		}
		_ = proc.Signal(syscall.SIGTERM)
	}
}

func postJSON(ctx context.Context, raw string, body any, dst any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, raw, strings.NewReader(string(payload)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-xbl-contract-version", "1")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	buf, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("status %d", res.StatusCode)
	}
	return json.Unmarshal(buf, dst)
}

func pkce() (verifier, challenge string) {
	verifier = randomToken()
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge
}

func randomToken() string {
	buf := make([]byte, 32)
	_, _ = rand.Read(buf)
	return base64.RawURLEncoding.EncodeToString(buf)
}
