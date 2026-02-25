package mysqlrepo

import (
	"database/sql"

	"fetch-bilibili/internal/repo"
)

type Repo struct {
	db *sql.DB
}

func New(db *sql.DB) *Repo {
	return &Repo{db: db}
}

func (r *Repo) Creators() repo.CreatorRepository {
	return &creatorRepo{db: r.db}
}

func (r *Repo) Videos() repo.VideoRepository {
	return &videoRepo{db: r.db}
}

func (r *Repo) VideoFiles() repo.VideoFileRepository {
	return &videoFileRepo{db: r.db}
}

func (r *Repo) Jobs() repo.JobRepository {
	return &jobRepo{db: r.db}
}

type creatorRepo struct {
	db *sql.DB
}

type videoRepo struct {
	db *sql.DB
}

type videoFileRepo struct {
	db *sql.DB
}

type jobRepo struct {
	db *sql.DB
}
