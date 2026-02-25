package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"fetch-bilibili/internal/app"
	"fetch-bilibili/internal/config"
)

type appRunner interface {
	Run(context.Context) error
}

var newApp = func(cfg config.Config) (appRunner, error) {
	return app.New(cfg)
}
var fatalf = log.Fatalf

func main() {
	configPath := os.Getenv("FETCH_CONFIG")
	if configPath == "" {
		configPath = "configs/config.yaml"
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, configPath); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, http.ErrServerClosed) {
		fatalf("服务退出: %v", err)
	}
}

func run(ctx context.Context, configPath string) error {
	cfg, err := loadConfig(configPath)
	if err != nil {
		return err
	}

	a, err := newApp(cfg)
	if err != nil {
		return err
	}
	return a.Run(ctx)
}

func loadConfig(configPath string) (config.Config, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fallback := "configs/config.example.yaml"
			if _, statErr := os.Stat(fallback); statErr == nil {
				log.Printf("未找到配置文件 %s，改用 %s", configPath, fallback)
				cfg, err = config.Load(fallback)
			}
		}
	}
	return cfg, err
}
