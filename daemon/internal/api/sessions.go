package api

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/danyx67800/mazimaos/daemon/internal/session"
	"github.com/gorilla/websocket"
)

func (s *Server) handleSessionApps(w http.ResponseWriter, r *http.Request) {
	if s.Sessions == nil {
		writeJSON(w, 200, map[string]any{"apps": []any{}, "supported": false, "reason": session.ErrUnsupported.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{
		"apps": s.Sessions.Apps(), "supported": runtimeIsLinux(), "video": s.Sessions.Video(),
	})
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	if s.Sessions == nil {
		writeJSON(w, 200, map[string]any{"sessions": []any{}})
		return
	}
	ac := s.authFor(r)
	writeJSON(w, 200, map[string]any{"sessions": s.Sessions.List(ac.User.Username)})
}

type startSessionRequest struct {
	AppID string `json:"app_id"`
	Title string `json:"title"`
}

func (s *Server) handleSessionStart(w http.ResponseWriter, r *http.Request) {
	if s.Sessions == nil {
		writeError(w, apiError{501, "streaming_unsupported", session.ErrUnsupported.Error()})
		return
	}
	ac := s.authFor(r)
	var req startSessionRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, err)
		return
	}
	if len(req.Title) > 120 {
		writeError(w, apiError{400, "invalid_title", "session title is too long"})
		return
	}
	info, err := s.Sessions.Start(r.Context(), ac.User.Username, req.AppID, req.Title)
	if err != nil {
		if errors.Is(err, session.ErrLimit) {
			writeError(w, apiError{429, "session_limit", err.Error()})
			return
		}
		s.record(r, "", "session.start", req.AppID, "error", map[string]string{"error": err.Error()})
		writeError(w, apiError{500, "session_start_failed", err.Error()})
		return
	}
	s.record(r, "", "session.start", info.ID, "ok", map[string]string{"app_id": req.AppID})
	writeJSON(w, 201, map[string]any{"session": info})
}

func (s *Server) handleSessionStop(w http.ResponseWriter, r *http.Request) {
	if s.Sessions == nil {
		writeError(w, apiError{501, "streaming_unsupported", session.ErrUnsupported.Error()})
		return
	}
	ac := s.authFor(r)
	id := r.PathValue("id")
	if err := s.Sessions.Stop(id, ac.User.Username); err != nil {
		writeError(w, apiError{404, "session_not_found", "session not found"})
		return
	}
	s.record(r, "", "session.stop", id, "ok", nil)
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) handleSessionToken(w http.ResponseWriter, r *http.Request) {
	if s.Sessions == nil {
		writeError(w, apiError{501, "streaming_unsupported", session.ErrUnsupported.Error()})
		return
	}
	ac := s.authFor(r)
	id := r.PathValue("id")
	if _, err := s.Sessions.Get(id, ac.User.Username); err != nil {
		writeError(w, apiError{404, "session_not_found", "session not found"})
		return
	}
	token, expires, err := s.Sessions.Token(id, ac.User.Username)
	if err != nil {
		writeError(w, apiError{500, "token_failed", err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"token": token, "expires_at": expires, "session_id": id})
}

func (s *Server) handleClipboardGet(w http.ResponseWriter, r *http.Request) {
	if s.Sessions == nil {
		writeError(w, apiError{501, "streaming_unsupported", session.ErrUnsupported.Error()})
		return
	}
	ac := s.authFor(r)
	sess, err := s.Sessions.Get(r.PathValue("id"), ac.User.Username)
	if err != nil {
		writeError(w, apiError{404, "session_not_found", "session not found"})
		return
	}
	ctx, cancel := contextWithTimeout(r, 5*time.Second)
	defer cancel()
	text, err := s.Sessions.ClipboardByID(ctx, sess.ID, ac.User.Username, "")
	if err != nil {
		writeError(w, apiError{500, "clipboard_failed", err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"text": text})
}

func (s *Server) handleClipboardSet(w http.ResponseWriter, r *http.Request) {
	if s.Sessions == nil {
		writeError(w, apiError{501, "streaming_unsupported", session.ErrUnsupported.Error()})
		return
	}
	ac := s.authFor(r)
	var body struct {
		Text string `json:"text"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, err)
		return
	}
	ctx, cancel := contextWithTimeout(r, 5*time.Second)
	defer cancel()
	if _, err := s.Sessions.ClipboardByID(ctx, r.PathValue("id"), ac.User.Username, body.Text); err != nil {
		writeError(w, apiError{500, "clipboard_failed", err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) handleSessionWS(w http.ResponseWriter, r *http.Request) {
	if s.Sessions == nil {
		http.Error(w, "streaming is not supported", http.StatusNotImplemented)
		return
	}
	id := r.URL.Query().Get("id")
	token := r.URL.Query().Get("token")
	if id == "" || token == "" {
		http.Error(w, "missing session or token", http.StatusBadRequest)
		return
	}
	sess, err := s.Sessions.ConsumeToken(token, id)
	if err != nil {
		http.Error(w, "invalid or expired stream token", http.StatusUnauthorized)
		return
	}
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	conn.SetReadLimit(128 * 1024)
	_ = conn.SetReadDeadline(time.Now().Add(75 * time.Second))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(75 * time.Second))
	})
	info, err := s.Sessions.Get(sess.ID, sess.Username)
	if err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"error","error":"session ended"}`))
		return
	}
	hello, _ := json.Marshal(map[string]any{
		"type": "hello", "session": info, "video": s.Sessions.Video(),
	})
	if err := conn.WriteMessage(websocket.TextMessage, hello); err != nil {
		return
	}
	frames, unsubscribe := sess.Subscribe()
	defer unsubscribe()
	type wsOut struct {
		mt   int
		data []byte
	}
	out := make(chan wsOut, 16)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			mt, b, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if mt != websocket.TextMessage {
				continue
			}
			_ = conn.SetReadDeadline(time.Now().Add(75 * time.Second))
			if len(b) > 128*1024 {
				continue
			}
			for _, m := range s.handleWSInput(sess, b) {
				select {
				case out <- m:
				default:
				}
			}
		}
	}()
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	header := make([]byte, 12)
	copy(header, "MZMF")
	for {
		select {
		case frame, ok := <-frames:
			if !ok {
				_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "session closed"))
				return
			}
			binary.BigEndian.PutUint64(header[4:], uint64(frame.At.UnixMilli()))
			msg := make([]byte, 12+len(frame.JPEG))
			copy(msg, header)
			copy(msg[12:], frame.JPEG)
			_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if err := conn.WriteMessage(websocket.BinaryMessage, msg); err != nil {
				return
			}
		case o := <-out:
			_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if err := conn.WriteMessage(o.mt, o.data); err != nil {
				return
			}
		case <-ticker.C:
			_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
			inc, _ := s.Sessions.Get(sess.ID, sess.Username)
			if inc != nil {
				msg, _ := json.Marshal(map[string]any{"type": "stats", "stats": inc.Stats})
				_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
				if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
					return
				}
			}
		case <-done:
			return
		case <-sess.Done():
			return
		}
	}
}

func (s *Server) handleWSInput(sess *session.Session, b []byte) []struct {
	mt   int
	data []byte
} {
	var msg struct {
		Type  string              `json:"type"`
		Event *session.InputEvent `json:"event"`
		Text  string              `json:"text"`
		T     int64               `json:"t"`
	}
	if err := json.Unmarshal(b, &msg); err != nil {
		return nil
	}
	switch msg.Type {
	case "input":
		if msg.Event == nil {
			return nil
		}
		if err := validateInput(msg.Event); err != nil {
			return nil
		}
		ctx, cancel := contextWithTimeoutRequest(sess, 5*time.Second)
		defer cancel()
		_ = sess.Input(ctx, *msg.Event)
	case "clipboard":
		ctx, cancel := contextWithTimeoutRequest(sess, 5*time.Second)
		defer cancel()
		text, err := sess.Clipboard(ctx, msg.Text)
		if err != nil {
			out, _ := json.Marshal(map[string]any{"type": "error", "error": err.Error()})
			return []struct {
				mt   int
				data []byte
			}{{websocket.TextMessage, out}}
		}
		if msg.Text == "" {
			out, _ := json.Marshal(map[string]any{"type": "clipboard", "text": text})
			return []struct {
				mt   int
				data []byte
			}{{websocket.TextMessage, out}}
		}
	case "ping":
		out, _ := json.Marshal(map[string]any{"type": "pong", "t": msg.T, "server_time": time.Now().UnixMilli()})
		return []struct {
			mt   int
			data []byte
		}{{websocket.TextMessage, out}}
	}
	return nil
}

func validateInput(ev *session.InputEvent) error {
	switch ev.Type {
	case "pointer", "button", "wheel", "key", "text":
	default:
		return errors.New("invalid input type")
	}
	if len(ev.Text) > 256 || len(ev.Key) > 32 || len(ev.Code) > 32 {
		return errors.New("input is too large")
	}
	if ev.X < 0 || ev.Y < 0 || ev.X > 20000 || ev.Y > 20000 {
		return errors.New("pointer out of range")
	}
	if ev.Button < 0 || ev.Button > 7 {
		return errors.New("invalid button")
	}
	return nil
}

func contextWithTimeoutRequest(sess *session.Session, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(sess.Context(), d)
}
