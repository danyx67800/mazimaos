package session

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sort"
	"sync"
	"time"
)

var (
	ErrUnsupported = errors.New("graphical sessions require Linux with Xvfb")
	ErrNotFound    = errors.New("session not found")
	ErrLimit       = errors.New("too many active graphical sessions")
)

type VideoMode struct {
	Width  int `json:"width"`
	Height int `json:"height"`
	FPS    int `json:"fps"`
}

type App struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Available   bool   `json:"available"`
	Missing     string `json:"missing,omitempty"`
}

type Stats struct {
	FPS           float64 `json:"fps"`
	Frames        uint64  `json:"frames"`
	Bytes         uint64  `json:"bytes"`
	Clients       int     `json:"clients"`
	LastFrameMS   int64   `json:"last_frame_ms"`
	SoftwareCodec bool    `json:"software_codec"`
}

type Info struct {
	ID        string    `json:"id"`
	AppID     string    `json:"app_id"`
	Title     string    `json:"title"`
	Username  string    `json:"username"`
	Display   string    `json:"display"`
	Width     int       `json:"width"`
	Height    int       `json:"height"`
	FPS       int       `json:"fps"`
	CreatedAt time.Time `json:"created_at"`
	LastSeen  time.Time `json:"last_seen"`
	Stats     Stats     `json:"stats"`
	State     string    `json:"state"`
}

type StreamToken struct {
	SessionID string
	Username  string
	Expires   time.Time
}

type Manager struct {
	mu       sync.Mutex
	sessions map[string]*Session
	tokens   map[string]StreamToken
	max      int
	idle     time.Duration
	now      func() time.Time
	dataDir  string
	video    VideoMode
	apps     []App
	commands map[string]Command
}

type Command struct {
	Path string
	Args []string
}

func NewManager(dataDir string, max int, idle time.Duration, video VideoMode) *Manager {
	if video.Width <= 0 {
		video.Width = 1280
	}
	if video.Height <= 0 {
		video.Height = 800
	}
	if video.FPS <= 0 || video.FPS > 60 {
		video.FPS = 30
	}
	if max <= 0 {
		max = 3
	}
	m := &Manager{
		sessions: map[string]*Session{},
		tokens:   map[string]StreamToken{},
		max:      max,
		idle:     idle,
		now:      time.Now,
		dataDir:  dataDir,
		video:    video,
	}
	m.commands = defaultCommands()
	for id, c := range m.commands {
		apps := App{ID: id, Available: commandAvailable(c.Path)}
		if !apps.Available {
			apps.Missing = c.Path
		}
		switch id {
		case "terminal":
			apps.Name = "Terminal"
			apps.Description = "A real Linux shell in an X11 terminal, streamed to your browser."
		case "browser":
			apps.Name = "Browser"
			apps.Description = "An isolated Chromium or Firefox window running on the server."
		}
		m.apps = append(m.apps, apps)
	}
	sort.Slice(m.apps, func(i, j int) bool { return m.apps[i].ID < m.apps[j].ID })
	return m
}

func (m *Manager) SetCommands(commands map[string]Command) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.commands = commands
	for i := range m.apps {
		if c, ok := commands[m.apps[i].ID]; ok {
			m.apps[i].Available = commandAvailable(c.Path)
		}
	}
}

func (m *Manager) Apps() []App {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := append([]App(nil), m.apps...)
	for i := range out {
		if c, ok := m.commands[out[i].ID]; ok {
			out[i].Available = commandAvailable(c.Path)
			if !out[i].Available {
				out[i].Missing = c.Path
			}
		}
	}
	return out
}

func (m *Manager) Video() VideoMode { return m.video }

func (m *Manager) Start(ctx context.Context, username, appID, title string) (*Info, error) {
	m.mu.Lock()
	if len(m.sessions) >= m.max {
		m.mu.Unlock()
		return nil, ErrLimit
	}
	cmd, ok := m.commands[appID]
	if !ok {
		m.mu.Unlock()
		return nil, ErrNotFound
	}
	if !commandAvailable(cmd.Path) {
		m.mu.Unlock()
		return nil, errors.New("application is not installed: " + cmd.Path)
	}
	m.mu.Unlock()
	return m.start(ctx, username, appID, title, cmd)
}

func (m *Manager) Get(id, username string) (*Info, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if !ok {
		return nil, ErrNotFound
	}
	if s.Username != username {
		return nil, ErrNotFound
	}
	info := s.info(m.now())
	return &info, nil
}

func (m *Manager) List(username string) []Info {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Info
	for _, s := range m.sessions {
		if s.Username == username {
			out = append(out, s.info(m.now()))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out
}

func (m *Manager) ClipboardByID(ctx context.Context, id, username, text string) (string, error) {
	m.mu.Lock()
	s, ok := m.sessions[id]
	m.mu.Unlock()
	if !ok || s.Username != username {
		return "", ErrNotFound
	}
	return s.Clipboard(ctx, text)
}

func (m *Manager) Stop(id, username string) error {
	m.mu.Lock()
	s, ok := m.sessions[id]
	m.mu.Unlock()
	if !ok || s.Username != username {
		return ErrNotFound
	}
	s.Close("stopped by user")
	return nil
}

func (m *Manager) CloseAll() {
	m.mu.Lock()
	sessions := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		sessions = append(sessions, s)
	}
	m.mu.Unlock()
	for _, s := range sessions {
		s.Close("daemon stopping")
	}
}

func (m *Manager) remove(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, id)
}

func (m *Manager) newID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (m *Manager) Token(sessionID, username string) (string, time.Time, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[sessionID]
	if !ok || s.Username != username {
		return "", time.Time{}, ErrNotFound
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", time.Time{}, err
	}
	tok := hex.EncodeToString(b)
	exp := m.now().Add(60 * time.Second)
	m.tokens[tok] = StreamToken{SessionID: sessionID, Username: username, Expires: exp}
	return tok, exp, nil
}

func (m *Manager) ConsumeToken(token, sessionID string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tokens[token]
	if !ok || t.SessionID != sessionID || m.now().After(t.Expires) {
		return nil, ErrNotFound
	}
	delete(m.tokens, token)
	s, ok := m.sessions[sessionID]
	if !ok {
		return nil, ErrNotFound
	}
	return s, nil
}

func (m *Manager) Cleanup() {
	m.mu.Lock()
	var stale []*Session
	for _, s := range m.sessions {
		if s.Username == "" {
			continue
		}
		if m.idle > 0 && !s.HasClients() && m.now().Sub(s.LastSeen()) > m.idle {
			stale = append(stale, s)
		}
	}
	m.mu.Unlock()
	for _, s := range stale {
		s.Close("idle timeout")
	}
}
