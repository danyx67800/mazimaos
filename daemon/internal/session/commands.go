package session

import (
	"os"
	"os/exec"
)

func commandAvailable(path string) bool {
	if path == "" {
		return false
	}
	if len(path) > 0 && (path[0] == '/' || path[0] == '.') {
		info, err := os.Stat(path)
		return err == nil && !info.IsDir() && info.Mode()&0o111 != 0
	}
	_, err := exec.LookPath(path)
	return err == nil
}

func firstCommand(names ...string) string {
	for _, name := range names {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	return names[0]
}

func defaultCommands() map[string]Command {
	return map[string]Command{
		"terminal": {
			Path: firstCommand("xterm", "uxterm", "xfce4-terminal"),
			Args: []string{"-ls", "-bg", "#0b1220", "-fg", "#e6edf7", "-fa", "Monospace", "-fs", "13", "-geometry", "118x34", "-title", "MazimaOS Terminal"},
		},
		"browser": {
			Path: firstCommand("chromium", "chromium-browser", "google-chrome", "firefox"),
			Args: []string{"--user-data-dir=%SESSION_DIR%/browser", "--no-first-run", "--no-default-browser-check", "--start-maximized", "about:blank"},
		},
	}
}
