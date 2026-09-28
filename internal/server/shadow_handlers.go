package server

import (
	"encoding/json"
	"net/http"
	"strconv"
)

// GET /api/admin/shadow/report?type=entry|exit|news|news_exit&days=14
// Admin-only, read-only evidence report (spec-013 FR-017, spec-014 FR-106).
func (s *Server) handleShadowReport(w http.ResponseWriter, r *http.Request) {
	tok := ""
	if h := r.Header.Get("Authorization"); len(h) > 7 && h[:7] == "Bearer " {
		tok = h[7:]
	}
	if tok == "" {
		tok = r.URL.Query().Get("token")
	}
	if s.authenticator == nil || !s.authenticator.ValidateTokenContext(r.Context(), tok) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	typ := r.URL.Query().Get("type")
	if typ != "entry" && typ != "exit" && typ != "news" && typ != "news_exit" {
		http.Error(w, "type must be entry|exit|news|news_exit", http.StatusBadRequest)
		return
	}
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	if s.dbStore == nil || s.dbStore.Pool == nil {
		http.Error(w, "component=shadow-report: database unavailable", http.StatusServiceUnavailable)
		return
	}
	var rep map[string]interface{}
	var err error
	if typ == "news_exit" {
		rep, err = s.dbStore.EarlyExitReport(r.Context(), days)
	} else {
		rep, err = s.dbStore.ShadowReport(r.Context(), typ, days)
	}
	if err != nil {
		http.Error(w, "component=shadow-report: report unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(rep)
}
