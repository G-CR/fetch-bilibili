package config

import (
	"os"
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

func TestApplyDefaultsFillsZeroValueRuntimeConfig(t *testing.T) {
	cfg := Config{
		Storage: StorageConfig{
			RootDir: "/data/archive",
		},
		MySQL: MySQLConfig{
			DSN: "fetch:fetchpass@tcp(localhost:3306)/fetch",
		},
	}

	applyDefaults(&cfg)

	if cfg.Server.Addr != ":8080" || cfg.Server.ReadTimeout != 10*time.Second || cfg.Server.WriteTimeout != 30*time.Second {
		t.Fatalf("unexpected server defaults: %+v", cfg.Server)
	}
	if cfg.Storage.MaxBytes == 0 || cfg.Storage.SafeBytes != cfg.Storage.MaxBytes*9/10 || cfg.Storage.CleanupRetentionHours != 168 {
		t.Fatalf("unexpected storage defaults: %+v", cfg.Storage)
	}
	if cfg.Scheduler.FetchInterval != 45*time.Minute || cfg.Scheduler.CheckInterval != 24*time.Hour || cfg.Scheduler.CleanupInterval != 24*time.Hour || cfg.Scheduler.CheckStableDays != 30 {
		t.Fatalf("unexpected scheduler defaults: %+v", cfg.Scheduler)
	}
	if cfg.Discovery.Interval != 24*time.Hour || cfg.Discovery.MaxKeywordsPerRun != 20 || cfg.Discovery.MaxPagesPerKeyword != 2 || cfg.Discovery.MaxCandidatesPerRun != 100 || cfg.Discovery.MaxRelatedPerCreator != 10 || cfg.Discovery.ScoreVersion != "v1" {
		t.Fatalf("unexpected discovery defaults: %+v", cfg.Discovery)
	}
	if cfg.Discovery.ScoreWeights.KeywordRisk.Max != 40 ||
		cfg.Discovery.ScoreWeights.Activity30D.Low != 5 ||
		cfg.Discovery.ScoreWeights.Activity30D.Medium != 10 ||
		cfg.Discovery.ScoreWeights.Activity30D.High != 15 ||
		cfg.Discovery.ScoreWeights.Similarity.Weak != 5 ||
		cfg.Discovery.ScoreWeights.Similarity.Medium != 10 ||
		cfg.Discovery.ScoreWeights.Similarity.Strong != 20 ||
		cfg.Discovery.ScoreWeights.DeletionTrace.Single != 10 ||
		cfg.Discovery.ScoreWeights.DeletionTrace.Max != 20 ||
		cfg.Discovery.ScoreWeights.AccountSize.SmallBonus != 10 ||
		cfg.Discovery.ScoreWeights.AccountSize.OversizePenalty != -5 ||
		cfg.Discovery.ScoreWeights.Feedback.IgnorePenalty != -15 {
		t.Fatalf("unexpected discovery score weight defaults: %+v", cfg.Discovery.ScoreWeights)
	}
	if cfg.Limits.DownloadConcurrency != 4 || cfg.Limits.CheckConcurrency != 8 || cfg.Limits.GlobalQPS != 2 || cfg.Limits.PerCreatorQPS != 1 {
		t.Fatalf("unexpected limits defaults: %+v", cfg.Limits)
	}
	if cfg.Creators.ReloadInterval != time.Minute {
		t.Fatalf("unexpected creator reload interval: %s", cfg.Creators.ReloadInterval)
	}
	if cfg.Bilibili.ResolveNameCacheTTL != 24*time.Hour ||
		cfg.Bilibili.RequestTimeout != 10*time.Second ||
		cfg.Bilibili.UserAgent != "fetch-bilibili/1.0" ||
		cfg.Bilibili.FetchPageSize != 5 ||
		cfg.Bilibili.AuthCheckInterval != 12*time.Hour ||
		cfg.Bilibili.AuthReloadInterval != 30*time.Minute ||
		cfg.Bilibili.RiskBackoffBase != 2*time.Second ||
		cfg.Bilibili.RiskBackoffMax != 30*time.Second {
		t.Fatalf("unexpected bilibili defaults: %+v", cfg.Bilibili)
	}
	if cfg.MySQL.MaxOpenConns != 20 || cfg.MySQL.MaxIdleConns != 10 || cfg.MySQL.ConnMaxLifetime != 30*time.Minute {
		t.Fatalf("unexpected mysql defaults: %+v", cfg.MySQL)
	}
	if cfg.Logging.Level != "info" || cfg.Logging.Format != "json" || cfg.Logging.Output != "stdout" {
		t.Fatalf("unexpected logging defaults: %+v", cfg.Logging)
	}
}

func TestLoadReadsConfigFile(t *testing.T) {
	path := t.TempDir() + "/config.yaml"
	content := []byte(`
storage:
  root_dir: /data/archive
mysql:
  dsn: fetch:fetchpass@tcp(localhost:3306)/fetch
`)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if cfg.Storage.RootDir != "/data/archive" || cfg.MySQL.DSN == "" {
		t.Fatalf("unexpected loaded config: %+v", cfg)
	}
}

func TestValidateRejectsInvalidDiscoveryBounds(t *testing.T) {
	base := Config{
		Storage: StorageConfig{RootDir: "/data/archive"},
		MySQL:   MySQLConfig{DSN: "fetch:fetchpass@tcp(localhost:3306)/fetch"},
	}
	cases := []struct {
		name string
		edit func(*Config)
		want string
	}{
		{
			name: "negative_cleanup_retention",
			edit: func(cfg *Config) {
				cfg.Storage.CleanupRetentionHours = -1
			},
			want: "storage.cleanup_retention_hours 不能小于 0",
		},
		{
			name: "negative_fetch_page_size",
			edit: func(cfg *Config) {
				cfg.Bilibili.FetchPageSize = -1
			},
			want: "bilibili.fetch_page_size 必须大于 0",
		},
		{
			name: "negative_interval",
			edit: func(cfg *Config) {
				cfg.Discovery.Interval = -time.Second
			},
			want: "discovery.interval 不能小于 0",
		},
		{
			name: "negative_max_keywords",
			edit: func(cfg *Config) {
				cfg.Discovery.MaxKeywordsPerRun = -1
			},
			want: "discovery.max_keywords_per_run 不能小于 0",
		},
		{
			name: "negative_max_pages",
			edit: func(cfg *Config) {
				cfg.Discovery.MaxPagesPerKeyword = -1
			},
			want: "discovery.max_pages_per_keyword 不能小于 0",
		},
		{
			name: "negative_max_candidates",
			edit: func(cfg *Config) {
				cfg.Discovery.MaxCandidatesPerRun = -1
			},
			want: "discovery.max_candidates_per_run 不能小于 0",
		},
		{
			name: "negative_max_related",
			edit: func(cfg *Config) {
				cfg.Discovery.MaxRelatedPerCreator = -1
			},
			want: "discovery.max_related_per_creator 不能小于 0",
		},
		{
			name: "enabled_zero_interval",
			edit: func(cfg *Config) {
				cfg.Discovery.Enabled = true
				cfg.Discovery.Interval = 0
			},
			want: "discovery.interval 必须大于 0",
		},
		{
			name: "enabled_zero_max_keywords",
			edit: func(cfg *Config) {
				cfg.Discovery.Enabled = true
				cfg.Discovery.Interval = time.Hour
				cfg.Discovery.MaxKeywordsPerRun = 0
			},
			want: "discovery.max_keywords_per_run 必须大于 0",
		},
		{
			name: "enabled_zero_max_pages",
			edit: func(cfg *Config) {
				cfg.Discovery.Enabled = true
				cfg.Discovery.Interval = time.Hour
				cfg.Discovery.MaxKeywordsPerRun = 1
				cfg.Discovery.MaxPagesPerKeyword = 0
			},
			want: "discovery.max_pages_per_keyword 必须大于 0",
		},
		{
			name: "enabled_zero_max_candidates",
			edit: func(cfg *Config) {
				cfg.Discovery.Enabled = true
				cfg.Discovery.Interval = time.Hour
				cfg.Discovery.MaxKeywordsPerRun = 1
				cfg.Discovery.MaxPagesPerKeyword = 1
				cfg.Discovery.MaxCandidatesPerRun = 0
			},
			want: "discovery.max_candidates_per_run 必须大于 0",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := base
			tc.edit(&cfg)
			err := validate(cfg)
			if err == nil {
				t.Fatalf("expected validation error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q in error, got %v", tc.want, err)
			}
		})
	}
}
