package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/danyx67800/mazimaos/daemon/internal/system"
)

func (s *Server) handleSystem(w http.ResponseWriter, r *http.Request) {
	snap, err := s.System.Snapshot()
	if err != nil {
		writeError(w, apiError{500, "system_unavailable", err.Error()})
		return
	}
	writeJSON(w, 200, snap)
}

func (s *Server) handleSystemHistory(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"history": s.System.History()})
}

func (s *Server) handleProcesses(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	procs, err := s.System.TopProcesses(limit)
	if err != nil {
		writeError(w, apiError{500, "processes_unavailable", err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"processes": procs})
}

func (s *Server) handleServices(w http.ResponseWriter, r *http.Request) {
	if s.requireAdmin(w, r) == nil {
		return
	}
	services, err := system.Services()
	if err != nil {
		writeError(w, apiError{500, "services_unavailable", err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"services": services})
}

type serviceActionRequest struct {
	Name   string `json:"name"`
	Action string `json:"action"`
}

func (s *Server) handleServiceAction(w http.ResponseWriter, r *http.Request) {
	u := s.requireAdmin(w, r)
	if u == nil {
		return
	}
	var req serviceActionRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, err)
		return
	}
	ctx, cancel := contextWithTimeout(r, 30*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- system.ServiceAction(req.Name, req.Action) }()
	select {
	case err := <-done:
		if err != nil {
			s.record(r, "", "service."+req.Action, req.Name, "error", map[string]string{"error": err.Error()})
			writeError(w, apiError{500, "service_action_failed", err.Error()})
			return
		}
	case <-ctx.Done():
		writeError(w, apiError{504, "timeout", "service action timed out"})
		return
	}
	s.record(r, "", "service."+req.Action, req.Name, "ok", nil)
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	if s.requireAdmin(w, r) == nil {
		return
	}
	lines, _ := strconv.Atoi(r.URL.Query().Get("lines"))
	out, err := system.LogsUnit(r.URL.Query().Get("unit"), lines)
	if err != nil && strings.TrimSpace(out) == "" {
		writeError(w, apiError{500, "logs_unavailable", err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"logs": out})
}

type powerRequest struct {
	Action   string `json:"action"`
	Password string `json:"password"`
	Confirm  string `json:"confirm"`
}

func (s *Server) handlePower(w http.ResponseWriter, r *http.Request) {
	u := s.requireAdmin(w, r)
	if u == nil {
		return
	}
	var req powerRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, err)
		return
	}
	want := map[string]string{"reboot": "REBOOT", "poweroff": "POWEROFF"}
	expected, ok := want[req.Action]
	if !ok {
		writeError(w, apiError{400, "invalid_action", "action must be reboot or poweroff"})
		return
	}
	if req.Confirm != expected {
		writeError(w, apiError{400, "confirmation_required", "type " + expected + " to confirm"})
		return
	}
	if !s.Auth.VerifyUserPassword(u.Username, req.Password) {
		writeError(w, apiError{403, "invalid_password", "password is incorrect"})
		return
	}
	if err := system.Power(req.Action); err != nil {
		s.record(r, "", "power."+req.Action, "host", "error", map[string]string{"error": err.Error()})
		writeError(w, apiError{500, "power_failed", err.Error()})
		return
	}
	s.record(r, "", "power."+req.Action, "host", "ok", nil)
	writeJSON(w, 200, map[string]any{"ok": true})
}

type processSignalRequest struct {
	PID      int    `json:"pid"`
	Signal   string `json:"signal"`
	Password string `json:"password"`
	Confirm  bool   `json:"confirm"`
}

func (s *Server) handleProcessSignal(w http.ResponseWriter, r *http.Request) {
	u := s.requireAdmin(w, r)
	if u == nil {
		return
	}
	var req processSignalRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, err)
		return
	}
	if !req.Confirm {
		writeError(w, apiError{400, "confirmation_required", "explicit confirmation is required"})
		return
	}
	if !s.Auth.VerifyUserPassword(u.Username, req.Password) {
		writeError(w, apiError{403, "invalid_password", "password is incorrect"})
		return
	}
	if err := system.SignalProcess(req.PID, strings.ToUpper(req.Signal)); err != nil {
		s.record(r, "", "process.signal", strconv.Itoa(req.PID), "error", map[string]string{"error": err.Error()})
		writeError(w, apiError{400, "signal_failed", err.Error()})
		return
	}
	s.record(r, "", "process.signal", strconv.Itoa(req.PID), "ok", map[string]string{"signal": req.Signal})
	writeJSON(w, 200, map[string]any{"ok": true})
}

func contextWithTimeout(r *http.Request, d time.Duration) (ctx context.Context, cancel context.CancelFunc) {
	return context.WithTimeout(r.Context(), d)
}
