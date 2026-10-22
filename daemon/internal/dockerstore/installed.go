package dockerstore

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func LoadInstalledManifest(dir string) (*Manifest, error) {
	b, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return nil, err
	}
	var meta appMeta
	if err := json.Unmarshal(b, &meta); err != nil {
		return nil, err
	}
	if err := meta.Manifest.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", dir, err)
	}
	env, _ := os.ReadFile(filepath.Join(dir, ".env"))
	meta.Manifest.HostPort = parseEnvInt(string(env), "MAZIMA_APP_PORT")
	meta.Manifest.Installed = true
	meta.Manifest.InstalledAt = meta.InstalledAt
	meta.Manifest.URL = appURL(meta.Manifest)
	return &meta.Manifest, nil
}
