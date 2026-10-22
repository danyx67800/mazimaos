package api

import (
	"net/http"
	"strconv"
	"time"
)

func (s *Server) dockerAvailable(w http.ResponseWriter) bool {
	if s.Docker == nil {
		writeError(w, apiError{503, "docker_unavailable", "Docker integration is not configured"})
		return false
	}
	return true
}

func (s *Server) handleContainers(w http.ResponseWriter, r *http.Request) {
	if s.requireAdmin(w, r) == nil || !s.dockerAvailable(w) {
		return
	}
	ctx, cancel := contextWithTimeout(r, 20*time.Second)
	defer cancel()
	all := r.URL.Query().Get("all") == "true"
	list, err := s.Docker.ListContainers(ctx, all)
	if err != nil {
		writeError(w, apiError{500, "docker_error", err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"containers": list})
}

func (s *Server) handleContainerAction(w http.ResponseWriter, r *http.Request) {
	if s.requireAdmin(w, r) == nil || !s.dockerAvailable(w) {
		return
	}
	id := r.PathValue("id")
	action := r.PathValue("action")
	if id == "" || len(id) > 128 {
		writeError(w, apiError{400, "invalid_container", "invalid container id"})
		return
	}
	if action == "remove" {
		if r.Header.Get("X-Mazima-Confirm") != id {
			writeError(w, apiError{400, "confirmation_required", "send X-Mazima-Confirm with the container id"})
			return
		}
	}
	ctx, cancel := contextWithTimeout(r, 60*time.Second)
	defer cancel()
	var err error
	switch action {
	case "start":
		err = s.Docker.StartContainer(ctx, id)
	case "stop":
		err = s.Docker.StopContainer(ctx, id)
	case "restart":
		err = s.Docker.RestartContainer(ctx, id)
	case "remove":
		force := r.URL.Query().Get("force") == "true"
		err = s.Docker.RemoveContainer(ctx, id, force)
	default:
		writeError(w, apiError{400, "invalid_action", "action must be start, stop, restart or remove"})
		return
	}
	if err != nil {
		s.record(r, "", "docker.container."+action, id, "error", map[string]string{"error": err.Error()})
		writeError(w, apiError{500, "docker_action_failed", err.Error()})
		return
	}
	s.record(r, "", "docker.container."+action, id, "ok", nil)
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) handleContainerLogs(w http.ResponseWriter, r *http.Request) {
	if s.requireAdmin(w, r) == nil || !s.dockerAvailable(w) {
		return
	}
	tail, _ := strconv.Atoi(r.URL.Query().Get("tail"))
	tty := r.URL.Query().Get("tty") == "true"
	ctx, cancel := contextWithTimeout(r, 30*time.Second)
	defer cancel()
	out, err := s.Docker.Logs(ctx, r.PathValue("id"), tail, tty)
	if err != nil && out == "" {
		writeError(w, apiError{500, "docker_logs_failed", err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"logs": out})
}

func (s *Server) handleContainerStats(w http.ResponseWriter, r *http.Request) {
	if s.requireAdmin(w, r) == nil || !s.dockerAvailable(w) {
		return
	}
	ctx, cancel := contextWithTimeout(r, 20*time.Second)
	defer cancel()
	stats, err := s.Docker.Stats(ctx, r.PathValue("id"))
	if err != nil {
		writeError(w, apiError{500, "docker_stats_failed", err.Error()})
		return
	}
	writeJSON(w, 200, stats)
}

func (s *Server) handleImages(w http.ResponseWriter, r *http.Request) {
	if s.requireAdmin(w, r) == nil || !s.dockerAvailable(w) {
		return
	}
	ctx, cancel := contextWithTimeout(r, 20*time.Second)
	defer cancel()
	images, err := s.Docker.ListImages(ctx)
	if err != nil {
		writeError(w, apiError{500, "docker_error", err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"images": images})
}

func (s *Server) handleImageRemove(w http.ResponseWriter, r *http.Request) {
	if s.requireAdmin(w, r) == nil || !s.dockerAvailable(w) {
		return
	}
	id := r.PathValue("id")
	if r.Header.Get("X-Mazima-Confirm") != id {
		writeError(w, apiError{400, "confirmation_required", "send X-Mazima-Confirm with the image id"})
		return
	}
	ctx, cancel := contextWithTimeout(r, 60*time.Second)
	defer cancel()
	if err := s.Docker.RemoveImage(ctx, id, r.URL.Query().Get("force") == "true"); err != nil {
		writeError(w, apiError{500, "image_remove_failed", err.Error()})
		return
	}
	s.record(r, "", "docker.image.remove", id, "ok", nil)
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) handleVolumes(w http.ResponseWriter, r *http.Request) {
	if s.requireAdmin(w, r) == nil || !s.dockerAvailable(w) {
		return
	}
	ctx, cancel := contextWithTimeout(r, 20*time.Second)
	defer cancel()
	vols, err := s.Docker.ListVolumes(ctx)
	if err != nil {
		writeError(w, apiError{500, "docker_error", err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"volumes": vols})
}

func (s *Server) handleNetworks(w http.ResponseWriter, r *http.Request) {
	if s.requireAdmin(w, r) == nil || !s.dockerAvailable(w) {
		return
	}
	ctx, cancel := contextWithTimeout(r, 20*time.Second)
	defer cancel()
	nets, err := s.Docker.ListNetworks(ctx)
	if err != nil {
		writeError(w, apiError{500, "docker_error", err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"networks": nets})
}

func (s *Server) handleDockerDF(w http.ResponseWriter, r *http.Request) {
	if s.requireAdmin(w, r) == nil || !s.dockerAvailable(w) {
		return
	}
	ctx, cancel := contextWithTimeout(r, 20*time.Second)
	defer cancel()
	df, err := s.Docker.SystemDF(ctx)
	if err != nil {
		writeError(w, apiError{500, "docker_error", err.Error()})
		return
	}
	writeJSON(w, 200, df)
}

type pruneRequest struct {
	Confirm string `json:"confirm"`
}

func (s *Server) handleDockerPrune(w http.ResponseWriter, r *http.Request) {
	if s.requireAdmin(w, r) == nil || !s.dockerAvailable(w) {
		return
	}
	var req pruneRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, err)
		return
	}
	if req.Confirm != "PRUNE" {
		writeError(w, apiError{400, "confirmation_required", "type PRUNE to confirm"})
		return
	}
	ctx, cancel := contextWithTimeout(r, 2*time.Minute)
	defer cancel()
	res, err := s.Docker.PruneDanglingImages(ctx)
	if err != nil {
		writeError(w, apiError{500, "prune_failed", err.Error()})
		return
	}
	s.record(r, "", "docker.prune", "images", "ok", nil)
	writeJSON(w, 200, map[string]any{"ok": true, "result": res})
}
