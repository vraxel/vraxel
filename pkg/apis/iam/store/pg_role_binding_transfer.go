package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"vraxel.io/vraxel/pkg/db"
	"vraxel.io/vraxel/pkg/db/generated"
	"vraxel.io/vraxel/pkg/db/pgerrors"
)

func (s *pgRoleBindingStore) TransferOwnership(ctx context.Context, scope string, resourceID int64, callerID int64, callerIsPlatformAdmin bool, newOwnerUserID int64, adminRoleName string) (int64, error) {
	if scope != ScopeWorkspace && scope != ScopeNamespace {
		return 0, fmt.Errorf("unsupported scope for ownership transfer: %s", scope)
	}

	oldOwnerUserID, err := db.WithTxReturning(ctx, s.DB, func(ctx context.Context, q *generated.Queries) (int64, error) {
		return transferOwnershipTx(ctx, q, scope, resourceID, callerID, callerIsPlatformAdmin, newOwnerUserID, adminRoleName)
	})
	if err != nil {
		return 0, err
	}

	s.notifyUserChange(ctx, oldOwnerUserID)
	s.notifyUserChange(ctx, newOwnerUserID)
	return oldOwnerUserID, nil
}

// transferOwnershipTx runs the ownership-transfer mutations within an open
// transaction: lock + load the current owner, authorize the caller, verify the
// new owner is a member, clear the current owner, look up the scoped admin
// role, and upsert the new owner binding. Returns the old owner's user ID.
func transferOwnershipTx(ctx context.Context, q *generated.Queries, scope string, resourceID int64, callerID int64, callerIsPlatformAdmin bool, newOwnerUserID int64, adminRoleName string) (int64, error) {
	oldOwner, err := findOwnerForUpdate(ctx, q, scope, resourceID)
	if err != nil {
		return 0, err
	}

	if !callerIsPlatformAdmin && callerID != oldOwner {
		return 0, fmt.Errorf("only the current owner or platform admin can transfer ownership: %w", pgerrors.ErrForbidden)
	}

	isMember, err := checkMembership(ctx, q, scope, resourceID, newOwnerUserID)
	if err != nil {
		return 0, fmt.Errorf("check membership: %w", err)
	}
	if !isMember {
		return 0, fmt.Errorf("new owner must be a member of the resource: %w", pgerrors.ErrBadRequest)
	}

	if err := clearOwner(ctx, q, scope, resourceID); err != nil {
		return 0, fmt.Errorf("clear current ownership: %w", err)
	}

	adminRoleID, err := lookupAdminRoleID(ctx, q, scope, resourceID, adminRoleName)
	if err != nil {
		return 0, err
	}

	if err := upsertOwner(ctx, q, scope, resourceID, newOwnerUserID, adminRoleID); err != nil {
		return 0, fmt.Errorf("upsert owner binding: %w", err)
	}
	return oldOwner, nil
}

func findOwnerForUpdate(ctx context.Context, q *generated.Queries, scope string, resourceID int64) (int64, error) {
	rid := &resourceID
	var (
		userID int64
		err    error
	)
	if scope == ScopeWorkspace {
		userID, err = q.GetWorkspaceOwnerForUpdate(ctx, rid)
	} else {
		userID, err = q.GetNamespaceOwnerForUpdate(ctx, rid)
	}
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, fmt.Errorf("owner for scope=%s id=%d: %w", scope, resourceID, pgerrors.ErrNotFound)
		}
		return 0, fmt.Errorf("find current owner: %w", err)
	}
	return userID, nil
}

func checkMembership(ctx context.Context, q *generated.Queries, scope string, resourceID, userID int64) (bool, error) {
	rid := &resourceID
	if scope == ScopeWorkspace {
		return q.ExistsWorkspaceMember(ctx, generated.ExistsWorkspaceMemberParams{
			UserID:      userID,
			WorkspaceID: rid,
		})
	}
	return q.ExistsNamespaceMember(ctx, generated.ExistsNamespaceMemberParams{
		UserID:      userID,
		NamespaceID: rid,
	})
}

func clearOwner(ctx context.Context, q *generated.Queries, scope string, resourceID int64) error {
	rid := &resourceID
	if scope == ScopeWorkspace {
		return q.ClearWorkspaceOwner(ctx, rid)
	}
	return q.ClearNamespaceOwner(ctx, rid)
}

func lookupAdminRoleID(ctx context.Context, q *generated.Queries, scope string, resourceID int64, adminRoleName string) (int64, error) {
	rid := &resourceID
	var (
		roleID int64
		err    error
	)
	if scope == ScopeWorkspace {
		row, qerr := q.GetRoleByNameAndWorkspace(ctx, generated.GetRoleByNameAndWorkspaceParams{
			Name:        adminRoleName,
			WorkspaceID: rid,
		})
		roleID, err = row.ID, qerr
	} else {
		row, qerr := q.GetRoleByNameAndNamespace(ctx, generated.GetRoleByNameAndNamespaceParams{
			Name:        adminRoleName,
			NamespaceID: rid,
		})
		roleID, err = row.ID, qerr
	}
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, fmt.Errorf("admin role %q not found", adminRoleName)
		}
		return 0, fmt.Errorf("look up admin role: %w", err)
	}
	return roleID, nil
}

func upsertOwner(ctx context.Context, q *generated.Queries, scope string, resourceID, newOwnerUserID, adminRoleID int64) error {
	if scope == ScopeWorkspace {
		rid := &resourceID
		return q.UpsertWorkspaceOwner(ctx, generated.UpsertWorkspaceOwnerParams{
			UserID:      newOwnerUserID,
			RoleID:      adminRoleID,
			WorkspaceID: rid,
		})
	}

	wsID, err := q.GetNamespaceWorkspaceID(ctx, resourceID)
	if err != nil {
		return fmt.Errorf("look up namespace workspace: %w", err)
	}
	nsID := &resourceID
	wsIDPtr := &wsID
	return q.UpsertNamespaceOwner(ctx, generated.UpsertNamespaceOwnerParams{
		UserID:      newOwnerUserID,
		RoleID:      adminRoleID,
		WorkspaceID: wsIDPtr,
		NamespaceID: nsID,
	})
}
