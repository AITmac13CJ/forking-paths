package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

type switchWait struct {
	verifier string
	until    time.Time
}

var switchWaits sync.Map

func BrowserLogin(provider string) (string, error) {
	switch provider {
	case "xbox":
		q := url.Values{}
		q.Set("client_id", "000000004C12AE6F")
		q.Set("redirect_uri", "https://login.live.com/oauth20_desktop.srf")
		q.Set("response_type", "code")
		q.Set("scope", "service::user.auth.xboxlive.com::MBI_SSL")
		return "https://login.live.com/oauth20_authorize.srf?" + q.Encode(), nil
	case "psn":
		return "https://ca.account.sony.com/api/v1/ssocookie", nil
	case "switch":
		verifier, challenge := pkce()
		state := randomToken()
		switchWaits.Store(state, switchWait{verifier: verifier, until: time.Now().Add(10 * time.Minute)})
		q := url.Values{}
		q.Set("state", state)
		q.Set("redirect_uri", "npf"+nintendoClientID+"://auth")
		q.Set("client_id", nintendoClientID)
		q.Set("scope", "openid user user.mii user.email user.links[].id")
		q.Set("response_type", "session_token_code")
		q.Set("session_token_code_challenge", challenge)
		q.Set("session_token_code_challenge_method", "S256")
		q.Set("theme", "login_form")
		return "https://accounts.nintendo.com/connect/1.0.0/authorize?" + q.Encode(), nil
	default:
		return "", fmt.Errorf("未知的平台")
	}
}

func FinishBrowserLogin(ctx context.Context, provider, raw string) (Session, error) {
	values := pastedParams(raw)
	switch provider {
	case "xbox":
		if code := values.Get("code"); code != "" {
			access, refresh, err := exchangeLiveCode(ctx, code)
			if err != nil || access == "" {
				return Session{}, fmt.Errorf("微软登录没有完成")
			}
			return exchangeXbox(ctx, access, refresh)
		}
		if token := values.Get("access_token"); token != "" {
			return exchangeXbox(ctx, token, values.Get("refresh_token"))
		}
		return Session{}, fmt.Errorf("这段网址里没有微软登录结果")
	case "psn":
		code := values.Get("code")
		if code == "" {
			return Session{}, fmt.Errorf("这段网址里没有 PlayStation 登录结果")
		}
		return exchangePSNCode(ctx, code)
	case "switch":
		code := values.Get("session_token_code")
		state := values.Get("state")
		if code == "" || state == "" {
			return Session{}, fmt.Errorf("这段网址里没有任天堂登录结果")
		}
		item, ok := switchWaits.LoadAndDelete(state)
		if !ok {
			return Session{}, fmt.Errorf("登录已过期，请重新打开任天堂页面")
		}
		wait := item.(switchWait)
		if time.Now().After(wait.until) {
			return Session{}, fmt.Errorf("登录已过期，请重新打开任天堂页面")
		}
		return nintendoSession(ctx, code, wait.verifier)
	default:
		return Session{}, fmt.Errorf("未知的平台")
	}
}

func AcceptNPSSO(ctx context.Context, raw string) (Session, error) {
	npsso := parseNPSSO(raw)
	if npsso == "" {
		return Session{}, fmt.Errorf("这里没有看到 npsso")
	}
	access, refresh, err := NewClient().psnOAuth(ctx, npsso, "")
	if err != nil || access == "" {
		if err != nil && strings.Contains(err.Error(), "无效或已过期") {
			return Session{}, err
		}
		return Session{}, fmt.Errorf("NPSSO 无效或已过期")
	}
	secret, err := json.Marshal(map[string]string{
		"npsso":         npsso,
		"access_token":  access,
		"refresh_token": refresh,
	})
	if err != nil {
		return Session{}, err
	}
	account := psnOnlineID(ctx, access)
	if account == "" {
		account = "PlayStation"
	}
	return Session{Account: account, Secret: string(secret)}, nil
}

func parseNPSSO(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.Contains(raw, "npsso") {
		var body struct {
			NPSSO string `json:"npsso"`
		}
		if json.Unmarshal([]byte(raw), &body) == nil {
			if value := strings.TrimSpace(body.NPSSO); value != "" {
				return value
			}
		}
		if match := npssoPattern.FindStringSubmatch(raw); len(match) == 2 {
			return strings.TrimSpace(match[1])
		}
	}
	if strings.ContainsAny(raw, " \n\t{}") || len(raw) < 32 {
		return ""
	}
	return raw
}

func psnOnlineID(ctx context.Context, access string) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://m.np.playstation.com/api/userProfile/v1/internal/users/me/profiles", nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Authorization", "Bearer "+access)
	res, err := NewClient().HTTP.Do(req)
	if err != nil {
		return ""
	}
	defer res.Body.Close()
	var body struct {
		OnlineID string `json:"onlineId"`
	}
	if json.NewDecoder(res.Body).Decode(&body) != nil {
		return ""
	}
	return strings.TrimSpace(body.OnlineID)
}

var npssoPattern = regexp.MustCompile(`"npsso"\s*:\s*"([^"]+)"`)

func exchangePSNCode(ctx context.Context, code string) (Session, error) {
	form := url.Values{}
	form.Set("token_format", "jwt")
	form.Set("scope", "psn:mobile.v2.core psn:clientapp")
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", "com.scee.psxandroid.scecompcall://redirect")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://ca.account.sony.com/api/authz/v3/oauth/token", strings.NewReader(form.Encode()))
	if err != nil {
		return Session{}, fmt.Errorf("PSN 登录没有完成")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Basic MDk1MTUxNTktNzIzNy00MzcwLTliNDAtMzgwNmU2N2MwODkxOnVjUGprYTV0bnRCMktxc1A=")
	res, err := NewClient().HTTP.Do(req)
	if err != nil {
		return Session{}, fmt.Errorf("PSN 登录没有完成")
	}
	defer res.Body.Close()
	access, refresh, err := readPSNToken(res)
	if err != nil || access == "" {
		return Session{}, fmt.Errorf("PSN 登录没有完成")
	}
	raw, err := json.Marshal(map[string]string{"access_token": access, "refresh_token": refresh})
	if err != nil {
		return Session{}, err
	}
	return Session{Account: "PlayStation", Secret: string(raw)}, nil
}

func pastedParams(raw string) url.Values {
	raw = strings.TrimSpace(raw)
	for _, marker := range []string{"npf", "com.scee.", "https://", "http://"} {
		i := strings.Index(raw, marker)
		if i > 0 && strings.Contains(raw[i:], "://") {
			raw = raw[i:]
			break
		}
	}
	fields := strings.Fields(raw)
	if len(fields) == 0 {
		return url.Values{}
	}
	u, err := url.Parse(fields[0])
	values := url.Values{}
	if err == nil && u != nil {
		values = u.Query()
		if u.Fragment != "" {
			frag, _ := url.ParseQuery(strings.TrimPrefix(u.Fragment, "?"))
			for key, list := range frag {
				values[key] = list
			}
		}
	}
	return values
}
