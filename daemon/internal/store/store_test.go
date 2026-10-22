package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestOpenCreatesAndPersistsState(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.state.SchemaVersion != SchemaVersion {
		t.Fatalf("schema = %d", s.state.SchemaVersion)
	}
	if err := s.PutUser(&User{Username: "admin", Role: RoleAdmin, DisplayName: "Admin"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("state permissions = %o", info.Mode().Perm())
	}
	s2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	u, err := s2.User("admin")
	if err != nil {
		t.Fatal(err)
	}
	if u.Role != RoleAdmin {
		t.Fatalf("role = %s", u.Role)
	}
}

func TestUpdateIsAtomicOnError(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	want := s.state.Settings.Hostname
	err = s.Update(func(st *State) error {
		st.Settings.Hostname = "changed"
		return os.ErrInvalid
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if s.state.Settings.Hostname != want {
		t.Fatalf("mutation leaked: %q", s.state.Settings.Hostname)
	}
}

func TestSessionsPurge(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	s.now = func() time.Time { return time.Unix(1000, 0) }
	if err := s.PutSession("h", Session{Username: "u", ExpiresAt: time.Unix(500, 0)}); err != nil {
		t.Fatal(err)
	}
	n, err := s.PurgeExpiredSessions(time.Unix(1001, 0))
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("purged %d", n)
	}
	if _, err := s.Session("h"); err != ErrNotFound {
		t.Fatalf("err = %v", err)
	}
}
