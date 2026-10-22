package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

type bundleInfo struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

type manifest struct {
	Schema     int               `json:"schema"`
	Version    string            `json:"version"`
	Channel    string            `json:"channel"`
	MinVersion string            `json:"min_version,omitempty"`
	ReleasedAt time.Time         `json:"released_at"`
	Bundle     bundleInfo        `json:"bundle"`
	Notes      string            `json:"notes,omitempty"`
	NotesI18N  map[string]string `json:"notes_i18n,omitempty"`
	Migrations []string          `json:"migrations,omitempty"`
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "keygen":
		err = keygen(os.Args[2:])
	case "public":
		err = public(os.Args[2:])
	case "sign":
		err = sign(os.Args[2:])
	case "verify":
		err = verify(os.Args[2:])
	case "manifest":
		err = makeManifest(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "mazimaota:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `usage:
  mazimaota keygen -private signing/ota.key -public signing/ota.pub [-force]
  mazimaota public -private signing/ota.key
  mazimaota sign -key signing/ota.key -in manifest.json -out manifest.json.sig
  mazimaota verify -public signing/ota.pub -in manifest.json -sig manifest.json.sig
  mazimaota manifest -version 1.0.0 -bundle dist/mazima.tar.gz -notes notes.md -out manifest.json [-channel stable] [-min-version 0.1.0]
`)
}

func keygen(args []string) error {
	fs := flag.NewFlagSet("keygen", flag.ExitOnError)
	privatePath := fs.String("private", "signing/ota.key", "private key output path")
	publicPath := fs.String("public", "signing/ota.pub", "public key output path")
	force := fs.Bool("force", false, "overwrite existing keys")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if !*force {
		for _, path := range []string{*privatePath, *publicPath} {
			if _, err := os.Stat(path); err == nil {
				return fmt.Errorf("%s already exists (use -force to overwrite)", path)
			}
		}
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(*privatePath), 0o700); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(*publicPath), 0o755); err != nil {
		return err
	}
	seed := base64.StdEncoding.EncodeToString(priv.Seed()) + "\n"
	if err := os.WriteFile(*privatePath, []byte(seed), 0o600); err != nil {
		return err
	}
	if err := os.WriteFile(*publicPath, []byte(base64.StdEncoding.EncodeToString(pub)+"\n"), 0o644); err != nil {
		return err
	}
	fmt.Printf("private key: %s\npublic key:  %s\npublic key (embed with UPDATE_PUBLIC_KEY): %s\n",
		*privatePath, *publicPath, base64.StdEncoding.EncodeToString(pub))
	return nil
}

func public(args []string) error {
	fs := flag.NewFlagSet("public", flag.ExitOnError)
	privatePath := fs.String("private", "signing/ota.key", "private key path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	_, priv, err := loadPrivate(*privatePath)
	if err != nil {
		return err
	}
	fmt.Println(base64.StdEncoding.EncodeToString(priv.Public().(ed25519.PublicKey)))
	return nil
}

func sign(args []string) error {
	fs := flag.NewFlagSet("sign", flag.ExitOnError)
	keyPath := fs.String("key", "signing/ota.key", "private key path")
	inPath := fs.String("in", "manifest.json", "manifest to sign")
	outPath := fs.String("out", "manifest.json.sig", "signature output path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	_, priv, err := loadPrivate(*keyPath)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(*inPath)
	if err != nil {
		return err
	}
	sig := ed25519.Sign(priv, data)
	if err := os.WriteFile(*outPath, []byte(base64.StdEncoding.EncodeToString(sig)+"\n"), 0o644); err != nil {
		return err
	}
	fmt.Printf("signed %s -> %s\n", *inPath, *outPath)
	return nil
}

func verify(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ExitOnError)
	publicPath := fs.String("public", "signing/ota.pub", "public key path")
	inPath := fs.String("in", "manifest.json", "signed manifest")
	sigPath := fs.String("sig", "manifest.json.sig", "signature file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	raw, err := os.ReadFile(*publicPath)
	if err != nil {
		return err
	}
	pub, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil {
		return errors.New("public key is not base64")
	}
	if len(pub) != ed25519.PublicKeySize {
		return fmt.Errorf("public key must be %d bytes", ed25519.PublicKeySize)
	}
	data, err := os.ReadFile(*inPath)
	if err != nil {
		return err
	}
	rawSig, err := os.ReadFile(*sigPath)
	if err != nil {
		return err
	}
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(rawSig)))
	if err != nil {
		return errors.New("signature is not base64")
	}
	if !ed25519.Verify(ed25519.PublicKey(pub), data, sig) {
		return errors.New("signature verification failed")
	}
	fmt.Printf("verified %s with %s\n", *inPath, *publicPath)
	return nil
}

func makeManifest(args []string) error {
	fs := flag.NewFlagSet("manifest", flag.ExitOnError)
	version := fs.String("version", "", "release version (without v)")
	channel := fs.String("channel", "stable", "release channel")
	minVersion := fs.String("min-version", "", "minimum version that can install this release")
	bundlePath := fs.String("bundle", "", "bundle tarball")
	notesPath := fs.String("notes", "", "release notes file")
	outPath := fs.String("out", "manifest.json", "manifest output path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*version) == "" {
		return errors.New("-version is required")
	}
	if strings.TrimSpace(*bundlePath) == "" {
		return errors.New("-bundle is required")
	}
	if !validChannel(*channel) {
		return fmt.Errorf("invalid channel %q", *channel)
	}
	if !semver.IsValid("v" + strings.TrimPrefix(*version, "v")) {
		return fmt.Errorf("invalid version %q", *version)
	}
	if *minVersion != "" && !semver.IsValid("v"+strings.TrimPrefix(*minVersion, "v")) {
		return fmt.Errorf("invalid minimum version %q", *minVersion)
	}
	f, err := os.Open(*bundlePath)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	m := manifest{
		Schema: 1, Version: strings.TrimPrefix(*version, "v"), Channel: *channel,
		MinVersion: strings.TrimPrefix(*minVersion, "v"), ReleasedAt: time.Now().UTC(),
		Bundle: bundleInfo{Name: filepath.Base(*bundlePath), SHA256: hex.EncodeToString(h.Sum(nil)), Size: info.Size()},
	}
	if *notesPath != "" {
		n, err := os.ReadFile(*notesPath)
		if err != nil {
			return err
		}
		m.Notes = string(n)
	}
	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	out = append(out, '\n')
	if err := os.WriteFile(*outPath, out, 0o644); err != nil {
		return err
	}
	fmt.Printf("manifest %s -> %s (%d bytes)\n", m.Version, *outPath, info.Size())
	return nil
}

func validChannel(channel string) bool {
	switch channel {
	case "stable", "beta", "nightly":
		return true
	default:
		return false
	}
}

func loadPrivate(path string) ([]byte, ed25519.PrivateKey, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	seed, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil {
		return nil, nil, errors.New("private key is not base64")
	}
	if len(seed) != ed25519.SeedSize {
		return nil, nil, fmt.Errorf("private key seed must be %d bytes", ed25519.SeedSize)
	}
	return seed, ed25519.NewKeyFromSeed(seed), nil
}
