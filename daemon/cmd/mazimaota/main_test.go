package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManifestSignAndVerify(t *testing.T) {
	dir := t.TempDir()
	bundle := filepath.Join(dir, "mazima-1.2.3.tar.gz")
	if err := os.WriteFile(bundle, []byte("release bundle"), 0o644); err != nil {
		t.Fatal(err)
	}
	notes := filepath.Join(dir, "notes.md")
	if err := os.WriteFile(notes, []byte("release notes"), 0o644); err != nil {
		t.Fatal(err)
	}
	privateKey := filepath.Join(dir, "ota.key")
	publicKey := filepath.Join(dir, "ota.pub")
	if err := keygen([]string{"-private", privateKey, "-public", publicKey}); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(dir, "manifest.json")
	if err := makeManifest([]string{
		"-version", "1.2.3", "-bundle", bundle, "-notes", notes, "-out", manifestPath,
		"-channel", "stable", "-min-version", "0.1.0",
	}); err != nil {
		t.Fatal(err)
	}
	sigPath := filepath.Join(dir, "manifest.json.sig")
	if err := sign([]string{"-key", privateKey, "-in", manifestPath, "-out", sigPath}); err != nil {
		t.Fatal(err)
	}
	pubRaw, err := os.ReadFile(publicKey)
	if err != nil {
		t.Fatal(err)
	}
	pubBytes, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(pubRaw)))
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	sigRaw, err := os.ReadFile(sigPath)
	if err != nil {
		t.Fatal(err)
	}
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(sigRaw)))
	if err != nil {
		t.Fatal(err)
	}
	if !ed25519.Verify(ed25519.PublicKey(pubBytes), manifest, sig) {
		t.Fatal("signature does not verify")
	}
	if !strings.Contains(string(manifest), `"version": "1.2.3"`) {
		t.Fatalf("unexpected manifest: %s", manifest)
	}
}
