package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"forking-paths/internal/platform"
	"forking-paths/internal/store"
)

func (s *Server) syncRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/sync", s.syncAccounts)
	mux.HandleFunc("GET /api/sync/catch", s.catchLogin)
	mux.HandleFunc("POST /api/sync/catch", s.catchDone)
	mux.HandleFunc("GET /api/sync/steam/login", s.steamLogin)
	mux.HandleFunc("GET /api/sync/steam/callback", s.steamCallback)
	mux.HandleFunc("GET /api/sync/xbox/callback", s.xboxCallback)
	mux.HandleFunc("POST /api/sync/psn/npsso", s.saveNPSSO)
	mux.HandleFunc("POST /api/sync/psn/profile", s.savePSNProfile)
	mux.HandleFunc("POST /api/sync/switch/code", s.saveSwitchCode)
	mux.HandleFunc("POST /api/sync/switch/finish", s.finishSwitch)
	mux.HandleFunc("POST /api/sync/{provider}/login", s.platformLogin)
	mux.HandleFunc("PUT /api/sync/{provider}", s.saveAccount)
	mux.HandleFunc("GET /api/sync/jobs", s.listJobs)
	mux.HandleFunc("POST /api/sync/jobs/{id}/ack", s.ackJob)
	mux.HandleFunc("POST /api/sync/{provider}", s.runSync)
	mux.HandleFunc("POST /api/sync/{provider}/import", s.importSync)
	mux.HandleFunc("PUT /api/sync/covers/key", s.saveGridKey)
	mux.HandleFunc("POST /api/sync/covers", s.matchCovers)
}

func (s *Server) syncAccounts(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.Accounts(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) saveNPSSO(w http.ResponseWriter, r *http.Request) {
	var body struct {
		NPSSO string `json:"npsso"`
	}
	if err := decode(w, r, &body); err != nil {
		writeErr(w, err)
		return
	}
	session, err := platform.AcceptNPSSO(r.Context(), body.NPSSO)
	if err != nil {
		writeErr(w, &store.ValidationError{Msg: err.Error()})
		return
	}
	if err := s.store.SaveAccount(r.Context(), "psn", session.Account, session.Secret); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"account": session.Account})
}

func (s *Server) savePSNProfile(w http.ResponseWriter, r *http.Request) {
	var body struct {
		OnlineID string `json:"onlineId"`
	}
	if err := decode(w, r, &body); err != nil {
		writeErr(w, err)
		return
	}
	onlineID, ok := platform.NormalizeOnlineID(body.OnlineID)
	if !ok {
		writeErr(w, &store.ValidationError{Msg: "PSN 用户名不对，用 PlayStation 上显示的那个"})
		return
	}
	secret, err := json.Marshal(map[string]string{"online_id": onlineID})
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := s.store.SaveAccount(r.Context(), "psn", onlineID, string(secret)); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"account": onlineID})
}

func (s *Server) saveSwitchCode(w http.ResponseWriter, r *http.Request) {
	var body struct {
		FriendCode string `json:"friendCode"`
	}
	if err := decode(w, r, &body); err != nil {
		writeErr(w, err)
		return
	}
	code, ok := platform.NormalizeFriendCode(body.FriendCode)
	if !ok {
		writeErr(w, &store.ValidationError{Msg: "好友代码不对，应是 SW- 开头的 12 位数字"})
		return
	}
	secret, err := json.Marshal(map[string]string{"friend_code": code})
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := s.store.SaveAccount(r.Context(), "switch", code, string(secret)); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"account": code})
}

func (s *Server) finishSwitch(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL string `json:"url"`
	}
	if err := decode(w, r, &body); err != nil {
		writeErr(w, err)
		return
	}
	session, err := platform.FinishBrowserLogin(r.Context(), "switch", body.URL)
	if err != nil {
		writeErr(w, &store.ValidationError{Msg: err.Error()})
		return
	}
	if session.Account == "" {
		session.Account = "任天堂"
	}
	if err := s.store.SaveAccount(r.Context(), "switch", session.Account, session.Secret); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"account": session.Account})
}

func (s *Server) saveAccount(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Account string `json:"account"`
		Secret  string `json:"secret"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, &store.ValidationError{Msg: "请求格式不对"})
		return
	}
	provider := r.PathValue("provider")
	if err := s.store.SaveAccount(r.Context(), provider, body.Account, body.Secret); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) runSync(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	if !store.KnownProvider(provider) {
		writeErr(w, &store.ValidationError{Msg: "未知的平台"})
		return
	}
	if _, _, err := s.store.AccountSecret(r.Context(), provider); err != nil {
		writeErr(w, err)
		return
	}
	job, err := s.store.StartJob(r.Context(), provider)
	if err != nil {
		writeErr(w, err)
		return
	}
	uid, _ := store.UserID(r.Context())
	go s.performSync(job.ID, uid, provider)
	writeJSON(w, http.StatusAccepted, job)
}

func (s *Server) importSync(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	var body struct {
		Games []platform.Game `json:"games"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil {
		writeErr(w, &store.ValidationError{Msg: "请求格式不对"})
		return
	}
	clean := make([]platform.Game, 0, len(body.Games))
	for _, game := range body.Games {
		game.Title = strings.TrimSpace(game.Title)
		game.Ref = strings.TrimSpace(game.Ref)
		if game.Title == "" || strings.Contains(game.Ref, ":") {
			continue
		}
		clean = append(clean, game)
	}
	n, err := s.store.ImportSync(r.Context(), provider, clean, nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"created": n})
}
