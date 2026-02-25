package bilibili

import (
	"context"
	"log"
	"time"
)

type AuthInfo struct {
	IsLogin bool
	Mid     int64
	Uname   string
}

type AuthClient interface {
	ReloadAuth() (bool, error)
	CheckAuth(ctx context.Context) (AuthInfo, error)
}

type AuthWatcher struct {
	client         AuthClient
	reloadInterval time.Duration
	checkInterval  time.Duration
	logger         *log.Logger
}

func NewAuthWatcher(client AuthClient, reloadInterval, checkInterval time.Duration, logger *log.Logger) *AuthWatcher {
	if logger == nil {
		logger = log.Default()
	}
	return &AuthWatcher{
		client:         client,
		reloadInterval: reloadInterval,
		checkInterval:  checkInterval,
		logger:         logger,
	}
}

func (w *AuthWatcher) Start(ctx context.Context) {
	if w.client == nil {
		w.logger.Print("认证监控未启用：未配置客户端")
		return
	}

	var reloadTicker *time.Ticker
	var checkTicker *time.Ticker
	var reloadCh <-chan time.Time
	var checkCh <-chan time.Time

	w.runReload()
	if w.checkInterval > 0 {
		w.runCheck(ctx)
	}

	if w.reloadInterval > 0 {
		reloadTicker = time.NewTicker(w.reloadInterval)
		reloadCh = reloadTicker.C
	}
	if w.checkInterval > 0 {
		checkTicker = time.NewTicker(w.checkInterval)
		checkCh = checkTicker.C
	}

	defer func() {
		if reloadTicker != nil {
			reloadTicker.Stop()
		}
		if checkTicker != nil {
			checkTicker.Stop()
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case <-reloadCh:
			w.runReload()
		case <-checkCh:
			w.runCheck(ctx)
		}
	}
}

func (w *AuthWatcher) runReload() {
	updated, err := w.client.ReloadAuth()
	if err != nil {
		w.logger.Printf("刷新 Cookie 失败: %v", err)
		return
	}
	if updated {
		w.logger.Printf("已刷新 Cookie 配置")
	}
}

func (w *AuthWatcher) runCheck(ctx context.Context) {
	info, err := w.client.CheckAuth(ctx)
	if err != nil {
		w.logger.Printf("Cookie 有效性检查失败: %v", err)
		return
	}
	if !info.IsLogin {
		w.logger.Printf("Cookie 已失效或未登录")
	} else {
		w.logger.Printf("Cookie 有效，用户: %s(%d)", info.Uname, info.Mid)
	}
}
