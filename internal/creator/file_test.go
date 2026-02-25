package creator

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadEntriesWrapped(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "creators.yaml")
	content := `
creators:
  - uid: "1"
    name: "a"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	entries, err := LoadEntries(path)
	if err != nil {
		t.Fatalf("load error: %v", err)
	}
	if len(entries) != 1 || entries[0].UID != "1" {
		t.Fatalf("unexpected entries")
	}
}

func TestLoadEntriesList(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "creators.yaml")
	content := `
- uid: "2"
  name: "b"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	entries, err := LoadEntries(path)
	if err != nil {
		t.Fatalf("load error: %v", err)
	}
	if len(entries) != 1 || entries[0].UID != "2" {
		t.Fatalf("unexpected entries")
	}
}

func TestLoadEntriesInvalid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "creators.yaml")
	if err := os.WriteFile(path, []byte(":bad"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	if _, err := LoadEntries(path); err == nil {
		t.Fatalf("expected error")
	}
}
