package httpapi

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"forking-paths/internal/store"
)

type Server struct {
	store *store.Store
}

func New(st *store.Store) http.Handler {
	s := &Server{store: st}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("GET /api/meta", s.meta)
	mux.HandleFunc("GET /api/stats", s.stats)
	mux.HandleFunc("GET /api/entries", s.listEntries)
	mux.HandleFunc("POST /api/entries", s.createEntry)
	mux.HandleFunc("GET /api/entries/{id}", s.getEntry)
	mux.HandleFunc("PUT /api/entries/{id}", s.updateEntry)
	mux.HandleFunc("DELETE /api/entries/{id}", s.deleteEntry)
	mux.HandleFunc("POST /api/tags", s.createTag)
	mux.HandleFunc("POST /api/media", s.createMedium)
	mux.HandleFunc("POST /api/vias", s.createVia)
	mux.HandleFunc("POST /api/covers", s.uploadCover)
	s.authRoutes(mux)
	s.plazaRoutes(mux)
	s.syncRoutes(mux)
	go s.keepLogins()
	return withCORS(s.withUser(mux))
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) meta(w http.ResponseWriter, r *http.Request) {
	meta, err := s.store.Meta(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, meta)
}

func (s *Server) stats(w http.ResponseWriter, r *http.Request) {
	year := time.Now().Year()
	if raw := r.URL.Query().Get("year"); raw != "" {
		y, err := strconv.Atoi(raw)
		if err != nil || y < 1970 || y > 2100 {
			writeErr(w, &store.ValidationError{Msg: "年份不正确"})
			return
		}
		year = y
	}
	pathID, _ := strconv.Atoi(r.URL.Query().Get("pathId"))
	stats, err := s.store.Stats(r.Context(), year, pathID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (s *Server) listEntries(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	pathID, _ := strconv.Atoi(q.Get("pathId"))
	f := store.Filter{
		PathID:  pathID,
		Q:       q.Get("q"),
		Status:  q.Get("status"),
		Release: q.Get("release"),
		Finish:  q.Get("finish"),
	}
	for _, raw := range q["mediumId"] {
		id, err := strconv.Atoi(raw)
		if err != nil {
			writeErr(w, &store.ValidationError{Msg: "载体筛选不正确"})
			return
		}
		f.MediumIDs = append(f.MediumIDs, id)
	}
	for _, raw := range q["tagId"] {
		id, err := strconv.Atoi(raw)
		if err != nil {
			writeErr(w, &store.ValidationError{Msg: "标签筛选不正确"})
			return
		}
		f.TagIDs = append(f.TagIDs, id)
	}
	for _, raw := range q["sort"] {
		field, dir, ok := strings.Cut(raw, ":")
		if !ok {
			dir = "desc"
		}
		f.Sorts = append(f.Sorts, store.Sort{Field: field, Dir: dir})
	}
	entries, err := s.store.List(r.Context(), f)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": entries, "total": len(entries)})
}

func (s *Server) getEntry(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	entry, err := s.store.Get(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, entry)
}

func (s *Server) createEntry(w http.ResponseWriter, r *http.Request) {
	var in store.EntryInput
	if err := decode(w, r, &in); err != nil {
		writeErr(w, err)
		return
	}
	entry, err := s.store.Create(r.Context(), in)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, entry)
}

func (s *Server) updateEntry(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	var in store.EntryInput
	if err := decode(w, r, &in); err != nil {
		writeErr(w, err)
		return
	}
	entry, err := s.store.Update(r.Context(), id, in)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, entry)
}

func (s *Server) deleteEntry(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := s.store.Delete(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) createTag(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := decode(w, r, &body); err != nil {
		writeErr(w, err)
		return
	}
	tag, err := s.store.CreateTag(r.Context(), body.Name)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, tag)
}

func (s *Server) createMedium(w http.ResponseWriter, r *http.Request) {
	var body struct {
		PathID int    `json:"pathId"`
		Name   string `json:"name"`
		Color  string `json:"color"`
	}
	if err := decode(w, r, &body); err != nil {
		writeErr(w, err)
		return
	}
	medium, err := s.store.CreateMedium(r.Context(), body.PathID, body.Name, body.Color)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, medium)
}

func (s *Server) createVia(w http.ResponseWriter, r *http.Request) {
	var body struct {
		PathID int    `json:"pathId"`
		Name   string `json:"name"`
	}
	if err := decode(w, r, &body); err != nil {
		writeErr(w, err)
		return
	}
	via, err := s.store.CreateVia(r.Context(), body.PathID, body.Name)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, via)
}

func pathID(r *http.Request) (int, error) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id <= 0 {
		return 0, &store.ValidationError{Msg: "编号不正确"}
	}
	return id, nil
}

func decode(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(dst); err != nil {
		return &store.ValidationError{Msg: "请求格式不正确"}
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, err error) {
	var ve *store.ValidationError
	if errors.As(err, &ve) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": ve.Error()})
		return
	}
	if errors.Is(err, store.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "没有这条记录"})
		return
	}
	if errors.Is(err, store.ErrUnauthorized) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "请先登录"})
		return
	}
	log.Printf("api: %v", err)
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "服务器出错了"})
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
