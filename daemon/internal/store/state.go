package store

import "time"

const SchemaVersion = 3

type Role string

const (
	RoleAdmin Role = "admin"
	RoleUser  Role = "user"
	RoleGuest Role = "guest"
)

func ValidRole(r Role) bool {
	return r == RoleAdmin || r == RoleUser || r == RoleGuest
}

type Passkey struct {
	ID           string    `json:"id"`
	PublicKey    []byte    `json:"public_key"`
	SignCount    uint32    `json:"sign_count"`
	Name         string    `json:"name"`
	CreatedAt    time.Time `json:"created_at"`
	LastUsedAt   time.Time `json:"last_used_at,omitempty"`
	BackupElig   bool      `json:"backup_eligible"`
	BackupState  bool      `json:"backup_state"`
	Transports   []string  `json:"transports,omitempty"`
	Attestation  []byte    `json:"attestation,omitempty"`
	AAGUID       []byte    `json:"aaguid,omitempty"`
	CloneWarning bool      `json:"clone_warning,omitempty"`
}

type User struct {
	Username       string    `json:"username"`
	DisplayName    string    `json:"display_name"`
	Role           Role      `json:"role"`
	PasswordHash   string    `json:"password_hash"`
	TOTPSecret     string    `json:"totp_secret,omitempty"`
	TOTPEnabled    bool      `json:"totp_enabled"`
	RecoveryHashes []string  `json:"recovery_hashes,omitempty"`
	Passkeys       []Passkey `json:"passkeys,omitempty"`
	Disabled       bool      `json:"disabled"`
	MustChangePass bool      `json:"must_change_password,omitempty"`
	Language       string    `json:"language,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	LastLoginAt    time.Time `json:"last_login_at,omitempty"`
}

type Session struct {
	Username  string    `json:"username"`
	CSRF      string    `json:"csrf"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
	LastSeen  time.Time `json:"last_seen"`
	IP        string    `json:"ip,omitempty"`
	UserAgent string    `json:"user_agent,omitempty"`
}

type APIKey struct {
	ID         string    `json:"id"`
	Username   string    `json:"username"`
	Name       string    `json:"name"`
	Hash       string    `json:"hash"`
	CreatedAt  time.Time `json:"created_at"`
	LastUsedAt time.Time `json:"last_used_at,omitempty"`
	ExpiresAt  time.Time `json:"expires_at,omitempty"`
}

type DesktopWindow struct {
	ID        string  `json:"id"`
	AppID     string  `json:"app_id"`
	Title     string  `json:"title"`
	X         float64 `json:"x"`
	Y         float64 `json:"y"`
	Width     float64 `json:"width"`
	Height    float64 `json:"height"`
	Minimized bool    `json:"minimized"`
	Maximized bool    `json:"maximized"`
	Desktop   int     `json:"desktop"`
	Snap      string  `json:"snap,omitempty"`
	Z         int     `json:"z"`
	SessionID string  `json:"session_id,omitempty"`
	Path      string  `json:"path,omitempty"`
}

type DesktopState struct {
	Windows         []DesktopWindow `json:"windows"`
	VirtualDesktops int             `json:"virtual_desktops"`
	ActiveDesktop   int             `json:"active_desktop"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

type Settings struct {
	Hostname       string `json:"hostname"`
	Timezone       string `json:"timezone"`
	Language       string `json:"language"`
	Layout         string `json:"layout"`
	Theme          string `json:"theme"`
	Accent         string `json:"accent"`
	Transparency   int    `json:"transparency"`
	Wallpaper      string `json:"wallpaper"`
	UIScale        int    `json:"ui_scale"`
	ReduceMotion   bool   `json:"reduce_motion"`
	TOTPRequired   bool   `json:"totp_required"`
	SSOEnabled     bool   `json:"sso_enabled"`
	FunnelEnabled  bool   `json:"funnel_enabled"`
	UpdateChannel  string `json:"update_channel"`
	AutoUpdate     bool   `json:"auto_update"`
	SkippedVersion string `json:"skipped_version,omitempty"`
	RemoteAccess   bool   `json:"remote_access"`
}

func DefaultSettings() Settings {
	return Settings{
		Hostname:      "mazima",
		Timezone:      "Europe/Rome",
		Language:      "it",
		Layout:        "windows",
		Theme:         "auto",
		Accent:        "#4f8cff",
		Transparency:  68,
		Wallpaper:     "aurora",
		UIScale:       100,
		TOTPRequired:  false,
		UpdateChannel: "stable",
		AutoUpdate:    true,
	}
}

type State struct {
	SchemaVersion int                     `json:"schema_version"`
	SetupComplete bool                    `json:"setup_complete"`
	Users         map[string]*User        `json:"users"`
	Sessions      map[string]*Session     `json:"sessions"`
	APIKeys       map[string]*APIKey      `json:"api_keys"`
	Desktops      map[string]DesktopState `json:"desktops"`
	Settings      Settings                `json:"settings"`
	CreatedAt     time.Time               `json:"created_at"`
	UpdatedAt     time.Time               `json:"updated_at"`
}

func NewState() State {
	now := time.Now().UTC()
	return State{
		SchemaVersion: SchemaVersion,
		Users:         map[string]*User{},
		Sessions:      map[string]*Session{},
		APIKeys:       map[string]*APIKey{},
		Desktops:      map[string]DesktopState{},
		Settings:      DefaultSettings(),
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}
