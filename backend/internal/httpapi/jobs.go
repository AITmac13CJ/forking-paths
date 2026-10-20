package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"forking-paths/internal/platform"
	"forking-paths/internal/store"
)

func (s *Server) listJobs(w http.ResponseWriter, r *http.Request) {
	jobs, err := s.store.ListJobs(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, jobs)
}

func (s *Server) ackJob(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id <= 0 {
		writeErr(w, &store.ValidationError{Msg: "找不到这次同步"})
		return
	}
	if err := s.store.AckJob(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) performSync(jobID, uid int, provider string) {
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("同步中断 %s: %v", provider, rec)
			s.finishJob(uid, jobID, "error", "同步中断了", 0, 0)
		}
	}()
	work, cancel := context.WithTimeout(store.WithUser(context.Background(), uid), 45*time.Minute)
	defer cancel()
	report := func(done, total int, message string) {
		ctx, cancel := context.WithTimeout(store.WithUser(context.Background(), uid), 10*time.Second)
		defer cancel()
		_ = s.store.TouchJob(ctx, jobID, done, total, message)
	}
	account, secret, err := s.store.AccountSecret(work, provider)
	if err != nil {
		s.finishJob(uid, jobID, "error", err.Error(), 0, 0)
		return
	}
	if provider == "steam" {
		secret = s.hydrateSteam(work, secret)
		secret = s.borrowSteamAccess(work, secret)
	}
	if provider == "psn" {
		secret = s.borrowPSNAccess(work, secret)
	}
	client := platform.NewClient()
	if next, changed, rerr := client.Renew(work, provider, secret); changed {
		secret = next
		_ = s.store.UpdateSecret(work, provider, secret)
	} else if rerr != nil {
		s.finishJob(uid, jobID, "error", rerr.Error(), 0, 0)
		return
	}
	games, err := client.Fetch(work, provider, account, secret, report)
	if err != nil {
		s.finishJob(uid, jobID, "error", err.Error(), 0, 0)
		return
	}
	report(0, len(games), "正在写入花园")
	result, err := s.store.ApplySync(work, provider, games, func(done, total int) {
		report(done, total, "正在写入花园")
	})
	if err != nil {
		s.finishJob(uid, jobID, "error", err.Error(), 0, 0)
		return
	}
	msg := fmt.Sprintf("已同步。更新 %d 条，新加入 %d 条。有时长的在「仍在途中」，没时长的在总览的未开始里。", result.Updated, result.Created)
	s.finishJob(uid, jobID, "done", msg, result.Updated, result.Created)
}

func (s *Server) borrowSteamAccess(ctx context.Context, secret string) string {
	var cred struct {
		SteamID string `json:"steam_id"`
		Access  string `json:"access_token"`
		Refresh string `json:"refresh_token"`
	}
	if json.Unmarshal([]byte(secret), &cred) != nil || cred.SteamID == "" || platform.SteamAccessUsable(cred.Access) {
		return secret
	}
	tokens, err := s.store.SteamAccessTokens(ctx)
	if err != nil {
		return secret
	}
	for _, token := range tokens {
		if !platform.SteamAccessUsable(token) {
			continue
		}
		cred.Access = token
		cred.Refresh = ""
		next, err := json.Marshal(cred)
		if err != nil {
			return secret
		}
		return string(next)
	}
	return secret
}

func (s *Server) borrowPSNAccess(ctx context.Context, secret string) string {
	var cred struct {
		OnlineID string `json:"online_id"`
		NPSSO    string `json:"npsso"`
		Access   string `json:"access_token"`
	}
	if json.Unmarshal([]byte(secret), &cred) != nil || cred.OnlineID == "" || cred.NPSSO != "" || platform.SteamAccessUsable(cred.Access) {
		return secret
	}
	stored, err := s.store.PSNSecrets(ctx)
	if err != nil {
		return secret
	}
	client := platform.NewClient()
	for _, raw := range stored {
		access := platform.PSNAccessFromSecret(ctx, client, raw)
		if !platform.SteamAccessUsable(access) {
			continue
		}
		cred.Access = access
		next, err := json.Marshal(cred)
		if err != nil {
			return secret
		}
		return string(next)
	}
	return secret
}

func (s *Server) hydrateSteam(ctx context.Context, secret string) string {
	var cred struct {
		SteamID string `json:"steam_id"`
		Access  string `json:"access_token"`
		Refresh string `json:"refresh_token"`
	}
	if json.Unmarshal([]byte(secret), &cred) != nil || cred.SteamID == "" || platform.SteamAccessUsable(cred.Access) {
		return secret
	}
	access, refresh, ok := s.store.SteamWebSession(ctx, cred.SteamID)
	if !ok || !platform.SteamAccessUsable(access) {
		return secret
	}
	cred.Access = access
	if refresh != "" {
		cred.Refresh = refresh
	}
	next, err := json.Marshal(cred)
	if err != nil {
		return secret
	}
	if err := s.store.UpdateSecret(ctx, "steam", string(next)); err != nil {
		return secret
	}
	return string(next)
}

func (s *Server) finishJob(uid, jobID int, status, message string, updated, created int) {
	ctx, cancel := context.WithTimeout(store.WithUser(context.Background(), uid), 10*time.Second)
	defer cancel()
	if err := s.store.FinishJob(ctx, jobID, status, message, updated, created); err != nil {
		log.Printf("同步结果没有写下: %v", err)
	}
}
