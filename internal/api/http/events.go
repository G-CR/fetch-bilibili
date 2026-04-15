package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"fetch-bilibili/internal/live"
)

var heartbeatInterval = 15 * time.Second

type eventSubscriber interface {
	Subscribe(ctx context.Context, buffer int) <-chan live.Event
}

type eventsStreamHandler struct {
	broker eventSubscriber
}

func newEventsStreamHandler(broker eventSubscriber) http.Handler {
	return &eventsStreamHandler{broker: broker}
}

func (h *eventsStreamHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if h.broker == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	controller := http.NewResponseController(w)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	sub := h.broker.Subscribe(ctx, 32)
	heartbeatTicker := time.NewTicker(heartbeatInterval)
	defer heartbeatTicker.Stop()

	if err := writeSSE(w, flusher, controller, "hello", sseServerTimePayload()); err != nil {
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-heartbeatTicker.C:
			if err := writeSSE(w, flusher, controller, "heartbeat", sseServerTimePayload()); err != nil {
				return
			}
		case evt, ok := <-sub:
			if !ok {
				return
			}
			if err := writeSSE(w, flusher, controller, evt.Type, evt.Payload); err != nil {
				return
			}
		}
	}
}

func sseServerTimePayload() map[string]string {
	return map[string]string{"server_time": time.Now().Format(time.RFC3339)}
}

func writeSSE(w io.Writer, flusher http.Flusher, controller *http.ResponseController, eventType string, payload any) error {
	if err := clearWriteDeadline(controller); err != nil {
		return err
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err = fmt.Fprintf(w, "event: %s\n", eventType); err != nil {
		return err
	}
	if _, err = fmt.Fprintf(w, "data: %s\n\n", encoded); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

func clearWriteDeadline(controller *http.ResponseController) error {
	if controller == nil {
		return nil
	}

	if err := controller.SetWriteDeadline(time.Time{}); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return err
	}

	return nil
}
