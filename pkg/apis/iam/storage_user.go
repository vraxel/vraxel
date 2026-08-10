package iam

import (
	"context"
	"encoding/json"
	"fmt"

	apierrors "vraxel.io/vraxel/lib/api/errors"
	"vraxel.io/vraxel/lib/apiserver"
	"vraxel.io/vraxel/lib/list"
	"vraxel.io/vraxel/lib/logger"
	"vraxel.io/vraxel/lib/oidc"
	"vraxel.io/vraxel/lib/runtime"
	modstore "vraxel.io/vraxel/pkg/apis/iam/store"
)

// ===== userOps 用户存储 =====

// userOps 用户资源的典型 Ops 实现，支持 CRUD、批量删除和密码管理。
type userOps struct {
	dbStore    modstore.UserStore
	hashPasswd PasswordHasher
}

// UsersDef declares the platform users resource: full CRUD + the two
// password actions + the four read verbs. The self-user rule (a user may
// always read/change-password themselves) rides the ExtraAllow hook,
// replacing v1's hardcoded isSelfUserQuery URL sniffing.
func UsersDef(s modstore.Stores) apiserver.ResourceDef[User] {
	o := userOps{dbStore: s.User, hashPasswd: HashPassword}
	return apiserver.ResourceDef[User]{
		Group: "iam", Name: "users",
		Ops: apiserver.Ops[User]{
			List:        o.list,
			Get:         o.get,
			Create:      o.create,
			Update:      o.update,
			Patch:       o.patch,
			Delete:      o.delete,
			BatchDelete: o.batchDelete,
		},
		Sensitive: true,
		Actions: []apiserver.ActionDef{
			apiserver.Action("change-password", "POST", []string{"iam:users:change-password"},
				NewChangePasswordAction(s.User, s.RefreshToken, HashPassword, VerifyPassword)),
			apiserver.Action("reset-password", "POST", []string{"iam:users:reset-password"},
				NewResetPasswordAction(s.User, s.RefreshToken, HashPassword)),
		},
		Verbs: []apiserver.VerbDef{
			apiserver.Verb("workspaces", NewUserWorkspacesVerb(s.RoleBinding)),
			apiserver.Verb("namespaces", NewUserNamespacesVerb(s.RoleBinding)),
			apiserver.Verb("rolebindings", NewUserRoleBindingsVerb(s.RoleBinding)),
			apiserver.VerbAny("permissions", NewUserPermissionsVerb(s.RoleBinding, s.Permission)),
		},
		ExtraAllow: selfUserAllow,
	}
}

// get 获取用户详情。
// +openapi:summary=获取用户详情
func (o userOps) get(ctx apiserver.Ctx, id int64) (*User, error) {
	user, err := o.dbStore.GetByID(ctx, id)
	if err != nil {
		return nil, domainErr(err)
	}

	return userToAPI(user), nil
}

// list 获取用户列表，支持分页、排序和筛选。
// +openapi:summary=获取用户列表
func (o userOps) list(ctx apiserver.Ctx, query list.Query) (*list.Result[User], error) {
	result, err := o.dbStore.List(ctx, query)
	if err != nil {
		return nil, domainErr(err)
	}

	items := make([]User, len(result.Items))
	for i, item := range result.Items {
		items[i] = *userWithNamespacesToAPI(&item)
	}

	return &list.Result[User]{Items: items, TotalCount: result.TotalCount}, nil
}

// create 创建用户。如果提供了密码，会进行密码策略验证并使用 bcrypt 哈希存储。
// +openapi:summary=创建用户
func (o userOps) create(ctx apiserver.Ctx, user *User) (*User, error) {
	if errs := ValidateUserCreate(&user.Spec); errs.HasErrors() {
		return nil, apierrors.NewBadRequest("validation failed", errs)
	}

	// Validate and hash password if provided
	if user.Spec.Password != "" {
		if errs := ValidatePassword(user.Spec.Password); errs.HasErrors() {
			return nil, apierrors.NewBadRequest("validation failed", errs)
		}
	}

	if ctx.DryRun {
		user.Spec.Password = ""
		return user, nil
	}

	created, err := o.dbStore.Create(ctx, modstore.UserCreateInput{
		Username:    user.Spec.Username,
		Email:       user.Spec.Email,
		DisplayName: user.Spec.DisplayName,
		Phone:       user.Spec.Phone,
		AvatarURL:   user.Spec.AvatarURL,
		Status:      user.Spec.Status,
	})
	if err != nil {
		return nil, domainErr(err)
	}

	// Hash and store password after user creation
	if user.Spec.Password != "" && o.hashPasswd != nil {
		hash, err := o.hashPasswd(user.Spec.Password)
		if err != nil {
			return nil, apierrors.NewInternalError(fmt.Errorf("hash password: %w", err))
		}
		if err := o.dbStore.SetPasswordHash(ctx, created.ID, hash); err != nil {
			return nil, apierrors.NewInternalError(fmt.Errorf("set password: %w", err))
		}
	}

	return userToAPI(created), nil
}

// update 全量更新用户信息。
// +openapi:summary=更新用户信息（全量）
func (o userOps) update(ctx apiserver.Ctx, id int64, user *User) (*User, error) {
	if errs := ValidateUserUpdate(&user.Spec); errs.HasErrors() {
		return nil, apierrors.NewBadRequest("validation failed", errs)
	}

	if ctx.DryRun {
		return user, nil
	}

	updated, err := o.dbStore.Update(ctx, modstore.UserUpdateInput{
		ID:          id,
		Username:    user.Spec.Username,
		Email:       user.Spec.Email,
		DisplayName: user.Spec.DisplayName,
		Phone:       user.Spec.Phone,
		AvatarURL:   user.Spec.AvatarURL,
		Status:      user.Spec.Status,
	})
	if err != nil {
		return nil, domainErr(err)
	}
	return userToAPI(updated), nil
}

// patch 部分更新用户信息，仅更新请求中提供的字段。
// +openapi:summary=更新用户信息（部分）
func (o userOps) patch(ctx apiserver.Ctx, id int64, body json.RawMessage) (*User, error) {
	user, err := apiserver.DecodePatch[User](body)
	if err != nil {
		return nil, err
	}

	if ctx.DryRun {
		existing, err := o.get(ctx, id)
		if err != nil {
			return nil, domainErr(err)
		}
		return existing, nil
	}

	patchIn := modstore.UserPatchInput{ID: id}
	if user.Spec.Email != "" {
		v := user.Spec.Email
		patchIn.Email = &v
	}
	if user.Spec.Phone != "" {
		v := user.Spec.Phone
		patchIn.Phone = &v
	}
	if user.Spec.DisplayName != "" {
		v := user.Spec.DisplayName
		patchIn.DisplayName = &v
	}
	if user.Spec.AvatarURL != "" {
		v := user.Spec.AvatarURL
		patchIn.AvatarURL = &v
	}
	if user.Spec.Status != "" {
		v := user.Spec.Status
		patchIn.Status = &v
	}
	patched, err := o.dbStore.Patch(ctx, patchIn)
	if err != nil {
		return nil, domainErr(err)
	}
	return userToAPI(patched), nil
}

// delete 删除单个用户。
// +openapi:summary=删除用户
func (o userOps) delete(ctx apiserver.Ctx, id int64) error {
	if ctx.DryRun {
		return nil
	}

	if err := o.dbStore.Delete(ctx, id); err != nil {
		return domainErr(err)
	}
	return nil
}

// batchDelete 批量删除用户。
// +openapi:summary=批量删除用户
func (o userOps) batchDelete(ctx apiserver.Ctx, ids []int64) (*apiserver.BatchResult, error) {
	if ctx.DryRun {
		return &apiserver.BatchResult{
			SuccessCount: len(ids),
			FailedCount:  0,
		}, nil
	}

	count, err := o.dbStore.DeleteByIDs(ctx, ids)
	if err != nil {
		return nil, domainErr(err)
	}

	return &apiserver.BatchResult{
		SuccessCount: int(count),
		FailedCount:  len(ids) - int(count),
	}, nil
}

// ===== change-password 修改密码操作 =====

// ChangePasswordRequest 修改密码请求：包含旧密码和新密码。
type ChangePasswordRequest struct {
	OldPassword string `json:"oldPassword"`
	NewPassword string `json:"newPassword"`
}

// StatusResponse 操作结果响应。
type StatusResponse struct {
	runtime.TypeMeta `json:",inline"`
	Status           string `json:"status"`
	Message          string `json:"message"`
}

func (s *StatusResponse) GetTypeMeta() *runtime.TypeMeta { return &s.TypeMeta }

// NewChangePasswordAction 创建修改密码的操作处理器。验证旧密码后设置新密码，并吊销该用户所有已有的刷新令牌。
// +openapi:action=change-password
// +openapi:resource=User
// +openapi:summary=修改用户密码
func NewChangePasswordAction(userStore modstore.UserStore, refreshStore modstore.RefreshTokenStore, hashPasswd PasswordHasher, verifyPasswd func(password, hash string) error) func(apiserver.Ctx, *ChangePasswordRequest) (*StatusResponse, error) {
	return func(ctx apiserver.Ctx, req *ChangePasswordRequest) (*StatusResponse, error) {
		return changePasswordExec(ctx, req, userStore, refreshStore, hashPasswd, verifyPasswd)
	}
}

// changePasswordExec 是 NewChangePasswordAction 闭包体的命名实现：校验请求、
// 核验旧密码后设置新密码并吊销刷新令牌。目标 userId 由框架解析进 ctx.ID。
func changePasswordExec(ctx apiserver.Ctx, req *ChangePasswordRequest, userStore modstore.UserStore, refreshStore modstore.RefreshTokenStore, hashPasswd PasswordHasher, verifyPasswd func(password, hash string) error) (*StatusResponse, error) {
	uid := ctx.ID

	if err := changePasswordValidateRequest(req); err != nil {
		return nil, err
	}

	// Get user first to find their username, then get auth data
	user, err := userStore.GetByID(ctx, uid)
	if err != nil {
		return nil, domainErr(err)
	}
	authUser, err := userStore.GetUserForAuth(ctx, user.Username)
	if err != nil {
		return nil, domainErr(err)
	}

	// Verify old password
	if err := verifyPasswd(req.OldPassword, authUser.PasswordHash); err != nil {
		return nil, apierrors.NewBadRequest("old password is incorrect", nil)
	}

	if err := setUserPasswordAndRevoke(ctx, uid, req.NewPassword, userStore, refreshStore, hashPasswd); err != nil {
		return nil, err
	}

	return &StatusResponse{
		TypeMeta: runtime.TypeMeta{Kind: "Status"},
		Status:   "Success",
		Message:  "password changed successfully",
	}, nil
}

// changePasswordValidateRequest 校验修改密码请求体（JSON 解码由框架完成）。
func changePasswordValidateRequest(req *ChangePasswordRequest) error {
	if req.OldPassword == "" || req.NewPassword == "" {
		return apierrors.NewBadRequest("oldPassword and newPassword are required", nil)
	}

	if errs := ValidatePassword(req.NewPassword); errs.HasErrors() {
		return apierrors.NewBadRequest("validation failed", errs)
	}
	return nil
}

// setUserPasswordAndRevoke 哈希并写入新密码，随后吊销该用户全部刷新令牌（吊销失败仅记日志，不阻断）。
// change-password 与 reset-password 共用此尾段逻辑。
func setUserPasswordAndRevoke(ctx context.Context, uid int64, newPassword string, userStore modstore.UserStore, refreshStore modstore.RefreshTokenStore, hashPasswd PasswordHasher) error {
	// Hash and set new password
	hash, err := hashPasswd(newPassword)
	if err != nil {
		return apierrors.NewInternalError(fmt.Errorf("hash password: %w", err))
	}
	if err := userStore.SetPasswordHash(ctx, uid, hash); err != nil {
		return apierrors.NewInternalError(fmt.Errorf("set password: %w", err))
	}

	// Revoke all existing refresh tokens for this user
	if refreshStore != nil {
		if err := refreshStore.RevokeByUserID(ctx, uid); err != nil {
			logger.Infof("failed to revoke refresh tokens for user %d: %v", uid, err)
		}
	}
	return nil
}

// ===== reset-password 管理员重置用户密码 =====

// ResetPasswordRequest 管理员重置密码请求：仅含新密码，由调用方鉴权 + handler 拒绝重置自己。
type ResetPasswordRequest struct {
	NewPassword string `json:"newPassword"`
}

// NewResetPasswordAction 创建管理员重置密码的操作处理器。
// 与 change-password 不同：不要求旧密码；要求调用者不是目标用户本人；调用者身份从 ctx 取，
// 鉴权由 RBAC filter (iam:users:reset-password) 完成。
// 设置新密码后吊销目标用户全部刷新令牌，强制其下线重新登录。
// +openapi:action=reset-password
// +openapi:resource=User
// +openapi:summary=管理员重置用户密码
func NewResetPasswordAction(userStore modstore.UserStore, refreshStore modstore.RefreshTokenStore, hashPasswd PasswordHasher) func(apiserver.Ctx, *ResetPasswordRequest) (*StatusResponse, error) {
	return func(ctx apiserver.Ctx, req *ResetPasswordRequest) (*StatusResponse, error) {
		return resetPasswordExec(ctx, req, userStore, refreshStore, hashPasswd)
	}
}

// resetPasswordExec 是 NewResetPasswordAction 闭包体的命名实现：校验请求、
// 拒绝重置自己、确认目标用户存在后设置新密码并吊销刷新令牌。目标 userId 由框架解析进 ctx.ID。
func resetPasswordExec(ctx apiserver.Ctx, req *ResetPasswordRequest, userStore modstore.UserStore, refreshStore modstore.RefreshTokenStore, hashPasswd PasswordHasher) (*StatusResponse, error) {
	uid := ctx.ID

	if err := resetPasswordValidateRequest(req); err != nil {
		return nil, err
	}

	// Disallow resetting one's own password via this admin action; self-service
	// should go through change-password (which requires the old password).
	if callerID, ok := oidc.UserIDFromContext(ctx); ok && callerID == uid {
		return nil, apierrors.NewBadRequest("cannot reset your own password; use change-password instead", nil)
	}

	// Verify target user exists.
	if _, err := userStore.GetByID(ctx, uid); err != nil {
		return nil, domainErr(err)
	}

	if err := setUserPasswordAndRevoke(ctx, uid, req.NewPassword, userStore, refreshStore, hashPasswd); err != nil {
		return nil, err
	}

	return &StatusResponse{
		TypeMeta: runtime.TypeMeta{Kind: "Status"},
		Status:   "Success",
		Message:  "password reset successfully",
	}, nil
}

// resetPasswordValidateRequest 校验管理员重置密码请求体（JSON 解码由框架完成）。
func resetPasswordValidateRequest(req *ResetPasswordRequest) error {
	if req.NewPassword == "" {
		return apierrors.NewBadRequest("newPassword is required", nil)
	}

	if errs := ValidatePassword(req.NewPassword); errs.HasErrors() {
		return apierrors.NewBadRequest("validation failed", errs)
	}
	return nil
}
