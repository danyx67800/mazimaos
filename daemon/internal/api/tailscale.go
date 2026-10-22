package api

import (
	"net/http"
	"strings"
	"time"
)

func (s *Server) handleTailscale(w http.ResponseWriter, r *http.Request) {
	if s.requireAdmin(w, r) == nil {
		return
	}
	if s.Tailscale == nil {
		writeError(w, apiError{503, "tailscale_unavailable", "Tailscale integration is not configured"})
		return
	}
	ctx, cancel := contextWithTimeout(r, 15*time.Second)
	defer cancel()
	st, err := s.Tailscale.Status(ctx)
	if err != nil {
		writeError(w, apiError{500, "tailscale_error", err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"status": st})
}

type tailscaleLoginRequest struct {
	AuthKey string `json:"auth_key"`
}

func (s *Server) handleTailscaleLogin(w http.ResponseWriter, r *http.Request) {
	if s.requireAdmin(w, r) == nil {
		return
	}
	if s.Tailscale == nil {
		writeError(w, apiError{503, "tailscale_unavailable", "Tailscale integration is not configured"})
		return
	}
	var req tailscaleLoginRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, err)
		return
	}
	ctx, cancel := contextWithTimeout(r, 60*time.Second)
	defer cancel()
	if strings.TrimSpace(req.AuthKey) != "" {
		if err := s.Tailscale.LoginWithAuthKey(ctx, strings.TrimSpace(req.AuthKey)); err != nil {
			s.record(r, "", "tailscale.login", "node", "error", map[string]string{"method": "auth_key", "error": err.Error()})
			writeError(w, apiError{400, "tailscale_login_failed", err.Error()})
			return
		}
		s.record(r, "", "tailscale.login", "node", "ok", map[string]string{"method": "auth_key"})
	} else {
		if err := s.Tailscale.InteractiveLogin(ctx); err != nil {
			writeError(w, apiError{500, "tailscale_login_failed", err.Error()})
			return
		}
		s.record(r, "", "tailscale.login", "node", "ok", map[string]string{"method": "interactive"})
	}
	st, _ := s.Tailscale.Status(ctx)
	writeJSON(w, 200, map[string]any{"status": st})
}

type confirmRequest struct {
	Password string `json:"password"`
	Confirm  string `json:"confirm"`
}

func (s *Server) handleTailscaleLogout(w http.ResponseWriter, r *http.Request) {
	u := s.requireAdmin(w, r)
	if u == nil || s.Tailscale == nil {
		return
	}
	var req confirmRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, err)
		return
	}
	if req.Confirm != "LOGOUT" {
		writeError(w, apiError{400, "confirmation_required", "type LOGOUT to confirm"})
		return
	}
	if !s.Auth.VerifyUserPassword(u.Username, req.Password) {
		writeError(w, apiError{403, "invalid_password", "password is incorrect"})
		return
	}
	ctx, cancel := contextWithTimeout(r, 15*time.Second)
	defer cancel()
	if err := s.Tailscale.Logout(ctx); err != nil {
		writeError(w, apiError{500, "logout_failed", err.Error()})
		return
	}
	s.record(r, "", "tailscale.logout", "node", "ok", nil)
	writeJSON(w, 200, map[string]any{"ok": true})
}

type serveRequest struct {
	Funnel   bool   `json:"funnel"`
	Password string `json:"password"`
	Code     string `json:"code"`
	Confirm  string `json:"confirm"`
}

func (s *Server) handleTailscaleServe(w http.ResponseWriter, r *http.Request) {
	u := s.requireAdmin(w, r)
	if u == nil {
		return
	}
	if s.Tailscale == nil {
		writeError(w, apiError{503, "tailscale_unavailable", "Tailscale integration is not configured"})
		return
	}
	var req serveRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, err)
		return
	}
	expected := "ENABLE SERVE"
	if req.Funnel {
		expected = "ENABLE FUNNEL"
		if !u.TOTPEnabled {
			writeError(w, apiError{403, "totp_required", "enable two-factor authentication before enabling Funnel"})
			return
		}
		if !s.Auth.VerifyTOTPForUser(u.Username, strings.TrimSpace(req.Code)) {
			writeError(w, apiError{403, "invalid_code", "a valid two-factor code is required"})
			return
		}
	}
	if req.Confirm != expected {
		writeError(w, apiError{400, "confirmation_required", "type " + expected + " to confirm"})
		return
	}
	if !s.Auth.VerifyUserPassword(u.Username, req.Password) {
		writeError(w, apiError{403, "invalid_password", "password is incorrect"})
		return
	}
	ctx, cancel := contextWithTimeout(r, 30*time.Second)
	defer cancel()
	if err := s.Tailscale.EnableServe(ctx, req.Funnel); err != nil {
		s.record(r, "", "tailscale.serve", "node", "error", map[string]string{"error": err.Error(), "funnel": boolString(req.Funnel)})
		writeError(w, apiError{500, "serve_failed", err.Error()})
		return
	}
	s.record(r, "", "tailscale.serve", "node", "ok", map[string]string{"funnel": boolString(req.Funnel)})
	writeJSON(w, 200, map[string]any{"ok": true, "funnel": req.Funnel, "restart_required": false})
}

func (s *Server) handleTailscaleServeDisable(w http.ResponseWriter, r *http.Request) {
	u := s.requireAdmin(w, r)
	if u == nil {
		return
	}
	if s.Tailscale == nil {
		writeError(w, apiError{503, "tailscale_unavailable", "Tailscale integration is not configured"})
		return
	}
	var req confirmRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, err)
		return
	}
	if req.Confirm != "DISABLE" {
		writeError(w, apiError{400, "confirmation_required", "type DISABLE to confirm"})
		return
	}
	if !s.Auth.VerifyUserPassword(u.Username, req.Password) {
		writeError(w, apiError{403, "invalid_password", "password is incorrect"})
		return
	}
	ctx, cancel := contextWithTimeout(r, 20*time.Second)
	defer cancel()
	if err := s.Tailscale.DisableServe(ctx); err != nil {
		writeError(w, apiError{500, "serve_disable_failed", err.Error()})
		return
	}
	s.record(r, "", "tailscale.serve_disable", "node", "ok", nil)
	writeJSON(w, 200, map[string]any{"ok": true})
}

func boolString(v bool) string {
	if v {
		return "true"
	}
	return "false"
}
