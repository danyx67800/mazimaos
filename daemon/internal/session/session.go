package session

import (
	"context"
	"errors"
	"sync"
	"time"
)

type Frame struct {
	At   time.Time
	JPEG []byte
}

type InputEvent struct {
	Type   string  `json:"type"`
	X      float64 `json:"x,omitempty"`
	Y      float64 `json:"y,omitempty"`
	Button int     `json:"button,omitempty"`
	Down   bool    `json:"down,omitempty"`
	Key    string  `json:"key,omitempty"`
	Code   string  `json:"code,omitempty"`
	Text   string  `json:"text,omitempty"`
	Ctrl   bool    `json:"ctrl,omitempty"`
	Alt    bool    `json:"alt,omitempty"`
	Shift  bool    `json:"shift,omitempty"`
	Meta   bool    `json:"meta,omitempty"`
	DeltaX float64 `json:"delta_x,omitempty"`
	DeltaY float64 `json:"delta_y,omitempty"`
}

type Session struct {
	ID        string
	AppID     string
	Title     string
	Username  string
	Display   string
	Width     int
	Height    int
	FPS       int
	CreatedAt time.Time

	manager *Manager
	ctx     context.Context
	cancel  context.CancelFunc
	plat    platform

	mu        sync.Mutex
	clients   map[chan Frame]struct{}
	latest    *Frame
	closed    bool
	closeOnce sync.Once
	lastSeen  time.Time
	frames    uint64
	bytes     uint64
	lastFPS   time.Time
	fpsCount  uint64
	fps       float64
}

func (m *Manager) start(parent context.Context, username, appID, title string, cmd Command) (*Info, error) {
	id, err := m.newID()
	if err != nil {
		return nil, err
	}
	if title == "" {
		title = appID
	}
	ctx, cancel := context.WithCancel(parent)
	s := &Session{
		ID: id, AppID: appID, Title: title, Username: username,
		Width: m.video.Width, Height: m.video.Height, FPS: m.video.FPS,
		CreatedAt: m.now(), lastSeen: m.now(), manager: m, ctx: ctx, cancel: cancel,
		clients: map[chan Frame]struct{}{},
	}
	plat, display, err := startPlatform(ctx, s, m.dataDir, cmd)
	if err != nil {
		cancel()
		return nil, err
	}
	s.plat = plat
	s.Display = display
	m.mu.Lock()
	if len(m.sessions) >= m.max {
		m.mu.Unlock()
		plat.Close()
		cancel()
		return nil, ErrLimit
	}
	m.sessions[id] = s
	m.mu.Unlock()
	info := s.info(m.now())
	return &info, nil
}

func (s *Session) info(now time.Time) Info {
	s.mu.Lock()
	defer s.mu.Unlock()
	state := "running"
	if s.closed {
		state = "closed"
	}
	lastMS := int64(0)
	if s.latest != nil {
		lastMS = s.latest.At.UnixMilli()
	}
	return Info{
		ID: s.ID, AppID: s.AppID, Title: s.Title, Username: s.Username,
		Display: s.Display, Width: s.Width, Height: s.Height, FPS: s.FPS,
		CreatedAt: s.CreatedAt, LastSeen: s.lastSeen, State: state,
		Stats: Stats{
			FPS: s.fps, Frames: s.frames, Bytes: s.bytes, Clients: len(s.clients),
			LastFrameMS: lastMS, SoftwareCodec: true,
		},
	}
}

func (s *Session) LastSeen() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastSeen
}

func (s *Session) HasClients() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.clients) > 0
}

func (s *Session) Subscribe() (<-chan Frame, func()) {
	ch := make(chan Frame, 1)
	s.mu.Lock()
	if s.closed {
		close(ch)
		s.mu.Unlock()
		return ch, func() {}
	}
	s.clients[ch] = struct{}{}
	s.lastSeen = time.Now()
	if s.latest != nil {
		ch <- *s.latest
	}
	s.mu.Unlock()
	cancel := func() {
		s.mu.Lock()
		if _, ok := s.clients[ch]; ok {
			delete(s.clients, ch)
			close(ch)
		}
		s.mu.Unlock()
	}
	return ch, cancel
}

func (s *Session) broadcastFrame(f Frame) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.latest = &f
	s.frames++
	s.bytes += uint64(len(f.JPEG))
	now := time.Now()
	s.fpsCount++
	if s.lastFPS.IsZero() {
		s.lastFPS = now
	}
	if elapsed := now.Sub(s.lastFPS); elapsed >= time.Second {
		s.fps = float64(s.fpsCount) / elapsed.Seconds()
		s.fpsCount = 0
		s.lastFPS = now
	}
	for ch := range s.clients {
		select {
		case ch <- f:
		default:
		}
	}
	s.mu.Unlock()
}

func (s *Session) Input(ctx context.Context, ev InputEvent) error {
	s.mu.Lock()
	s.lastSeen = time.Now()
	closed := s.closed
	p := s.plat
	s.mu.Unlock()
	if closed || p == nil {
		return ErrNotFound
	}
	if ev.Type == "ping" {
		return nil
	}
	return p.Input(ctx, ev)
}

func (s *Session) Clipboard(ctx context.Context, text string) (string, error) {
	s.mu.Lock()
	p := s.plat
	s.mu.Unlock()
	if p == nil {
		return "", ErrNotFound
	}
	if text != "" {
		return "", p.ClipboardSet(ctx, text)
	}
	return p.ClipboardGet(ctx)
}

func (s *Session) Close(reason string) {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		for ch := range s.clients {
			delete(s.clients, ch)
			close(ch)
		}
		p := s.plat
		s.mu.Unlock()
		s.cancel()
		if p != nil {
			p.Close()
		}
		s.manager.remove(s.ID)
	})
}

func (s *Session) Done() <-chan struct{} { return s.ctx.Done() }

func (s *Session) Context() context.Context { return s.ctx }

var errNoPlatform = errors.New("graphical platform is unavailable")
