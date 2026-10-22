package proxy

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/danyx67800/mazimaos/daemon/internal/dockerstore"
)

func TestRegenerateSeparatesPathAndAliasSnippets(t *testing.T) {
	dir := t.TempDir()
	appsDir := filepath.Join(dir, "apps")
	writeInstalledApp(t, filepath.Join(appsDir, "files"), `{
		"manifest": {"id":"files","name":"Files","version":"1","port":80,"proxy":{"mode":"path","container_port":80}},
		"host_port": 41001
	}`, 41001)
	writeInstalledApp(t, filepath.Join(appsDir, "docs"), `{
		"manifest": {"id":"docs","name":"Docs","version":"1","port":80,"proxy":{"mode":"alias","hostname":"docs.example.test","container_port":80}},
		"host_port": 41002
	}`, 41002)

	snippet := filepath.Join(dir, "caddy", "apps.caddy")
	c := NewCaddy(appsDir, snippet, filepath.Join(dir, "Caddyfile"))
	c.Enabled = false
	if err := c.ApplyApp(dockerstore.Manifest{}); err != nil {
		t.Fatal(err)
	}
	routes, err := os.ReadFile(snippet)
	if err != nil {
		t.Fatal(err)
	}
	aliases, err := os.ReadFile(c.AliasSnippet)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(routes), "handle_path /apps/files/*") || !strings.Contains(string(routes), "127.0.0.1:41001") {
		t.Fatalf("path route missing:\n%s", routes)
	}
	if strings.Contains(string(routes), "docs.example.test") {
		t.Fatalf("alias leaked into path snippet:\n%s", routes)
	}
	if !strings.Contains(string(aliases), "https://docs.example.test") || !strings.Contains(string(aliases), "127.0.0.1:41002") {
		t.Fatalf("alias route missing:\n%s", aliases)
	}
}

func writeInstalledApp(t *testing.T, dir, manifest string, port int) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	env := "MAZIMA_APP_PORT=" + strconv.Itoa(port) + "\n"
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(env), 0o644); err != nil {
		t.Fatal(err)
	}
}
