package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/danyx67800/mazimaos/daemon/internal/auth"
	"github.com/danyx67800/mazimaos/daemon/internal/config"
	"github.com/danyx67800/mazimaos/daemon/internal/store"
	"github.com/danyx67800/mazimaos/daemon/internal/version"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "mazimactl:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return usage()
	}
	switch args[0] {
	case "version", "--version", "-version":
		info := version.Get()
		fmt.Printf("%s %s (%s, %s, %s)\n", info.Product, info.Version, info.Commit, info.BuildDate, info.Platform)
		return nil
	case "selftest":
		return selftest(args[1:])
	case "setup-token":
		return setupToken(args[1:])
	case "help", "--help", "-h":
		return usage()
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func usage() error {
	fmt.Fprintln(os.Stderr, "usage: mazimactl <version|selftest|setup-token> [flags]")
	return nil
}

func loadConfig(args []string) (*config.Config, error) {
	fs := flag.NewFlagSet("config", flag.ContinueOnError)
	configPath := fs.String("config", os.Getenv("MAZIMA_CONFIG"), "path to config JSON")
	dataDir := fs.String("data-dir", "", "data directory")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	cfg := config.Default()
	var err error
	if *configPath != "" {
		cfg, err = config.Load(*configPath)
		if err != nil {
			return nil, err
		}
	}
	if *dataDir != "" {
		cfg.DataDir = *dataDir
	}
	if err := cfg.Finalize(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func selftest(args []string) error {
	cfg, err := loadConfig(args)
	if err != nil {
		return err
	}
	for _, path := range []string{cfg.StatePath(), cfg.MasterKeyPath(), cfg.AuditPath()} {
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("required file %s: %w", path, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("required file %s is not a regular file", path)
		}
	}
	st, err := store.Open(cfg.StatePath())
	if err != nil {
		return fmt.Errorf("open state: %w", err)
	}
	defer st.Close()
	if _, err := st.HasUsers(); err != nil {
		return fmt.Errorf("read state: %w", err)
	}
	key, err := auth.LoadOrCreateMasterKey(cfg.MasterKeyPath())
	if err != nil {
		return fmt.Errorf("master key: %w", err)
	}
	if _, err := auth.NewSealer(key); err != nil {
		return fmt.Errorf("sealer: %w", err)
	}
	out := map[string]any{"ok": true, "version": version.Get().Version, "data_dir": cfg.DataDir, "state": "readable", "master_key": "valid"}
	b, _ := json.Marshal(out)
	fmt.Println(string(b))
	return nil
}

func setupToken(args []string) error {
	cfg, err := loadConfig(args)
	if err != nil {
		return err
	}
	b, err := os.ReadFile(cfg.SetupTokenPath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return errors.New("no setup token: setup is already complete or the daemon has not started yet")
		}
		return err
	}
	token := strings.TrimSpace(string(b))
	if token == "" {
		return errors.New("setup token file is empty")
	}
	fmt.Println(token)
	return nil
}
