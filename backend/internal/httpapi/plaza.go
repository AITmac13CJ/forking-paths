package httpapi

import (
	"net/http"
	"strconv"

	"forking-paths/internal/store"
)

func (s *Server) plazaRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/plaza", s.plaza)
	mux.HandleFunc("GET /api/plaza/{id}", s.plazaOne)
	mux.HandleFunc("PUT /api/me/public", s.setPublic)
}

func (s *Server) plaza(w http.ResponseWriter, r *http.Request) {
	people, err := s.store.Plaza(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"people": people})
}

func (s *Server) plazaOne(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id <= 0 {
		writeErr(w, &store.ValidationError{Msg: "没有这个人"})
		return
	}
	person, err := s.store.PlazaOne(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, person)
}

func (s *Server) setPublic(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Public bool `json:"public"`
	}
	if err := decode(w, r, &body); err != nil {
		writeErr(w, err)
		return
	}
	if err := s.store.SetGardenPublic(r.Context(), body.Public); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"public": body.Public})
}
