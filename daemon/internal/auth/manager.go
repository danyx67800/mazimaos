package auth

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/danyx67800/mazimaos/daemon/internal/store"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrTwoFactorRequired  = errors.New("two-factor authentication required")
	ErrTwoFactorInvalid   = errors.New("invalid one-time code")
	ErrAccountDisabled    = errors.New("account disabled")
	ErrSessionExpired     = errors.New("session expired")
	ErrSessionInvalid     = errors.New("session invalid")
	ErrChallengeExpired   = errors.New("login challenge expired")
	ErrChallengeAttempts  = errors.New("too many login challenge attempts")
)

var usernameRE = regexp.MustCompile(`^[a-z][a-z0-9_-]{2,31}$`)

type LoginStatus string

const (
	LoginComplete LoginStatus = "complete"
	LoginTOTP     LoginStatus = "totp_required"
)

type LoginResult struct {
	Status        LoginStatus
	User          *store.User
	SessionToken  string
	CSRF          string
	ExpiresAt     time.Time
	Challenge     string
	ChallengeLeft int
}

type Challenge struct {
	Username  string
	IP        string
	UserAgent string
	Expires   time.Time
	Attempts  int
}

type Manager struct {
	Store      *store.Store
	Sealer     *Sealer
	Limiter    *Limiter
	SessionTTL time.Duration
	Now        func() time.Time

	mu         sync.Mutex
	challenges map[string]*Challenge
	totpSetup  map[string]*totpPending
}

type totpPending struct {
	Secret       string
	Recovery     []string
	RecoveryHash []string
	Expires      time.Time
}

func NewManager(st *store.Store, sealer *Sealer, ttl time.Duration) *Manager {
	return &Manager{
		Store:      st,
		Sealer:     sealer,
		Limiter:    NewLimiter(),
		SessionTTL: ttl,
		Now:        time.Now,
		challenges: map[string]*Challenge{},
		totpSetup:  map[string]*totpPending{},
	}
}

func ValidateUsername(name string) bool {
	return usernameRE.MatchString(name)
}

func (m *Manager) BeginLogin(username, password, ip, ua string) (*LoginResult, error) {
	key := ip + "|" + strings.ToLower(username)
	if ok, wait := m.Limiter.Allowed(key); !ok {
		return nil, fmt.Errorf("%w: retry in %s", ErrInvalidCredentials, wait.Round(time.Second))
	}
	u, err := m.Store.User(username)
	if errors.Is(err, store.ErrNotFound) {
		d := m.Limiter.Failure(key)
		time.Sleep(minDuration(d, 250*time.Millisecond))
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}
	if u.Disabled {
		m.Limiter.Failure(key)
		return nil, ErrInvalidCredentials
	}
	ok, err := VerifyPassword(u.PasswordHash, password)
	if err != nil || !ok {
		m.Limiter.Failure(key)
		return nil, ErrInvalidCredentials
	}
	needs := u.TOTPEnabled || (len(u.Passkeys) > 0)
	if needs {
		ch, err := m.newChallenge(u.Username, ip, ua)
		if err != nil {
			return nil, err
		}
		return &LoginResult{Status: LoginTOTP, Challenge: ch, ChallengeLeft: 5}, nil
	}
	m.Limiter.Success(key)
	res, err := m.createSession(u, ip, ua)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (m *Manager) FinishChallenge(challenge, code, kind, ip, ua string) (*LoginResult, error) {
	m.mu.Lock()
	ch, ok := m.challenges[challenge]
	if !ok || m.Now().After(ch.Expires) {
		if ok {
			delete(m.challenges, challenge)
		}
		m.mu.Unlock()
		return nil, ErrChallengeExpired
	}
	if ch.IP != ip {
		m.mu.Unlock()
		return nil, ErrChallengeExpired
	}
	ch.Attempts++
	if ch.Attempts > 5 {
		delete(m.challenges, challenge)
		m.mu.Unlock()
		return nil, ErrChallengeAttempts
	}
	m.mu.Unlock()

	u, err := m.Store.User(ch.Username)
	if err != nil {
		return nil, ErrInvalidCredentials
	}
	if u.Disabled {
		return nil, ErrInvalidCredentials
	}

	valid := false
	switch kind {
	case "", "totp":
		if u.TOTPEnabled {
			secret, err := m.decryptTOTP(u)
			if err == nil && VerifyTOTP(secret, code, m.Now()) {
				valid = true
			}
		}
	case "recovery":
		remaining, ok := ConsumeRecoveryCode(code, u.RecoveryHashes)
		if ok {
			u.RecoveryHashes = remaining
			if err := m.Store.PutUser(u); err != nil {
				return nil, err
			}
			valid = true
		}
	}
	if !valid {
		m.Limiter.Failure(ip + "|" + strings.ToLower(ch.Username))
		return nil, ErrTwoFactorInvalid
	}
	m.mu.Lock()
	delete(m.challenges, challenge)
	m.mu.Unlock()
	m.Limiter.Success(ip + "|" + strings.ToLower(ch.Username))
	return m.createSession(u, ip, ua)
}

func (m *Manager) newChallenge(username, ip, ua string) (string, error) {
	token, err := NewToken(32)
	if err != nil {
		return "", err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.Now()
	for k, c := range m.challenges {
		if now.After(c.Expires) {
			delete(m.challenges, k)
		}
	}
	m.challenges[token] = &Challenge{Username: username, IP: ip, UserAgent: ua, Expires: now.Add(5 * time.Minute)}
	return token, nil
}

func (m *Manager) createSession(u *store.User, ip, ua string) (*LoginResult, error) {
	token, err := NewToken(32)
	if err != nil {
		return nil, err
	}
	csrf, err := NewCSRF()
	if err != nil {
		return nil, err
	}
	now := m.Now().UTC()
	sess := store.Session{
		Username:  u.Username,
		CSRF:      csrf,
		CreatedAt: now,
		ExpiresAt: now.Add(m.SessionTTL),
		LastSeen:  now,
		IP:        ip,
		UserAgent: ua,
	}
	if err := m.Store.PutSession(TokenHash(token), sess); err != nil {
		return nil, err
	}
	u.LastLoginAt = now
	if err := m.Store.PutUser(u); err != nil {
		return nil, err
	}
	return &LoginResult{
		Status:       LoginComplete,
		User:         u,
		SessionToken: token,
		CSRF:         csrf,
		ExpiresAt:    sess.ExpiresAt,
	}, nil
}

func (m *Manager) Authenticate(token string) (*store.User, *store.Session, error) {
	if token == "" {
		return nil, nil, ErrSessionInvalid
	}
	hash := TokenHash(token)
	sess, err := m.Store.Session(hash)
	if err != nil {
		return nil, nil, ErrSessionInvalid
	}
	if m.Now().After(sess.ExpiresAt) {
		_ = m.Store.DeleteSession(hash)
		return nil, nil, ErrSessionExpired
	}
	u, err := m.Store.User(sess.Username)
	if err != nil || u.Disabled {
		return nil, nil, ErrSessionInvalid
	}
	return u, sess, nil
}

func (m *Manager) Logout(token string) error {
	if token == "" {
		return nil
	}
	return m.Store.DeleteSession(TokenHash(token))
}

func (m *Manager) RevokeAll(username string) error {
	sessions, err := m.Store.SessionsFor(username)
	if err != nil {
		return err
	}
	for hash := range sessions {
		if err := m.Store.DeleteSession(hash); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) SetupAdmin(username, displayName, password, language, timezone, hostname string) (*store.User, error) {
	hasUsers, err := m.Store.HasUsers()
	if err != nil {
		return nil, err
	}
	if hasUsers {
		return nil, errors.New("setup already completed")
	}
	if !ValidateUsername(username) {
		return nil, errors.New("username must be 3-32 characters: lowercase letters, digits, dash or underscore")
	}
	if err := PasswordStrengthError(password); err != nil {
		return nil, err
	}
	if language != "it" && language != "en" {
		return nil, errors.New("unsupported language")
	}
	if timezone == "" {
		timezone = "Europe/Rome"
	}
	if hostname == "" {
		hostname = "mazima"
	}
	if len(displayName) > 80 {
		return nil, errors.New("display name is too long")
	}
	hash, err := HashPassword(password)
	if err != nil {
		return nil, err
	}
	now := m.Now().UTC()
	u := &store.User{
		Username:     username,
		DisplayName:  displayName,
		Role:         store.RoleAdmin,
		PasswordHash: hash,
		Language:     language,
		CreatedAt:    now,
	}
	err = m.Store.Update(func(st *store.State) error {
		if len(st.Users) > 0 {
			return errors.New("setup already completed")
		}
		st.Users[username] = u
		st.SetupComplete = true
		st.Settings.Language = language
		st.Settings.Timezone = timezone
		st.Settings.Hostname = hostname
		return nil
	})
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (m *Manager) ChangePassword(username, oldPassword, newPassword string) error {
	u, err := m.Store.User(username)
	if err != nil {
		return err
	}
	ok, err := VerifyPassword(u.PasswordHash, oldPassword)
	if err != nil || !ok {
		return ErrInvalidCredentials
	}
	if err := PasswordStrengthError(newPassword); err != nil {
		return err
	}
	hash, err := HashPassword(newPassword)
	if err != nil {
		return err
	}
	u.PasswordHash = hash
	u.MustChangePass = false
	return m.Store.PutUser(u)
}

func (m *Manager) StartTOTPSetup(username string) (otpauth, secret string, recovery []string, err error) {
	u, err := m.Store.User(username)
	if err != nil {
		return "", "", nil, err
	}
	if u.TOTPEnabled {
		return "", "", nil, errors.New("two-factor authentication is already enabled")
	}
	secret, err = NewTOTPSecret()
	if err != nil {
		return
	}
	recovery, hashes, err := NewRecoveryCodes(10)
	if err != nil {
		return
	}
	m.mu.Lock()
	m.totpSetup[username] = &totpPending{Secret: secret, Recovery: recovery, RecoveryHash: hashes, Expires: m.Now().Add(10 * time.Minute)}
	m.mu.Unlock()
	otpauth = TOTPURI("MazimaOS", username, secret)
	return otpauth, secret, recovery, nil
}

func (m *Manager) ConfirmTOTP(username, code string) error {
	m.mu.Lock()
	p, ok := m.totpSetup[username]
	m.mu.Unlock()
	if !ok || m.Now().After(p.Expires) {
		return errors.New("two-factor setup expired; start again")
	}
	if !VerifyTOTP(p.Secret, code, m.Now()) {
		return ErrTwoFactorInvalid
	}
	enc, err := m.Sealer.Seal([]byte(p.Secret))
	if err != nil {
		return err
	}
	u, err := m.Store.User(username)
	if err != nil {
		return err
	}
	u.TOTPSecret = enc
	u.TOTPEnabled = true
	u.RecoveryHashes = p.RecoveryHash
	if err := m.Store.PutUser(u); err != nil {
		return err
	}
	m.mu.Lock()
	delete(m.totpSetup, username)
	m.mu.Unlock()
	return nil
}

func (m *Manager) DisableTOTP(username, password, code string) error {
	u, err := m.Store.User(username)
	if err != nil {
		return err
	}
	ok, err := VerifyPassword(u.PasswordHash, password)
	if err != nil || !ok {
		return ErrInvalidCredentials
	}
	if u.TOTPEnabled {
		secret, err := m.decryptTOTP(u)
		if err != nil {
			return err
		}
		if !VerifyTOTP(secret, code, m.Now()) {
			return ErrTwoFactorInvalid
		}
	}
	u.TOTPEnabled = false
	u.TOTPSecret = ""
	u.RecoveryHashes = nil
	return m.Store.PutUser(u)
}

func (m *Manager) decryptTOTP(u *store.User) (string, error) {
	if u.TOTPSecret == "" {
		return "", errors.New("missing TOTP secret")
	}
	b, err := m.Sealer.Open(u.TOTPSecret)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (m *Manager) VerifyUserPassword(username, password string) bool {
	u, err := m.Store.User(username)
	if err != nil {
		return false
	}
	ok, err := VerifyPassword(u.PasswordHash, password)
	return err == nil && ok
}

func (m *Manager) HasTOTP(username string) bool {
	u, err := m.Store.User(username)
	return err == nil && u.TOTPEnabled
}

func (m *Manager) VerifyTOTPForUser(username, code string) bool {
	u, err := m.Store.User(username)
	if err != nil || !u.TOTPEnabled {
		return false
	}
	secret, err := m.decryptTOTP(u)
	if err != nil {
		return false
	}
	return VerifyTOTP(secret, code, m.Now())
}

func (m *Manager) ConstantTimeEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
