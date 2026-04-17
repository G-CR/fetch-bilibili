package config

import (
	"strings"
	"testing"
	"time"
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

func TestParseDefaultsDiscoveryConfig(t *testing.T) {
	cfg, err := Parse([]byte(`
storage:
  root_dir: /data/archive
mysql:
  dsn: fetch:fetchpass@tcp(localhost:3306)/fetch
`))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if cfg.Discovery.Enabled {
		t.Fatalf("expected discovery disabled by default")
	}
	if cfg.Discovery.Interval != 24*time.Hour {
		t.Fatalf("expected default discovery interval 24h, got %s", cfg.Discovery.Interval)
	}
	if cfg.Discovery.MaxKeywordsPerRun != 20 {
		t.Fatalf("expected default max keywords 20, got %d", cfg.Discovery.MaxKeywordsPerRun)
	}
	if cfg.Discovery.ScoreVersion != "v1" {
		t.Fatalf("expected default score version v1, got %q", cfg.Discovery.ScoreVersion)
	}
}

func TestParseRejectsEnabledDiscoveryWithoutKeywords(t *testing.T) {
	_, err := Parse([]byte(`
storage:
  root_dir: /data/archive
mysql:
  dsn: fetch:fetchpass@tcp(localhost:3306)/fetch
discovery:
  enabled: true
  keywords: []
`))
	if err == nil {
		t.Fatalf("expected parse error")
	}
	if !strings.Contains(err.Error(), "discovery.keywords 至少配置 1 个关键词") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseAcceptsConfiguredDiscoveryWeights(t *testing.T) {
	cfg, err := Parse([]byte(`
storage:
  root_dir: /data/archive
mysql:
  dsn: fetch:fetchpass@tcp(localhost:3306)/fetch
discovery:
  enabled: true
  interval: 12h
  keywords:
    - 影视剪辑
  score_version: custom-v2
  score_weights:
    keyword_risk:
      max: 50
    similarity:
      strong: 30
    feedback:
      ignore_penalty: -20
`))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if cfg.Discovery.Interval != 12*time.Hour {
		t.Fatalf("expected configured interval 12h, got %s", cfg.Discovery.Interval)
	}
	if cfg.Discovery.ScoreVersion != "custom-v2" {
		t.Fatalf("expected custom score version, got %q", cfg.Discovery.ScoreVersion)
	}
	if cfg.Discovery.ScoreWeights.KeywordRisk.Max != 50 {
		t.Fatalf("expected keyword max 50, got %d", cfg.Discovery.ScoreWeights.KeywordRisk.Max)
	}
	if cfg.Discovery.ScoreWeights.Similarity.Strong != 30 {
		t.Fatalf("expected strong similarity score 30, got %d", cfg.Discovery.ScoreWeights.Similarity.Strong)
	}
	if cfg.Discovery.ScoreWeights.Feedback.IgnorePenalty != -20 {
		t.Fatalf("expected ignore penalty -20, got %d", cfg.Discovery.ScoreWeights.Feedback.IgnorePenalty)
	}
}
