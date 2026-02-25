package config

import (
	"errors"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server    ServerConfig    `yaml:"server"`
	Storage   StorageConfig   `yaml:"storage"`
	Scheduler SchedulerConfig `yaml:"scheduler"`
	Limits    LimitsConfig    `yaml:"limits"`
	Creators  CreatorsConfig  `yaml:"creators"`
	Bilibili  BilibiliConfig  `yaml:"bilibili"`
	MySQL     MySQLConfig     `yaml:"mysql"`
	Logging   LoggingConfig   `yaml:"logging"`
}

type ServerConfig struct {
	Addr         string        `yaml:"addr"`
	ReadTimeout  time.Duration `yaml:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout"`
}

type StorageConfig struct {
	RootDir        string             `yaml:"root_dir"`
	MaxBytes       int64              `yaml:"max_bytes"`
	SafeBytes      int64              `yaml:"safe_bytes"`
	KeepOutOfPrint bool               `yaml:"keep_out_of_print"`
	DeleteWeights  DeleteWeightConfig `yaml:"delete_weights"`
}

type DeleteWeightConfig struct {
	OutOfPrintPenalty  int64 `yaml:"out_of_print_penalty"`
	StableBonus        int64 `yaml:"stable_bonus"`
	DownloadedBonus    int64 `yaml:"downloaded_bonus"`
	FollowerWeight     int64 `yaml:"follower_weight"`
	ViewWeight         int64 `yaml:"view_weight"`
	FavoriteWeight     int64 `yaml:"favorite_weight"`
	Age30DBonus        int64 `yaml:"age_30d_bonus"`
	SizeGBBonus        int64 `yaml:"size_gb_bonus"`
	LastAccess30DBonus int64 `yaml:"last_access_30d_bonus"`
}

type SchedulerConfig struct {
	FetchInterval   time.Duration `yaml:"fetch_interval"`
	CheckInterval   time.Duration `yaml:"check_interval"`
	CleanupInterval time.Duration `yaml:"cleanup_interval"`
	CheckStableDays int           `yaml:"check_stable_days"`
}

type LimitsConfig struct {
	GlobalQPS           int `yaml:"global_qps"`
	PerCreatorQPS       int `yaml:"per_creator_qps"`
	DownloadConcurrency int `yaml:"download_concurrency"`
	CheckConcurrency    int `yaml:"check_concurrency"`
}

type CreatorsConfig struct {
	File           string        `yaml:"file"`
	ReloadInterval time.Duration `yaml:"reload_interval"`
}

type BilibiliConfig struct {
	ResolveNameCacheTTL time.Duration `yaml:"resolve_name_cache_ttl"`
	RequestTimeout      time.Duration `yaml:"request_timeout"`
	UserAgent           string        `yaml:"user_agent"`
	Cookie              string        `yaml:"cookie"`
	SESSDATA            string        `yaml:"sessdata"`
	CookieFile          string        `yaml:"cookie_file"`
	SESSDATAFile        string        `yaml:"sessdata_file"`
	AuthCheckInterval   time.Duration `yaml:"auth_check_interval"`
	AuthReloadInterval  time.Duration `yaml:"auth_reload_interval"`
	RiskBackoffBase     time.Duration `yaml:"risk_backoff_base"`
	RiskBackoffMax      time.Duration `yaml:"risk_backoff_max"`
	RiskBackoffJitter   float64       `yaml:"risk_backoff_jitter"`
}

type MySQLConfig struct {
	DSN             string        `yaml:"dsn"`
	MaxOpenConns    int           `yaml:"max_open_conns"`
	MaxIdleConns    int           `yaml:"max_idle_conns"`
	ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime"`
}

type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
	Output string `yaml:"output"`
}

func Default() Config {
	return Config{
		Server: ServerConfig{
			Addr:         ":8080",
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 30 * time.Second,
		},
		Storage: StorageConfig{
			MaxBytes:       2 * 1024 * 1024 * 1024 * 1024,
			SafeBytes:      int64(2*1024*1024*1024*1024) * 9 / 10,
			KeepOutOfPrint: true,
			DeleteWeights: DeleteWeightConfig{
				OutOfPrintPenalty:  -100,
				StableBonus:        30,
				DownloadedBonus:    20,
				FollowerWeight:     -10,
				ViewWeight:         -8,
				FavoriteWeight:     -6,
				Age30DBonus:        5,
				SizeGBBonus:        3,
				LastAccess30DBonus: 5,
			},
		},
		Scheduler: SchedulerConfig{
			FetchInterval:   45 * time.Minute,
			CheckInterval:   24 * time.Hour,
			CleanupInterval: 24 * time.Hour,
			CheckStableDays: 30,
		},
		Limits: LimitsConfig{
			GlobalQPS:           2,
			PerCreatorQPS:       1,
			DownloadConcurrency: 4,
			CheckConcurrency:    8,
		},
		Creators: CreatorsConfig{
			ReloadInterval: time.Minute,
		},
		Bilibili: BilibiliConfig{
			ResolveNameCacheTTL: 24 * time.Hour,
			RequestTimeout:      10 * time.Second,
			UserAgent:           "fetch-bilibili/1.0",
			AuthCheckInterval:   12 * time.Hour,
			AuthReloadInterval:  30 * time.Minute,
			RiskBackoffBase:     2 * time.Second,
			RiskBackoffMax:      30 * time.Second,
			RiskBackoffJitter:   0.3,
		},
		MySQL: MySQLConfig{
			MaxOpenConns:    20,
			MaxIdleConns:    10,
			ConnMaxLifetime: 30 * time.Minute,
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "json",
			Output: "stdout",
		},
	}
}

func Load(path string) (Config, error) {
	cfg := Default()

	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}

	applyDefaults(&cfg)
	if err := validate(cfg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func applyDefaults(cfg *Config) {
	if cfg.Server.Addr == "" {
		cfg.Server.Addr = ":8080"
	}
	if cfg.Server.ReadTimeout == 0 {
		cfg.Server.ReadTimeout = 10 * time.Second
	}
	if cfg.Server.WriteTimeout == 0 {
		cfg.Server.WriteTimeout = 30 * time.Second
	}

	if cfg.Storage.MaxBytes == 0 {
		cfg.Storage.MaxBytes = 2 * 1024 * 1024 * 1024 * 1024
	}
	if cfg.Storage.SafeBytes == 0 {
		cfg.Storage.SafeBytes = cfg.Storage.MaxBytes * 9 / 10
	}
	if cfg.Scheduler.FetchInterval == 0 {
		cfg.Scheduler.FetchInterval = 45 * time.Minute
	}
	if cfg.Scheduler.CheckInterval == 0 {
		cfg.Scheduler.CheckInterval = 24 * time.Hour
	}
	if cfg.Scheduler.CleanupInterval == 0 {
		cfg.Scheduler.CleanupInterval = 24 * time.Hour
	}
	if cfg.Scheduler.CheckStableDays == 0 {
		cfg.Scheduler.CheckStableDays = 30
	}
	if cfg.Limits.DownloadConcurrency == 0 {
		cfg.Limits.DownloadConcurrency = 4
	}
	if cfg.Limits.CheckConcurrency == 0 {
		cfg.Limits.CheckConcurrency = 8
	}
	if cfg.Limits.GlobalQPS == 0 {
		cfg.Limits.GlobalQPS = 2
	}
	if cfg.Limits.PerCreatorQPS == 0 {
		cfg.Limits.PerCreatorQPS = 1
	}
	if cfg.Creators.ReloadInterval == 0 {
		cfg.Creators.ReloadInterval = time.Minute
	}
	if cfg.Bilibili.ResolveNameCacheTTL == 0 {
		cfg.Bilibili.ResolveNameCacheTTL = 24 * time.Hour
	}
	if cfg.Bilibili.RequestTimeout == 0 {
		cfg.Bilibili.RequestTimeout = 10 * time.Second
	}
	if cfg.Bilibili.UserAgent == "" {
		cfg.Bilibili.UserAgent = "fetch-bilibili/1.0"
	}
	if cfg.Bilibili.AuthCheckInterval == 0 {
		cfg.Bilibili.AuthCheckInterval = 12 * time.Hour
	}
	if cfg.Bilibili.AuthReloadInterval == 0 {
		cfg.Bilibili.AuthReloadInterval = 30 * time.Minute
	}
	if cfg.Bilibili.RiskBackoffBase == 0 {
		cfg.Bilibili.RiskBackoffBase = 2 * time.Second
	}
	if cfg.Bilibili.RiskBackoffMax == 0 {
		cfg.Bilibili.RiskBackoffMax = 30 * time.Second
	}
	if cfg.MySQL.MaxOpenConns == 0 {
		cfg.MySQL.MaxOpenConns = 20
	}
	if cfg.MySQL.MaxIdleConns == 0 {
		cfg.MySQL.MaxIdleConns = 10
	}
	if cfg.MySQL.ConnMaxLifetime == 0 {
		cfg.MySQL.ConnMaxLifetime = 30 * time.Minute
	}
	if cfg.Logging.Level == "" {
		cfg.Logging.Level = "info"
	}
	if cfg.Logging.Format == "" {
		cfg.Logging.Format = "json"
	}
	if cfg.Logging.Output == "" {
		cfg.Logging.Output = "stdout"
	}
}

func validate(cfg Config) error {
	if cfg.Storage.RootDir == "" {
		return errors.New("storage.root_dir 必须配置")
	}
	if cfg.MySQL.DSN == "" {
		return errors.New("mysql.dsn 必须配置")
	}
	return nil
}
