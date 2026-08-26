package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"vraxel.io/vraxel/pkg/apis/shared/scope"
	"vraxel.io/vraxel/pkg/db"
	"vraxel.io/vraxel/pkg/db/generated"
	"vraxel.io/vraxel/pkg/db/pgerrors"
)

// HostProcessesRow is one host's workload snapshot: the jsonb as stored,
// passed through undecoded, plus when it arrived.
//
// Undecoded because the server has no reason to look inside. The agent
// encoded a shape both sides declare in lib/agent/types, the REST layer
// decodes it into the API type for the schema generators, and a decode
// here would only be a third copy of the same struct that could drift
// from the other two.
type HostProcessesRow struct {
	Groups     []byte
	ReportedAt time.Time
}

// HostAccountsRow is one host's account inventory, on the same terms.
type HostAccountsRow struct {
	Users     []byte
	Groups    []byte
	SudoRules []byte
	// SSHD decides which of the accounts above can actually get in.
	SSHD       []byte
	ReportedAt time.Time
}

// HostRuntimeStore reads the two runtime inventories.
//
// Read-only, and separate from HostStore rather than two more methods on
// it: HostStore is the operator-facing hosts surface with several
// implementors, and neither of these participates in its list, filter or
// lifecycle machinery.
type HostRuntimeStore interface {
	// GetProcesses returns the host's workload snapshot. Returns
	// pgerrors.ErrNotFound when the host does not exist, is outside the
	// caller's scope, or has never reported -- three cases the caller
	// answers the same way, because "no such host for you" and "nothing
	// reported yet" must not be distinguishable from outside.
	GetProcesses(ctx context.Context, hostID int64, sf scope.Filter) (*HostProcessesRow, error)
	GetAccounts(ctx context.Context, hostID int64, sf scope.Filter) (*HostAccountsRow, error)
}

type pgHostRuntimeStore struct {
	db.Store
}

// NewPGHostRuntimeStore creates a PostgreSQL-backed HostRuntimeStore.
func NewPGHostRuntimeStore(d *db.DB) HostRuntimeStore {
	return &pgHostRuntimeStore{Store: db.Store{DB: d}}
}

func (s *pgHostRuntimeStore) GetProcesses(ctx context.Context, hostID int64, sf scope.Filter) (*HostProcessesRow, error) {
	row, err := s.Q().GetHostProcesses(ctx, generated.GetHostProcessesParams{
		HostID:            hostID,
		WorkspaceIDFilter: sf.WorkspaceID,
		NamespaceIDFilter: sf.NamespaceID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("host %d processes: %w", hostID, pgerrors.ErrNotFound)
		}
		return nil, fmt.Errorf("get host processes: %w", pgerrors.CheckPG(err))
	}
	return &HostProcessesRow{Groups: row.Groups, ReportedAt: row.ReportedAt}, nil
}

func (s *pgHostRuntimeStore) GetAccounts(ctx context.Context, hostID int64, sf scope.Filter) (*HostAccountsRow, error) {
	row, err := s.Q().GetHostAccounts(ctx, generated.GetHostAccountsParams{
		HostID:            hostID,
		WorkspaceIDFilter: sf.WorkspaceID,
		NamespaceIDFilter: sf.NamespaceID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("host %d accounts: %w", hostID, pgerrors.ErrNotFound)
		}
		return nil, fmt.Errorf("get host accounts: %w", pgerrors.CheckPG(err))
	}
	return &HostAccountsRow{
		Users:      row.Users,
		Groups:     row.Groups,
		SudoRules:  row.SudoRules,
		SSHD:       row.Sshd,
		ReportedAt: row.ReportedAt,
	}, nil
}
