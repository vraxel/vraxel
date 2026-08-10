package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"vraxel.io/vraxel/lib/list"
	"vraxel.io/vraxel/pkg/db"
	"vraxel.io/vraxel/pkg/db/generated"
	"vraxel.io/vraxel/pkg/db/pgerrors"
)

type pgUserStore struct {
	db.Store
}

func NewPGUserStore(d *db.DB) UserStore {
	return &pgUserStore{Store: db.Store{DB: d}}
}

func userFromCreate(r generated.CreateUserRow) UserRow {
	return UserRow{
		ID: r.ID, Username: r.Username, Email: r.Email,
		DisplayName: r.DisplayName, Phone: r.Phone, AvatarURL: r.AvatarUrl,
		Status: r.Status, Builtin: r.Builtin,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func userFromGetByID(r generated.GetUserByIDRow) UserRow {
	return UserRow{
		ID: r.ID, Username: r.Username, Email: r.Email,
		DisplayName: r.DisplayName, Phone: r.Phone, AvatarURL: r.AvatarUrl,
		Status: r.Status, Builtin: r.Builtin,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func userFromGetByUsername(r generated.GetUserByUsernameRow) UserRow {
	return UserRow{
		ID: r.ID, Username: r.Username, Email: r.Email,
		DisplayName: r.DisplayName, Phone: r.Phone, AvatarURL: r.AvatarUrl,
		Status: r.Status, Builtin: r.Builtin,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func userFromUpdate(r generated.UpdateUserRow) UserRow {
	return UserRow{
		ID: r.ID, Username: r.Username, Email: r.Email,
		DisplayName: r.DisplayName, Phone: r.Phone, AvatarURL: r.AvatarUrl,
		Status: r.Status, Builtin: r.Builtin,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func userFromPatch(r generated.PatchUserRow) UserRow {
	return UserRow{
		ID: r.ID, Username: r.Username, Email: r.Email,
		DisplayName: r.DisplayName, Phone: r.Phone, AvatarURL: r.AvatarUrl,
		Status: r.Status, Builtin: r.Builtin,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func (s *pgUserStore) Create(ctx context.Context, input UserCreateInput) (*UserRow, error) {
	row, err := s.Q().CreateUser(ctx, generated.CreateUserParams{
		Username:    input.Username,
		Email:       input.Email,
		DisplayName: input.DisplayName,
		Phone:       input.Phone,
		AvatarUrl:   input.AvatarURL,
		Status:      input.Status,
	})
	if err != nil {
		return nil, fmt.Errorf("create user: %w", pgerrors.CheckPG(err))
	}
	u := userFromCreate(row)
	return &u, nil
}

func (s *pgUserStore) GetByID(ctx context.Context, id int64) (*UserRow, error) {
	row, err := s.Q().GetUserByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("user %d: %w", id, pgerrors.ErrNotFound)
		}
		return nil, fmt.Errorf("get user by id: %w", err)
	}
	u := userFromGetByID(row)
	return &u, nil
}

func (s *pgUserStore) GetByUsername(ctx context.Context, username string) (*UserRow, error) {
	row, err := s.Q().GetUserByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("user %s: %w", username, pgerrors.ErrNotFound)
		}
		return nil, fmt.Errorf("get user by username: %w", err)
	}
	u := userFromGetByUsername(row)
	return &u, nil
}

func (s *pgUserStore) Update(ctx context.Context, input UserUpdateInput) (*UserRow, error) {
	row, err := s.Q().UpdateUser(ctx, generated.UpdateUserParams{
		ID:          input.ID,
		Username:    input.Username,
		Email:       input.Email,
		DisplayName: input.DisplayName,
		Phone:       input.Phone,
		AvatarUrl:   input.AvatarURL,
		Status:      input.Status,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("user %d: %w", input.ID, pgerrors.ErrNotFound)
		}
		return nil, fmt.Errorf("update user: %w", pgerrors.CheckPG(err))
	}
	u := userFromUpdate(row)
	return &u, nil
}

func (s *pgUserStore) Patch(ctx context.Context, input UserPatchInput) (*UserRow, error) {
	params := generated.PatchUserParams{ID: input.ID}
	if input.Email != nil {
		params.Email = input.Email
	}
	if input.Phone != nil {
		params.Phone = input.Phone
	}
	if input.DisplayName != nil {
		params.DisplayName = input.DisplayName
	}
	if input.AvatarURL != nil {
		params.AvatarUrl = input.AvatarURL
	}
	if input.Status != nil {
		params.Status = input.Status
	}
	row, err := s.Q().PatchUser(ctx, params)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("user %d: %w", input.ID, pgerrors.ErrNotFound)
		}
		return nil, fmt.Errorf("patch user: %w", pgerrors.CheckPG(err))
	}
	u := userFromPatch(row)
	return &u, nil
}

func (s *pgUserStore) UpdateLastLogin(ctx context.Context, id int64) error {
	if err := s.Q().UpdateUserLastLogin(ctx, id); err != nil {
		return fmt.Errorf("update user last login: %w", err)
	}
	return nil
}

func (s *pgUserStore) Delete(ctx context.Context, id int64) error {
	rowsAffected, err := s.Q().DeleteUser(ctx, id)
	if err != nil {
		return fmt.Errorf("delete user: %w", pgerrors.CheckPG(err))
	}
	if rowsAffected != 0 {
		return nil
	}
	// 0 rows means either the user doesn't exist OR is builtin (DeleteUser
	// SQL filters AND NOT builtin). Distinguish so the caller surfaces the
	// right HTTP status; "builtin user, not deletable" is a forbidden op,
	// not a missing resource.
	row, getErr := s.Q().GetUserByID(ctx, id)
	if getErr != nil {
		if errors.Is(getErr, pgx.ErrNoRows) {
			return fmt.Errorf("user %d: %w", id, pgerrors.ErrNotFound)
		}
		return fmt.Errorf("delete user (probe): %w", getErr)
	}
	if row.Builtin {
		return fmt.Errorf("user %d (%s): %w: builtin user", id, row.Username, pgerrors.ErrForbidden)
	}
	return fmt.Errorf("user %d: %w", id, pgerrors.ErrNotFound)
}

func (s *pgUserStore) DeleteByIDs(ctx context.Context, ids []int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	deletedIDs, err := s.Q().DeleteUsersByIDs(ctx, ids)
	if err != nil {
		return 0, fmt.Errorf("delete users by ids: %w", pgerrors.CheckPG(err))
	}
	return int64(len(deletedIDs)), nil
}

func (s *pgUserStore) SetBuiltin(ctx context.Context, id int64, builtin bool) error {
	if err := s.Q().SetUserBuiltin(ctx, generated.SetUserBuiltinParams{
		ID:      id,
		Builtin: builtin,
	}); err != nil {
		return fmt.Errorf("set user builtin: %w", err)
	}
	return nil
}

func (s *pgUserStore) List(ctx context.Context, q list.Query) (*list.Result[UserWithNamespacesRow], error) {
	offset, limit := list.PaginationToOffsetLimit(q.Pagination)

	filterParams := generated.CountUsersParams{
		Status: list.FilterStr(q.Filters, "status"),
		Search: list.FilterStr(q.Filters, "search"),
	}

	count, err := s.Q().CountUsers(ctx, filterParams)
	if err != nil {
		return nil, fmt.Errorf("count users: %w", err)
	}

	sortOrder := q.SortOrder
	if sortOrder == "" {
		sortOrder = "desc"
	}

	rows, err := s.Q().ListUsers(ctx, generated.ListUsersParams{
		Status:     filterParams.Status,
		Search:     filterParams.Search,
		SortField:  q.SortBy,
		SortOrder:  sortOrder,
		PageOffset: offset,
		PageSize:   limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}

	items := make([]UserWithNamespacesRow, 0, len(rows))
	for _, r := range rows {
		items = append(items, UserWithNamespacesRow{
			UserRow: UserRow{
				ID:          r.ID,
				Username:    r.Username,
				Email:       r.Email,
				DisplayName: r.DisplayName,
				Phone:       r.Phone,
				AvatarURL:   r.AvatarUrl,
				Status:      r.Status,
				Builtin:     r.Builtin,
				CreatedAt:   r.CreatedAt,
				UpdatedAt:   r.UpdatedAt,
			},
			NamespaceNames: r.NamespaceNames,
		})
	}

	return &list.Result[UserWithNamespacesRow]{Items: items, TotalCount: count}, nil
}

func (s *pgUserStore) GetUserForAuth(ctx context.Context, identifier string) (*UserAuthRow, error) {
	row, err := s.Q().GetUserForAuth(ctx, identifier)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("user %s: %w", identifier, pgerrors.ErrNotFound)
		}
		return nil, fmt.Errorf("get user for auth: %w", err)
	}
	return &UserAuthRow{
		ID:           row.ID,
		Username:     row.Username,
		Email:        row.Email,
		DisplayName:  row.DisplayName,
		Phone:        row.Phone,
		Status:       row.Status,
		PasswordHash: row.PasswordHash,
	}, nil
}

func (s *pgUserStore) SetPasswordHash(ctx context.Context, id int64, hash string) error {
	if err := s.Q().SetPasswordHash(ctx, generated.SetPasswordHashParams{
		ID:           id,
		PasswordHash: hash,
	}); err != nil {
		return fmt.Errorf("set password hash: %w", err)
	}
	return nil
}
