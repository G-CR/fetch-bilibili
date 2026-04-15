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

	for {
		err := run(ctx, configPath)
		if errors.Is(err, app.ErrRestartRequested) {
			log.Println("配置已更新，正在重启服务…")
			continue
		}
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, http.ErrServerClosed) {
			fatalf("服务退出: %v", err)
		}
		break
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
	return config.Load(configPath)
}
