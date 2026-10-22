package dockerstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type LocalizedText map[string]string

type ProxyConfig struct {
	Mode          string `json:"mode" yaml:"mode"`
	Hostname      string `json:"hostname,omitempty" yaml:"hostname,omitempty"`
	ContainerPort int    `json:"container_port" yaml:"container_port"`
	HealthPath    string `json:"health_path,omitempty" yaml:"health_path,omitempty"`
	Scheme        string `json:"scheme,omitempty" yaml:"scheme,omitempty"`
	WebSocket     bool   `json:"websocket,omitempty" yaml:"websocket,omitempty"`
	BasePath      string `json:"base_path,omitempty" yaml:"base_path,omitempty"`
}

type Manifest struct {
	ID              string            `json:"id" yaml:"id"`
	Name            string            `json:"name" yaml:"name"`
	NameI18N        LocalizedText     `json:"name_i18n,omitempty" yaml:"name_i18n,omitempty"`
	Description     string            `json:"description" yaml:"description"`
	DescriptionI18N LocalizedText     `json:"description_i18n,omitempty" yaml:"description_i18n,omitempty"`
	Version         string            `json:"version" yaml:"version"`
	Category        string            `json:"category" yaml:"category"`
	Icon            string            `json:"icon,omitempty" yaml:"icon,omitempty"`
	Website         string            `json:"website,omitempty" yaml:"website,omitempty"`
	License         string            `json:"license,omitempty" yaml:"license,omitempty"`
	Maintainer      string            `json:"maintainer,omitempty" yaml:"maintainer,omitempty"`
	MinRAMMB        int               `json:"min_ram_mb,omitempty" yaml:"min_ram_mb,omitempty"`
	GPU             bool              `json:"gpu,omitempty" yaml:"gpu,omitempty"`
	Port            int               `json:"port" yaml:"port"`
	Proxy           ProxyConfig       `json:"proxy" yaml:"proxy"`
	Env             map[string]string `json:"env,omitempty" yaml:"env,omitempty"`
	Compose         string            `json:"-" yaml:"-"`
	Notes           LocalizedText     `json:"notes,omitempty" yaml:"notes,omitempty"`
	Source          string            `json:"source,omitempty" yaml:"source,omitempty"`
	Installed       bool              `json:"installed,omitempty" yaml:"-"`
	InstalledAt     time.Time         `json:"installed_at,omitempty" yaml:"-"`
	HostPort        int               `json:"host_port,omitempty" yaml:"-"`
	Status          string            `json:"status,omitempty" yaml:"-"`
	URL             string            `json:"url,omitempty" yaml:"-"`
	DataSize        int64             `json:"data_size,omitempty" yaml:"-"`
}

type AppManager struct {
	Docker   *Client
	StoreDir string
	AppsDir  string
	Proxy    ProxyManager
	Now      func() time.Time
}

type ProxyManager interface {
	ApplyApp(app Manifest) error
	RemoveApp(id string) error
	ListRoutes() ([]string, error)
}

type appMeta struct {
	Manifest    Manifest          `json:"manifest"`
	ComposeName string            `json:"compose_name"`
	InstalledAt time.Time         `json:"installed_at"`
	HostPort    int               `json:"host_port"`
	ExtraEnv    map[string]string `json:"extra_env,omitempty"`
}

var appIDRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,47}$`)

func NewAppManager(dc *Client, storeDir, appsDir string) *AppManager {
	return &AppManager{Docker: dc, StoreDir: storeDir, AppsDir: appsDir, Now: time.Now}
}

func (m *AppManager) Catalog(ctx context.Context) ([]Manifest, error) {
	entries, err := os.ReadDir(filepath.Join(m.StoreDir, "apps"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []Manifest{}, nil
		}
		return nil, err
	}
	var out []Manifest
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		man, err := loadCatalogManifest(filepath.Join(m.StoreDir, "apps", e.Name()))
		if err != nil {
			continue
		}
		installed, _ := m.Installed(man.ID)
		if installed != nil {
			man.Installed = true
			man.InstalledAt = installed.InstalledAt
			man.HostPort = installed.HostPort
			man.URL = appURL(*man)
		}
		out = append(out, *man)
	}
	installedApps, _ := m.installedMetadata()
	seen := map[string]bool{}
	for _, a := range out {
		seen[a.ID] = true
	}
	for _, meta := range installedApps {
		if seen[meta.Manifest.ID] {
			continue
		}
		meta.Manifest.Installed = true
		meta.Manifest.InstalledAt = meta.InstalledAt
		meta.Manifest.HostPort = meta.HostPort
		meta.Manifest.URL = appURL(meta.Manifest)
		out = append(out, meta.Manifest)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Installed != out[j].Installed {
			return out[i].Installed
		}
		if out[i].Category != out[j].Category {
			return out[i].Category < out[j].Category
		}
		return out[i].Name < out[j].Name
	})
	if containers, err := m.Docker.ListContainers(ctx, true); err == nil {
		state := map[string]string{}
		for _, c := range containers {
			if c.AppID != "" {
				state[c.AppID] = c.State
			}
		}
		for i := range out {
			if s, ok := state[out[i].ID]; ok {
				out[i].Status = s
			}
		}
	}
	return out, nil
}

func (m *AppManager) CatalogApp(id string) (*Manifest, error) {
	if !appIDRE.MatchString(id) {
		return nil, errors.New("invalid app id")
	}
	man, err := loadCatalogManifest(filepath.Join(m.StoreDir, "apps", id))
	if err != nil {
		return nil, err
	}
	installed, _ := m.Installed(id)
	if installed != nil {
		man.Installed = true
		man.InstalledAt = installed.InstalledAt
		man.HostPort = installed.HostPort
		man.URL = appURL(*man)
	}
	return man, nil
}

func loadCatalogManifest(dir string) (*Manifest, error) {
	b, err := os.ReadFile(filepath.Join(dir, "manifest.yaml"))
	if err != nil {
		return nil, err
	}
	var man Manifest
	if err := yaml.Unmarshal(b, &man); err != nil {
		return nil, err
	}
	if err := man.Validate(); err != nil {
		return nil, err
	}
	compose, err := os.ReadFile(filepath.Join(dir, filepath.Base(man.ComposeFile())))
	if err != nil {
		return nil, err
	}
	man.Compose = string(compose)
	man.Source = dir
	return &man, nil
}

func (m Manifest) ComposeFile() string {
	if m.Source != "" {
		return "compose.yaml"
	}
	return "compose.yaml"
}

func (m *Manifest) Validate() error {
	if !appIDRE.MatchString(m.ID) {
		return errors.New("app id must be lowercase letters, digits and dashes")
	}
	if strings.TrimSpace(m.Name) == "" {
		return errors.New("app name is required")
	}
	if m.Port <= 0 || m.Port > 65535 {
		return errors.New("app port must be 1-65535")
	}
	if m.Proxy.Mode == "" {
		m.Proxy.Mode = "path"
	}
	switch m.Proxy.Mode {
	case "path", "alias":
	default:
		return errors.New("proxy mode must be path or alias")
	}
	if m.Proxy.ContainerPort == 0 {
		m.Proxy.ContainerPort = m.Port
	}
	if m.Proxy.Scheme == "" {
		m.Proxy.Scheme = "http"
	}
	if m.Proxy.Mode == "alias" && m.Proxy.Hostname == "" {
		m.Proxy.Hostname = m.ID + ".mazima.local"
	}
	if m.Proxy.HealthPath == "" {
		m.Proxy.HealthPath = "/"
	}
	if m.Proxy.Mode == "path" && m.Proxy.BasePath != "" && !strings.HasPrefix(m.Proxy.BasePath, "/") {
		return errors.New("proxy base_path must start with /")
	}
	return nil
}

func (m *AppManager) Install(ctx context.Context, id string) (*Manifest, error) {
	man, err := m.CatalogApp(id)
	if err != nil {
		return nil, err
	}
	if err := man.Validate(); err != nil {
		return nil, err
	}
	if _, err := os.Stat(filepath.Join(m.AppsDir, id)); err == nil {
		return nil, errors.New("app is already installed")
	}
	if err := ValidateCompose(man.Compose, man.GPU); err != nil {
		return nil, err
	}
	port, err := freeLoopbackPort()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(m.AppsDir, id)
	if err := os.MkdirAll(filepath.Join(dir, "data"), 0o750); err != nil {
		return nil, err
	}
	meta := appMeta{Manifest: *man, ComposeName: "compose.yaml", InstalledAt: m.now(), HostPort: port}
	meta.Manifest.Compose = ""
	meta.Manifest.Source = ""
	metaBytes, _ := json.MarshalIndent(meta, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), metaBytes, 0o640); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte(man.Compose), 0o640); err != nil {
		return nil, err
	}
	env := appEnv(dir, port, man.Env)
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(env), 0o600); err != nil {
		return nil, err
	}
	if m.Proxy != nil {
		if err := m.Proxy.ApplyApp(*man); err != nil {
			_ = os.RemoveAll(dir)
			return nil, fmt.Errorf("configure proxy: %w", err)
		}
	}
	out, err := m.Docker.ComposeUp(ctx, id, false)
	if err != nil {
		if m.Proxy != nil {
			_ = m.Proxy.RemoveApp(id)
		}
		return nil, fmt.Errorf("start app: %w: %s", err, out)
	}
	man.Installed = true
	man.InstalledAt = meta.InstalledAt
	man.HostPort = port
	man.URL = appURL(*man)
	return man, nil
}

func (m *AppManager) InstallRaw(ctx context.Context, man Manifest, compose string) (*Manifest, error) {
	if err := man.Validate(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(compose) == "" {
		return nil, errors.New("compose YAML is required")
	}
	if err := ValidateCompose(compose, man.GPU); err != nil {
		return nil, err
	}
	if _, err := os.Stat(filepath.Join(m.AppsDir, man.ID)); err == nil {
		return nil, errors.New("app is already installed")
	}
	port, err := freeLoopbackPort()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(m.AppsDir, man.ID)
	if err := os.MkdirAll(filepath.Join(dir, "data"), 0o750); err != nil {
		return nil, err
	}
	meta := appMeta{Manifest: man, ComposeName: "compose.yaml", InstalledAt: m.now(), HostPort: port}
	meta.Manifest.Compose = ""
	metaBytes, _ := json.MarshalIndent(meta, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), metaBytes, 0o640); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte(compose), 0o640); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(appEnv(dir, port, man.Env)), 0o600); err != nil {
		return nil, err
	}
	if m.Proxy != nil {
		if err := m.Proxy.ApplyApp(man); err != nil {
			_ = os.RemoveAll(dir)
			return nil, fmt.Errorf("configure proxy: %w", err)
		}
	}
	if out, err := m.Docker.ComposeUp(ctx, man.ID, false); err != nil {
		if m.Proxy != nil {
			_ = m.Proxy.RemoveApp(man.ID)
		}
		return nil, fmt.Errorf("start app: %w: %s", err, out)
	}
	man.Installed = true
	man.HostPort = port
	man.URL = appURL(man)
	return &man, nil
}

func (m *AppManager) Installed(id string) (*appMeta, error) {
	b, err := os.ReadFile(filepath.Join(m.AppsDir, id, "manifest.json"))
	if err != nil {
		return nil, err
	}
	var meta appMeta
	if err := json.Unmarshal(b, &meta); err != nil {
		return nil, err
	}
	envBytes, _ := os.ReadFile(filepath.Join(m.AppsDir, id, ".env"))
	meta.HostPort = parseEnvInt(string(envBytes), "MAZIMA_APP_PORT")
	return &meta, nil
}

func (m *AppManager) installedMetadata() ([]appMeta, error) {
	entries, err := os.ReadDir(m.AppsDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var out []appMeta
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		meta, err := m.Installed(e.Name())
		if err == nil {
			meta.Manifest.Installed = true
			meta.Manifest.InstalledAt = meta.InstalledAt
			meta.Manifest.HostPort = meta.HostPort
			meta.Manifest.URL = appURL(meta.Manifest)
			out = append(out, *meta)
		}
	}
	return out, nil
}

func (m *AppManager) Update(ctx context.Context, id string) (*Manifest, error) {
	meta, err := m.Installed(id)
	if err != nil {
		return nil, err
	}
	catalog, catErr := m.CatalogApp(id)
	if catErr == nil {
		if err := ValidateCompose(catalog.Compose, catalog.GPU); err != nil {
			return nil, err
		}
		meta.Manifest = *catalog
		meta.Manifest.Compose = ""
		metaBytes, _ := json.MarshalIndent(meta, "", "  ")
		if err := os.WriteFile(filepath.Join(m.AppsDir, id, "manifest.json"), metaBytes, 0o640); err != nil {
			return nil, err
		}
		if err := os.WriteFile(filepath.Join(m.AppsDir, id, "compose.yaml"), []byte(catalog.Compose), 0o640); err != nil {
			return nil, err
		}
	}
	out, err := m.Docker.ComposeUp(ctx, id, true)
	if err != nil {
		return nil, fmt.Errorf("update app: %w: %s", err, out)
	}
	meta.Manifest.Installed = true
	meta.Manifest.HostPort = meta.HostPort
	meta.Manifest.URL = appURL(meta.Manifest)
	if m.Proxy != nil {
		_ = m.Proxy.ApplyApp(meta.Manifest)
	}
	return &meta.Manifest, nil
}

func (m *AppManager) Remove(ctx context.Context, id string, purge bool) error {
	if !appIDRE.MatchString(id) {
		return errors.New("invalid app id")
	}
	meta, err := m.Installed(id)
	if err != nil {
		return err
	}
	_ = meta
	if out, err := m.Docker.ComposeDown(ctx, id, purge); err != nil {
		return fmt.Errorf("stop app: %w: %s", err, out)
	}
	if m.Proxy != nil {
		_ = m.Proxy.RemoveApp(id)
	}
	dir := filepath.Join(m.AppsDir, id)
	if purge {
		return os.RemoveAll(dir)
	}
	return os.WriteFile(filepath.Join(dir, ".removed"), []byte(m.now().UTC().Format(time.RFC3339)+"\n"), 0o640)
}

func (m *AppManager) Logs(ctx context.Context, id string, tail int) (string, error) {
	return m.Docker.ComposeLogs(ctx, id, tail)
}

func (m *AppManager) DataPath(id string) string {
	return filepath.Join(m.AppsDir, id, "data")
}

func (m *AppManager) now() time.Time {
	if m.Now != nil {
		return m.Now().UTC()
	}
	return time.Now().UTC()
}

func appURL(man Manifest) string {
	if man.Proxy.Mode == "alias" {
		return "https://" + man.Proxy.Hostname + "/"
	}
	return "/apps/" + man.ID + "/"
}

func appEnv(dir string, port int, extra map[string]string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "MAZIMA_APP_PORT=%d\n", port)
	fmt.Fprintf(&b, "MAZIMA_DATA_DIR=%s\n", filepath.Join(dir, "data"))
	fmt.Fprintf(&b, "MAZIMA_APP_ID=%s\n", filepath.Base(dir))
	fmt.Fprintf(&b, "PUID=%d\nPGID=%d\n", os.Getuid(), os.Getgid())
	for k, v := range extra {
		if validEnvKey(k) {
			fmt.Fprintf(&b, "%s=%s\n", k, escapeEnv(v))
		}
	}
	return b.String()
}

func validEnvKey(k string) bool {
	if k == "" {
		return false
	}
	for i, r := range k {
		if !(r == '_' || r >= 'A' && r <= 'Z' || (i > 0 && r >= '0' && r <= '9')) {
			return false
		}
	}
	return true
}

func escapeEnv(v string) string {
	return strings.ReplaceAll(strings.ReplaceAll(v, "\n", ""), "\r", "")
}

func parseEnvInt(env, key string) int {
	for _, line := range strings.Split(env, "\n") {
		if strings.HasPrefix(line, key+"=") {
			n, _ := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, key+"=")))
			return n
		}
	}
	return 0
}

func freeLoopbackPort() (int, error) {
	for i := 0; i < 50; i++ {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return 0, err
		}
		p := l.Addr().(*net.TCPAddr).Port
		_ = l.Close()
		if p >= 30000 && p <= 39999 {
			return p, nil
		}
	}
	return 0, errors.New("could not allocate an app port")
}

func ValidateCompose(compose string, allowGPU bool) error {
	var doc struct {
		Services map[string]map[string]any `yaml:"services"`
	}
	if err := yaml.Unmarshal([]byte(compose), &doc); err != nil {
		return fmt.Errorf("compose YAML: %w", err)
	}
	if len(doc.Services) == 0 {
		return errors.New("compose has no services")
	}
	for name, svc := range doc.Services {
		if strings.TrimSpace(name) == "" {
			return errors.New("compose service has an empty name")
		}
		if v, ok := svc["privileged"].(bool); ok && v {
			return fmt.Errorf("service %s requests privileged mode", name)
		}
		if _, ok := svc["build"]; ok {
			return fmt.Errorf("service %s uses a local build; only prebuilt images are allowed", name)
		}
		for _, key := range []string{"pid", "ipc", "uts", "cgroup"} {
			if v, ok := svc[key].(string); ok && v == "host" {
				return fmt.Errorf("service %s uses %s: host", name, key)
			}
		}
		if v, ok := svc["network_mode"].(string); ok && v == "host" {
			return fmt.Errorf("service %s uses host networking", name)
		}
		if caps, ok := svc["cap_add"].([]any); ok {
			for _, c := range caps {
				cs, _ := c.(string)
				switch strings.ToUpper(cs) {
				case "SYS_ADMIN", "SYS_PTRACE", "SYS_MODULE", "DAC_READ_SEARCH", "DAC_OVERRIDE", "ALL":
					return fmt.Errorf("service %s requests dangerous capability %s", name, cs)
				}
			}
		}
		if vols, ok := svc["volumes"].([]any); ok {
			for _, v := range vols {
				checkBind := func(src string) error {
					clean := filepath.Clean(src)
					if clean == "/var/run/docker.sock" || clean == "/run/docker.sock" {
						return fmt.Errorf("service %s mounts the Docker socket", name)
					}
					if allowGPU && (clean == "/dev/dri" || strings.HasPrefix(clean, "/dev/dri/")) {
						return nil
					}
					if strings.HasPrefix(clean, "/dev/") {
						return fmt.Errorf("service %s mounts host device %s", name, src)
					}
					if clean == "." || (!strings.HasPrefix(clean, "/") && !strings.HasPrefix(clean, "..")) {
						return nil
					}
					return fmt.Errorf("service %s mounts host path %s; use ${MAZIMA_DATA_DIR} or a relative path", name, src)
				}
				switch vv := v.(type) {
				case string:
					src := strings.SplitN(vv, ":", 2)[0]
					if strings.HasPrefix(src, "/") {
						if err := checkBind(src); err != nil {
							return err
						}
					}
				case map[string]any:
					if typ, _ := vv["type"].(string); typ == "bind" {
						src, _ := vv["source"].(string)
						if err := checkBind(src); err != nil {
							return err
						}
					}
				}
			}
		}
		if devs, ok := svc["devices"].([]any); ok && len(devs) > 0 {
			if !allowGPU {
				return fmt.Errorf("service %s requests host devices", name)
			}
			for _, d := range devs {
				ds, _ := d.(string)
				if !strings.Contains(ds, "/dev/dri") && !strings.Contains(ds, "/dev/nvidia") {
					return fmt.Errorf("service %s requests unsupported device %s", name, ds)
				}
			}
		}
		if ports, ok := svc["ports"].([]any); ok {
			for _, p := range ports {
				switch pv := p.(type) {
				case string:
					if pv == "" {
						continue
					}
					if !strings.HasPrefix(pv, "127.0.0.1:${MAZIMA_APP_PORT}:") || strings.Contains(pv, "/") {
						return fmt.Errorf("service %s port %q must bind to 127.0.0.1 and use ${MAZIMA_APP_PORT}", name, pv)
					}
				case map[string]any:
					hostIP, _ := pv["host_ip"].(string)
					published, _ := pv["published"].(string)
					if hostIP != "127.0.0.1" || !strings.Contains(published, "MAZIMA_APP_PORT") {
						return fmt.Errorf("service %s has a port mapping that is not loopback-only on ${MAZIMA_APP_PORT}", name)
					}
				default:
					return fmt.Errorf("service %s has an unsupported port mapping", name)
				}
			}
		}
	}
	return nil
}

func CopyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o640)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
