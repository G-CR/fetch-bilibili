package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestLoadAppliesDefaults(t *testing.T) {
	path := writeTempConfig(t, `
server:
  addr: ":9090"
storage:
  root_dir: "/tmp/bilibili"
mysql:
  dsn: "user:pass@tcp(localhost:3306)/db"
scheduler:
  fetch_interval: "1h"
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if cfg.Server.Addr != ":9090" {
		t.Fatalf("server addr want :9090 got %s", cfg.Server.Addr)
	}
	if cfg.Server.ReadTimeout != 10*time.Second {
		t.Fatalf("default read timeout not applied")
	}
	if cfg.Scheduler.FetchInterval != time.Hour {
		t.Fatalf("fetch interval parse failed")
	}
	if cfg.Scheduler.CheckStableDays != 30 {
		t.Fatalf("default stable days not applied")
	}
	if cfg.Bilibili.UserAgent == "" {
		t.Fatalf("default user agent not applied")
	}
}

func TestLoadMissingRootDir(t *testing.T) {
	path := writeTempConfig(t, `
mysql:
  dsn: "user:pass@tcp(localhost:3306)/db"
`)
	if _, err := Load(path); err == nil {
		t.Fatalf("expected error for missing root_dir")
	}
}

func TestLoadMissingDSN(t *testing.T) {
	path := writeTempConfig(t, `
storage:
  root_dir: "/tmp/bilibili"
`)
	if _, err := Load(path); err == nil {
		t.Fatalf("expected error for missing dsn")
	}
}

func TestLoadInvalidYAML(t *testing.T) {
	path := writeTempConfig(t, `:bad`)
	if _, err := Load(path); err == nil {
		t.Fatalf("expected error for invalid yaml")
	}
}

func TestApplyDefaultsAll(t *testing.T) {
	cfg := Config{}
	applyDefaults(&cfg)

	if cfg.Server.Addr == "" {
		t.Fatalf("expected server addr")
	}
	if cfg.Storage.MaxBytes == 0 || cfg.Storage.SafeBytes == 0 {
		t.Fatalf("expected storage defaults")
	}
	if cfg.Scheduler.FetchInterval == 0 || cfg.Scheduler.CheckInterval == 0 || cfg.Scheduler.CleanupInterval == 0 {
		t.Fatalf("expected scheduler defaults")
	}
	if cfg.Limits.DownloadConcurrency == 0 || cfg.Limits.CheckConcurrency == 0 {
		t.Fatalf("expected limits defaults")
	}
	if cfg.Creators.ReloadInterval == 0 {
		t.Fatalf("expected creators defaults")
	}
	if cfg.Bilibili.UserAgent == "" {
		t.Fatalf("expected bilibili defaults")
	}
	if cfg.Bilibili.AuthCheckInterval == 0 || cfg.Bilibili.AuthReloadInterval == 0 {
		t.Fatalf("expected auth defaults")
	}
	if cfg.MySQL.MaxOpenConns == 0 || cfg.MySQL.MaxIdleConns == 0 || cfg.MySQL.ConnMaxLifetime == 0 {
		t.Fatalf("expected mysql defaults")
	}
	if cfg.Logging.Level == "" || cfg.Logging.Format == "" || cfg.Logging.Output == "" {
		t.Fatalf("expected logging defaults")
	}
}

func TestDefaultValues(t *testing.T) {
	cfg := Default()
	if cfg.Server.Addr == "" {
		t.Fatalf("default server addr empty")
	}
	if cfg.Scheduler.CheckStableDays != 30 {
		t.Fatalf("default stable days mismatch")
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := Load("/path/does/not/exist.yaml"); err == nil {
		t.Fatalf("expected error for missing file")
	}
}
