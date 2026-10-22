package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Config struct {
	Listen           string        `json:"listen"`
	PublicURL        string        `json:"public_url"`
	DataDir          string        `json:"data_dir"`
	UIDir            string        `json:"ui_dir"`
	StoreDir         string        `json:"store_dir"`
	InstallDir       string        `json:"install_dir"`
	AppsDir          string        `json:"apps_dir"`
	DockerHost       string        `json:"docker_host"`
	TailscaleSocket  string        `json:"tailscale_socket"`
	CaddyConfigPath  string        `json:"caddy_config_path"`
	TrustedProxy     string        `json:"trusted_proxy"`
	InsecureCookies  bool          `json:"insecure_cookies"`
	Dev              bool          `json:"dev"`
	AutoUpdate       bool          `json:"auto_update"`
	UpdateRepository string        `json:"update_repository"`
	UpdateChannel    string        `json:"update_channel"`
	UpdateInterval   time.Duration `json:"-"`
	UpdateIntervalS  int           `json:"update_interval_seconds"`
	SessionTTL       time.Duration `json:"-"`
	SessionTTLS      int           `json:"session_ttl_seconds"`
	StreamIdle       time.Duration `json:"-"`
	StreamIdleS      int           `json:"stream_idle_seconds"`
	AllowedOrigins   []string      `json:"allowed_origins"`
}

func Default() Config {
	return Config{
		Listen:           "127.0.0.1:8088",
		PublicURL:        "https://mazima.local",
		DataDir:          "/var/lib/mazima",
		UIDir:            "/opt/mazima/current/ui",
		StoreDir:         "/opt/mazima/current/store",
		InstallDir:       "/opt/mazima",
		AppsDir:          "/data/apps",
		DockerHost:       "unix:///var/run/docker.sock",
		TailscaleSocket:  "/var/run/tailscale/tailscaled.sock",
		CaddyConfigPath:  "/etc/mazima/caddy/apps.caddy",
		TrustedProxy:     "127.0.0.1",
		UpdateRepository: "",
		UpdateChannel:    "stable",
		UpdateIntervalS:  6 * 60 * 60,
		SessionTTLS:      12 * 60 * 60,
		StreamIdleS:      30 * 60,
	}
}

func Load(path string) (Config, error) {
	cfg := Default()
	if path == "" {
		return cfg, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return cfg, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := cfg.Finalize(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func (c *Config) Finalize() error {
	if c.Listen == "" {
		c.Listen = "127.0.0.1:8088"
	}
	if c.DataDir == "" {
		return errors.New("data_dir must not be empty")
	}
	if c.PublicURL == "" {
		c.PublicURL = "https://mazima.local"
	}
	if c.UpdateChannel == "" {
		c.UpdateChannel = "stable"
	}
	switch c.UpdateChannel {
	case "stable", "beta", "nightly":
	default:
		return fmt.Errorf("unsupported update channel %q", c.UpdateChannel)
	}
	if c.UpdateIntervalS <= 0 {
		c.UpdateIntervalS = 6 * 60 * 60
	}
	if c.SessionTTLS <= 0 {
		c.SessionTTLS = 12 * 60 * 60
	}
	if c.StreamIdleS <= 0 {
		c.StreamIdleS = 30 * 60
	}
	c.UpdateInterval = time.Duration(c.UpdateIntervalS) * time.Second
	c.SessionTTL = time.Duration(c.SessionTTLS) * time.Second
	c.StreamIdle = time.Duration(c.StreamIdleS) * time.Second
	if !isLoopbackListen(c.Listen) && !c.Dev {
		return fmt.Errorf("listen %q is not loopback; put mazimad behind Caddy", c.Listen)
	}
	return nil
}

func isLoopbackListen(addr string) bool {
	host := addr
	if i := strings.LastIndex(addr, ":"); i >= 0 {
		host = addr[:i]
	}
	host = strings.Trim(host, "[]")
	return host == "127.0.0.1" || host == "localhost" || host == "::1"
}

func (c *Config) ApplyDataPaths() error {
	if err := os.MkdirAll(c.DataDir, 0o750); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(c.DataDir, "sessions"), 0o750); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(c.DataDir, "releases"), 0o750); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(c.DataDir, "backups"), 0o750); err != nil {
		return err
	}
	if err := os.MkdirAll(c.AppsDir, 0o750); err != nil && !c.Dev {
		return err
	}
	return nil
}

func (c *Config) StatePath() string {
	return filepath.Join(c.DataDir, "state.json")
}

func (c *Config) AuditPath() string {
	return filepath.Join(c.DataDir, "audit.jsonl")
}

func (c *Config) MasterKeyPath() string {
	return filepath.Join(c.DataDir, "master.key")
}

func (c *Config) SetupTokenPath() string {
	return filepath.Join(c.DataDir, "setup.token")
}

func (c *Config) UpdateCachePath() string {
	return filepath.Join(c.DataDir, "updates")
}

func (c *Config) BackupDir() string {
	return filepath.Join(c.DataDir, "backups")
}
