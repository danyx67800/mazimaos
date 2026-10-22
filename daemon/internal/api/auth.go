package api

import (
	"encoding/base64"
	"errors"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/danyx67800/mazimaos/daemon/internal/auth"
	"github.com/danyx67800/mazimaos/daemon/internal/store"
	"github.com/danyx67800/mazimaos/daemon/internal/version"
	qrcode "github.com/skip2/go-qrcode"
)

type userView struct {
	Username       string    `json:"username"`
	DisplayName    string    `json:"display_name"`
	Role           string    `json:"role"`
	Language       string    `json:"language,omitempty"`
	TOTPEnabled    bool      `json:"totp_enabled"`
	MustChangePass bool      `json:"must_change_password"`
	CreatedAt      time.Time `json:"created_at"`
	LastLoginAt    time.Time `json:"last_login_at,omitempty"`
}

func viewUser(u *store.User) userView {
	return userView{
		Username: u.Username, DisplayName: u.DisplayName, Role: string(u.Role),
		Language: u.Language, TOTPEnabled: u.TOTPEnabled,
		MustChangePass: u.MustChangePass, CreatedAt: u.CreatedAt, LastLoginAt: u.LastLoginAt,
	}
}

var hostnameRE = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

func (s *Server) handleSetupStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{
		"setup_complete":       s.setupComplete(),
		"setup_token_required": s.SetupToken != "",
		"version":              versionString(),
		"languages":            []string{"it", "en"},
	})
}

type setupRequest struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Password    string `json:"password"`
	Language    string `json:"language"`
	Timezone    string `json:"timezone"`
	Hostname    string `json:"hostname"`
	SetupToken  string `json:"setup_token"`
}

func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	if s.setupComplete() {
		writeError(w, apiError{409, "setup_complete", "MazimaOS is already configured"})
		return
	}
	ip := clientIP(r, s.Cfg.TrustedProxy)
	if ok, wait := s.setup.Allowed(ip); !ok {
		writeError(w, apiError{429, "rate_limited", "too many attempts; retry in " + wait.Round(time.Second).String()})
		return
	}
	var req setupRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, err)
		return
	}
	if s.SetupToken != "" && !s.Auth.ConstantTimeEqual(strings.TrimSpace(req.SetupToken), s.SetupToken) {
		s.setup.Failure(ip)
		time.Sleep(300 * time.Millisecond)
		s.record(r, "", "setup.denied", "system", "denied", nil)
		writeError(w, apiError{403, "setup_token", "invalid setup token; read it from the mazimad console or " + s.Cfg.SetupTokenPath()})
		return
	}
	req.Username = strings.ToLower(strings.TrimSpace(req.Username))
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	if req.DisplayName == "" {
		req.DisplayName = req.Username
	}
	if req.Hostname == "" {
		req.Hostname = "mazima"
	}
	if !hostnameRE.MatchString(req.Hostname) || len(req.Hostname) > 63 {
		writeError(w, apiError{400, "invalid_hostname", "hostname must contain only lowercase letters, digits and dashes"})
		return
	}
	u, err := s.Auth.SetupAdmin(req.Username, req.DisplayName, req.Password, req.Language, req.Timezone, req.Hostname)
	if err != nil {
		s.setup.Failure(ip)
		writeError(w, apiError{400, "setup_failed", err.Error()})
		return
	}
	s.setup.Success(ip)
	_ = os.Remove(s.Cfg.SetupTokenPath())
	s.SetupToken = ""
	s.record(r, u.Username, "setup.complete", "system", "ok", map[string]string{"hostname": req.Hostname})
	res, err := s.Auth.BeginLogin(u.Username, req.Password, ip, r.UserAgent())
	if err != nil || res.Status != auth.LoginComplete {
		writeJSON(w, 201, map[string]any{"setup_complete": true, "login_required": true})
		return
	}
	s.setSessionCookie(w, r, res.SessionToken, res.ExpiresAt)
	s.record(r, u.Username, "auth.login", "session", "ok", map[string]string{"method": "setup"})
	writeJSON(w, 201, map[string]any{
		"setup_complete": true, "user": viewUser(u), "csrf": res.CSRF, "expires_at": res.ExpiresAt,
	})
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, err)
		return
	}
	req.Username = strings.ToLower(strings.TrimSpace(req.Username))
	if req.Username == "" || req.Password == "" || len(req.Password) > 1024 {
		s.record(r, "", "auth.login", "session", "denied", map[string]string{"reason": "invalid_request"})
		writeError(w, apiError{401, "invalid_credentials", auth.ErrInvalidCredentials.Error()})
		return
	}
	ip := clientIP(r, s.Cfg.TrustedProxy)
	res, err := s.Auth.BeginLogin(req.Username, req.Password, ip, r.UserAgent())
	if err != nil {
		s.record(r, "", "auth.login", "session", "denied", map[string]string{"username": req.Username})
		status := 401
		code := "invalid_credentials"
		msg := auth.ErrInvalidCredentials.Error()
		if errors.Is(err, auth.ErrInvalidCredentials) && strings.Contains(err.Error(), "retry in") {
			status, code, msg = 429, "rate_limited", err.Error()
		}
		writeError(w, apiError{status, code, msg})
		return
	}
	if res.Status == auth.LoginTOTP {
		s.record(r, req.Username, "auth.challenge", "session", "ok", nil)
		writeJSON(w, 200, map[string]any{
			"status": "totp_required", "challenge": res.Challenge, "attempts_left": res.ChallengeLeft,
		})
		return
	}
	s.setSessionCookie(w, r, res.SessionToken, res.ExpiresAt)
	s.record(r, req.Username, "auth.login", "session", "ok", map[string]string{"method": "password"})
	writeJSON(w, 200, map[string]any{
		"status": "complete", "user": viewUser(res.User), "csrf": res.CSRF, "expires_at": res.ExpiresAt,
	})
}

type twoFactorRequest struct {
	Challenge string `json:"challenge"`
	Code      string `json:"code"`
	Kind      string `json:"kind"`
}

func (s *Server) handleTwoFactor(w http.ResponseWriter, r *http.Request) {
	var req twoFactorRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, err)
		return
	}
	if req.Challenge == "" || req.Code == "" || len(req.Code) > 32 {
		writeError(w, apiError{400, "invalid_request", "challenge and code are required"})
		return
	}
	if req.Kind == "" {
		req.Kind = "totp"
	}
	ip := clientIP(r, s.Cfg.TrustedProxy)
	res, err := s.Auth.FinishChallenge(req.Challenge, strings.TrimSpace(req.Code), req.Kind, ip, r.UserAgent())
	if err != nil {
		s.record(r, "", "auth.two_factor", "session", "denied", map[string]string{"kind": req.Kind})
		writeError(w, apiError{401, "invalid_code", err.Error()})
		return
	}
	s.setSessionCookie(w, r, res.SessionToken, res.ExpiresAt)
	s.record(r, "", "auth.login", "session", "ok", map[string]string{"method": req.Kind})
	writeJSON(w, 200, map[string]any{
		"status": "complete", "user": viewUser(res.User), "csrf": res.CSRF, "expires_at": res.ExpiresAt,
	})
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	ac := s.authFor(r)
	writeJSON(w, 200, map[string]any{
		"user": viewUser(ac.User), "csrf": ac.Session.CSRF, "expires_at": ac.Session.ExpiresAt,
	})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	ac := s.authFor(r)
	_ = s.Auth.Logout(sessionToken(r))
	s.clearSessionCookie(w, r)
	s.record(r, ac.User.Username, "auth.logout", "session", "ok", nil)
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	ac := s.authFor(r)
	writeJSON(w, 200, map[string]any{"user": viewUser(ac.User)})
}

type updateMeRequest struct {
	DisplayName string `json:"display_name"`
	Language    string `json:"language"`
}

func (s *Server) handleUpdateMe(w http.ResponseWriter, r *http.Request) {
	ac := s.authFor(r)
	var req updateMeRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, err)
		return
	}
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	if req.DisplayName == "" || len(req.DisplayName) > 80 {
		writeError(w, apiError{400, "invalid_display_name", "display name must be between 1 and 80 characters"})
		return
	}
	if req.Language != "" && req.Language != "it" && req.Language != "en" {
		writeError(w, apiError{400, "invalid_language", "language must be it or en"})
		return
	}
	var updated *store.User
	err := s.Store.Update(func(st *store.State) error {
		u, ok := st.Users[ac.User.Username]
		if !ok {
			return errors.New("user no longer exists")
		}
		u.DisplayName = req.DisplayName
		if req.Language != "" {
			u.Language = req.Language
		}
		updated = u
		return nil
	})
	if err != nil {
		writeError(w, apiError{500, "update_failed", err.Error()})
		return
	}
	s.record(r, "", "profile.update", "user", "ok", nil)
	writeJSON(w, 200, map[string]any{"user": viewUser(updated)})
}

type changePasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	ac := s.authFor(r)
	var req changePasswordRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, err)
		return
	}
	if err := s.Auth.ChangePassword(ac.User.Username, req.OldPassword, req.NewPassword); err != nil {
		s.record(r, "", "profile.password_change", "user", "denied", nil)
		writeError(w, apiError{400, "password_change_failed", err.Error()})
		return
	}
	_ = s.Auth.RevokeAll(ac.User.Username)
	s.clearSessionCookie(w, r)
	s.record(r, "", "profile.password_change", "user", "ok", nil)
	writeJSON(w, 200, map[string]any{"ok": true, "reauthenticate": true})
}

type totpStartRequest struct {
	Password string `json:"password"`
}

func (s *Server) handleTOTPStart(w http.ResponseWriter, r *http.Request) {
	ac := s.authFor(r)
	var req totpStartRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, err)
		return
	}
	if !s.Auth.VerifyUserPassword(ac.User.Username, req.Password) {
		s.record(r, "", "auth.totp_setup", "user", "denied", nil)
		writeError(w, apiError{403, "invalid_password", "password is incorrect"})
		return
	}
	uri, secret, recovery, err := s.Auth.StartTOTPSetup(ac.User.Username)
	if err != nil {
		writeError(w, apiError{400, "totp_setup_failed", err.Error()})
		return
	}
	png, err := qrcode.Encode(uri, qrcode.Medium, 256)
	if err != nil {
		writeError(w, apiError{500, "qr_failed", err.Error()})
		return
	}
	s.record(r, "", "auth.totp_setup", "user", "ok", nil)
	writeJSON(w, 200, map[string]any{
		"secret": secret, "otpauth": uri, "recovery_codes": recovery,
		"qr": "data:image/png;base64," + base64String(png),
	})
}

type totpConfirmRequest struct {
	Code string `json:"code"`
}

func (s *Server) handleTOTPConfirm(w http.ResponseWriter, r *http.Request) {
	ac := s.authFor(r)
	var req totpConfirmRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, err)
		return
	}
	if err := s.Auth.ConfirmTOTP(ac.User.Username, strings.TrimSpace(req.Code)); err != nil {
		s.record(r, "", "auth.totp_enable", "user", "denied", nil)
		writeError(w, apiError{400, "totp_confirm_failed", err.Error()})
		return
	}
	s.record(r, "", "auth.totp_enable", "user", "ok", nil)
	writeJSON(w, 200, map[string]any{"ok": true, "totp_enabled": true})
}

type totpDisableRequest struct {
	Password string `json:"password"`
	Code     string `json:"code"`
}

func (s *Server) handleTOTPDisable(w http.ResponseWriter, r *http.Request) {
	ac := s.authFor(r)
	var req totpDisableRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, err)
		return
	}
	if err := s.Auth.DisableTOTP(ac.User.Username, req.Password, strings.TrimSpace(req.Code)); err != nil {
		s.record(r, "", "auth.totp_disable", "user", "denied", nil)
		writeError(w, apiError{400, "totp_disable_failed", err.Error()})
		return
	}
	s.record(r, "", "auth.totp_disable", "user", "ok", nil)
	writeJSON(w, 200, map[string]any{"ok": true, "totp_enabled": false})
}

func versionString() string { return version.Version }

func base64String(b []byte) string { return base64.StdEncoding.EncodeToString(b) }
