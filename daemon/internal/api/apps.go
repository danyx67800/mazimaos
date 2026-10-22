package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *Server) handleApps(w http.ResponseWriter, r *http.Request) {
	if s.Apps == nil {
		writeError(w, apiError{503, "apps_unavailable", "app store is not available"})
		return
	}
	ctx, cancel := contextWithTimeout(r, 30*time.Second)
	defer cancel()
	catalog, err := s.Apps.Catalog(ctx)
	if err != nil {
		writeError(w, apiError{500, "catalog_failed", err.Error()})
		return
	}
	dockerOK := false
	var dockerErr string
	if s.Docker != nil {
		err := s.Docker.Ping(ctx)
		dockerOK = err == nil
		dockerErr = errString(err)
	}
	writeJSON(w, 200, map[string]any{
		"apps":   catalog,
		"docker": map[string]any{"available": dockerOK, "error": dockerErr, "compose": s.Docker != nil && s.Docker.ComposeAvailable()},
	})
}

func (s *Server) handleApp(w http.ResponseWriter, r *http.Request) {
	if s.Apps == nil {
		writeError(w, apiError{503, "apps_unavailable", "app store is not available"})
		return
	}
	app, err := s.Apps.CatalogApp(r.PathValue("id"))
	if err != nil {
		writeError(w, apiError{404, "app_not_found", "app not found"})
		return
	}
	writeJSON(w, 200, map[string]any{"app": app})
}

func (s *Server) handleAppInstall(w http.ResponseWriter, r *http.Request) {
	if s.requireAdmin(w, r) == nil {
		return
	}
	if s.Apps == nil {
		writeError(w, apiError{503, "apps_unavailable", "app store is not available"})
		return
	}
	id := r.PathValue("id")
	ctx, cancel := contextWithTimeout(r, 15*time.Minute)
	defer cancel()
	app, err := s.Apps.Install(ctx, id)
	if err != nil {
		s.record(r, "", "app.install", id, "error", map[string]string{"error": err.Error()})
		writeError(w, apiError{500, "install_failed", err.Error()})
		return
	}
	s.record(r, "", "app.install", id, "ok", map[string]string{"version": app.Version})
	writeJSON(w, 200, map[string]any{"app": app})
}

func (s *Server) handleAppUpdate(w http.ResponseWriter, r *http.Request) {
	if s.requireAdmin(w, r) == nil {
		return
	}
	if s.Apps == nil {
		writeError(w, apiError{503, "apps_unavailable", "app store is not available"})
		return
	}
	id := r.PathValue("id")
	ctx, cancel := contextWithTimeout(r, 15*time.Minute)
	defer cancel()
	app, err := s.Apps.Update(ctx, id)
	if err != nil {
		s.record(r, "", "app.update", id, "error", map[string]string{"error": err.Error()})
		writeError(w, apiError{500, "update_failed", err.Error()})
		return
	}
	s.record(r, "", "app.update", id, "ok", map[string]string{"version": app.Version})
	writeJSON(w, 200, map[string]any{"app": app})
}

func (s *Server) handleAppRemove(w http.ResponseWriter, r *http.Request) {
	if s.requireAdmin(w, r) == nil {
		return
	}
	if s.Apps == nil {
		writeError(w, apiError{503, "apps_unavailable", "app store is not available"})
		return
	}
	id := r.PathValue("id")
	if r.Header.Get("X-Mazima-Confirm") != id {
		writeError(w, apiError{400, "confirmation_required", "send X-Mazima-Confirm with the app id"})
		return
	}
	purge := r.URL.Query().Get("purge") == "true"
	ctx, cancel := contextWithTimeout(r, 5*time.Minute)
	defer cancel()
	if err := s.Apps.Remove(ctx, id, purge); err != nil {
		s.record(r, "", "app.remove", id, "error", map[string]string{"error": err.Error(), "purge": strconv.FormatBool(purge)})
		writeError(w, apiError{500, "remove_failed", err.Error()})
		return
	}
	s.record(r, "", "app.remove", id, "ok", map[string]string{"purge": strconv.FormatBool(purge)})
	writeJSON(w, 200, map[string]any{"ok": true, "data_removed": purge})
}

func (s *Server) handleAppLogs(w http.ResponseWriter, r *http.Request) {
	if s.requireAdmin(w, r) == nil {
		return
	}
	if s.Apps == nil {
		writeError(w, apiError{503, "apps_unavailable", "app store is not available"})
		return
	}
	tail, _ := strconv.Atoi(r.URL.Query().Get("tail"))
	if tail <= 0 || tail > 2000 {
		tail = 200
	}
	ctx, cancel := contextWithTimeout(r, 30*time.Second)
	defer cancel()
	out, err := s.Apps.Logs(ctx, r.PathValue("id"), tail)
	if err != nil && strings.TrimSpace(out) == "" {
		writeError(w, apiError{500, "logs_failed", err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"logs": out})
}
