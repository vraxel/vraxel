package audit

import (
	"vraxel.io/vraxel/lib/apiserver"
	libaudit "vraxel.io/vraxel/lib/audit"
	"vraxel.io/vraxel/pkg/apis/audit/store"
	"vraxel.io/vraxel/pkg/db"
)

// ModuleResult holds the output of Audit module initialization.
// Served by the v2 apiserver (strangler migration).
type ModuleResult struct {
	Register func(*apiserver.Server)
}

// NewModule initializes the Audit module.
func NewModule(database *db.DB) ModuleResult {
	stores := store.NewStores(database)
	return ModuleResult{
		Register: func(s *apiserver.Server) { register(s, stores) },
	}
}

// NewAuditWriter builds the async audit log writer. Kept in the audit
// module's business layer so the top-level assembly never reaches into
// audit/store directly; apis/install.go only imports this package.
func NewAuditWriter(database *db.DB) *libaudit.Writer {
	sink := store.NewPGAuditLogStore(database)
	return libaudit.NewWriter(sink, libaudit.WriterConfig{})
}
