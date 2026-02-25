package mysqlrepo

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestRepoFactories(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	r := New(db)
	if r.Creators() == nil {
		t.Fatalf("expected creators repo")
	}
	if r.Videos() == nil {
		t.Fatalf("expected videos repo")
	}
	if r.VideoFiles() == nil {
		t.Fatalf("expected video files repo")
	}
	if r.Jobs() == nil {
		t.Fatalf("expected jobs repo")
	}
}
