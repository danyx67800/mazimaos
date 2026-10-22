package audit

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAppendRedactsSecrets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	l := New(path)
	if err := l.Append(Event{
		Actor: "admin", Action: "login", Result: "ok",
		Details: map[string]string{"password": "hunter2", "token": "abc", "note": "hello"},
	}); err != nil {
		t.Fatal(err)
	}
	events, err := l.Read(10, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d", len(events))
	}
	if events[0].Details["password"] != "[redacted]" || events[0].Details["token"] != "[redacted]" {
		t.Fatalf("not redacted: %#v", events[0].Details)
	}
	if events[0].Details["note"] != "hello" {
		t.Fatalf("note = %q", events[0].Details["note"])
	}
	b, _ := os.ReadFile(path)
	if string(b) == "" {
		t.Fatal("empty audit log")
	}
}

func TestReadNewestFirstAndLimit(t *testing.T) {
	l := New(filepath.Join(t.TempDir(), "audit.jsonl"))
	for _, action := range []string{"a", "b", "c"} {
		if err := l.Append(Event{Actor: "x", Action: action, Result: "ok"}); err != nil {
			t.Fatal(err)
		}
	}
	events, err := l.Read(2, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].Action != "c" || events[1].Action != "b" {
		t.Fatalf("events = %#v", events)
	}
}
