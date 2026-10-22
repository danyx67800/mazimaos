package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var ErrNotFound = errors.New("not found")

type Store struct {
	mu    sync.RWMutex
	path  string
	state State
	now   func() time.Time
}

func Open(path string) (*Store, error) {
	s := &Store{path: path, now: time.Now}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, err
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		s.state = NewState()
		if err := s.saveLocked(&s.state); err != nil {
			return nil, err
		}
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &s.state); err != nil {
		return nil, fmt.Errorf("read state: %w", err)
	}
	if s.state.Users == nil {
		s.state.Users = map[string]*User{}
	}
	if s.state.Sessions == nil {
		s.state.Sessions = map[string]*Session{}
	}
	if s.state.APIKeys == nil {
		s.state.APIKeys = map[string]*APIKey{}
	}
	if s.state.Desktops == nil {
		s.state.Desktops = map[string]DesktopState{}
	}
	if s.state.SchemaVersion == 0 {
		s.state.SchemaVersion = SchemaVersion
	}
	if s.state.SchemaVersion > SchemaVersion {
		return nil, fmt.Errorf("state schema %d is newer than supported %d", s.state.SchemaVersion, SchemaVersion)
	}
	if s.state.SchemaVersion < SchemaVersion {
		if err := migrate(&s.state); err != nil {
			return nil, err
		}
		if err := s.saveLocked(&s.state); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func migrate(st *State) error {
	if st.Settings.Timezone == "" {
		st.Settings = DefaultSettings()
	}
	if st.Settings.Hostname == "" {
		st.Settings.Hostname = "mazima"
	}
	st.SchemaVersion = SchemaVersion
	return nil
}

func cloneState(in State) (State, error) {
	b, err := json.Marshal(in)
	if err != nil {
		return State{}, err
	}
	var out State
	if err := json.Unmarshal(b, &out); err != nil {
		return State{}, err
	}
	return out, nil
}

func (s *Store) saveLocked(st *State) error {
	st.UpdatedAt = s.now().UTC()
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".state-*.json")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, s.path); err != nil {
		return err
	}
	if d, err := os.Open(filepath.Dir(s.path)); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	s.state = *st
	return nil
}

func (s *Store) Read(fn func(*State) error) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cp, err := cloneState(s.state)
	if err != nil {
		return err
	}
	return fn(&cp)
}

func (s *Store) Update(fn func(*State) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp, err := cloneState(s.state)
	if err != nil {
		return err
	}
	if err := fn(&cp); err != nil {
		return err
	}
	return s.saveLocked(&cp)
}

func (s *Store) Snapshot() (State, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneState(s.state)
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveLocked(&s.state)
}

func (s *Store) User(username string) (*User, error) {
	var out *User
	err := s.Read(func(st *State) error {
		u, ok := st.Users[username]
		if !ok {
			return ErrNotFound
		}
		cp := *u
		cp.Passkeys = append([]Passkey(nil), u.Passkeys...)
		out = &cp
		return nil
	})
	return out, err
}

func (s *Store) Users() ([]User, error) {
	var out []User
	err := s.Read(func(st *State) error {
		for _, u := range st.Users {
			out = append(out, *u)
		}
		return nil
	})
	return out, err
}

func (s *Store) HasUsers() (bool, error) {
	var has bool
	err := s.Read(func(st *State) error {
		has = len(st.Users) > 0
		return nil
	})
	return has, err
}

func (s *Store) PutUser(u *User) error {
	return s.Update(func(st *State) error {
		cp := *u
		cp.Passkeys = append([]Passkey(nil), u.Passkeys...)
		st.Users[u.Username] = &cp
		return nil
	})
}

func (s *Store) DeleteUser(username string) error {
	return s.Update(func(st *State) error {
		if _, ok := st.Users[username]; !ok {
			return ErrNotFound
		}
		delete(st.Users, username)
		for h, sess := range st.Sessions {
			if sess.Username == username {
				delete(st.Sessions, h)
			}
		}
		for id, key := range st.APIKeys {
			if key.Username == username {
				delete(st.APIKeys, id)
			}
		}
		delete(st.Desktops, username)
		return nil
	})
}

func (s *Store) PutSession(hash string, sess Session) error {
	return s.Update(func(st *State) error {
		st.Sessions[hash] = &sess
		return nil
	})
}

func (s *Store) Session(hash string) (*Session, error) {
	var out *Session
	err := s.Read(func(st *State) error {
		sess, ok := st.Sessions[hash]
		if !ok {
			return ErrNotFound
		}
		cp := *sess
		out = &cp
		return nil
	})
	return out, err
}

func (s *Store) TouchSession(hash string, now time.Time) error {
	return s.Update(func(st *State) error {
		if sess, ok := st.Sessions[hash]; ok {
			sess.LastSeen = now
		}
		return nil
	})
}

func (s *Store) DeleteSession(hash string) error {
	return s.Update(func(st *State) error {
		delete(st.Sessions, hash)
		return nil
	})
}

func (s *Store) PurgeExpiredSessions(now time.Time) (int, error) {
	n := 0
	err := s.Update(func(st *State) error {
		for h, sess := range st.Sessions {
			if now.After(sess.ExpiresAt) {
				delete(st.Sessions, h)
				n++
			}
		}
		return nil
	})
	return n, err
}

func (s *Store) SessionsFor(username string) (map[string]Session, error) {
	out := map[string]Session{}
	err := s.Read(func(st *State) error {
		for h, sess := range st.Sessions {
			if sess.Username == username {
				out[h] = *sess
			}
		}
		return nil
	})
	return out, err
}

func (s *Store) PutDesktop(username string, d DesktopState) error {
	return s.Update(func(st *State) error {
		d.UpdatedAt = s.now().UTC()
		st.Desktops[username] = d
		return nil
	})
}

func (s *Store) Desktop(username string) (DesktopState, error) {
	var out DesktopState
	err := s.Read(func(st *State) error {
		out = st.Desktops[username]
		return nil
	})
	return out, err
}

func (s *Store) PutAPIKey(key *APIKey) error {
	return s.Update(func(st *State) error {
		st.APIKeys[key.ID] = key
		return nil
	})
}

func (s *Store) APIKeys() ([]APIKey, error) {
	var out []APIKey
	err := s.Read(func(st *State) error {
		for _, k := range st.APIKeys {
			out = append(out, *k)
		}
		return nil
	})
	return out, err
}

func (s *Store) APIKeyByHash(hash string) (*APIKey, error) {
	var out *APIKey
	err := s.Read(func(st *State) error {
		for _, k := range st.APIKeys {
			if subtleEqual(k.Hash, hash) {
				cp := *k
				out = &cp
				return nil
			}
		}
		return ErrNotFound
	})
	return out, err
}

func (s *Store) DeleteAPIKey(id string) error {
	return s.Update(func(st *State) error {
		delete(st.APIKeys, id)
		return nil
	})
}

func subtleEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var v byte
	for i := range a {
		v |= a[i] ^ b[i]
	}
	return v == 0
}
