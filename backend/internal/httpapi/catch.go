package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"runtime"
	"sync"
	"time"

	"forking-paths/internal/platform"
	"forking-paths/internal/store"
)

const catchSite = "https://atimac13cj.com"

var catchClient = &http.Client{
	Timeout: 20 * time.Second,
	Transport: &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			if host, port, err := net.SplitHostPort(addr); err == nil && host == "atimac13cj.com" {
				addr = net.JoinHostPort("20.229.113.191", port)
			}
			return (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, network, addr)
		},
	},
}

type catchTicket struct {
	userID   int
	provider string
	until    time.Time
}

var catchTickets sync.Map

func newCatchTicket(userID int, provider string) (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	ticket := hex.EncodeToString(buf)
	catchTickets.Store(ticket, catchTicket{userID: userID, provider: provider, until: time.Now().Add(10 * time.Minute)})
	return ticket, nil
}

func takeCatchTicket(ticket, provider string) (catchTicket, error) {
	item, ok := catchTickets.LoadAndDelete(ticket)
	if !ok {
		return catchTicket{}, &store.ValidationError{Msg: "登录已过期，请回到花园再点一次"}
	}
	wait := item.(catchTicket)
	if time.Now().After(wait.until) || wait.provider != provider {
		return catchTicket{}, &store.ValidationError{Msg: "登录已过期，请回到花园再点一次"}
	}
	return wait, nil
}

func (s *Server) catchLogin(w http.ResponseWriter, r *http.Request) {
	if runtime.GOOS != "darwin" {
		loginPage(w, "请在自己的电脑上打开花园再登录。")
		return
	}
	provider := r.URL.Query().Get("provider")
	ticket := r.URL.Query().Get("ticket")
	name := r.URL.Query().Get("name")
	if (provider != "steam" && provider != "psn" && provider != "switch") || len(ticket) < 32 {
		loginPage(w, "登录没有开始")
		return
	}
	label := "PlayStation"
	switch provider {
	case "switch":
		label = "任天堂"
	case "steam":
		label = "Steam"
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, "<!doctype html><meta charset=utf-8><title>登录</title><p style=\"font:16px sans-serif;padding:24px\">正在用这台电脑打开%s官网。在弹出的窗口里登录，密码只交给官网。完成后回到花园账号 %s。</p>", html.EscapeString(label), html.EscapeString(name))
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	session, err := platform.Login(r.Context(), provider)
	if err != nil {
		fmt.Fprintf(w, "<p style=\"font:16px sans-serif;padding:24px\">%s</p>", html.EscapeString(err.Error()))
		return
	}
	if err := deliverCatch(r.Context(), ticket, provider, session); err != nil {
		fmt.Fprintf(w, "<p style=\"font:16px sans-serif;padding:24px\">%s</p>", html.EscapeString(err.Error()))
		return
	}
	fmt.Fprintf(w, "<script>location.replace(%q)</script>", catchSite+"/")
}

func deliverCatch(ctx context.Context, ticket, provider string, session platform.Session) error {
	body, err := json.Marshal(map[string]string{
		"ticket":   ticket,
		"provider": provider,
		"account":  session.Account,
		"secret":   session.Secret,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, catchSite+"/api/sync/catch", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := catchClient.Do(req)
	if err != nil {
		return fmt.Errorf("登录结果没有送回花园")
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(res.Body, 512))
		var payload struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(msg, &payload) == nil && payload.Error != "" {
			return fmt.Errorf("%s", payload.Error)
		}
		return fmt.Errorf("登录结果没有送回花园")
	}
	return nil
}

func (s *Server) catchDone(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Ticket   string `json:"ticket"`
		Provider string `json:"provider"`
		Account  string `json:"account"`
		Secret   string `json:"secret"`
	}
	if err := decode(w, r, &body); err != nil {
		writeErr(w, err)
		return
	}
	if body.Provider != "steam" && body.Provider != "psn" && body.Provider != "switch" {
		writeErr(w, &store.ValidationError{Msg: "未知的平台"})
		return
	}
	wait, err := takeCatchTicket(body.Ticket, body.Provider)
	if err != nil {
		writeErr(w, err)
		return
	}
	ctx := store.WithUser(r.Context(), wait.userID)
	if err := s.store.SaveAccount(ctx, body.Provider, body.Account, body.Secret); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
