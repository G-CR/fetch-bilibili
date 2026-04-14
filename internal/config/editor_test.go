package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEditorLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	body := []byte("storage:\n  root_dir: /data/test\nmysql:\n  dsn: test\n")
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	editor := NewEditor(path, nil)
	doc, err := editor.Load(context.Background())
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if doc.Path != path {
		t.Fatalf("unexpected path: %s", doc.Path)
	}
	if doc.Content != string(body) {
		t.Fatalf("unexpected content: %q", doc.Content)
	}
}

func TestEditorSaveNoChange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	body := []byte("storage:\n  root_dir: /data/test\nmysql:\n  dsn: test\n")
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	restarted := false
	editor := NewEditor(path, func() {
		restarted = true
	})

	result, err := editor.Save(context.Background(), string(body))
	if err != nil {
		t.Fatalf("Save error: %v", err)
	}
	if result.Changed {
		t.Fatalf("expected unchanged result")
	}
	if result.RestartScheduled {
		t.Fatalf("expected restart not scheduled")
	}
	if restarted {
		t.Fatalf("expected restart callback not called")
	}
}

func TestEditorSaveChangedSchedulesRestart(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	before := []byte("storage:\n  root_dir: /data/test\nmysql:\n  dsn: test\n")
	after := "storage:\n  root_dir: /data/new\nmysql:\n  dsn: test\n"
	if err := os.WriteFile(path, before, 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	restartCh := make(chan struct{}, 1)
	editor := NewEditor(path, func() {
		restartCh <- struct{}{}
	})

	result, err := editor.Save(context.Background(), after)
	if err != nil {
		t.Fatalf("Save error: %v", err)
	}
	if !result.Changed {
		t.Fatalf("expected changed result")
	}
	if !result.RestartScheduled {
		t.Fatalf("expected restart scheduled")
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(got) != after {
		t.Fatalf("unexpected saved content: %q", string(got))
	}

	select {
	case <-restartCh:
	case <-time.After(2 * time.Second):
		t.Fatalf("expected restart callback")
	}
}

func TestEditorSaveRejectsInvalidConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	before := []byte("storage:\n  root_dir: /data/test\nmysql:\n  dsn: test\n")
	if err := os.WriteFile(path, before, 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	editor := NewEditor(path, nil)
	if _, err := editor.Save(context.Background(), "storage:\n  root_dir: ''\n"); err == nil {
		t.Fatalf("expected validation error")
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(got) != string(before) {
		t.Fatalf("expected file unchanged")
	}
}
