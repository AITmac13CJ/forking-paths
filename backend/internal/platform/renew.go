package platform

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (c *Client) Renew(ctx context.Context, provider, secret string) (string, bool, error) {
	switch provider {
	case "steam":
		return c.renewSteam(ctx, secret)
	case "psn":
		return c.renewPSN(ctx, secret)
	case "xbox":
		return c.renewXbox(ctx, secret)
	default:
		return secret, false, nil
	}
}

func (c *Client) renewSteam(ctx context.Context, secret string) (string, bool, error) {
	var cred struct {
		SteamID string `json:"steam_id"`
		Access  string `json:"access_token"`
		Refresh string `json:"refresh_token"`
	}
	if json.Unmarshal([]byte(secret), &cred) != nil || cred.SteamID == "" {
		return secret, false, fmt.Errorf("请重新登录 Steam")
	}
	if cred.Access == "" {
		return secret, false, nil
	}
	foundRefresh := false
	if cred.Refresh == "" {
		id, refresh := localSteamRefresh()
		if refresh != "" && (id == "" || id == cred.SteamID) {
			cred.Refresh = refresh
			foundRefresh = true
		}
	}
	if cred.Refresh == "" {
		if jwtExpiring(cred.Access, 0) {
			return secret, false, fmt.Errorf("Steam 登录已过期，请重新登录")
		}
		return secret, false, nil
	}
	if !foundRefresh && !jwtExpiring(cred.Access, 6*time.Hour) {
		return secret, false, nil
	}
	if !jwtExpiring(cred.Access, 6*time.Hour) {
		next, err := json.Marshal(cred)
		if err != nil {
			return secret, false, nil
		}
		return string(next), true, nil
	}
	access, refresh, err := c.steamRefresh(ctx, cred.SteamID, cred.Refresh)
	if err != nil {
		if jwtExpiring(cred.Access, 0) {
			if steamRejected(err) {
				return secret, false, fmt.Errorf("Steam 登录已过期，请重新登录")
			}
			return secret, false, fmt.Errorf("Steam 暂时连不上，登录凭证还在，请稍后再试")
		}
		if foundRefresh {
			next, merr := json.Marshal(cred)
			if merr == nil {
				return string(next), true, nil
			}
		}
		return secret, false, nil
	}
	cred.Access = access
	if refresh != "" {
		cred.Refresh = refresh
	}
	next, err := json.Marshal(cred)
	if err != nil {
		return secret, false, nil
	}
	return string(next), true, nil
}

func (c *Client) steamRefresh(ctx context.Context, steamID, refresh string) (string, string, error) {
	form := url.Values{}
	form.Set("steamid", steamID)
	form.Set("refresh_token", refresh)
	var body struct {
		Response struct {
			Access  string `json:"access_token"`
			Refresh string `json:"refresh_token"`
		} `json:"response"`
		Access  string `json:"access_token"`
		Refresh string `json:"refresh_token"`
	}
	if err := c.postForm(ctx, "https://api.steampowered.com/IAuthenticationService/GenerateAccessTokenForApp/v1/", form, &body); err != nil {
		return "", "", err
	}
	access := body.Response.Access
	if access == "" {
		access = body.Access
	}
	nextRefresh := body.Response.Refresh
	if nextRefresh == "" {
		nextRefresh = body.Refresh
	}
	if access == "" {
		return "", "", fmt.Errorf("status 401")
	}
	return access, nextRefresh, nil
}

func parsePSNCred(secret string) (psnCred, bool) {
	secret = strings.TrimSpace(secret)
	if strings.HasPrefix(secret, "{") {
		var cred psnCred
		if json.Unmarshal([]byte(secret), &cred) != nil {
			return psnCred{}, false
		}
		return cred, true
	}
	if secret == "" {
		return psnCred{}, false
	}
	return psnCred{NPSSO: secret}, true
}

func (c *Client) renewPSN(ctx context.Context, secret string) (string, bool, error) {
	cred, ok := parsePSNCred(secret)
	if !ok {
		return secret, false, fmt.Errorf("请重新登录 PSN")
	}
	if cred.OnlineID != "" && cred.NPSSO == "" && cred.Access == "" && cred.Refresh == "" {
		return secret, false, nil
	}
	if cred.Access != "" && !jwtExpiring(cred.Access, time.Hour) {
		return secret, false, nil
	}
	var access, refresh string
	var err error
	if cred.Refresh != "" {
		access, refresh, err = c.psnOAuth(ctx, "", cred.Refresh)
	}
	if access == "" && cred.NPSSO != "" {
		access, refresh, err = c.psnOAuth(ctx, cred.NPSSO, "")
	}
	if access == "" {
		if cred.Access != "" && !jwtExpiring(cred.Access, 0) {
			return secret, false, nil
		}
		if err != nil {
			return secret, false, err
		}
		return secret, false, fmt.Errorf("请重新登录 PSN")
	}
	if refresh != "" {
		cred.Refresh = refresh
	}
	cred.Access = access
	next, merr := json.Marshal(cred)
	if merr != nil {
		return secret, false, nil
	}
	return string(next), true, nil
}

type psnCred struct {
	NPSSO    string `json:"npsso,omitempty"`
	Access   string `json:"access_token,omitempty"`
	Refresh  string `json:"refresh_token,omitempty"`
	OnlineID string `json:"online_id,omitempty"`
}

func (c *Client) psnOAuth(ctx context.Context, npsso, refresh string) (string, string, error) {
	form := url.Values{}
	form.Set("token_format", "jwt")
	form.Set("scope", "psn:mobile.v2.core psn:clientapp")
	if refresh != "" {
		form.Set("grant_type", "refresh_token")
		form.Set("refresh_token", refresh)
	} else {
		code, err := c.psnCode(ctx, npsso)
		if err != nil {
			return "", "", err
		}
		form.Set("grant_type", "authorization_code")
		form.Set("code", code)
		form.Set("redirect_uri", "com.scee.psxandroid.scecompcall://redirect")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://ca.account.sony.com/api/authz/v3/oauth/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", "", fmt.Errorf("PSN 令牌交换失败")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Basic MDk1MTUxNTktNzIzNy00MzcwLTliNDAtMzgwNmU2N2MwODkxOnVjUGprYTV0bnRCMktxc1A=")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("PSN 令牌交换失败")
	}
	defer res.Body.Close()
	return readPSNToken(res)
}

func (c *Client) psnCode(ctx context.Context, npsso string) (string, error) {
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
		return "", fmt.Errorf("PSN 登录没有完成")
	}
	req.Header.Set("Cookie", "npsso="+npsso)
	res, err := noRedirect.Do(req)
	if err != nil {
		return "", fmt.Errorf("PSN 登录没有完成")
	}
	defer res.Body.Close()
	loc := res.Header.Get("Location")
	if u, err := url.Parse(loc); err == nil {
		if code := u.Query().Get("code"); code != "" {
			return code, nil
		}
	}
	return "", fmt.Errorf("NPSSO 无效或已过期")
}

func readPSNToken(res *http.Response) (string, string, error) {
	var tok struct {
		Access  string `json:"access_token"`
		Refresh string `json:"refresh_token"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&tok); err != nil || tok.Access == "" {
		return "", "", fmt.Errorf("PSN 没有返回访问令牌")
	}
	return tok.Access, tok.Refresh, nil
}

func (c *Client) renewXbox(ctx context.Context, secret string) (string, bool, error) {
	var cred struct {
		UHS       string `json:"uhs"`
		XSTS      string `json:"xsts"`
		XUID      string `json:"xuid"`
		Refresh   string `json:"refresh_token"`
		MSRefresh string `json:"ms_refresh"`
	}
	if json.Unmarshal([]byte(secret), &cred) != nil || cred.XSTS == "" {
		return secret, false, nil
	}
	if cred.MSRefresh != "" {
		if !jwtExpiring(cred.XSTS, 2*time.Hour) {
			return secret, false, nil
		}
		access, refresh, err := refreshMicrosoft(ctx, cred.MSRefresh)
		if err != nil {
			if jwtExpiring(cred.XSTS, 0) {
				return secret, false, fmt.Errorf("Xbox 登录已过期，请重新登录")
			}
			return secret, false, nil
		}
		if refresh == "" {
			refresh = cred.MSRefresh
		}
		session, err := finishXbox(ctx, access, refresh, true)
		if err != nil {
			if jwtExpiring(cred.XSTS, 0) {
				return secret, false, err
			}
			return secret, false, nil
		}
		return session.Secret, true, nil
	}
	if cred.Refresh == "" || !jwtExpiring(cred.XSTS, 2*time.Hour) {
		return secret, false, nil
	}
	access, refresh, err := refreshLive(ctx, cred.Refresh)
	if err != nil {
		if jwtExpiring(cred.XSTS, 0) {
			return secret, false, fmt.Errorf("Xbox 登录已过期，请重新登录")
		}
		return secret, false, nil
	}
	if refresh == "" {
		refresh = cred.Refresh
	}
	session, err := exchangeXbox(ctx, access, refresh)
	if err != nil {
		if jwtExpiring(cred.XSTS, 0) {
			return secret, false, err
		}
		return secret, false, nil
	}
	return session.Secret, true, nil
}

func exchangeLiveCode(ctx context.Context, code string) (string, string, error) {
	form := url.Values{}
	form.Set("client_id", "000000004C12AE6F")
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", "https://login.live.com/oauth20_desktop.srf")
	form.Set("scope", "service::user.auth.xboxlive.com::MBI_SSL")
	return liveToken(ctx, form)
}

func refreshLive(ctx context.Context, refresh string) (string, string, error) {
	form := url.Values{}
	form.Set("client_id", "000000004C12AE6F")
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refresh)
	form.Set("scope", "service::user.auth.xboxlive.com::MBI_SSL")
	return liveToken(ctx, form)
}

func liveToken(ctx context.Context, form url.Values) (string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://login.live.com/oauth20_token.srf", strings.NewReader(form.Encode()))
	if err != nil {
		return "", "", fmt.Errorf("Xbox 登录已过期，请重新登录")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("Xbox 登录已过期，请重新登录")
	}
	defer res.Body.Close()
	var tok struct {
		Access  string `json:"access_token"`
		Refresh string `json:"refresh_token"`
	}
	if json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&tok) != nil || tok.Access == "" {
		return "", "", fmt.Errorf("Xbox 登录已过期，请重新登录")
	}
	return tok.Access, tok.Refresh, nil
}

func steamRejected(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "status 401") || strings.Contains(msg, "status 403")
}

func localSteamRefresh() (string, string) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", ""
	}
	dir = filepath.Join(dir, "forking-paths", "login-steam")
	key, err := chromeSafeKey()
	if err != nil {
		return "", ""
	}
	raw, err := profileCookie(dir, key, "steamRefresh_steam", "login.steampowered.com")
	if err != nil || raw == "" {
		return "", ""
	}
	if decoded, err := url.QueryUnescape(raw); err == nil {
		raw = decoded
	}
	raw = strings.TrimSpace(raw)
	if id, token, ok := parseSteamLogin(raw); ok {
		return id, token
	}
	return jwtSub(raw), raw
}

func SteamAccessUsable(token string) bool {
	return token != "" && !jwtExpiring(token, time.Minute)
}

func jwtExpiring(token string, within time.Duration) bool {
	exp, ok := jwtExp(token)
	if !ok {
		return true
	}
	return time.Until(exp) <= within
}

func jwtExp(token string) (time.Time, bool) {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return time.Time{}, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}, false
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if json.Unmarshal(raw, &claims) != nil || claims.Exp == 0 {
		return time.Time{}, false
	}
	return time.Unix(claims.Exp, 0), true
}

func jwtSub(token string) string {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return ""
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var claims struct {
		Sub string `json:"sub"`
	}
	if json.Unmarshal(raw, &claims) != nil {
		return ""
	}
	return claims.Sub
}
