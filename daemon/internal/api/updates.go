package api

import (
	"net/http"
	"time"

	"github.com/danyx67800/mazimaos/daemon/internal/version"
)

func (s *Server) handleUpdateState(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{
		"current": version.Version, "repository": s.Cfg.UpdateRepository,
		"channel": s.Cfg.UpdateChannel, "configured": s.Cfg.UpdateRepository != "" && s.Updater != nil,
	}
	if s.Updater != nil {
		st, err := s.Updater.State()
		if err == nil {
			out["state"] = st
		}
	}
	writeJSON(w, 200, out)
}

func (s *Server) handleUpdateCheck(w http.ResponseWriter, r *http.Request) {
	if s.requireAdmin(w, r) == nil {
		return
	}
	if s.Updater == nil {
		writeError(w, apiError{503, "updates_unavailable", "updater is not configured"})
		return
	}
	ctx, cancel := contextWithTimeout(r, 90*time.Second)
	defer cancel()
	av, err := s.Updater.Check(ctx)
	if err != nil {
		s.record(r, "", "update.check", "system", "error", map[string]string{"error": err.Error()})
		writeError(w, apiError{502, "update_check_failed", err.Error()})
		return
	}
	s.record(r, "", "update.check", "system", "ok", nil)
	if av == nil {
		writeJSON(w, 200, map[string]any{"available": false, "current": version.Version})
		return
	}
	writeJSON(w, 200, map[string]any{"available": true, "update": av, "current": version.Version})
}

type installUpdateRequest struct {
	Password string `json:"password"`
	Confirm  string `json:"confirm"`
	Version  string `json:"version"`
}

func (s *Server) handleUpdateInstall(w http.ResponseWriter, r *http.Request) {
	u := s.requireAdmin(w, r)
	if u == nil {
		return
	}
	if s.Updater == nil {
		writeError(w, apiError{503, "updates_unavailable", "updater is not configured"})
		return
	}
	var req installUpdateRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, err)
		return
	}
	if req.Confirm != "INSTALL" {
		writeError(w, apiError{400, "confirmation_required", "type INSTALL to confirm"})
		return
	}
	if !s.Auth.VerifyUserPassword(u.Username, req.Password) {
		writeError(w, apiError{403, "invalid_password", "password is incorrect"})
		return
	}
	ctx, cancel := contextWithTimeout(r, 20*time.Minute)
	defer cancel()
	av, err := s.Updater.Check(ctx)
	if err != nil {
		writeError(w, apiError{502, "update_check_failed", err.Error()})
		return
	}
	if av == nil {
		writeError(w, apiError{409, "no_update", "no update is available"})
		return
	}
	if req.Version != "" && req.Version != av.Manifest.Version {
		writeError(w, apiError{409, "version_changed", "the available version changed; check again"})
		return
	}
	st, err := s.Updater.Install(ctx, av)
	if err != nil {
		s.record(r, "", "update.install", av.Manifest.Version, "error", map[string]string{"error": err.Error()})
		writeError(w, apiError{500, "update_failed", err.Error()})
		return
	}
	s.record(r, "", "update.install", av.Manifest.Version, "ok", nil)
	writeJSON(w, 200, map[string]any{"ok": true, "state": st, "restart_required": true})
}

type rollbackRequest struct {
	Password string `json:"password"`
	Confirm  string `json:"confirm"`
}

func (s *Server) handleUpdateRollback(w http.ResponseWriter, r *http.Request) {
	u := s.requireAdmin(w, r)
	if u == nil {
		return
	}
	if s.Updater == nil {
		writeError(w, apiError{503, "updates_unavailable", "updater is not configured"})
		return
	}
	var req rollbackRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, err)
		return
	}
	if req.Confirm != "ROLLBACK" {
		writeError(w, apiError{400, "confirmation_required", "type ROLLBACK to confirm"})
		return
	}
	if !s.Auth.VerifyUserPassword(u.Username, req.Password) {
		writeError(w, apiError{403, "invalid_password", "password is incorrect"})
		return
	}
	if err := s.Updater.Rollback(); err != nil {
		s.record(r, "", "update.rollback", "system", "error", map[string]string{"error": err.Error()})
		writeError(w, apiError{500, "rollback_failed", err.Error()})
		return
	}
	s.record(r, "", "update.rollback", "system", "ok", nil)
	writeJSON(w, 200, map[string]any{"ok": true, "restart_required": true})
}
