package internal

import (
	"context"
	"database/sql"
	"time"
)

type Store struct {
	db  *sql.DB
	idg *Generator
}

func NewStore(db *sql.DB) *Store {
	return &Store{
		db:  db,
		idg: NewGenerator(),
	}
}

func (s *Store) SaveDistributor(ctx context.Context, name string) error {
	ulid := s.idg.New().String()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO data (id, name, created_at) VALUES (?, ?, ?)`,
		ulid, name, time.Now().UTC(),
	)
	return err
}
