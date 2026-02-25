package creator

import (
	"context"
	"errors"
	"testing"

	"fetch-bilibili/internal/repo"
)

type stubRepo struct {
	last      repo.Creator
	id        int64
	err       error
	count     int
	list      []repo.Creator
	listErr   error
	statuses  map[int64]string
	statusErr error
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
	return repo.ErrNotImplemented
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
	return repo.Creator{}, repo.ErrNotImplemented
}

func (s *stubRepo) ListActive(ctx context.Context, limit int) ([]repo.Creator, error) {
	return nil, repo.ErrNotImplemented
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

type stubResolver struct {
	uid  string
	name string
	err  error
}

func (s *stubResolver) ResolveUID(ctx context.Context, keyword string) (string, string, error) {
	if s.err != nil {
		return "", "", s.err
	}
	return s.uid, s.name, nil
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
