package creator

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"fetch-bilibili/internal/live"
	"fetch-bilibili/internal/repo"
)

type stubRepo struct {
	last      repo.Creator
	updated   repo.Creator
	id        int64
	deletedID int64
	deleted   int64
	err       error
	count     int
	list      []repo.Creator
	listErr   error
	statuses  map[int64]string
	statusErr error
	creators  map[int64]repo.Creator
}

func (s *stubRepo) Upsert(ctx context.Context, c repo.Creator) (int64, error) {
	s.last = c
	s.count++
	if s.err != nil {
		return 0, s.err
	}
	if s.id == 0 {
		s.id = 1
	}
	return s.id, nil
}

func (s *stubRepo) Create(ctx context.Context, c repo.Creator) (int64, error) {
	return 0, repo.ErrNotImplemented
}

func (s *stubRepo) Update(ctx context.Context, c repo.Creator) error {
	s.updated = c
	if s.creators == nil {
		s.creators = make(map[int64]repo.Creator)
	}
	s.creators[c.ID] = c
	return s.err
}

func (s *stubRepo) UpdateStatus(ctx context.Context, id int64, status string) error {
	if s.statuses == nil {
		s.statuses = make(map[int64]string)
	}
	s.statuses[id] = status
	if s.statusErr != nil {
		return s.statusErr
	}
	return nil
}

func (s *stubRepo) FindByID(ctx context.Context, id int64) (repo.Creator, error) {
	if c, ok := s.creators[id]; ok {
		return c, nil
	}
	return repo.Creator{}, sql.ErrNoRows
}

func (s *stubRepo) FindByPlatformUID(ctx context.Context, platform, uid string) (repo.Creator, error) {
	for _, c := range s.creators {
		matchPlatform := c.Platform
		if matchPlatform == "" {
			matchPlatform = "bilibili"
		}
		if matchPlatform == platform && c.UID == uid {
			return c, nil
		}
	}
	return repo.Creator{}, sql.ErrNoRows
}

func (s *stubRepo) ListActive(ctx context.Context, limit int) ([]repo.Creator, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	out := append([]repo.Creator(nil), s.list...)
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *stubRepo) ListActiveAfter(ctx context.Context, lastID int64, limit int) ([]repo.Creator, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	var out []repo.Creator
	for _, c := range s.list {
		if c.ID > lastID && c.Status == "active" {
			out = append(out, c)
		}
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *stubRepo) CountActive(ctx context.Context) (int64, error) {
	if s.listErr != nil {
		return 0, s.listErr
	}
	var count int64
	for _, c := range s.list {
		if c.Status == "" || c.Status == "active" {
			count++
		}
	}
	return count, nil
}

func (s *stubRepo) DeleteByID(ctx context.Context, id int64) (int64, error) {
	s.deletedID = id
	if s.err != nil {
		return 0, s.err
	}
	return s.deleted, nil
}

type stubResolver struct {
	uid       string
	name      string
	err       error
	nameByUID map[string]string
}

func (s *stubResolver) ResolveUID(ctx context.Context, keyword string) (string, string, error) {
	if s.err != nil {
		return "", "", s.err
	}
	return s.uid, s.name, nil
}

func (s *stubResolver) ResolveName(ctx context.Context, uid string) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	if s.nameByUID == nil {
		return "", nil
	}
	return s.nameByUID[uid], nil
}

type stubCreatorEventPublisher struct {
	events []live.Event
}

func (s *stubCreatorEventPublisher) Publish(evt live.Event) {
	s.events = append(s.events, evt)
}

func mustFindCreatorChangedEvent(t *testing.T, events []live.Event) live.Event {
	t.Helper()

	for _, evt := range events {
		if evt.Type == "creator.changed" {
			return evt
		}
	}
	t.Fatalf("expected creator.changed event, got %+v", events)
	return live.Event{}
}

func assertCreatorChangedPayloadShape(t *testing.T, payload map[string]any) {
	t.Helper()
	for _, key := range []string{"id", "uid", "name", "platform", "status"} {
		if _, ok := payload[key]; !ok {
			t.Fatalf("expected creator.changed payload key %q", key)
		}
	}
}

func TestServiceUpsertByUID(t *testing.T) {
	repoStub := &stubRepo{}
	svc := NewService(repoStub, nil, nil)

	entry := Entry{UID: "123", Name: "name"}
	creator, err := svc.Upsert(context.Background(), entry)
	if err != nil {
		t.Fatalf("upsert error: %v", err)
	}
	if creator.ID == 0 {
		t.Fatalf("expected id")
	}
	if repoStub.last.Platform != "bilibili" || repoStub.last.Status != "active" {
		t.Fatalf("expected defaults applied")
	}
}

func TestServiceUpsertByName(t *testing.T) {
	repoStub := &stubRepo{}
	resolver := &stubResolver{uid: "999", name: "resolved"}
	svc := NewService(repoStub, resolver, nil)

	entry := Entry{Name: "input-name"}
	creator, err := svc.Upsert(context.Background(), entry)
	if err != nil {
		t.Fatalf("upsert error: %v", err)
	}
	if creator.UID != "999" {
		t.Fatalf("expected resolved uid")
	}
}

func TestServiceUpsertByNameUsesResolvedName(t *testing.T) {
	repoStub := &stubRepo{}
	resolver := &stubResolver{uid: "888", name: "resolved-name"}
	svc := NewService(repoStub, resolver, nil)

	entry := Entry{Name: "input"}
	creator, err := svc.Upsert(context.Background(), entry)
	if err != nil {
		t.Fatalf("upsert error: %v", err)
	}
	if creator.Name != "resolved-name" {
		t.Fatalf("expected resolved name")
	}
}

func TestServiceUpsertByUIDBackfillsMissingName(t *testing.T) {
	repoStub := &stubRepo{}
	resolver := &stubResolver{
		nameByUID: map[string]string{
			"123": "resolved-name",
		},
	}
	svc := NewService(repoStub, resolver, nil)

	creator, err := svc.Upsert(context.Background(), Entry{UID: "123"})
	if err != nil {
		t.Fatalf("upsert error: %v", err)
	}
	if creator.Name != "resolved-name" {
		t.Fatalf("expected resolved name, got %+v", creator)
	}
	if repoStub.last.Name != "resolved-name" {
		t.Fatalf("expected repo to persist resolved name, got %+v", repoStub.last)
	}
}

func TestServiceUpsertRepoError(t *testing.T) {
	repoStub := &stubRepo{err: errors.New("repo")}
	svc := NewService(repoStub, nil, nil)

	if _, err := svc.Upsert(context.Background(), Entry{UID: "1"}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestServiceUpsertMissing(t *testing.T) {
	repoStub := &stubRepo{}
	svc := NewService(repoStub, nil, nil)

	if _, err := svc.Upsert(context.Background(), Entry{}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestServiceUpsertResolveError(t *testing.T) {
	repoStub := &stubRepo{}
	resolver := &stubResolver{err: errors.New("resolve")}
	svc := NewService(repoStub, resolver, nil)

	if _, err := svc.Upsert(context.Background(), Entry{Name: "bad"}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestServiceUpsertNoRepo(t *testing.T) {
	svc := NewService(nil, nil, nil)
	if _, err := svc.Upsert(context.Background(), Entry{UID: "1"}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestServiceUpsertNameNoResolver(t *testing.T) {
	repoStub := &stubRepo{}
	svc := NewService(repoStub, nil, nil)
	if _, err := svc.Upsert(context.Background(), Entry{Name: "name"}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestServiceListActive(t *testing.T) {
	repoStub := &stubRepo{
		list: []repo.Creator{
			{ID: 1, UID: "1"},
			{ID: 2, UID: "2"},
		},
	}
	svc := NewService(repoStub, nil, nil)
	creators, err := svc.ListActive(context.Background(), 10)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(creators) != 2 {
		t.Fatalf("expected 2 creators")
	}
}

func TestServiceListActiveBackfillsMissingNames(t *testing.T) {
	repoStub := &stubRepo{
		list: []repo.Creator{
			{ID: 1, UID: "123", Platform: "bilibili", Status: "active"},
			{ID: 2, UID: "456", Name: "kept", Platform: "bilibili", Status: "active"},
		},
		creators: map[int64]repo.Creator{
			1: {ID: 1, UID: "123", Platform: "bilibili", Status: "active"},
			2: {ID: 2, UID: "456", Name: "kept", Platform: "bilibili", Status: "active"},
		},
	}
	resolver := &stubResolver{
		nameByUID: map[string]string{
			"123": "resolved-name",
		},
	}
	svc := NewService(repoStub, resolver, nil)

	creators, err := svc.ListActive(context.Background(), 10)
	if err != nil {
		t.Fatalf("list active error: %v", err)
	}
	if creators[0].Name != "resolved-name" {
		t.Fatalf("expected first creator name backfilled, got %+v", creators[0])
	}
	if repoStub.updated.ID != 1 || repoStub.updated.Name != "resolved-name" {
		t.Fatalf("expected repo update for missing name, got %+v", repoStub.updated)
	}
	if creators[1].Name != "kept" {
		t.Fatalf("expected existing name kept, got %+v", creators[1])
	}
}

func TestServiceListActiveNoRepo(t *testing.T) {
	svc := NewService(nil, nil, nil)
	if _, err := svc.ListActive(context.Background(), 10); err == nil {
		t.Fatalf("expected error")
	}
}

func TestServicePatchStatus(t *testing.T) {
	repoStub := &stubRepo{
		creators: map[int64]repo.Creator{
			7: {ID: 7, UID: "777", Name: "old", Status: "active", Platform: "bilibili"},
		},
	}
	svc := NewService(repoStub, nil, nil)
	name := "new"
	status := "paused"

	creator, err := svc.Patch(context.Background(), 7, Patch{
		Name:   &name,
		Status: &status,
	})
	if err != nil {
		t.Fatalf("patch error: %v", err)
	}
	if creator.Name != "new" || creator.Status != "paused" {
		t.Fatalf("unexpected creator after patch: %+v", creator)
	}
	if repoStub.updated.ID != 7 {
		t.Fatalf("expected repo update called")
	}
}

func TestServicePatchRequiresFields(t *testing.T) {
	repoStub := &stubRepo{
		creators: map[int64]repo.Creator{
			7: {ID: 7, UID: "777", Name: "old", Status: "active", Platform: "bilibili"},
		},
	}
	svc := NewService(repoStub, nil, nil)

	if _, err := svc.Patch(context.Background(), 7, Patch{}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestServiceListActiveRepoError(t *testing.T) {
	repoStub := &stubRepo{listErr: errors.New("list")}
	svc := NewService(repoStub, nil, nil)
	if _, err := svc.ListActive(context.Background(), 10); err == nil {
		t.Fatalf("expected error")
	}
}

func TestServiceDelete(t *testing.T) {
	repoStub := &stubRepo{
		creators: map[int64]repo.Creator{
			7: {ID: 7, UID: "777", Name: "old", Status: "active", Platform: "bilibili"},
		},
	}
	svc := NewService(repoStub, nil, nil)

	if err := svc.Delete(context.Background(), 7); err != nil {
		t.Fatalf("delete error: %v", err)
	}
	if repoStub.statuses[7] != "removed" {
		t.Fatalf("expected status removed, got %+v", repoStub.statuses)
	}
}

func TestServiceDeleteNotFound(t *testing.T) {
	repoStub := &stubRepo{}
	svc := NewService(repoStub, nil, nil)

	if err := svc.Delete(context.Background(), 7); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
}

func TestServiceUpsertFromFileSkipsRemovedCreator(t *testing.T) {
	repoStub := &stubRepo{
		creators: map[int64]repo.Creator{
			7: {ID: 7, UID: "777", Name: "old", Status: "removed", Platform: "bilibili"},
		},
	}
	svc := NewService(repoStub, nil, nil)

	creator, skipped, err := svc.upsertFromFile(context.Background(), Entry{UID: "777", Name: "old"})
	if err != nil {
		t.Fatalf("upsert from file error: %v", err)
	}
	if !skipped {
		t.Fatalf("expected skipped")
	}
	if creator.ID != 7 || repoStub.count != 0 {
		t.Fatalf("expected removed creator kept untouched, creator=%+v count=%d", creator, repoStub.count)
	}
}

func TestServiceUpsertPublishesCreatorEvent(t *testing.T) {
	repoStub := &stubRepo{}
	publisher := &stubCreatorEventPublisher{}
	svc := NewService(repoStub, nil, nil)
	svc.SetPublisher(publisher)

	creator, err := svc.Upsert(context.Background(), Entry{UID: "123", Name: "name"})
	if err != nil {
		t.Fatalf("upsert error: %v", err)
	}

	evt := mustFindCreatorChangedEvent(t, publisher.events)
	payload, ok := evt.Payload.(map[string]any)
	if !ok {
		t.Fatalf("expected map payload, got %T", evt.Payload)
	}
	assertCreatorChangedPayloadShape(t, payload)
	if got := payload["id"]; got != creator.ID {
		t.Fatalf("expected id=%d, got %v", creator.ID, got)
	}
	if got := payload["status"]; got != "active" {
		t.Fatalf("expected active status, got %v", got)
	}
}

func TestServicePatchPublishesCreatorEvent(t *testing.T) {
	repoStub := &stubRepo{
		creators: map[int64]repo.Creator{
			7: {ID: 7, UID: "777", Name: "old", Status: "active", Platform: "bilibili"},
		},
	}
	publisher := &stubCreatorEventPublisher{}
	svc := NewService(repoStub, nil, nil)
	svc.SetPublisher(publisher)
	status := "paused"

	_, err := svc.Patch(context.Background(), 7, Patch{Status: &status})
	if err != nil {
		t.Fatalf("patch error: %v", err)
	}

	evt := mustFindCreatorChangedEvent(t, publisher.events)
	payload, ok := evt.Payload.(map[string]any)
	if !ok {
		t.Fatalf("expected map payload, got %T", evt.Payload)
	}
	assertCreatorChangedPayloadShape(t, payload)
	if got := payload["status"]; got != "paused" {
		t.Fatalf("expected paused status, got %v", got)
	}
}

func TestServiceDeletePublishesCreatorEvent(t *testing.T) {
	repoStub := &stubRepo{
		creators: map[int64]repo.Creator{
			7: {ID: 7, UID: "777", Name: "old", Status: "active", Platform: "bilibili"},
		},
	}
	publisher := &stubCreatorEventPublisher{}
	svc := NewService(repoStub, nil, nil)
	svc.SetPublisher(publisher)

	if err := svc.Delete(context.Background(), 7); err != nil {
		t.Fatalf("delete error: %v", err)
	}

	evt := mustFindCreatorChangedEvent(t, publisher.events)
	payload, ok := evt.Payload.(map[string]any)
	if !ok {
		t.Fatalf("expected map payload, got %T", evt.Payload)
	}
	assertCreatorChangedPayloadShape(t, payload)
	if got := payload["status"]; got != "removed" {
		t.Fatalf("expected removed status, got %v", got)
	}
}

func TestServiceListActiveBackfillNamePublishesCreatorEvent(t *testing.T) {
	repoStub := &stubRepo{
		list: []repo.Creator{
			{ID: 1, UID: "123", Platform: "bilibili", Status: "active"},
		},
		creators: map[int64]repo.Creator{
			1: {ID: 1, UID: "123", Platform: "bilibili", Status: "active"},
		},
	}
	resolver := &stubResolver{
		nameByUID: map[string]string{
			"123": "resolved-name",
		},
	}
	publisher := &stubCreatorEventPublisher{}
	svc := NewService(repoStub, resolver, nil)
	svc.SetPublisher(publisher)

	_, err := svc.ListActive(context.Background(), 10)
	if err != nil {
		t.Fatalf("list active error: %v", err)
	}

	evt := mustFindCreatorChangedEvent(t, publisher.events)
	payload, ok := evt.Payload.(map[string]any)
	if !ok {
		t.Fatalf("expected map payload, got %T", evt.Payload)
	}
	assertCreatorChangedPayloadShape(t, payload)
	if got := payload["name"]; got != "resolved-name" {
		t.Fatalf("expected resolved name, got %v", got)
	}
}
