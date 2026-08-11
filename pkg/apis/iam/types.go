package iam

import (
	"vraxel.io/vraxel/lib/api/types"
	"vraxel.io/vraxel/lib/runtime"
)

// --- User types ---

// User
// +openapi:description=用户管理：平台用户的增删改查，支持密码设置与修改。用户可被添加为租户或项目的成员。
type User struct {
	runtime.TypeMeta `json:",inline"`
	types.ObjectMeta `json:"metadata"`
	Spec             UserSpec `json:"spec"`
}

func (u *User) GetTypeMeta() *runtime.TypeMeta { return &u.TypeMeta }

// UserSpec
// +openapi:description=用户属性：包含用户名、邮箱、显示名称、手机号、头像、密码等字段。
type UserSpec struct {
	// +openapi:required
	// +openapi:description=用户名，3-50个字符，仅支持字母、数字、下划线和连字符
	Username string `json:"username"`
	// +openapi:required
	// +openapi:description=邮箱地址
	// +openapi:format=email
	Email string `json:"email"`
	// +openapi:description=用户显示名称
	DisplayName string `json:"displayName,omitempty"`
	// +openapi:description=手机号码
	Phone string `json:"phone,omitempty"`
	// +openapi:description=用户头像 URL
	// +openapi:format=uri
	AvatarURL string `json:"avatarUrl,omitempty"`
	// +openapi:description=用户密码（只写字段，创建时设置，响应中不返回）
	// +openapi:format=password
	Password string `json:"password,omitempty"`
	// +openapi:description=账户状态
	// +openapi:enum=active,inactive
	Status string `json:"status,omitempty"`
	// +openapi:description=是否为内置用户（admin 等系统账户，前端据此屏蔽删除按钮）。只读，由 seed/migration 设置。
	Builtin bool `json:"builtin,omitempty"`
	// +openapi:description=用户所属的项目列表（仅在列表查询时返回）
	Namespaces []string `json:"namespaces,omitempty"`
	// +openapi:description=用户在租户或项目中持有的角色（仅成员列表查询时返回，可多个）
	Roles []string `json:"roles,omitempty"`
	// +openapi:description=用户加入租户或项目的时间（仅成员列表查询时返回）
	JoinedAt string `json:"joinedAt,omitempty"`
}

// UserList
// +openapi:description=用户列表：分页返回的用户集合。
type UserList struct {
	runtime.TypeMeta `json:",inline"`
	Items            []User `json:"items"`
	TotalCount       int64  `json:"totalCount"`
}

func (u *UserList) GetTypeMeta() *runtime.TypeMeta { return &u.TypeMeta }

// --- Workspace types ---

// Workspace
// +openapi:description=租户管理：租户是平台的顶层租户/组织单元，包含多个项目（Namespace）和成员。创建租户时会自动创建默认项目并将创建者设为所有者。
type Workspace struct {
	runtime.TypeMeta `json:",inline"`
	types.ObjectMeta `json:"metadata"`
	Spec             WorkspaceSpec `json:"spec"`
}

func (w *Workspace) GetTypeMeta() *runtime.TypeMeta { return &w.TypeMeta }

// WorkspaceSpec
// +openapi:description=租户属性：包含显示名称、描述、所有者和状态。
type WorkspaceSpec struct {
	// +openapi:description=租户显示名称
	DisplayName string `json:"displayName,omitempty"`
	// +openapi:description=租户描述
	Description string `json:"description,omitempty"`
	// +openapi:required
	// +openapi:description=租户所有者的用户 ID
	OwnerID string `json:"ownerId"`
	// +openapi:description=所有者用户名（只读）
	OwnerName string `json:"ownerName,omitempty"`
	// +openapi:description=创建人显示名（只读，由 created_by JOIN users 解析，display_name 优先回退 username）
	CreatedByName string `json:"createdByName,omitempty"`
	// +openapi:description=项目数量（只读）
	NamespaceCount int `json:"namespaceCount,omitempty"`
	// +openapi:description=成员数量（只读）
	MemberCount int `json:"memberCount,omitempty"`
	// +openapi:description=角色绑定数量（只读）
	RoleBindingCount int `json:"roleBindingCount,omitempty"`
	// +openapi:description=租户状态
	// +openapi:enum=active,inactive
	Status string `json:"status,omitempty"`
	// +openapi:description=当前用户在此租户持有的全部角色（仅 custom verb 查询时返回，owner 角色在前）
	Roles []string `json:"roles,omitempty"`
	// +openapi:description=当前用户在此租户持有的全部角色显示名称（顺序与 roles 一致）
	RoleDisplayNames []string `json:"roleDisplayNames,omitempty"`
	// +openapi:description=当前用户加入此租户的时间（仅 custom verb 查询时返回）
	JoinedAt string `json:"joinedAt,omitempty"`
}

// WorkspaceList
// +openapi:description=租户列表：分页返回的租户集合。
type WorkspaceList struct {
	runtime.TypeMeta `json:",inline"`
	Items            []Workspace `json:"items"`
	TotalCount       int64       `json:"totalCount"`
}

func (w *WorkspaceList) GetTypeMeta() *runtime.TypeMeta { return &w.TypeMeta }

// --- Namespace types ---

// Namespace
// +openapi:description=项目管理：项目（Namespace）是租户下的子单元，用于组织团队和资源。项目归属于某个租户，拥有独立的成员列表。添加项目成员时会自动将其加入父租户。
type Namespace struct {
	runtime.TypeMeta `json:",inline"`
	types.ObjectMeta `json:"metadata"`
	Spec             NamespaceSpec `json:"spec"`
}

func (n *Namespace) GetTypeMeta() *runtime.TypeMeta { return &n.TypeMeta }

// NamespaceSpec
// +openapi:description=项目属性：包含显示名称、描述、所属租户、所有者、可见性、成员上限和状态。
type NamespaceSpec struct {
	// +openapi:description=项目显示名称
	DisplayName string `json:"displayName,omitempty"`
	// +openapi:description=项目描述
	Description string `json:"description,omitempty"`
	// +openapi:required
	// +openapi:description=所属租户 ID
	WorkspaceID string `json:"workspaceId"`
	// +openapi:required
	// +openapi:description=项目所有者的用户 ID
	OwnerID string `json:"ownerId"`
	// +openapi:description=项目可见性
	// +openapi:enum=public,private
	Visibility string `json:"visibility,omitempty"`
	// +openapi:description=项目最大成员数（0 表示不限制）
	MaxMembers int `json:"maxMembers"`
	// +openapi:description=项目所有者用户名（只读）
	OwnerName string `json:"ownerName,omitempty"`
	// +openapi:description=创建人显示名（只读，由 created_by JOIN users 解析，display_name 优先回退 username）
	CreatedByName string `json:"createdByName,omitempty"`
	// +openapi:description=项目成员数量（只读）
	MemberCount int `json:"memberCount,omitempty"`
	// +openapi:description=角色绑定数量（只读）
	RoleBindingCount int `json:"roleBindingCount,omitempty"`
	// +openapi:description=所属租户名称（只读）
	WorkspaceName string `json:"workspaceName,omitempty"`
	// +openapi:description=项目状态
	// +openapi:enum=active,inactive
	Status string `json:"status,omitempty"`
	// +openapi:description=当前用户在此项目持有的全部角色（仅 custom verb 查询时返回，owner 角色在前）
	Roles []string `json:"roles,omitempty"`
	// +openapi:description=当前用户在此项目持有的全部角色显示名称（顺序与 roles 一致）
	RoleDisplayNames []string `json:"roleDisplayNames,omitempty"`
	// +openapi:description=当前用户加入此项目的时间（仅 custom verb 查询时返回）
	JoinedAt string `json:"joinedAt,omitempty"`
}

// NamespaceList
// +openapi:description=项目列表：分页返回的项目集合。
type NamespaceList struct {
	runtime.TypeMeta `json:",inline"`
	Items            []Namespace `json:"items"`
	TotalCount       int64       `json:"totalCount"`
}

func (n *NamespaceList) GetTypeMeta() *runtime.TypeMeta { return &n.TypeMeta }

// --- Batch request type ---

// BatchRequest 批量操作请求：用于批量添加或移除成员。
type BatchRequest struct {
	runtime.TypeMeta `json:",inline"`
	IDs              []string `json:"ids"`
	RoleID           string   `json:"roleId,omitempty"`
}

func (b *BatchRequest) GetTypeMeta() *runtime.TypeMeta { return &b.TypeMeta }

// --- OIDC request/response types (for OpenAPI documentation) ---

// OIDCLoginRequest
// +openapi:schema
// +openapi:description=OIDC 登录请求
type OIDCLoginRequest struct {
	// +openapi:required
	// +openapi:description=用户名或邮箱
	Username string `json:"username"`
	// +openapi:required
	// +openapi:description=密码
	Password string `json:"password"`
	// +openapi:description=授权请求 ID，由 /oidc/authorize 重定向时传入。提供时完成 OIDC 授权流程，否则执行直接登录
	RequestID string `json:"requestId"`
}

// OIDCLoginResponse
// +openapi:schema
// +openapi:description=OIDC 登录响应（授权流程模式返回重定向地址，直接登录模式返回会话信息）
type OIDCLoginResponse struct {
	// +openapi:description=授权回调重定向地址（授权流程模式）
	RedirectURI string `json:"redirectUri,omitempty"`
	// +openapi:description=会话 ID（直接登录模式）
	SessionID string `json:"sessionId,omitempty"`
	// +openapi:description=用户 ID（直接登录模式）
	UserID string `json:"userId,omitempty"`
}

// OIDCTokenRequest
// +openapi:schema
// +openapi:description=OIDC 令牌请求（application/x-www-form-urlencoded）
type OIDCTokenRequest struct {
	// +openapi:required
	// +openapi:description=授权类型：authorization_code 或 refresh_token
	// +openapi:enum=authorization_code,refresh_token
	GrantType string `json:"grant_type"`
	// +openapi:description=授权码（grant_type=authorization_code 时必填）
	Code string `json:"code,omitempty"`
	// +openapi:description=重定向地址（grant_type=authorization_code 时需与授权请求一致）
	RedirectURI string `json:"redirect_uri,omitempty"`
	// +openapi:required
	// +openapi:description=客户端 ID
	ClientID string `json:"client_id"`
	// +openapi:description=客户端密钥（机密客户端必填）
	ClientSecret string `json:"client_secret,omitempty"`
	// +openapi:description=PKCE 验证码（公开客户端必填）
	CodeVerifier string `json:"code_verifier,omitempty"`
	// +openapi:description=刷新令牌（grant_type=refresh_token 时必填）
	RefreshToken string `json:"refresh_token,omitempty"`
	// +openapi:description=请求的权限范围
	Scope string `json:"scope,omitempty"`
}

// OIDCTokenResponse
// +openapi:schema
// +openapi:description=OIDC 令牌响应
type OIDCTokenResponse struct {
	// +openapi:required
	// +openapi:description=访问令牌
	AccessToken string `json:"access_token"`
	// +openapi:description=ID 令牌
	IDToken string `json:"id_token,omitempty"`
	// +openapi:description=刷新令牌
	RefreshToken string `json:"refresh_token,omitempty"`
	// +openapi:required
	// +openapi:description=令牌类型，固定为 Bearer
	TokenType string `json:"token_type"`
	// +openapi:required
	// +openapi:description=访问令牌过期时间（秒）
	ExpiresIn int64 `json:"expires_in"`
	// +openapi:description=授权范围
	Scope string `json:"scope,omitempty"`
}

// OIDCUserInfoResponse
// +openapi:schema
// +openapi:description=OIDC 用户信息响应
type OIDCUserInfoResponse struct {
	// +openapi:required
	// +openapi:description=用户唯一标识
	Sub string `json:"sub"`
	// +openapi:description=用户名称
	Name string `json:"name,omitempty"`
	// +openapi:description=邮箱地址
	Email string `json:"email,omitempty"`
	// +openapi:description=手机号码
	PhoneNumber string `json:"phone_number,omitempty"`
}

// OIDCDiscoveryResponse
// +openapi:schema
// +openapi:description=OpenID Connect 发现文档
type OIDCDiscoveryResponse struct {
	// +openapi:required
	// +openapi:description=签发者标识
	Issuer string `json:"issuer"`
	// +openapi:required
	// +openapi:description=授权端点
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	// +openapi:required
	// +openapi:description=令牌端点
	TokenEndpoint string `json:"token_endpoint"`
	// +openapi:required
	// +openapi:description=用户信息端点
	UserinfoEndpoint string `json:"userinfo_endpoint"`
	// +openapi:required
	// +openapi:description=JSON Web Key Set 地址
	JwksURI string `json:"jwks_uri"`
	// +openapi:description=支持的响应类型
	ResponseTypesSupported []string `json:"response_types_supported"`
	// +openapi:description=支持的主体标识类型
	SubjectTypesSupported []string `json:"subject_types_supported"`
	// +openapi:description=支持的 ID Token 签名算法
	IDTokenSigningAlgValuesSupported []string `json:"id_token_signing_alg_values_supported"`
	// +openapi:description=支持的权限范围
	ScopesSupported []string `json:"scopes_supported"`
	// +openapi:description=支持的授权类型
	GrantTypesSupported []string `json:"grant_types_supported"`
	// +openapi:description=支持的 PKCE 挑战方法
	CodeChallengeMethodsSupported []string `json:"code_challenge_methods_supported"`
}

// OIDCErrorResponse
// +openapi:schema
// +openapi:description=OAuth2 标准错误响应
type OIDCErrorResponse struct {
	// +openapi:required
	// +openapi:description=错误码
	Error string `json:"error"`
	// +openapi:required
	// +openapi:description=错误描述
	ErrorDescription string `json:"error_description"`
}

// --- RBAC API types ---

// Permission
// +openapi:description=权限管理：权限由路由自动注册，表示一个 API 操作（如 iam:users:list）。
type Permission struct {
	runtime.TypeMeta `json:",inline"`
	types.ObjectMeta `json:"metadata"`
	Spec             PermissionSpec `json:"spec"`
}

func (p *Permission) GetTypeMeta() *runtime.TypeMeta { return &p.TypeMeta }

// PermissionSpec
// +openapi:description=权限属性：包含权限标识码、HTTP 方法、路径和描述。
type PermissionSpec struct {
	// +openapi:required
	// +openapi:description=权限标识码，如 iam:users:list，首段为模块名
	Code string `json:"code"`
	// +openapi:required
	// +openapi:description=HTTP 方法
	Method string `json:"method"`
	// +openapi:required
	// +openapi:description=API 路径
	Path string `json:"path"`
	// +openapi:required
	// +openapi:description=权限作用域
	// +openapi:enum=platform,workspace,namespace
	Scope string `json:"scope"`
	// +openapi:description=权限描述
	Description string `json:"description,omitempty"`
}

// PermissionList
// +openapi:description=权限列表：分页返回的权限集合。
type PermissionList struct {
	runtime.TypeMeta `json:",inline"`
	Items            []Permission `json:"items"`
	TotalCount       int64        `json:"totalCount"`
}

func (p *PermissionList) GetTypeMeta() *runtime.TypeMeta { return &p.TypeMeta }

// Role
// +openapi:description=角色管理：角色定义了一组权限规则，可绑定到用户。支持内置角色和自定义角色。
type Role struct {
	runtime.TypeMeta `json:",inline"`
	types.ObjectMeta `json:"metadata"`
	Spec             RoleSpec `json:"spec"`
}

func (r *Role) GetTypeMeta() *runtime.TypeMeta { return &r.TypeMeta }

// RoleSpec
// +openapi:description=角色属性：包含名称、作用域、是否内置以及权限规则列表。
type RoleSpec struct {
	// +openapi:required
	// +openapi:description=角色唯一名称，如 platform-admin
	Name string `json:"name"`
	// +openapi:description=角色显示名称
	DisplayName string `json:"displayName,omitempty"`
	// +openapi:description=角色描述
	Description string `json:"description,omitempty"`
	// +openapi:required
	// +openapi:description=角色作用域
	// +openapi:enum=platform,workspace,namespace
	Scope string `json:"scope"`
	// +openapi:description=是否为内置角色（只读）
	Builtin bool `json:"builtin,omitempty"`
	// +openapi:description=权限规则数量（仅列表返回）
	RuleCount *int32 `json:"ruleCount,omitempty"`
	// +openapi:description=权限规则列表，支持精确码和通配符模式（如 *:*、iam:*、iam:users:list）
	Rules []string `json:"rules,omitempty"`
}

// RoleList
// +openapi:description=角色列表：分页返回的角色集合。
type RoleList struct {
	runtime.TypeMeta `json:",inline"`
	Items            []Role `json:"items"`
	TotalCount       int64  `json:"totalCount"`
}

func (r *RoleList) GetTypeMeta() *runtime.TypeMeta { return &r.TypeMeta }

// RoleBinding
// +openapi:description=角色绑定：将用户与角色在特定作用域（平台/租户/项目）下关联。
type RoleBinding struct {
	runtime.TypeMeta `json:",inline"`
	types.ObjectMeta `json:"metadata"`
	Spec             RoleBindingSpec `json:"spec"`
}

func (rb *RoleBinding) GetTypeMeta() *runtime.TypeMeta { return &rb.TypeMeta }

// RoleBindingSpec
// +openapi:description=角色绑定属性：包含用户、角色、作用域和资源 ID。
type RoleBindingSpec struct {
	// +openapi:required
	// +openapi:description=用户 ID
	UserID string `json:"userId"`
	// +openapi:required
	// +openapi:description=角色 ID
	RoleID string `json:"roleId"`
	// +openapi:required
	// +openapi:description=绑定作用域
	// +openapi:enum=platform,workspace,namespace
	Scope string `json:"scope"`
	// +openapi:description=租户 ID（workspace/namespace scope 时必填）
	WorkspaceID *string `json:"workspaceId,omitempty"`
	// +openapi:description=项目 ID（namespace scope 时必填）
	NamespaceID *string `json:"namespaceId,omitempty"`
	// +openapi:description=是否为资源所有者
	IsOwner bool `json:"isOwner,omitempty"`
	// +openapi:description=角色名称（只读）
	RoleName string `json:"roleName,omitempty"`
	// +openapi:description=角色显示名称（只读）
	RoleDisplayName string `json:"roleDisplayName,omitempty"`
	// +openapi:description=用户名（只读）
	Username string `json:"username,omitempty"`
	// +openapi:description=用户显示名称（只读）
	UserDisplayName string `json:"userDisplayName,omitempty"`
	// +openapi:description=租户名称（只读）
	WorkspaceName string `json:"workspaceName,omitempty"`
	// +openapi:description=项目名称（只读）
	NamespaceName string `json:"namespaceName,omitempty"`
}

// RoleBindingList
// +openapi:description=角色绑定列表：分页返回的角色绑定集合。
type RoleBindingList struct {
	runtime.TypeMeta `json:",inline"`
	Items            []RoleBinding `json:"items"`
	TotalCount       int64         `json:"totalCount"`
}

func (rb *RoleBindingList) GetTypeMeta() *runtime.TypeMeta { return &rb.TypeMeta }

// UserPermissions
// +openapi:schema
// +openapi:description=用户权限视图：返回用户在各作用域下的角色和权限集合。
type UserPermissions struct {
	runtime.TypeMeta `json:",inline"`
	Spec             UserPermissionsSpec `json:"spec"`
}

func (up *UserPermissions) GetTypeMeta() *runtime.TypeMeta { return &up.TypeMeta }

// UserPermissionsSpec
// +openapi:description=用户权限详情：按平台、租户、项目维度展示。
type UserPermissionsSpec struct {
	// +openapi:description=是否为平台管理员
	IsPlatformAdmin bool `json:"isPlatformAdmin"`
	// +openapi:description=平台级权限码列表
	Platform []string `json:"platform"`
	// +openapi:description=租户级权限（key 为 workspaceId）
	Workspaces map[string]WorkspaceScopePerms `json:"workspaces"`
	// +openapi:description=项目级权限（key 为 namespaceId）
	Namespaces map[string]NamespaceScopePerms `json:"namespaces"`
}

// WorkspaceScopePerms represents permissions within a workspace scope.
// +openapi:schema
// +openapi:description=租户级权限：包含角色名称列表和展开后的权限码。
type WorkspaceScopePerms struct {
	// +openapi:description=角色名称列表
	RoleNames []string `json:"roleNames"`
	// +openapi:description=展开后的权限码列表
	Permissions []string `json:"permissions"`
}

// NamespaceScopePerms represents permissions within a namespace scope.
// +openapi:schema
// +openapi:description=项目级权限：包含角色名称列表、所属租户 ID 和展开后的权限码。
type NamespaceScopePerms struct {
	// +openapi:description=角色名称列表
	RoleNames []string `json:"roleNames"`
	// +openapi:description=所属租户 ID
	WorkspaceID string `json:"workspaceId"`
	// +openapi:description=展开后的权限码列表
	Permissions []string `json:"permissions"`
}
