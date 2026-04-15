package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"fetch-bilibili/internal/live"
)

var eventsBroker = live.NewBroker()
var heartbeatInterval = 15 * time.Second

func EventsBroker() *live.Broker {
	return eventsBroker
}

type eventsStreamHandler struct {
	broker *live.Broker
}

func newEventsStreamHandler(broker *live.Broker) http.Handler {
	if broker == nil {
		broker = eventsBroker
	}
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

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	if err := writeSSE(w, flusher, "hello", sseServerTimePayload()); err != nil {
		return
	}

	sub := h.broker.Subscribe(r.Context(), 32)
	heartbeatTicker := time.NewTicker(heartbeatInterval)
	defer heartbeatTicker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-heartbeatTicker.C:
			if err := writeSSE(w, flusher, "heartbeat", sseServerTimePayload()); err != nil {
				return
			}
		case evt, ok := <-sub:
			if !ok {
				return
			}
			if err := writeSSE(w, flusher, evt.Type, evt.Payload); err != nil {
				return
			}
		}
	}
}

func sseServerTimePayload() map[string]string {
	return map[string]string{"server_time": time.Now().Format(time.RFC3339)}
}

func writeSSE(w io.Writer, flusher http.Flusher, eventType string, payload any) error {
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
