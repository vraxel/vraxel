package db

import "vraxel.io/vraxel/pkg/db/generated"

// Store is the embeddable base for every pg store impl in pkg/apis/*/store/.
// It holds the DB handle and exposes Q() as the canonical accessor for the
// current *generated.Queries. Calling Q() per-method (rather than caching
// Queries at construction time) keeps stores correct across DB.Reload.
type Store struct {
	DB *DB
}

// Q returns the Queries bound to the current pool.
func (s *Store) Q() *generated.Queries { return s.DB.GetQueries() }
