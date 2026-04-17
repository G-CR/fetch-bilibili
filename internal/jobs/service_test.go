package jobs

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"fetch-bilibili/internal/live"
	"fetch-bilibili/internal/repo"
)

type stubJobRepo struct {
	last      repo.Job
	err       error
	enqueueID int64
}

func (s *stubJobRepo) Enqueue(ctx context.Context, job repo.Job) (int64, error) {
	s.last = job
	id := s.enqueueID
	if id == 0 {
		id = 1
	}
	return id, s.err
}

func (s *stubJobRepo) FetchQueued(ctx context.Context, limit int) ([]repo.Job, error) {
	return nil, nil
}

func (s *stubJobRepo) UpdateStatus(ctx context.Context, id int64, status string, errMsg string) error {
	return nil
}

func (s *stubJobRepo) ListRecent(ctx context.Context, filter repo.JobListFilter) ([]repo.Job, error) {
	return nil, nil
}

func (s *stubJobRepo) CountByStatuses(ctx context.Context, statuses []string) (int64, error) {
	return 0, nil
}

type stubEventPublisher struct {
	events []live.Event
}

func (s *stubEventPublisher) Publish(evt live.Event) {
	s.events = append(s.events, evt)
}

func expectJobSnapshotPayload(t *testing.T, payload map[string]any) {
	t.Helper()

	requiredKeys := []string{
		"id",
		"type",
		"status",
		"payload",
		"error_msg",
		"not_before",
		"started_at",
		"finished_at",
		"created_at",
		"updated_at",
	}
	for _, key := range requiredKeys {
		if _, ok := payload[key]; !ok {
			t.Fatalf("expected payload key %q", key)
		}
	}

	stringKeys := []string{"error_msg", "not_before", "started_at", "finished_at", "created_at", "updated_at"}
	for _, key := range stringKeys {
		if _, ok := payload[key].(string); !ok {
			t.Fatalf("expected %s as string, got %T", key, payload[key])
		}
	}
}

func TestEnqueueFetch(t *testing.T) {
	repo := &stubJobRepo{}
	svc := NewService(repo, nil)
	if err := svc.EnqueueFetch(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.last.Type != TypeFetch {
		t.Fatalf("expected type fetch")
	}
	if repo.last.Status != StatusQueued {
		t.Fatalf("expected status queued")
	}
}

func TestEnqueueCleanup(t *testing.T) {
	repo := &stubJobRepo{}
	svc := NewService(repo, nil)
	if err := svc.EnqueueCleanup(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.last.Type != TypeCleanup {
		t.Fatalf("expected type cleanup")
	}
}

func TestEnqueueCheck(t *testing.T) {
	repo := &stubJobRepo{}
	svc := NewService(repo, nil)
	if err := svc.EnqueueCheck(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.last.Type != TypeCheck {
		t.Fatalf("expected type check")
	}
}

func TestEnqueueError(t *testing.T) {
	repo := &stubJobRepo{err: errors.New("fail")}
	svc := NewService(repo, nil)
	if err := svc.EnqueueCheck(context.Background()); err == nil {
		t.Fatalf("expected error")
	}
}

func TestEnqueueCheckIgnoresDuplicate(t *testing.T) {
	repo := &stubJobRepo{err: ErrJobAlreadyActive}
	svc := NewService(repo, nil)
	if err := svc.EnqueueCheck(context.Background()); err != nil {
		t.Fatalf("expected duplicate to be ignored, got %v", err)
	}
}

func TestEnqueueDownload(t *testing.T) {
	repo := &stubJobRepo{}
	svc := NewService(repo, nil)
	if err := svc.EnqueueDownload(context.Background(), 9); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.last.Type != TypeDownload || repo.last.Payload["video_id"] != int64(9) {
		t.Fatalf("unexpected job: %+v", repo.last)
	}
}

func TestEnqueueCheckVideo(t *testing.T) {
	repo := &stubJobRepo{}
	svc := NewService(repo, nil)
	if err := svc.EnqueueCheckVideo(context.Background(), 10); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.last.Type != TypeCheck || repo.last.Payload["video_id"] != int64(10) {
		t.Fatalf("unexpected job: %+v", repo.last)
	}
}

func TestEnqueueDiscover(t *testing.T) {
	repo := &stubJobRepo{}
	svc := NewService(repo, nil)
	if err := svc.EnqueueDiscover(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.last.Type != TypeDiscover {
		t.Fatalf("expected type discover, got %+v", repo.last)
	}
	if repo.last.Status != StatusQueued {
		t.Fatalf("expected status queued, got %+v", repo.last)
	}
}

func TestEnqueueFetchCreator(t *testing.T) {
	repo := &stubJobRepo{}
	svc := NewService(repo, nil)
	if err := svc.EnqueueFetchCreator(context.Background(), 123); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.last.Type != TypeFetch {
		t.Fatalf("expected type fetch, got %+v", repo.last)
	}
	if len(repo.last.Payload) != 1 || repo.last.Payload["creator_id"] != int64(123) {
		t.Fatalf("expected creator-scoped payload, got %+v", repo.last.Payload)
	}
}

func TestEnqueueMethodsPublishesEvent(t *testing.T) {
	testCases := []struct {
		name          string
		run           func(*Service) error
		wantType      string
		wantPayload   map[string]any
		wantEnqueueID int64
	}{
		{
			name: "fetch",
			run: func(svc *Service) error {
				return svc.EnqueueFetch(context.Background())
			},
			wantType:      TypeFetch,
			wantEnqueueID: 11,
		},
		{
			name: "check",
			run: func(svc *Service) error {
				return svc.EnqueueCheck(context.Background())
			},
			wantType:      TypeCheck,
			wantEnqueueID: 12,
		},
		{
			name: "cleanup",
			run: func(svc *Service) error {
				return svc.EnqueueCleanup(context.Background())
			},
			wantType:      TypeCleanup,
			wantEnqueueID: 13,
		},
		{
			name: "download",
			run: func(svc *Service) error {
				return svc.EnqueueDownload(context.Background(), 88)
			},
			wantType:      TypeDownload,
			wantPayload:   map[string]any{"video_id": int64(88)},
			wantEnqueueID: 14,
		},
		{
			name: "check video",
			run: func(svc *Service) error {
				return svc.EnqueueCheckVideo(context.Background(), 99)
			},
			wantType:      TypeCheck,
			wantPayload:   map[string]any{"video_id": int64(99)},
			wantEnqueueID: 15,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &stubJobRepo{enqueueID: tc.wantEnqueueID}
			publisher := &stubEventPublisher{}
			svc := NewService(repo, publisher)

			if err := tc.run(svc); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(publisher.events) != 1 {
				t.Fatalf("expected one event, got %d", len(publisher.events))
			}

			evt := publisher.events[0]
			if evt.Type != "job.changed" {
				t.Fatalf("expected type job.changed, got %s", evt.Type)
			}
			payload, ok := evt.Payload.(map[string]any)
			if !ok {
				t.Fatalf("expected map payload, got %T", evt.Payload)
			}
			if payload["id"] != tc.wantEnqueueID {
				t.Fatalf("expected id %d, got %#v", tc.wantEnqueueID, payload["id"])
			}
			if payload["type"] != tc.wantType {
				t.Fatalf("expected type %s, got %#v", tc.wantType, payload["type"])
			}
			if payload["status"] != StatusQueued {
				t.Fatalf("expected status queued, got %#v", payload["status"])
			}
			expectJobSnapshotPayload(t, payload)
			if gotPayload, ok := payload["payload"].(map[string]any); len(tc.wantPayload) == 0 {
				if ok && len(gotPayload) > 0 {
					t.Fatalf("expected empty payload, got %#v", gotPayload)
				}
			} else {
				if !ok {
					t.Fatalf("expected map payload field, got %T", payload["payload"])
				}
				if gotPayload["video_id"] != tc.wantPayload["video_id"] {
					t.Fatalf("expected video_id %#v, got %#v", tc.wantPayload["video_id"], gotPayload["video_id"])
				}
			}
			if payload["error_msg"] != "" {
				t.Fatalf("expected empty error_msg, got %#v", payload["error_msg"])
			}
			if payload["started_at"] != "" {
				t.Fatalf("expected empty started_at, got %#v", payload["started_at"])
			}
			if payload["finished_at"] != "" {
				t.Fatalf("expected empty finished_at, got %#v", payload["finished_at"])
			}
			if payload["created_at"] == "" || payload["updated_at"] == "" {
				t.Fatalf("expected created_at/updated_at non-empty, got created_at=%#v updated_at=%#v", payload["created_at"], payload["updated_at"])
			}
		})
	}
}

func TestEnqueueDuplicatePublishesEventOnlyOnSuccess(t *testing.T) {
	repo := &stubJobRepo{err: ErrJobAlreadyActive}
	publisher := &stubEventPublisher{}
	svc := NewService(repo, publisher)

	if err := svc.EnqueueCheck(context.Background()); err != nil {
		t.Fatalf("expected duplicate to be ignored, got %v", err)
	}
	if len(publisher.events) != 0 {
		t.Fatalf("expected no event for duplicate enqueue, got %d", len(publisher.events))
	}
}

func TestEnqueueErrorDoesNotPublishEvent(t *testing.T) {
	repo := &stubJobRepo{err: errors.New("enqueue failed")}
	publisher := &stubEventPublisher{}
	svc := NewService(repo, publisher)

	err := svc.EnqueueFetch(context.Background())
	if err == nil {
		t.Fatalf("expected enqueue error")
	}
	if got := fmt.Sprintf("%v", err); got != "enqueue failed" {
		t.Fatalf("expected enqueue failed error, got %s", got)
	}
	if len(publisher.events) != 0 {
		t.Fatalf("expected no event when enqueue fails, got %d", len(publisher.events))
	}
}
