package bilibili

import (
	"context"
	"errors"
	"testing"
	"time"
)

type stubAuthClient struct {
	reloadCount int
	checkCount  int
	reloadErr   error
	checkErr    error
	info        AuthInfo
}

func (s *stubAuthClient) ReloadAuth() (bool, error) {
	s.reloadCount++
	if s.reloadErr != nil {
		return false, s.reloadErr
	}
	return true, nil
}

func (s *stubAuthClient) CheckAuth(ctx context.Context) (AuthInfo, error) {
	s.checkCount++
	if s.checkErr != nil {
		return AuthInfo{}, s.checkErr
	}
	return s.info, nil
}

func TestAuthWatcherRuns(t *testing.T) {
	client := &stubAuthClient{info: AuthInfo{IsLogin: true, Mid: 1, Uname: "u"}}
	w := NewAuthWatcher(client, 5*time.Millisecond, 5*time.Millisecond, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	go w.Start(ctx)
	<-ctx.Done()

	if client.reloadCount == 0 {
		t.Fatalf("expected reload")
	}
	if client.checkCount == 0 {
		t.Fatalf("expected check")
	}
}

func TestAuthWatcherErrors(t *testing.T) {
	client := &stubAuthClient{reloadErr: errors.New("reload"), checkErr: errors.New("check")}
	w := NewAuthWatcher(client, 5*time.Millisecond, 5*time.Millisecond, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	go w.Start(ctx)
	<-ctx.Done()

	if client.reloadCount == 0 || client.checkCount == 0 {
		t.Fatalf("expected reload and check")
	}
}

func TestAuthWatcherNoClient(t *testing.T) {
	w := NewAuthWatcher(nil, 5*time.Millisecond, 5*time.Millisecond, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	w.Start(ctx)
}
