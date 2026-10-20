package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
)

func XboxWebConfigured() bool {
	id, secret := xboxApp()
	return id != "" && secret != ""
}

func xboxApp() (string, string) {
	return strings.TrimSpace(os.Getenv("XBOX_CLIENT_ID")), strings.TrimSpace(os.Getenv("XBOX_CLIENT_SECRET"))
}

func XboxWebAuthorize(redirectURI string) (string, string, error) {
	id, secret := xboxApp()
	if id == "" || secret == "" {
		return "", "", fmt.Errorf("Xbox 网站登录还没配好")
	}
	state := randomToken()
	q := url.Values{}
	q.Set("client_id", id)
	q.Set("response_type", "code")
	q.Set("redirect_uri", redirectURI)
	q.Set("response_mode", "query")
	q.Set("scope", "XboxLive.signin XboxLive.offline_access")
	q.Set("state", state)
	return "https://login.microsoftonline.com/consumers/oauth2/v2.0/authorize?" + q.Encode(), state, nil
}

func XboxWebFinish(ctx context.Context, redirectURI, code string) (Session, error) {
	id, secret := xboxApp()
	if id == "" || secret == "" || code == "" {
		return Session{}, fmt.Errorf("微软登录没有完成")
	}
	form := url.Values{}
	form.Set("client_id", id)
	form.Set("client_secret", secret)
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	access, refresh, err := microsoftToken(ctx, form)
	if err != nil || access == "" {
		return Session{}, fmt.Errorf("微软登录没有完成")
	}
	return finishXbox(ctx, access, refresh, true)
}

func refreshMicrosoft(ctx context.Context, refresh string) (string, string, error) {
	id, secret := xboxApp()
	if id == "" || secret == "" || refresh == "" {
		return "", "", fmt.Errorf("Xbox 登录已过期，请重新登录")
	}
	form := url.Values{}
	form.Set("client_id", id)
	form.Set("client_secret", secret)
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refresh)
	form.Set("scope", "XboxLive.signin XboxLive.offline_access")
	return microsoftToken(ctx, form)
}

func microsoftToken(ctx context.Context, form url.Values) (string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://login.microsoftonline.com/consumers/oauth2/v2.0/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", "", fmt.Errorf("微软登录没有完成")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("微软登录没有完成")
	}
	defer res.Body.Close()
	var tok struct {
		Access  string `json:"access_token"`
		Refresh string `json:"refresh_token"`
	}
	if json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&tok) != nil || tok.Access == "" {
		return "", "", fmt.Errorf("微软登录没有完成")
	}
	return tok.Access, tok.Refresh, nil
}
