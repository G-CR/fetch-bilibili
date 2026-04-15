package live

import (
	"context"
	"testing"
	"time"
)

func TestBrokerPublishToMultipleSubscribers(t *testing.T) {
	broker := NewBroker()
	ctx1, cancel1 := context.WithCancel(context.Background())
	defer cancel1()
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()

	sub1 := broker.Subscribe(ctx1, 1)
	sub2 := broker.Subscribe(ctx2, 1)

	evt := Event{ID: "e-1", Type: "video.updated", At: time.Now(), Payload: map[string]any{"id": 1}}
	broker.Publish(evt)

	select {
	case got := <-sub1:
		if got.ID != evt.ID || got.Type != evt.Type {
			t.Fatalf("sub1 unexpected event: %+v", got)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("sub1 did not receive event")
	}

	select {
	case got := <-sub2:
		if got.ID != evt.ID || got.Type != evt.Type {
			t.Fatalf("sub2 unexpected event: %+v", got)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("sub2 did not receive event")
	}
}

func TestBrokerAutoRemoveOnContextDone(t *testing.T) {
	broker := NewBroker()
	ctx, cancel := context.WithCancel(context.Background())
	sub := broker.Subscribe(ctx, 1)

	if got := broker.subscriberCount(); got != 1 {
		t.Fatalf("expected 1 subscriber, got %d", got)
	}

	cancel()

	select {
	case _, ok := <-sub:
		if ok {
			t.Fatal("expected subscriber channel to close")
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("subscriber channel was not closed after context done")
	}

	deadline := time.After(200 * time.Millisecond)
	for {
		if broker.subscriberCount() == 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("expected subscriber removed, still have %d", broker.subscriberCount())
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
}

func TestBrokerDropSlowSubscriberWithoutBlockingPublish(t *testing.T) {
	broker := NewBroker()

	slowCtx, slowCancel := context.WithCancel(context.Background())
	defer slowCancel()
	fastCtx, fastCancel := context.WithCancel(context.Background())
	defer fastCancel()

	slowSub := broker.Subscribe(slowCtx, 1)
	fastSub := broker.Subscribe(fastCtx, 2)

	broker.Publish(Event{ID: "first", Type: "tick", At: time.Now()})

	done := make(chan struct{})
	go func() {
		broker.Publish(Event{ID: "second", Type: "tick", At: time.Now()})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("publish blocked by slow subscriber")
	}

	select {
	case got := <-fastSub:
		if got.ID != "first" {
			t.Fatalf("fast subscriber first event mismatch: %+v", got)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("fast subscriber did not receive first event")
	}

	select {
	case got := <-fastSub:
		if got.ID != "second" {
			t.Fatalf("fast subscriber second event mismatch: %+v", got)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("fast subscriber did not receive second event")
	}

	deadline := time.After(200 * time.Millisecond)
	for {
		select {
		case _, ok := <-slowSub:
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("slow subscriber was not closed when buffer filled")
		}
	}
}

func TestBrokerSubscribeHonorsZeroBufferAndDropsUnreadSubscriber(t *testing.T) {
	broker := NewBroker()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sub := broker.Subscribe(ctx, 0)

	broker.Publish(Event{ID: "instant-drop", Type: "tick", At: time.Now()})

	select {
	case _, ok := <-sub:
		if ok {
			t.Fatal("expected zero-buffer subscriber to be closed when unread")
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected zero-buffer subscriber to be dropped immediately")
	}

	if got := broker.subscriberCount(); got != 0 {
		t.Fatalf("expected subscriber to be removed, got %d", got)
	}
}
