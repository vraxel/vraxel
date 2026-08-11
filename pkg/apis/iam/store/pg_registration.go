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

// RegisterLocalInput carries a self-service password signup.
type RegisterLocalInput struct {
	Username        string
	Email           string
	DisplayName     string
	PasswordHash    string
	DefaultRoleName string
}

// SocialLoginInput carries a resolved external-provider identity from a
// GitHub / Google callback.
type SocialLoginInput struct {
	Provider        string
	Subject         string
	Email           string
	Username        string
	DisplayName     string
	AvatarURL       string
	DefaultRoleName string
}

type pgRegistrationStore struct {
	db.Store
}

// NewPGRegistrationStore creates the provisioning store used by the public
// registration and social-login handlers. It spans users / user_identities /
// roles / role_bindings inside one transaction (same pattern as SeedRBAC).
func NewPGRegistrationStore(d *db.DB) RegistrationStore {
	return &pgRegistrationStore{Store: db.Store{DB: d}}
}

// IsConflict reports whether err is a unique/constraint conflict (e.g. a
// duplicate username or email). It lets handlers outside the store layer map
// registration failures to 409 without importing pkg/db.
func IsConflict(err error) bool {
	return errors.Is(err, pgerrors.ErrConflict)
}

func (s *pgRegistrationStore) RegisterLocal(ctx context.Context, in RegisterLocalInput) (*UserRow, error) {
	return db.WithTxReturning(ctx, s.DB, func(ctx context.Context, q *generated.Queries) (*UserRow, error) {
		row, err := q.CreateUser(ctx, generated.CreateUserParams{
			Username: in.Username, Email: in.Email, DisplayName: in.DisplayName,
			Phone: "", AvatarUrl: "", Status: "active",
		})
		if err != nil {
			return nil, fmt.Errorf("create user: %w", pgerrors.CheckPG(err))
		}
		if err := q.SetPasswordHash(ctx, generated.SetPasswordHashParams{ID: row.ID, PasswordHash: in.PasswordHash}); err != nil {
			return nil, fmt.Errorf("set password hash: %w", err)
		}
		if err := bindDefaultPlatformRole(ctx, q, row.ID, in.DefaultRoleName); err != nil {
			return nil, err
		}
		u := userFromCreate(row)
		return &u, nil
	})
}

func (s *pgRegistrationStore) FindOrCreateSocial(ctx context.Context, in SocialLoginInput) (*UserRow, error) {
	return db.WithTxReturning(ctx, s.DB, func(ctx context.Context, q *generated.Queries) (*UserRow, error) {
		// 1. Existing linked identity -> return its user.
		ident, err := q.GetUserIdentity(ctx, generated.GetUserIdentityParams{Provider: in.Provider, ProviderSubject: in.Subject})
		if err == nil {
			return getUserRowByID(ctx, q, ident.UserID)
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("get user identity: %w", err)
		}

		// 2. No identity yet. Link to an existing local user carrying the same
		// (provider-verified) email if one exists; otherwise create a fresh
		// user. Email match is safe because the caller only passes an email
		// the provider marked verified -- EXCEPT for builtin accounts: the
		// seeded admin's default email is a placeholder the deployer may not
		// own, so whoever does control that mailbox could verify it at the
		// provider and take over admin. Builtin users never auto-link.
		var userID int64
		if in.Email != "" {
			u, err := q.GetUserByEmail(ctx, in.Email)
			switch {
			case err == nil && u.Builtin:
				return nil, fmt.Errorf("social identity matches builtin user %q by email: %w", u.Username, pgerrors.ErrForbidden)
			case err == nil:
				userID = u.ID
			case !errors.Is(err, pgx.ErrNoRows):
				return nil, fmt.Errorf("get user by email: %w", err)
			}
		}
		if userID == 0 {
			row, err := q.CreateUser(ctx, generated.CreateUserParams{
				Username: in.Username, Email: in.Email, DisplayName: in.DisplayName,
				Phone: "", AvatarUrl: in.AvatarURL, Status: "active",
			})
			if err != nil {
				return nil, fmt.Errorf("create user: %w", pgerrors.CheckPG(err))
			}
			userID = row.ID
			if err := bindDefaultPlatformRole(ctx, q, userID, in.DefaultRoleName); err != nil {
				return nil, err
			}
		}

		// 3. Link the external identity to the resolved user.
		if _, err := q.CreateUserIdentity(ctx, generated.CreateUserIdentityParams{
			UserID: userID, Provider: in.Provider, ProviderSubject: in.Subject, Email: in.Email,
		}); err != nil {
			return nil, fmt.Errorf("create user identity: %w", pgerrors.CheckPG(err))
		}

		return getUserRowByID(ctx, q, userID)
	})
}

func getUserRowByID(ctx context.Context, q *generated.Queries, id int64) (*UserRow, error) {
	row, err := q.GetUserByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get user by id: %w", err)
	}
	u := userFromGetByID(row)
	return &u, nil
}

// bindDefaultPlatformRole grants a newly provisioned user the given
// platform-scoped built-in role (idempotent via ON CONFLICT DO NOTHING).
// Without it a self-registered user holds zero permissions and the SPA
// bounces them to /error?status=403 right after login.
func bindDefaultPlatformRole(ctx context.Context, q *generated.Queries, userID int64, roleName string) error {
	if roleName == "" {
		return nil
	}
	role, err := q.GetRoleByName(ctx, roleName)
	if err != nil {
		return fmt.Errorf("get default role %q: %w", roleName, err)
	}
	if err := q.CreateRoleBindingIfNotExists(ctx, generated.CreateRoleBindingIfNotExistsParams{
		UserID: userID, RoleID: role.ID, Scope: "platform", IsOwner: false,
	}); err != nil {
		return fmt.Errorf("bind default role: %w", err)
	}
	return nil
}
