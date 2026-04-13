package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"fetch-bilibili/internal/config"
)

type stubApp struct {
	err error
}

func (s *stubApp) Run(ctx context.Context) error {
	return s.err
}

func TestLoadConfigDoesNotFallbackToExample(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	tempDir := t.TempDir()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	if err := os.MkdirAll("configs", 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	fallback := filepath.Join("configs", "config.example.yaml")
	content := []byte("storage:\n  root_dir: '/tmp'\nmysql:\n  dsn: 'user:pass@tcp(localhost:3306)/db'\n")
	if err := os.WriteFile(fallback, content, 0o644); err != nil {
		t.Fatalf("write fallback: %v", err)
	}

	if _, err := loadConfig("configs/missing.yaml"); err == nil {
		t.Fatalf("expected error")
	}
}

func TestRunSuccess(t *testing.T) {
	path := writeTempConfig(t)
	orig := newApp
	newApp = func(cfg config.Config) (appRunner, error) {
		return &stubApp{}, nil
	}
	defer func() { newApp = orig }()

	if err := run(context.Background(), path); err != nil {
		t.Fatalf("run error: %v", err)
	}
}

func TestRunNewAppError(t *testing.T) {
	path := writeTempConfig(t)
	orig := newApp
	newApp = func(cfg config.Config) (appRunner, error) {
		return nil, errors.New("init error")
	}
	defer func() { newApp = orig }()

	if err := run(context.Background(), path); err == nil {
		t.Fatalf("expected error")
	}
}

func TestRunAppRunError(t *testing.T) {
	path := writeTempConfig(t)
	orig := newApp
	newApp = func(cfg config.Config) (appRunner, error) {
		return &stubApp{err: errors.New("run error")}, nil
	}
	defer func() { newApp = orig }()

	if err := run(context.Background(), path); err == nil {
		t.Fatalf("expected error")
	}
}

func TestLoadConfigMissing(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	tempDir := t.TempDir()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	if _, err := loadConfig("missing.yaml"); err == nil {
		t.Fatalf("expected error")
	}
}

func TestMainWithEnvConfig(t *testing.T) {
	path := writeTempConfig(t)
	orig := newApp
	origFatal := fatalf
	newApp = func(cfg config.Config) (appRunner, error) {
		return &stubApp{}, nil
	}
	fatalf = func(format string, args ...any) {
		t.Fatalf("unexpected fatal: "+format, args...)
	}
	defer func() {
		newApp = orig
		fatalf = origFatal
	}()

	old := os.Getenv("FETCH_CONFIG")
	if err := os.Setenv("FETCH_CONFIG", path); err != nil {
		t.Fatalf("setenv: %v", err)
	}
	defer func() { _ = os.Setenv("FETCH_CONFIG", old) }()

	main()
}

func TestMainDefaultConfigPath(t *testing.T) {
	orig := newApp
	origFatal := fatalf
	newApp = func(cfg config.Config) (appRunner, error) {
		return &stubApp{}, nil
	}
	fatalf = func(format string, args ...any) {
		t.Fatalf("unexpected fatal: "+format, args...)
	}
	defer func() {
		newApp = orig
		fatalf = origFatal
	}()

	old := os.Getenv("FETCH_CONFIG")
	_ = os.Unsetenv("FETCH_CONFIG")
	defer func() { _ = os.Setenv("FETCH_CONFIG", old) }()

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	tempDir := t.TempDir()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	if err := os.MkdirAll("configs", 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	configBody := "storage:\n  root_dir: /tmp\nmysql:\n  dsn: dsn\n"
	if err := os.WriteFile(filepath.Join("configs", "config.yaml"), []byte(configBody), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	main()
}

func TestMainFatalPath(t *testing.T) {
	path := writeTempConfig(t)
	origApp := newApp
	origFatal := fatalf
	defer func() {
		newApp = origApp
		fatalf = origFatal
	}()

	newApp = func(cfg config.Config) (appRunner, error) {
		return nil, errors.New("init error")
	}
	called := false
	fatalf = func(format string, args ...any) {
		called = true
	}

	old := os.Getenv("FETCH_CONFIG")
	if err := os.Setenv("FETCH_CONFIG", path); err != nil {
		t.Fatalf("setenv: %v", err)
	}
	defer func() { _ = os.Setenv("FETCH_CONFIG", old) }()

	main()
	if !called {
		t.Fatalf("expected fatalf to be called")
	}
}

func writeTempConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := []byte("storage:\n  root_dir: '/tmp'\nmysql:\n  dsn: 'user:pass@tcp(localhost:3306)/db'\n")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}
