package api

import (
	"net/http"
	"strconv"
)

func (s *Server) handleAudit(w http.ResponseWriter, r *http.Request) {
	if s.requireAdmin(w, r) == nil {
		return
	}
	if s.Audit == nil {
		writeError(w, apiError{503, "audit_unavailable", "audit log is not configured"})
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	events, err := s.Audit.Read(limit, r.URL.Query().Get("q"), r.URL.Query().Get("result"))
	if err != nil {
		writeError(w, apiError{500, "audit_read_failed", err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"events": events})
}
