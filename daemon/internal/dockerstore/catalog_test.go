package dockerstore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBundledCatalogManifests(t *testing.T) {
	root := filepath.Join("..", "..", "..", "store", "apps")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read catalog: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("catalog is empty")
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		t.Run(entry.Name(), func(t *testing.T) {
			man, err := loadCatalogManifest(filepath.Join(root, entry.Name()))
			if err != nil {
				t.Fatalf("load manifest: %v", err)
			}
			if man.ID != entry.Name() {
				t.Fatalf("manifest id %q does not match directory %q", man.ID, entry.Name())
			}
			if err := ValidateCompose(man.Compose, man.GPU); err != nil {
				t.Fatalf("compose: %v", err)
			}
		})
	}
}
