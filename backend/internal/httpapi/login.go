package httpapi

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"runtime"
	"strings"

	"forking-paths/internal/platform"
	"forking-paths/internal/store"
)

func (s *Server) steamLogin(w http.ResponseWriter, r *http.Request) {
	if user, ok := platform.LocalSteamUser(); ok {
		if err := s.store.SaveAccount(r.Context(), "steam", user.ID, "openid"); err != nil {
			loginPage(w, "登录状态没有保存")
			return
		}
		loginPage(w, "已使用 Steam 客户端登录："+user.Name+"。回到花园，点同步即可。")
		return
	}
	base := publicBase(r)
	q := url.Values{}
	q.Set("openid.ns", "http://specs.openid.net/auth/2.0")
	q.Set("openid.mode", "checkid_setup")
	q.Set("openid.claimed_id", "http://specs.openid.net/auth/2.0/identifier_select")
	q.Set("openid.identity", "http://specs.openid.net/auth/2.0/identifier_select")
	q.Set("openid.return_to", base+"/api/sync/steam/callback")
	q.Set("openid.realm", base)
	http.Redirect(w, r, "https://steamcommunity.com/openid/login?"+q.Encode(), http.StatusFound)
}

func (s *Server) steamCallback(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	if query.Get("openid.mode") != "id_res" {
		loginPage(w, "Steam 登录没有完成")
		return
	}
	claimed := query.Get("openid.claimed_id")
	steamID := claimed
	if i := strings.LastIndex(claimed, "/"); i >= 0 {
		steamID = claimed[i+1:]
	}
	if len(steamID) < 17 {
		loginPage(w, "Steam 登录没有完成")
		return
	}
	form := url.Values{}
	for key, values := range query {
		for _, value := range values {
			form.Add(key, value)
		}
	}
	form.Set("openid.mode", "check_authentication")
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, "https://steamcommunity.com/openid/login", strings.NewReader(form.Encode()))
	if err != nil {
		loginPage(w, "Steam 登录没有完成")
		return
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		loginPage(w, "Steam 登录没有完成")
		return
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if !strings.Contains(string(body), "is_valid:true") {
		loginPage(w, "Steam 登录没有通过校验")
		return
	}
	name := platform.SteamPersona(r.Context(), steamID)
	if name == "" {
		name = steamID
	}
	secret := map[string]string{"steam_id": steamID}
	if _, existing, err := s.store.AccountSecret(r.Context(), "steam"); err == nil {
		var prev struct {
			SteamID string `json:"steam_id"`
			Access  string `json:"access_token"`
			Refresh string `json:"refresh_token"`
		}
		if json.Unmarshal([]byte(existing), &prev) == nil && prev.SteamID == steamID {
			if prev.Access != "" {
				secret["access_token"] = prev.Access
			}
			if prev.Refresh != "" {
				secret["refresh_token"] = prev.Refresh
			}
		}
	}
	if secret["access_token"] == "" || !platform.SteamAccessUsable(secret["access_token"]) {
		if access, refresh, ok := s.store.SteamWebSession(r.Context(), steamID); ok && platform.SteamAccessUsable(access) {
			secret["access_token"] = access
			if refresh != "" {
				secret["refresh_token"] = refresh
			}
		}
	}
	raw, _ := json.Marshal(secret)
	if err := s.store.SaveAccount(r.Context(), "steam", name, string(raw)); err != nil {
		loginPage(w, "登录状态没有保存")
		return
	}
	http.Redirect(w, r, publicBase(r)+"/", http.StatusFound)
}

func (s *Server) platformLogin(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	if !store.KnownProvider(provider) {
		writeErr(w, &store.ValidationError{Msg: "未知的平台"})
		return
	}
	if runtime.GOOS != "darwin" {
		if provider == "steam" {
			raw, err := browserLoginURL(r, provider)
			if err != nil {
				writeErr(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"url": raw})
			return
		}
		if provider == "psn" {
			raw, err := platform.BrowserLogin(provider)
			if err != nil {
				writeErr(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"url": raw})
			return
		}
		if provider == "switch" {
			raw, err := platform.BrowserLogin(provider)
			if err != nil {
				writeErr(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"url": raw})
			return
		}
		if provider == "xbox" {
			raw, state, err := platform.XboxWebAuthorize(publicBase(r) + "/api/sync/xbox/callback")
			if err != nil {
				writeErr(w, &store.ValidationError{Msg: err.Error()})
				return
			}
			http.SetCookie(w, &http.Cookie{
				Name:     "xbox_oauth",
				Value:    state,
				Path:     "/api/sync/xbox/callback",
				MaxAge:   600,
				HttpOnly: true,
				SameSite: http.SameSiteLaxMode,
			})
			writeJSON(w, http.StatusOK, map[string]string{"url": raw})
			return
		}
		writeErr(w, &store.ValidationError{Msg: "这个平台不把登录结果交回网站"})
		return
	}
	session, err := platform.Login(r.Context(), provider)
	if err != nil {
		writeErr(w, &store.ValidationError{Msg: err.Error()})
		return
	}
	if err := s.store.SaveAccount(r.Context(), provider, session.Account, session.Secret); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"account": session.Account})
}

func (s *Server) xboxCallback(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	cookie, cookieErr := r.Cookie("xbox_oauth")
	http.SetCookie(w, &http.Cookie{
		Name:     "xbox_oauth",
		Value:    "",
		Path:     "/api/sync/xbox/callback",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	if query.Get("error") != "" || cookieErr != nil || cookie.Value == "" || cookie.Value != query.Get("state") || query.Get("code") == "" {
		loginPage(w, "微软登录没有完成")
		return
	}
	session, err := platform.XboxWebFinish(r.Context(), publicBase(r)+"/api/sync/xbox/callback", query.Get("code"))
	if err != nil {
		loginPage(w, err.Error())
		return
	}
	if err := s.store.SaveAccount(r.Context(), "xbox", session.Account, session.Secret); err != nil {
		loginPage(w, "登录状态没有保存")
		return
	}
	http.Redirect(w, r, publicBase(r)+"/", http.StatusFound)
}

func browserLoginURL(r *http.Request, provider string) (string, error) {
	if provider != "steam" {
		return "", &store.ValidationError{Msg: "未知的平台"}
	}
	base := publicBase(r)
	q := url.Values{}
	q.Set("openid.ns", "http://specs.openid.net/auth/2.0")
	q.Set("openid.mode", "checkid_setup")
	q.Set("openid.claimed_id", "http://specs.openid.net/auth/2.0/identifier_select")
	q.Set("openid.identity", "http://specs.openid.net/auth/2.0/identifier_select")
	q.Set("openid.return_to", base+"/api/sync/steam/callback")
	q.Set("openid.realm", base)
	return "https://steamcommunity.com/openid/login?" + q.Encode(), nil
}

func publicBase(r *http.Request) string {
	proto := r.Header.Get("X-Forwarded-Proto")
	if proto != "https" && proto != "http" {
		host := r.Host
		if strings.HasPrefix(host, "localhost") || strings.HasPrefix(host, "127.0.0.1") || strings.HasPrefix(host, "[::1]") {
			proto = "http"
		} else {
			proto = "https"
		}
	}
	return proto + "://" + r.Host
}

func loginPage(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, "<!doctype html><meta charset=utf-8><title>登录</title><p style=\"font:16px sans-serif;padding:24px\">%s</p>", html.EscapeString(message))
}
