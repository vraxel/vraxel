package audit

import (
	"vraxel.io/vraxel/lib/apiserver"
	libaudit "vraxel.io/vraxel/lib/audit"
	"vraxel.io/vraxel/pkg/apis/audit/store"
	"vraxel.io/vraxel/pkg/db"
)

// Registrar returns the module's route-registration closure. Building
// the stores only wraps the handle, so a nil database yields a registrar
// that declares every route without touching Postgres -- that is what
// lets openapi-gen read the real route table offline.
func Registrar(database *db.DB) func(*apiserver.Server) {
	stores := store.NewStores(database)
	return func(s *apiserver.Server) { register(s, stores) }
}

// NewAuditWriter builds the async audit log writer. Kept in the audit
// module's business layer so the top-level assembly never reaches into
// audit/store directly; apis/install.go only imports this package.
func NewAuditWriter(database *db.DB) *libaudit.Writer {
	sink := store.NewPGAuditLogStore(database)
	return libaudit.NewWriter(sink, libaudit.WriterConfig{})
}
