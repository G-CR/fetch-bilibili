package live

import (
	"context"
	"sync"
	"time"
)

// Event is a single message distributed by Broker.
type Event struct {
	ID      string    `json:"id"`
	Type    string    `json:"type"`
	At      time.Time `json:"at"`
	Payload any       `json:"payload"`
}

// Broker delivers events to multiple in-process subscribers.
type Broker struct {
	mu          sync.Mutex
	subscribers map[int]chan Event
	nextID      int
}

func NewBroker() *Broker {
	return &Broker{subscribers: make(map[int]chan Event)}
}

func (b *Broker) Subscribe(ctx context.Context, buffer int) <-chan Event {
	ch := make(chan Event, buffer)

	b.mu.Lock()
	subID := b.nextID
	b.nextID++
	b.subscribers[subID] = ch
	b.mu.Unlock()

	go func() {
		<-ctx.Done()
		b.removeSubscriber(subID)
	}()

	return ch
}

func (b *Broker) Publish(evt Event) {
	b.mu.Lock()
	for id, ch := range b.subscribers {
		select {
		case ch <- evt:
		default:
			close(ch)
			delete(b.subscribers, id)
		}
	}
	b.mu.Unlock()
}

func (b *Broker) removeSubscriber(id int) {
	b.mu.Lock()
	ch, ok := b.subscribers[id]
	if ok {
		delete(b.subscribers, id)
		close(ch)
	}
	b.mu.Unlock()
}

func (b *Broker) subscriberCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.subscribers)
}
