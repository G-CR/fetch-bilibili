package httpapi

import (
	"context"
	"net/http"

	"fetch-bilibili/internal/creator"
	"fetch-bilibili/internal/repo"
)

type CreatorService interface {
	Upsert(ctx context.Context, entry creator.Entry) (repo.Creator, error)
}

func NewRouter(creatorSvc CreatorService) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready"))
	})

	mux.Handle("/creators", newCreatorHandler(creatorSvc))

	return mux
}
