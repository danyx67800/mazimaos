package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	iofs "io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/danyx67800/mazimaos/daemon/internal/audit"
	"github.com/danyx67800/mazimaos/daemon/internal/auth"
	"github.com/danyx67800/mazimaos/daemon/internal/config"
	"github.com/danyx67800/mazimaos/daemon/internal/dockerstore"
	"github.com/danyx67800/mazimaos/daemon/internal/proxy"
	"github.com/danyx67800/mazimaos/daemon/internal/session"
	"github.com/danyx67800/mazimaos/daemon/internal/store"
	"github.com/danyx67800/mazimaos/daemon/internal/system"
	"github.com/danyx67800/mazimaos/daemon/internal/tailscale"
	"github.com/danyx67800/mazimaos/daemon/internal/updater"
	"github.com/danyx67800/mazimaos/daemon/internal/version"
	"github.com/gorilla/websocket"
)

type Server struct {
	Cfg        *config.Config
	Store      *store.Store
	Auth       *auth.Manager
	Audit      *audit.Logger
	System     *system.Collector
	Docker     *dockerstore.Client
	Apps       *dockerstore.AppManager
	Proxy      *proxy.Caddy
	Tailscale  *tailscale.Client
	Sessions   *session.Manager
	Updater    *updater.Updater
	Logger     *log.Logger
	SetupToken string

	handler   http.Handler
	upgrader  websocket.Upgrader
	startedAt time.Time
	setup     *auth.Limiter
}

type ctxKey int

const userKey ctxKey = 1

type authContext struct {
	User    *store.User
	Session *store.Session
}

type apiError struct {
	Status  int
	Code    string
	Message string
}

func (e apiError) Error() string { return e.Message }

func (s *Server) Handler() http.Handler {
	if s.handler != nil {
		return s.handler
	}
	if s.Logger == nil {
		s.Logger = log.New(os.Stderr, "mazimad: ", log.LstdFlags|log.LUTC)
	}
	s.setup = auth.NewLimiter()
	s.startedAt = time.Now().UTC()
	s.upgrader = websocket.Upgrader{
		HandshakeTimeout: 10 * time.Second,
		ReadBufferSize:   4096,
		WriteBufferSize:  64 * 1024,
		CheckOrigin:      sameOrigin,
	}
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/setup/status", s.handleSetupStatus)
	mux.HandleFunc("POST /api/setup", s.handleSetup)
	mux.HandleFunc("POST /api/auth/login", s.handleLogin)
	mux.HandleFunc("POST /api/auth/2fa", s.handleTwoFactor)
	mux.HandleFunc("GET /api/auth/session", s.auth(s.handleSession))
	mux.HandleFunc("POST /api/auth/logout", s.auth(s.handleLogout))

	mux.HandleFunc("GET /api/me", s.auth(s.handleMe))
	mux.HandleFunc("PATCH /api/me", s.auth(s.handleUpdateMe))
	mux.HandleFunc("POST /api/me/password", s.auth(s.handleChangePassword))
	mux.HandleFunc("POST /api/me/totp/start", s.auth(s.handleTOTPStart))
	mux.HandleFunc("POST /api/me/totp/confirm", s.auth(s.handleTOTPConfirm))
	mux.HandleFunc("POST /api/me/totp/disable", s.auth(s.handleTOTPDisable))

	mux.HandleFunc("GET /api/settings", s.auth(s.handleGetSettings))
	mux.HandleFunc("PATCH /api/settings", s.auth(s.handleUpdateSettings))

	mux.HandleFunc("GET /api/system", s.auth(s.handleSystem))
	mux.HandleFunc("GET /api/system/history", s.auth(s.handleSystemHistory))
	mux.HandleFunc("GET /api/system/processes", s.auth(s.handleProcesses))
	mux.HandleFunc("GET /api/system/services", s.auth(s.handleServices))
	mux.HandleFunc("POST /api/system/services", s.auth(s.handleServiceAction))
	mux.HandleFunc("GET /api/system/logs", s.auth(s.handleLogs))
	mux.HandleFunc("POST /api/system/power", s.auth(s.handlePower))
	mux.HandleFunc("POST /api/system/processes/signal", s.auth(s.handleProcessSignal))

	mux.HandleFunc("GET /api/apps", s.auth(s.handleApps))
	mux.HandleFunc("GET /api/apps/{id}", s.auth(s.handleApp))
	mux.HandleFunc("POST /api/apps/{id}/install", s.auth(s.handleAppInstall))
	mux.HandleFunc("POST /api/apps/{id}/update", s.auth(s.handleAppUpdate))
	mux.HandleFunc("DELETE /api/apps/{id}", s.auth(s.handleAppRemove))
	mux.HandleFunc("GET /api/apps/{id}/logs", s.auth(s.handleAppLogs))

	mux.HandleFunc("GET /api/sessions/apps", s.auth(s.handleSessionApps))
	mux.HandleFunc("GET /api/sessions", s.auth(s.handleSessions))
	mux.HandleFunc("POST /api/sessions", s.auth(s.handleSessionStart))
	mux.HandleFunc("DELETE /api/sessions/{id}", s.auth(s.handleSessionStop))
	mux.HandleFunc("POST /api/sessions/{id}/token", s.auth(s.handleSessionToken))
	mux.HandleFunc("GET /api/sessions/{id}/clipboard", s.auth(s.handleClipboardGet))
	mux.HandleFunc("PUT /api/sessions/{id}/clipboard", s.auth(s.handleClipboardSet))
	mux.HandleFunc("GET /api/ws/session", s.handleSessionWS)

	mux.HandleFunc("GET /api/desktop", s.auth(s.handleDesktopGet))
	mux.HandleFunc("PUT /api/desktop", s.auth(s.handleDesktopPut))

	mux.HandleFunc("GET /api/audit", s.auth(s.handleAudit))
	mux.HandleFunc("GET /api/updates", s.auth(s.handleUpdateState))
	mux.HandleFunc("POST /api/updates/check", s.auth(s.handleUpdateCheck))
	mux.HandleFunc("POST /api/updates/install", s.auth(s.handleUpdateInstall))
	mux.HandleFunc("POST /api/updates/rollback", s.auth(s.handleUpdateRollback))

	mux.HandleFunc("GET /api/tailscale", s.auth(s.handleTailscale))
	mux.HandleFunc("POST /api/tailscale/login", s.auth(s.handleTailscaleLogin))
	mux.HandleFunc("POST /api/tailscale/logout", s.auth(s.handleTailscaleLogout))
	mux.HandleFunc("POST /api/tailscale/serve", s.auth(s.handleTailscaleServe))
	mux.HandleFunc("POST /api/tailscale/serve/disable", s.auth(s.handleTailscaleServeDisable))

	mux.HandleFunc("GET /api/docker/containers", s.auth(s.handleContainers))
	mux.HandleFunc("POST /api/docker/containers/{id}/{action}", s.auth(s.handleContainerAction))
	mux.HandleFunc("GET /api/docker/containers/{id}/logs", s.auth(s.handleContainerLogs))
	mux.HandleFunc("GET /api/docker/containers/{id}/stats", s.auth(s.handleContainerStats))
	mux.HandleFunc("GET /api/docker/images", s.auth(s.handleImages))
	mux.HandleFunc("DELETE /api/docker/images/{id}", s.auth(s.handleImageRemove))
	mux.HandleFunc("GET /api/docker/volumes", s.auth(s.handleVolumes))
	mux.HandleFunc("GET /api/docker/networks", s.auth(s.handleNetworks))
	mux.HandleFunc("GET /api/docker/df", s.auth(s.handleDockerDF))
	mux.HandleFunc("POST /api/docker/prune", s.auth(s.handleDockerPrune))

	mux.HandleFunc("GET /healthz", s.handleLiveness)
	mux.Handle("/", s.staticHandler())

	s.handler = s.middleware(mux)
	return s.handler
}

func (s *Server) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				s.logf("panic serving %s %s: %v\n%s", r.Method, r.URL.Path, rec, debug.Stack())
				writeError(w, apiError{500, "internal_error", "internal server error"})
			}
		}()
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), usb=(), payment=()")
		if requestIsHTTPS(r) {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) staticHandler() http.Handler {
	ui := s.Cfg.UIDir
	files := http.FileServer(http.Dir(ui))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			writeError(w, apiError{405, "method_not_allowed", "method not allowed"})
			return
		}
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data: blob:; style-src 'self' 'unsafe-inline'; script-src 'self'; connect-src 'self' ws: wss:; font-src 'self' data:; object-src 'none'; base-uri 'self'; frame-ancestors 'none'; form-action 'self'")
		clean := filepath.ToSlash(filepath.Clean(strings.TrimPrefix(r.URL.Path, "/")))
		if clean == "." {
			clean = "index.html"
		}
		if !iofs.ValidPath(clean) {
			http.NotFound(w, r)
			return
		}
		if info, err := os.Stat(filepath.Join(ui, filepath.FromSlash(clean))); err == nil && !info.IsDir() {
			w.Header().Set("Cache-Control", "no-cache")
			files.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(clean, "assets/") || filepath.Ext(clean) != "" {
			http.NotFound(w, r)
			return
		}
		index := filepath.Join(ui, "index.html")
		if _, err := os.Stat(index); err != nil {
			http.Error(w, "MazimaOS UI is not installed", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, r, index)
	})
}

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := sessionToken(r)
		u, sess, err := s.Auth.Authenticate(token)
		if err != nil {
			if errors.Is(err, auth.ErrSessionExpired) {
				s.clearSessionCookie(w, r)
			}
			writeError(w, apiError{401, "unauthorized", "authentication required"})
			return
		}
		if isMutating(r.Method) {
			csrf := r.Header.Get("X-CSRF-Token")
			if csrf == "" || !s.Auth.ConstantTimeEqual(csrf, sess.CSRF) {
				writeError(w, apiError{403, "csrf", "missing or invalid CSRF token"})
				return
			}
		}
		ctx := context.WithValue(r.Context(), userKey, authContext{User: u, Session: sess})
		next(w, r.WithContext(ctx))
	}
}

func (s *Server) requireAdmin(w http.ResponseWriter, r *http.Request) *store.User {
	ac := currentAuth(r)
	if ac == nil {
		writeError(w, apiError{401, "unauthorized", "authentication required"})
		return nil
	}
	if ac.User.Role != store.RoleAdmin {
		writeError(w, apiError{403, "forbidden", "administrator role required"})
		return nil
	}
	return ac.User
}

func currentAuth(r *http.Request) *authContext {
	ac, _ := r.Context().Value(userKey).(authContext)
	if ac.User == nil {
		return nil
	}
	return &ac
}

func (s *Server) authFor(r *http.Request) *authContext {
	return currentAuth(r)
}

func (s *Server) record(r *http.Request, actor, action, resource, result string, details map[string]string) {
	if s.Audit == nil {
		return
	}
	ac := currentAuth(r)
	actorType := "guest"
	if actor == "" && ac != nil {
		actor = ac.User.Username
		actorType = "user"
	}
	if actor == "" {
		actor = "anonymous"
	}
	e := audit.Event{
		Actor: actor, ActorType: actorType, Action: action, Resource: resource,
		Result: result, IP: clientIP(r, s.Cfg.TrustedProxy), UserAgent: r.UserAgent(),
		RequestID: requestID(), Details: details,
	}
	if err := s.Audit.Append(e); err != nil {
		s.logf("audit append failed: %v", err)
	}
}

func (s *Server) now() time.Time { return time.Now().UTC() }

func (s *Server) logf(format string, args ...any) {
	if s.Logger != nil {
		s.Logger.Printf(format, args...)
	}
}

func (s *Server) sessionCookieName() string { return "mazima_session" }

func (s *Server) setSessionCookie(w http.ResponseWriter, r *http.Request, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name: s.sessionCookieName(), Value: token, Path: "/", HttpOnly: true,
		Secure: !s.Cfg.InsecureCookies && requestIsHTTPS(r), SameSite: http.SameSiteLaxMode,
		Expires: expires, MaxAge: int(time.Until(expires).Seconds()),
	})
}

func (s *Server) clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name: s.sessionCookieName(), Value: "", Path: "/", HttpOnly: true,
		Secure: !s.Cfg.InsecureCookies && requestIsHTTPS(r), SameSite: http.SameSiteLaxMode,
		MaxAge: -1, Expires: time.Unix(1, 0),
	})
}

func sessionToken(r *http.Request) string {
	c, err := r.Cookie("mazima_session")
	if err != nil {
		return ""
	}
	return c.Value
}

func requestIsHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https")
}

func isMutating(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	}
	return false
}

func clientIP(r *http.Request, trusted string) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if trusted != "" {
		if ip := net.ParseIP(host); ip != nil && (ip.IsLoopback() || ip.String() == trusted) {
			if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
				first := strings.TrimSpace(strings.Split(xff, ",")[0])
				if net.ParseIP(first) != nil {
					return first
				}
			}
		}
	}
	return host
}

func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := urlParse(origin)
	if err != nil {
		return false
	}
	return strings.EqualFold(u.Host, r.Host)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	if ct := r.Header.Get("Content-Type"); ct != "" && !strings.HasPrefix(strings.ToLower(ct), "application/json") {
		return apiError{415, "content_type", "Content-Type must be application/json"}
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			return apiError{400, "empty_body", "request body is required"}
		}
		return apiError{400, "invalid_json", "invalid JSON body: " + err.Error()}
	}
	if dec.More() {
		return apiError{400, "invalid_json", "unexpected data after JSON object"}
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(true)
	_ = enc.Encode(v)
}

func writeError(w http.ResponseWriter, err error) {
	var ae apiError
	if !errors.As(err, &ae) {
		ae = apiError{500, "internal_error", "internal server error"}
	}
	writeJSON(w, ae.Status, map[string]any{"error": ae.Message, "code": ae.Code})
}

func requestID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func urlParse(raw string) (*url.URL, error) { return url.Parse(raw) }

func (s *Server) handleLiveness(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	out := map[string]any{
		"ok": true, "version": version.Version, "product": version.Product,
		"uptime_seconds": int(time.Since(s.startedAt).Seconds()),
		"setup_complete": s.setupComplete(),
		"streaming":      runtimeIsLinux(),
	}
	if s.Docker != nil {
		err := s.Docker.Ping(ctx)
		out["docker"] = map[string]any{"available": err == nil, "error": errString(err)}
	}
	if s.Tailscale != nil {
		out["tailscale"] = map[string]any{"available": s.Tailscale.Available()}
	}
	if s.Updater != nil {
		out["updates"] = map[string]any{"configured": s.Cfg.UpdateRepository != ""}
	}
	writeJSON(w, 200, out)
}

func (s *Server) setupComplete() bool {
	if s.Store == nil {
		return false
	}
	ok, err := s.Store.HasUsers()
	return err == nil && ok
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
