package updater

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/danyx67800/mazimaos/daemon/internal/version"
	"golang.org/x/mod/semver"
)

const ManifestAsset = "mazima-manifest.json"
const SignatureAsset = "mazima-manifest.json.sig"

var EmbeddedPublicKey = ""

type Bundle struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

type Manifest struct {
	Schema     int               `json:"schema"`
	Version    string            `json:"version"`
	Channel    string            `json:"channel"`
	MinVersion string            `json:"min_version"`
	ReleasedAt time.Time         `json:"released_at"`
	Bundle     Bundle            `json:"bundle"`
	Notes      string            `json:"notes"`
	NotesI18N  map[string]string `json:"notes_i18n,omitempty"`
	Migrations []string          `json:"migrations,omitempty"`
}

type ReleaseAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
}

type Release struct {
	Tag        string         `json:"tag_name"`
	Name       string         `json:"name"`
	Draft      bool           `json:"draft"`
	Prerelease bool           `json:"prerelease"`
	Published  time.Time      `json:"published_at"`
	Body       string         `json:"body"`
	Assets     []ReleaseAsset `json:"assets"`
}

type Available struct {
	Manifest    Manifest `json:"manifest"`
	AssetURL    string   `json:"asset_url"`
	Size        int64    `json:"size"`
	Notes       string   `json:"notes,omitempty"`
	ManifestURL string   `json:"manifest_url,omitempty"`
}

type State struct {
	Current       string    `json:"current"`
	Previous      string    `json:"previous"`
	LastChecked   time.Time `json:"last_checked"`
	LastError     string    `json:"last_error,omitempty"`
	LastResult    string    `json:"last_result,omitempty"`
	UpdateLog     []string  `json:"update_log,omitempty"`
	HealthPending string    `json:"health_pending,omitempty"`
	BootAttempts  int       `json:"boot_attempts,omitempty"`
}

type Updater struct {
	mu           sync.Mutex
	Repository   string
	Channel      string
	InstallDir   string
	DataDir      string
	CurrentLink  string
	PreviousLink string
	HTTP         *http.Client
	Token        string
	Now          func() time.Time
	SelfTest     func(ctx context.Context) error
	Restart      func(ctx context.Context) error
	PublicKey    ed25519.PublicKey

	checkCache map[string]string
}

func New(repo, channel, installDir, dataDir, token string) (*Updater, error) {
	if repo == "" {
		return nil, errors.New("update repository is not configured")
	}
	if channel == "" {
		channel = "stable"
	}
	u := &Updater{
		Repository: repo, Channel: channel, InstallDir: installDir, DataDir: dataDir,
		CurrentLink:  filepath.Join(installDir, "current"),
		PreviousLink: filepath.Join(installDir, "previous"),
		HTTP:         &http.Client{Timeout: 5 * time.Minute},
		Token:        token, Now: time.Now, checkCache: map[string]string{},
	}
	if EmbeddedPublicKey != "" {
		pk, err := base64.StdEncoding.DecodeString(EmbeddedPublicKey)
		if err != nil || len(pk) != ed25519.PublicKeySize {
			return nil, errors.New("invalid embedded update public key")
		}
		u.PublicKey = ed25519.PublicKey(pk)
	}
	return u, nil
}

func (u *Updater) SetPublicKeyBase64(s string) error {
	pk, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s))
	if err != nil || len(pk) != ed25519.PublicKeySize {
		return errors.New("invalid public key")
	}
	u.PublicKey = ed25519.PublicKey(pk)
	return nil
}

func (u *Updater) Check(ctx context.Context) (*Available, error) {
	if u.Repository == "" {
		return nil, errors.New("no update repository configured")
	}
	if u.PublicKey == nil {
		return nil, errors.New("update signing public key is not embedded")
	}
	releases, etag, err := u.fetchReleases(ctx)
	if err != nil {
		return nil, err
	}
	rel := pickRelease(releases, u.Channel)
	if rel == nil {
		return nil, nil
	}
	manAsset, sigAsset := findAssets(rel.Assets)
	if manAsset == nil || sigAsset == nil {
		return nil, fmt.Errorf("release %s is missing %s or its signature", rel.Tag, ManifestAsset)
	}
	manBytes, err := u.download(ctx, manAsset.URL, 2<<20)
	if err != nil {
		return nil, err
	}
	sigBytes, err := u.download(ctx, sigAsset.URL, 64*1024)
	if err != nil {
		return nil, err
	}
	if err := verifySignature(u.PublicKey, manBytes, sigBytes); err != nil {
		return nil, fmt.Errorf("update manifest signature: %w", err)
	}
	var man Manifest
	if err := json.Unmarshal(manBytes, &man); err != nil {
		return nil, fmt.Errorf("update manifest: %w", err)
	}
	if err := validateManifest(man, u.Channel); err != nil {
		return nil, err
	}
	if semver.Compare("v"+man.Version, "v"+version.Version) <= 0 {
		return nil, nil
	}
	if man.MinVersion != "" && semver.Compare("v"+version.Version, "v"+man.MinVersion) < 0 {
		return nil, fmt.Errorf("this update requires MazimaOS %s or newer (current %s)", man.MinVersion, version.Version)
	}
	var assetURL string
	for _, a := range rel.Assets {
		if a.Name == man.Bundle.Name {
			assetURL = a.URL
		}
	}
	if assetURL == "" {
		return nil, fmt.Errorf("release %s does not contain bundle %s", rel.Tag, man.Bundle.Name)
	}
	if err := u.saveCache(etag, manBytes, sigBytes, man); err != nil {
		return nil, err
	}
	return &Available{Manifest: man, AssetURL: assetURL, Size: man.Bundle.Size, Notes: rel.Body, ManifestURL: manAsset.URL}, nil
}

func (u *Updater) fetchReleases(ctx context.Context) ([]Release, string, error) {
	u.mu.Lock()
	cachedETag := u.checkCache["etag-"+u.Channel]
	u.mu.Unlock()
	endpoint := fmt.Sprintf("https://api.github.com/repos/%s/releases?per_page=20", u.Repository)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if u.Token != "" {
		req.Header.Set("Authorization", "Bearer "+u.Token)
	}
	if cachedETag != "" {
		req.Header.Set("If-None-Match", cachedETag)
	}
	resp, err := u.HTTP.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotModified {
		u.mu.Lock()
		body := u.checkCache["body-"+u.Channel]
		u.mu.Unlock()
		if body == "" {
			return nil, "", errors.New("GitHub returned 304 without a cached release list")
		}
		var out []Release
		return out, cachedETag, json.Unmarshal([]byte(body), &out)
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, "", fmt.Errorf("GitHub releases: %s: %s", resp.Status, strings.TrimSpace(string(b)))
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, "", err
	}
	var releases []Release
	if err := json.Unmarshal(body, &releases); err != nil {
		return nil, "", err
	}
	etag := resp.Header.Get("ETag")
	u.mu.Lock()
	u.checkCache["etag-"+u.Channel] = etag
	u.checkCache["body-"+u.Channel] = string(body)
	u.mu.Unlock()
	if err := u.persistETag(etag); err != nil {
		return nil, "", err
	}
	return releases, etag, nil
}

func pickRelease(releases []Release, channel string) *Release {
	var eligible []Release
	for _, r := range releases {
		if r.Draft {
			continue
		}
		switch channel {
		case "stable":
			if r.Prerelease {
				continue
			}
		case "beta":
			// Beta accepts pre-releases and stable releases, not nightlies without semver.
		case "nightly":
		default:
			continue
		}
		if !semver.IsValid("v" + strings.TrimPrefix(r.Tag, "v")) {
			continue
		}
		eligible = append(eligible, r)
	}
	sort.Slice(eligible, func(i, j int) bool {
		return semver.Compare("v"+strings.TrimPrefix(eligible[i].Tag, "v"), "v"+strings.TrimPrefix(eligible[j].Tag, "v")) > 0
	})
	if len(eligible) == 0 {
		return nil
	}
	return &eligible[0]
}

func findAssets(assets []ReleaseAsset) (*ReleaseAsset, *ReleaseAsset) {
	var man, sig *ReleaseAsset
	for i := range assets {
		switch assets[i].Name {
		case ManifestAsset:
			man = &assets[i]
		case SignatureAsset:
			sig = &assets[i]
		}
	}
	return man, sig
}

func validateManifest(m Manifest, channel string) error {
	if m.Schema != 1 {
		return fmt.Errorf("unsupported update manifest schema %d", m.Schema)
	}
	if !semver.IsValid("v" + m.Version) {
		return fmt.Errorf("invalid update version %q", m.Version)
	}
	if m.Channel != channel && channel != "nightly" {
		return fmt.Errorf("update channel %q does not match %q", m.Channel, channel)
	}
	if m.Bundle.Name == "" || strings.ContainsAny(m.Bundle.Name, `/\`) || strings.HasPrefix(m.Bundle.Name, ".") {
		return errors.New("invalid update bundle name")
	}
	if len(m.Bundle.SHA256) != 64 {
		return errors.New("invalid update bundle sha256")
	}
	if m.Bundle.Size <= 0 || m.Bundle.Size > 8<<30 {
		return errors.New("invalid update bundle size")
	}
	for _, mig := range m.Migrations {
		if filepath.IsAbs(mig) || strings.Contains(mig, "..") || strings.Contains(mig, `\`) {
			return fmt.Errorf("invalid migration path %q", mig)
		}
	}
	return nil
}

func verifySignature(pk ed25519.PublicKey, manifest, sig []byte) error {
	s := strings.TrimSpace(string(sig))
	raw, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		raw, err = base64.RawStdEncoding.DecodeString(s)
	}
	if err != nil {
		return errors.New("signature is not base64")
	}
	if len(raw) != ed25519.SignatureSize {
		return errors.New("signature has invalid length")
	}
	if !ed25519.Verify(pk, manifest, raw) {
		return errors.New("signature verification failed")
	}
	return nil
}

func (u *Updater) download(ctx context.Context, rawurl string, max int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawurl, nil)
	if err != nil {
		return nil, err
	}
	if u.Token != "" && strings.HasPrefix(rawurl, "https://api.github.com/") {
		req.Header.Set("Authorization", "Bearer "+u.Token)
	}
	req.Header.Set("Accept", "application/octet-stream")
	resp, err := u.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download: %s", resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, max+1))
}

func (u *Updater) saveCache(etag string, manifest, sig []byte, man Manifest) error {
	dir := filepath.Join(u.DataDir, "updates")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "etag"), []byte(etag+"\n"), 0o640); err != nil {
		return err
	}
	_ = os.WriteFile(filepath.Join(dir, "manifest-"+man.Version+".json"), manifest, 0o640)
	_ = os.WriteFile(filepath.Join(dir, "manifest-"+man.Version+".sig"), sig, 0o640)
	return nil
}

func (u *Updater) persistETag(etag string) error {
	dir := filepath.Join(u.DataDir, "updates")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "etag"), []byte(etag+"\n"), 0o640)
}

func (u *Updater) Install(ctx context.Context, av *Available) (*State, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if av == nil {
		return nil, errors.New("no update selected")
	}
	if err := validateManifest(av.Manifest, u.Channel); err != nil {
		return nil, err
	}
	if semver.Compare("v"+av.Manifest.Version, "v"+version.Version) <= 0 {
		return nil, errors.New("update is not newer than the running version")
	}
	if err := os.MkdirAll(u.InstallDir, 0o755); err != nil {
		return nil, err
	}
	lock, err := os.OpenFile(filepath.Join(u.DataDir, "update.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return nil, errors.New("another update is already in progress")
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)

	logf := func(format string, args ...any) {
		line := fmt.Sprintf("%s %s", u.Now().UTC().Format(time.RFC3339), fmt.Sprintf(format, args...))
		u.appendLogLocked(line)
	}
	logf("starting update to %s", av.Manifest.Version)
	releaseDir := filepath.Join(u.InstallDir, "releases", av.Manifest.Version)
	if err := os.RemoveAll(releaseDir); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(releaseDir, 0o755); err != nil {
		return nil, err
	}
	bundlePath, err := u.downloadBundle(ctx, av)
	if err != nil {
		logf("download failed: %v", err)
		return nil, err
	}
	defer os.Remove(bundlePath)
	if err := extractTarGz(bundlePath, releaseDir); err != nil {
		logf("extract failed: %v", err)
		return nil, err
	}
	if err := runMigrations(ctx, releaseDir, version.Version, av.Manifest); err != nil {
		logf("migration failed: %v", err)
		return nil, err
	}
	old, _ := os.Readlink(u.CurrentLink)
	if old != "" {
		_ = replaceSymlink(u.PreviousLink, old)
	}
	if err := replaceSymlink(u.CurrentLink, releaseDir); err != nil {
		return nil, err
	}
	logf("switched current release to %s", av.Manifest.Version)
	if u.SelfTest != nil {
		testCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
		err := u.SelfTest(testCtx)
		cancel()
		if err != nil {
			logf("post-update self-test failed: %v; rolling back", err)
			if old != "" {
				_ = replaceSymlink(u.CurrentLink, old)
			}
			_ = os.RemoveAll(releaseDir)
			return nil, fmt.Errorf("post-update self-test failed, rolled back: %w", err)
		}
	}
	state := &State{Current: av.Manifest.Version, Previous: version.Version, LastChecked: u.Now().UTC(), LastResult: "success"}
	state.UpdateLog = append(state.UpdateLog, "updated to "+av.Manifest.Version)
	if err := u.writeState(state); err != nil {
		return nil, err
	}
	logf("update applied successfully")
	return state, nil
}

func (u *Updater) downloadBundle(ctx context.Context, av *Available) (string, error) {
	dir := filepath.Join(u.DataDir, "updates")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(dir, ".bundle-*.tar.gz")
	if err != nil {
		return "", err
	}
	name := f.Name()
	defer f.Close()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, av.AssetURL, nil)
	if err != nil {
		return "", err
	}
	if u.Token != "" && strings.HasPrefix(av.AssetURL, "https://api.github.com/") {
		req.Header.Set("Authorization", "Bearer "+u.Token)
	}
	req.Header.Set("Accept", "application/octet-stream")
	resp, err := u.HTTP.Do(req)
	if err != nil {
		os.Remove(name)
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		os.Remove(name)
		return "", fmt.Errorf("bundle download: %s", resp.Status)
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), io.LimitReader(resp.Body, av.Manifest.Bundle.Size+1))
	if err != nil {
		os.Remove(name)
		return "", err
	}
	if n != av.Manifest.Bundle.Size {
		os.Remove(name)
		return "", fmt.Errorf("bundle size %d does not match manifest %d", n, av.Manifest.Bundle.Size)
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != strings.ToLower(av.Manifest.Bundle.SHA256) {
		os.Remove(name)
		return "", errors.New("bundle sha256 mismatch")
	}
	return name, nil
}

func extractTarGz(path, dest string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		name := filepath.Clean(hdr.Name)
		if name == "." || strings.HasPrefix(name, "..") || filepath.IsAbs(name) {
			return fmt.Errorf("unsafe archive path %q", hdr.Name)
		}
		target := filepath.Join(dest, name)
		if !strings.HasPrefix(target+string(os.PathSeparator), filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("unsafe archive path %q", hdr.Name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			mode := os.FileMode(hdr.Mode & 0o777)
			if mode == 0 {
				mode = 0o644
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, io.LimitReader(tr, 512<<20)); err != nil {
				out.Close()
				return err
			}
			out.Close()
		default:
			return fmt.Errorf("unsupported archive entry %q", hdr.Name)
		}
	}
	return nil
}

func runMigrations(ctx context.Context, releaseDir, from string, man Manifest) error {
	for _, rel := range man.Migrations {
		clean := filepath.Clean(rel)
		if strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
			return fmt.Errorf("invalid migration %q", rel)
		}
		path := filepath.Join(releaseDir, clean)
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("migration %s missing", rel)
		}
		cmd := exec.CommandContext(ctx, "/bin/sh", path)
		cmd.Dir = releaseDir
		cmd.Env = append(os.Environ(), "MAZIMA_FROM_VERSION="+from, "MAZIMA_TO_VERSION="+man.Version)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("migration %s: %w: %s", rel, err, strings.TrimSpace(string(out)))
		}
	}
	return nil
}

func replaceSymlink(link, target string) error {
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		return err
	}
	tmp := link + ".tmp"
	_ = os.Remove(tmp)
	if err := os.Symlink(target, tmp); err != nil {
		return err
	}
	if err := os.Rename(tmp, link); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func (u *Updater) Rollback() error {
	u.mu.Lock()
	defer u.mu.Unlock()
	prev, err := os.Readlink(u.PreviousLink)
	if err != nil {
		return errors.New("no previous release available")
	}
	cur, _ := os.Readlink(u.CurrentLink)
	if err := replaceSymlink(u.CurrentLink, prev); err != nil {
		return err
	}
	if cur != "" {
		_ = replaceSymlink(u.PreviousLink, cur)
	}
	state := &State{Current: filepath.Base(prev), Previous: filepath.Base(cur), LastChecked: u.Now().UTC(), LastResult: "rollback"}
	state.UpdateLog = append(state.UpdateLog, "rolled back to "+filepath.Base(prev))
	_ = u.writeState(state)
	if u.Restart != nil {
		go func() { _ = u.Restart(context.Background()) }()
	}
	return nil
}

func (u *Updater) State() (State, error) {
	b, err := os.ReadFile(filepath.Join(u.DataDir, "update-state.json"))
	if errors.Is(err, os.ErrNotExist) {
		return State{Current: version.Version}, nil
	}
	if err != nil {
		return State{}, err
	}
	var st State
	if err := json.Unmarshal(b, &st); err != nil {
		return State{}, err
	}
	if st.Current == "" {
		st.Current = version.Version
	}
	return st, nil
}

func (u *Updater) writeState(st *State) error {
	if err := os.MkdirAll(u.DataDir, 0o750); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(st, "", "  ")
	tmp, err := os.CreateTemp(u.DataDir, ".update-state-*.json")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(0o640); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, filepath.Join(u.DataDir, "update-state.json"))
}

func (u *Updater) appendLogLocked(line string) {
	st, _ := u.State()
	st.UpdateLog = append(st.UpdateLog, line)
	if len(st.UpdateLog) > 200 {
		st.UpdateLog = st.UpdateLog[len(st.UpdateLog)-200:]
	}
	_ = u.writeState(&st)
}

func (u *Updater) ClearETag() {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.checkCache = map[string]string{}
	_ = os.Remove(filepath.Join(u.DataDir, "updates", "etag"))
}

func (u *Updater) ValidateDownloadURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if parsed.Scheme != "https" {
		return errors.New("update downloads must use HTTPS")
	}
	return nil
}
