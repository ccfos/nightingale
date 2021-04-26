package postgresql

import (
	"database/sql"
)

type Server struct {
	db *sql.DB
}
// NewServer establishes a new connection using DSN.
func NewServer(dsn string) (*Server, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	s := &Server{
		db:     db,
	}
	return s, nil
}

// Close disconnects from Postgres.
func (s *Server) Close() error {
	return s.db.Close()
}

// Ping checks connection availability and possibly invalidates the connection if it fails.
func (s *Server) Ping() error {
	if err := s.db.Ping(); err != nil {
		if cerr := s.Close(); cerr != nil {
			return cerr
		}
		return err
	}
	return nil
}
