package audit

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Event struct {
	Time      time.Time         `json:"time"`
	Actor     string            `json:"actor"`
	ActorType string            `json:"actor_type"`
	Action    string            `json:"action"`
	Resource  string            `json:"resource,omitempty"`
	Result    string            `json:"result"`
	IP        string            `json:"ip,omitempty"`
	UserAgent string            `json:"user_agent,omitempty"`
	RequestID string            `json:"request_id,omitempty"`
	Details   map[string]string `json:"details,omitempty"`
}

type Logger struct {
	mu       sync.Mutex
	path     string
	maxBytes int64
	now      func() time.Time
}

func New(path string) *Logger {
	return &Logger{path: path, maxBytes: 10 * 1024 * 1024, now: time.Now}
}

func (l *Logger) Append(e Event) error {
	if e.Time.IsZero() {
		e.Time = l.now().UTC()
	}
	e.Details = redactDetails(e.Details)
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(l.path), 0o750); err != nil {
		return err
	}
	if info, err := os.Stat(l.path); err == nil && info.Size() > l.maxBytes {
		_ = os.Rename(l.path, l.path+".1")
	}
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o640)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(append(b, '\n')); err != nil {
		return err
	}
	return nil
}

func (l *Logger) Read(limit int, query, result string) ([]Event, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	f, err := os.Open(l.path)
	if errors.Is(err, os.ErrNotExist) {
		return []Event{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	var all []Event
	q := strings.ToLower(query)
	for sc.Scan() {
		var e Event
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			continue
		}
		if result != "" && e.Result != result {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(e.Action+" "+e.Actor+" "+e.Resource), q) {
			continue
		}
		all = append(all, e)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(all) > limit {
		all = all[len(all)-limit:]
	}
	for i, j := 0, len(all)-1; i < j; i, j = i+1, j-1 {
		all[i], all[j] = all[j], all[i]
	}
	return all, nil
}

func redactDetails(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		lk := strings.ToLower(k)
		if strings.Contains(lk, "password") || strings.Contains(lk, "token") ||
			strings.Contains(lk, "secret") || strings.Contains(lk, "authorization") ||
			strings.Contains(lk, "cookie") || strings.Contains(lk, "key") {
			out[k] = "[redacted]"
			continue
		}
		if len(v) > 512 {
			v = v[:512] + "..."
		}
		out[k] = v
	}
	return out
}
