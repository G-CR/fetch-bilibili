package config

import (
	"strings"
	"testing"
)

func TestParseAcceptsInlineCookieConfig(t *testing.T) {
	cfg, err := Parse([]byte(`
storage:
  root_dir: /data/archive
mysql:
  dsn: fetch:fetchpass@tcp(localhost:3306)/fetch
bilibili:
  cookie: "SESSDATA=test-token"
`))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if cfg.Bilibili.Cookie != "SESSDATA=test-token" {
		t.Fatalf("unexpected cookie: %q", cfg.Bilibili.Cookie)
	}
}

func TestParseDefaultsBilibiliFetchPageSizeToFive(t *testing.T) {
	cfg, err := Parse([]byte(`
storage:
  root_dir: /data/archive
mysql:
  dsn: fetch:fetchpass@tcp(localhost:3306)/fetch
`))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if cfg.Bilibili.FetchPageSize != 5 {
		t.Fatalf("expected default fetch page size 5, got %d", cfg.Bilibili.FetchPageSize)
	}
}

func TestParseAcceptsConfiguredBilibiliFetchPageSize(t *testing.T) {
	cfg, err := Parse([]byte(`
storage:
  root_dir: /data/archive
mysql:
  dsn: fetch:fetchpass@tcp(localhost:3306)/fetch
bilibili:
  fetch_page_size: 7
`))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if cfg.Bilibili.FetchPageSize != 7 {
		t.Fatalf("expected configured fetch page size 7, got %d", cfg.Bilibili.FetchPageSize)
	}
}

func TestParseDefaultsCleanupRetentionHoursToOneWeek(t *testing.T) {
	cfg, err := Parse([]byte(`
storage:
  root_dir: /data/archive
mysql:
  dsn: fetch:fetchpass@tcp(localhost:3306)/fetch
`))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if cfg.Storage.CleanupRetentionHours != 168 {
		t.Fatalf("expected default cleanup retention hours 168, got %d", cfg.Storage.CleanupRetentionHours)
	}
}

func TestParseAcceptsConfiguredCleanupRetentionHours(t *testing.T) {
	cfg, err := Parse([]byte(`
storage:
  root_dir: /data/archive
  cleanup_retention_hours: 48
mysql:
  dsn: fetch:fetchpass@tcp(localhost:3306)/fetch
`))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if cfg.Storage.CleanupRetentionHours != 48 {
		t.Fatalf("expected configured cleanup retention hours 48, got %d", cfg.Storage.CleanupRetentionHours)
	}
}

func TestParseRejectsDeprecatedCookieFileFields(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    string
	}{
		{
			name: "cookie_file",
			content: `
storage:
  root_dir: /data/archive
mysql:
  dsn: fetch:fetchpass@tcp(localhost:3306)/fetch
bilibili:
  cookie_file: /app/secrets/bilibili_cookie.txt
`,
			want: "bilibili.cookie_file 已废弃",
		},
		{
			name: "sessdata_file",
			content: `
storage:
  root_dir: /data/archive
mysql:
  dsn: fetch:fetchpass@tcp(localhost:3306)/fetch
bilibili:
  sessdata_file: /app/secrets/bilibili_sessdata.txt
`,
			want: "bilibili.sessdata_file 已废弃",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]byte(tc.content))
			if err == nil {
				t.Fatalf("expected parse error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q in error, got %v", tc.want, err)
			}
		})
	}
}
