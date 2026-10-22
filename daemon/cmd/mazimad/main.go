package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/danyx67800/mazimaos/daemon/internal/api"
	"github.com/danyx67800/mazimaos/daemon/internal/audit"
	"github.com/danyx67800/mazimaos/daemon/internal/auth"
	"github.com/danyx67800/mazimaos/daemon/internal/config"
	"github.com/danyx67800/mazimaos/daemon/internal/dockerstore"
	"github.com/danyx67800/mazimaos/daemon/internal/proxy"
	"github.com/danyx67800/mazimaos/daemon/internal/session"
	"github.com/danyx67800/mazimaos/daemon/internal/store"
	"github.com/danyx67800/mazimaos/daemon/internal/system"
	"github.com/danyx67800/mazimaos/daemon/internal/tailscale"
	"github.com/danyx67800/mazimaos/daemon/internal/updater"
	"github.com/danyx67800/mazimaos/daemon/internal/version"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "mazimad:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		configPath = flag.String("config", os.Getenv("MAZIMA_CONFIG"), "path to config JSON")
		dev        = flag.Bool("dev", false, "run with local development paths")
		listen     = flag.String("listen", "", "listen address")
		dataDir    = flag.String("data-dir", "", "data directory")
		uiDir      = flag.String("ui-dir", "", "UI directory")
		showVer    = flag.Bool("version", false, "print version and exit")
	)
	flag.Parse()
	if *showVer {
		info := version.Get()
		fmt.Printf("%s %s (%s, %s, %s)\n", info.Product, info.Version, info.Commit, info.BuildDate, info.Platform)
		return nil
	}

	cfg := config.Default()
	var err error
	if *configPath != "" {
		cfg, err = config.Load(*configPath)
		if err != nil {
			return err
		}
	}
	if *dev {
		cfg = devConfig()
	}
	if *listen != "" {
		cfg.Listen = *listen
	}
	if *dataDir != "" {
		cfg.DataDir = *dataDir
	}
	if *uiDir != "" {
		cfg.UIDir = *uiDir
	}
	if err := cfg.Finalize(); err != nil {
		return err
	}
	if *dev {
		cfg.AppsDir = filepath.Join(cfg.DataDir, "apps")
		cfg.InstallDir = filepath.Join(cfg.DataDir, "install")
		cfg.StoreDir = filepath.Join(cfg.DataDir, "store")
		cfg.CaddyConfigPath = filepath.Join(cfg.DataDir, "caddy", "apps.caddy")
	}
	if err := cfg.ApplyDataPaths(); err != nil {
		return fmt.Errorf("prepare data directory: %w", err)
	}

	logger := log.New(os.Stderr, "mazimad: ", log.LstdFlags|log.LUTC)
	st, err := store.Open(cfg.StatePath())
	if err != nil {
		return fmt.Errorf("open state: %w", err)
	}
	defer st.Close()
	key, err := auth.LoadOrCreateMasterKey(cfg.MasterKeyPath())
	if err != nil {
		return fmt.Errorf("master key: %w", err)
	}
	sealer, err := auth.NewSealer(key)
	if err != nil {
		return err
	}
	authManager := auth.NewManager(st, sealer, cfg.SessionTTL)
	auditLog := audit.New(cfg.AuditPath())
	collector := system.NewCollector()
	collector.Start(context.Background())

	setupToken, err := ensureSetupToken(&cfg, st)
	if err != nil {
		return fmt.Errorf("setup token: %w", err)
	}

	dockerClient, err := dockerstore.NewClient(cfg.DockerHost, cfg.AppsDir)
	if err != nil {
		return fmt.Errorf("docker client: %w", err)
	}
	caddy := proxy.NewCaddy(cfg.AppsDir, cfg.CaddyConfigPath, filepath.Join(filepath.Dir(cfg.CaddyConfigPath), "Caddyfile"))
	if *dev {
		if _, err := exec.LookPath("caddy"); err != nil {
			caddy = nil
		}
	}
	apps := dockerstore.NewAppManager(dockerClient, cfg.StoreDir, cfg.AppsDir)
	if caddy != nil {
		apps.Proxy = caddy
	}

	sessions := session.NewManager(cfg.DataDir, 3, cfg.StreamIdle, session.VideoMode{Width: 1280, Height: 800, FPS: 30})
	sessions.SetCommands(map[string]session.Command{
		"terminal": {Path: firstExisting("xterm", "uxterm", "xfce4-terminal"), Args: []string{"-ls", "-bg", "#0b1220", "-fg", "#e6edf7", "-fa", "Monospace", "-fs", "13", "-geometry", "118x34", "-title", "MazimaOS Terminal"}},
		"browser":  {Path: firstExisting("chromium", "chromium-browser", "google-chrome", "firefox"), Args: []string{"--user-data-dir=%SESSION_DIR%/browser", "--no-first-run", "--no-default-browser-check", "--start-maximized", "about:blank"}},
	})

	tsClient := tailscale.NewClient(cfg.TailscaleSocket)
	var up *updater.Updater
	if cfg.UpdateRepository != "" {
		token := os.Getenv("MAZIMA_GITHUB_TOKEN")
		up, err = updater.New(cfg.UpdateRepository, cfg.UpdateChannel, cfg.InstallDir, cfg.DataDir, token)
		if err != nil {
			return fmt.Errorf("updater: %w", err)
		}
		up.Restart = func(ctx context.Context) error {
			cmd := exec.CommandContext(ctx, "systemctl", "restart", "mazimad")
			return cmd.Run()
		}
		up.SelfTest = func(ctx context.Context) error {
			bin := filepath.Join(cfg.InstallDir, "current", "bin", "mazimactl")
			cmd := exec.CommandContext(ctx, bin, "selftest", "--data-dir", cfg.DataDir)
			out, err := cmd.CombinedOutput()
			if err != nil {
				return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
			}
			return nil
		}
	}

	srv := &api.Server{
		Cfg: &cfg, Store: st, Auth: authManager, Audit: auditLog, System: collector,
		Docker: dockerClient, Apps: apps, Proxy: caddy, Tailscale: tsClient,
		Sessions: sessions, Updater: up, Logger: logger, SetupToken: setupToken,
	}
	handler := srv.Handler()
	httpServer := &http.Server{
		Addr: cfg.Listen, Handler: handler,
		ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 120 * time.Second, MaxHeaderBytes: 1 << 20,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		if setupToken != "" {
			logger.Printf("first-run setup token written to %s (read it locally, never share it)", cfg.SetupTokenPath())
		}
		logger.Printf("%s %s listening on %s", version.Product, version.Version, cfg.Listen)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Printf("http server: %v", err)
			stop()
		}
	}()
	go maintenance(ctx, st, sessions, logger)
	if up != nil && cfg.AutoUpdate {
		go autoUpdate(ctx, up, cfg.UpdateInterval, logger)
	}

	<-ctx.Done()
	logger.Printf("shutting down")
	sessions.CloseAll()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(shutdownCtx)
	return nil
}

func devConfig() config.Config {
	cfg := config.Default()
	cfg.Dev = true
	cfg.InsecureCookies = true
	cfg.DataDir = "./.data"
	cfg.UIDir = "./ui"
	cfg.StoreDir = "./store"
	cfg.AppsDir = "./.data/apps"
	cfg.InstallDir = "./.data/install"
	cfg.CaddyConfigPath = "./.data/caddy/apps.caddy"
	cfg.UpdateRepository = os.Getenv("MAZIMA_UPDATE_REPOSITORY")
	cfg.UpdateChannel = "stable"
	if runtime.GOOS == "darwin" {
		cfg.TailscaleSocket = "/var/run/tailscaled.sock"
	}
	return cfg
}

func ensureSetupToken(cfg *config.Config, st *store.Store) (string, error) {
	hasUsers, err := st.HasUsers()
	if err != nil {
		return "", err
	}
	if hasUsers {
		_ = os.Remove(cfg.SetupTokenPath())
		return "", nil
	}
	if b, err := os.ReadFile(cfg.SetupTokenPath()); err == nil {
		token := strings.TrimSpace(string(b))
		if len(token) >= 24 {
			return token, nil
		}
	}
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	token := hex.EncodeToString(buf)
	if err := os.MkdirAll(filepath.Dir(cfg.SetupTokenPath()), 0o750); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(filepath.Dir(cfg.SetupTokenPath()), ".setup-*.token")
	if err != nil {
		return "", err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return "", err
	}
	if _, err := tmp.WriteString(token + "\n"); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(name, cfg.SetupTokenPath()); err != nil {
		return "", err
	}
	return token, nil
}

func maintenance(ctx context.Context, st *store.Store, sessions *session.Manager, logger *log.Logger) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			sessions.Cleanup()
			if n, err := st.PurgeExpiredSessions(now); err != nil {
				logger.Printf("purge sessions: %v", err)
			} else if n > 0 {
				logger.Printf("purged %d expired sessions", n)
			}
		}
	}
}

func autoUpdate(ctx context.Context, up *updater.Updater, interval time.Duration, logger *log.Logger) {
	if interval <= 0 {
		interval = 6 * time.Hour
	}
	timer := time.NewTimer(10 * time.Minute)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			checkCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
			av, err := up.Check(checkCtx)
			if err != nil {
				logger.Printf("auto update check: %v", err)
			} else if av != nil {
				logger.Printf("auto update: installing %s", av.Manifest.Version)
				installCtx, cancelInstall := context.WithTimeout(ctx, 20*time.Minute)
				if _, err := up.Install(installCtx, av); err != nil {
					logger.Printf("auto update failed: %v", err)
				}
				cancelInstall()
			}
			cancel()
			timer.Reset(interval + time.Duration(time.Now().UnixNano()%int64(time.Hour)))
		}
	}
}

func firstExisting(names ...string) string {
	for _, name := range names {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	return names[0]
}
