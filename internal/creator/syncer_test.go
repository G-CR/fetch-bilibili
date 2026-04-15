package creator

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"fetch-bilibili/internal/repo"
)

func TestFileSyncerSyncOnce(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "creators.yaml")
	content := `
creators:
  - uid: "123"
  - name: "by-name"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	repoStub := &stubRepo{}
	resolver := &stubResolver{uid: "999", name: "resolved"}
	svc := NewService(repoStub, resolver, nil)
	syncer := NewFileSyncer(svc, path, 0, nil)

	syncer.syncOnce(context.Background(), true)
	if repoStub.last.UID == "" {
		t.Fatalf("expected upsert")
	}
}

func TestFileSyncerPauseMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "creators.yaml")
	content := `
creators:
  - uid: "123"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	repoStub := &stubRepo{
		id: 1,
		list: []repo.Creator{
			{ID: 1, UID: "123", Status: "active"},
			{ID: 2, UID: "456", Status: "active"},
		},
	}
	svc := NewService(repoStub, nil, nil)
	syncer := NewFileSyncer(svc, path, 0, nil)

	syncer.syncOnce(context.Background(), true)
	if repoStub.statuses[2] != "paused" {
		t.Fatalf("expected id 2 paused")
	}
}

func TestFileSyncerPauseMissingPublishesCreatorEvent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "creators.yaml")
	content := `
creators:
  - uid: "123"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	repoStub := &stubRepo{
		id: 1,
		list: []repo.Creator{
			{ID: 1, UID: "123", Name: "keep", Platform: "bilibili", Status: "active"},
			{ID: 2, UID: "456", Name: "to-pause", Platform: "bilibili", Status: "active"},
		},
	}
	publisher := &stubCreatorEventPublisher{}
	svc := NewService(repoStub, nil, nil)
	svc.SetPublisher(publisher)
	syncer := NewFileSyncer(svc, path, 0, nil)

	syncer.syncOnce(context.Background(), true)

	var found bool
	for _, evt := range publisher.events {
		if evt.Type != "creator.changed" {
			continue
		}
		payload, ok := evt.Payload.(map[string]any)
		if !ok {
			t.Fatalf("expected map payload, got %T", evt.Payload)
		}
		assertCreatorChangedPayloadShape(t, payload)
		if payload["id"] == int64(2) && payload["status"] == "paused" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected paused creator.changed for id=2, got %+v", publisher.events)
	}
}

func TestFileSyncerSkipUnchanged(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "creators.yaml")
	content := `
creators:
  - uid: "123"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	repoStub := &stubRepo{}
	svc := NewService(repoStub, nil, nil)
	syncer := NewFileSyncer(svc, path, 0, nil)

	syncer.syncOnce(context.Background(), true)
	count := atomic.LoadInt64(&repoStub.count)
	syncer.syncOnce(context.Background(), false)
	if atomic.LoadInt64(&repoStub.count) != count {
		t.Fatalf("expected unchanged to skip")
	}
}

func TestFileSyncerStartInterval(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "creators.yaml")
	content := `
creators:
  - uid: "123"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	repoStub := &stubRepo{}
	svc := NewService(repoStub, nil, nil)
	syncer := NewFileSyncer(svc, path, 5*time.Millisecond, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	go syncer.Start(ctx)
	<-ctx.Done()

	if atomic.LoadInt64(&repoStub.count) == 0 {
		t.Fatalf("expected sync to run")
	}
}

func TestFileSyncerMissingFile(t *testing.T) {
	repoStub := &stubRepo{}
	svc := NewService(repoStub, nil, nil)
	syncer := NewFileSyncer(svc, "/path/not/exist.yaml", 0, nil)

	syncer.syncOnce(context.Background(), true)
}

func TestFileSyncerSkipsRemovedCreator(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "creators.yaml")
	content := `
creators:
  - uid: "123"
    name: "should-skip"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	repoStub := &stubRepo{
		creators: map[int64]repo.Creator{
			1: {ID: 1, UID: "123", Name: "removed", Status: "removed", Platform: "bilibili"},
		},
	}
	svc := NewService(repoStub, nil, nil)
	syncer := NewFileSyncer(svc, path, 0, nil)

	syncer.syncOnce(context.Background(), true)
	if atomic.LoadInt64(&repoStub.count) != 0 {
		t.Fatalf("expected removed creator not re-upserted, got count=%d", atomic.LoadInt64(&repoStub.count))
	}
}

func TestFileSyncerStartNoFile(t *testing.T) {
	repoStub := &stubRepo{}
	svc := NewService(repoStub, nil, nil)
	syncer := NewFileSyncer(svc, "", 0, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	syncer.Start(ctx)
}
