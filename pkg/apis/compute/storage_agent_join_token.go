package compute

import (
	"strconv"
	"time"

	apierrors "vraxel.io/vraxel/lib/api/errors"
	"vraxel.io/vraxel/lib/apiserver"
	"vraxel.io/vraxel/lib/list"
	"vraxel.io/vraxel/lib/oidc"
	"vraxel.io/vraxel/pkg/apis/agentgw"
	modstore "vraxel.io/vraxel/pkg/apis/compute/store"
	"vraxel.io/vraxel/pkg/apis/shared/scope"
)

const (
	defaultJoinTokenTTLHours = 24
	maxJoinTokenTTLHours     = 24 * 30
)

type joinTokenOps struct {
	store     agentgw.JoinTokenStore
	hostStore modstore.HostStore
	// serverURL is this deployment's externally reachable address
	// (server.externalUrl), handed to the operator with the token so the
	// install command points somewhere the host can actually reach.
	serverURL string
}

// AgentJoinTokensDef declares the pending-onboarding resource at all
// three scopes.
//
// It borrows compute:hosts:create rather than owning a permission tree.
// A join token is the power to bring a host into this scope, which is
// exactly what compute:hosts:create means -- giving it four codes of its
// own would mean every role that can onboard hosts has to be granted
// them in lockstep, and forgetting one breaks onboarding silently.
//
// Not compute:hosts:* : permission matching is bidirectional
// (iam.hasAnyPermissionMatchRules), so a wildcard target is satisfied by
// a user holding any single compute:hosts verb. A read-only host viewer
// would be able to mint tokens.
//
// Sensitive keeps the create response, which carries the only copy of
// the plaintext, out of the audit log.
func AgentJoinTokensDef(store agentgw.JoinTokenStore, hostStore modstore.HostStore, serverURL string) apiserver.ResourceDef[AgentJoinToken] {
	o := joinTokenOps{store: store, hostStore: hostStore, serverURL: serverURL}
	return apiserver.ResourceDef[AgentJoinToken]{
		Group: "compute", Name: "agent-join-tokens",
		Scopes:            apiserver.ScopeAll,
		Sensitive:         true,
		PermissionTargets: []string{"compute:hosts:create"},
		Ops: apiserver.Ops[AgentJoinToken]{
			List:   o.list,
			Get:    o.get,
			Create: o.create,
			Delete: o.delete,
		},
	}
}

// +openapi:summary=获取待使用的 agent 接入令牌列表
// +openapi:summary.workspaces.agent-join-tokens=获取工作空间下的 agent 接入令牌列表
// +openapi:summary.workspaces.namespaces.agent-join-tokens=获取项目下的 agent 接入令牌列表
func (o joinTokenOps) list(ctx apiserver.Ctx, q list.Query) (*list.Result[AgentJoinToken], error) {
	if q.Filters == nil {
		q.Filters = map[string]any{}
	}
	switch ctx.Scope.Level {
	case apiserver.ScopeNamespace:
		q.Filters["namespace_id"] = ctx.Scope.NamespaceID
	case apiserver.ScopeWorkspace:
		q.Filters["workspace_id"] = ctx.Scope.WorkspaceID
	}

	result, err := o.store.List(ctx, q)
	if err != nil {
		return nil, domainErr(err)
	}
	items := make([]AgentJoinToken, len(result.Items))
	for i := range result.Items {
		items[i] = joinTokenToAPI(&result.Items[i], "")
	}
	return &list.Result[AgentJoinToken]{Items: items, TotalCount: result.TotalCount}, nil
}

// +openapi:summary=获取 agent 接入令牌详情
// +openapi:summary.workspaces.agent-join-tokens=获取工作空间下的 agent 接入令牌详情
// +openapi:summary.workspaces.namespaces.agent-join-tokens=获取项目下的 agent 接入令牌详情
func (o joinTokenOps) get(ctx apiserver.Ctx, id int64) (*AgentJoinToken, error) {
	row, err := o.store.GetByID(ctx, id, scope.FromIDs(ctx.Scope.WorkspaceID, ctx.Scope.NamespaceID))
	if err != nil {
		return nil, domainErr(err)
	}
	out := joinTokenToAPI(row, "")
	return &out, nil
}

// +openapi:summary=签发 agent 接入令牌
// +openapi:summary.workspaces.agent-join-tokens=在工作空间下签发 agent 接入令牌
// +openapi:summary.workspaces.namespaces.agent-join-tokens=在项目下签发 agent 接入令牌
//
// The response carries the only copy of the plaintext. Nothing can
// recover it afterwards; the row holds a SHA-256 hash.
//
// Scope comes from the URL, never from the body. That is what stops a
// token minted in one tenant from onboarding a machine into another:
// whatever scope the operator was standing in when they asked is the
// scope the machine lands in.
func (o joinTokenOps) create(ctx apiserver.Ctx, in *AgentJoinToken) (*AgentJoinToken, error) {
	sf := scope.FromIDs(ctx.Scope.WorkspaceID, ctx.Scope.NamespaceID)
	sc, wsID, nsID := sf.PartsForCreate()

	ttl := in.Spec.TTLHours
	if ttl == 0 {
		ttl = defaultJoinTokenTTLHours
	}
	if ttl < 1 || ttl > maxJoinTokenTTLHours {
		return nil, apierrors.NewBadRequest("ttlHours must be between 1 and 720", nil)
	}

	// A bound token adopts a host that already exists, so the host has to
	// exist and has to be the caller's to reach. Its own scope is what
	// the token gets stamped with, not the caller's: they are the same
	// for any request the scope filter lets through, and reading it from
	// the host means a mismatch is impossible rather than merely
	// unlikely.
	var targetHostID *int64
	var targetHostName string
	if raw := in.Spec.TargetHostID; raw != "" {
		hostID, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || hostID <= 0 {
			return nil, apierrors.NewBadRequest("targetHostId must be a host id", nil)
		}
		host, err := o.hostStore.GetByID(ctx, hostID, sf)
		if err != nil {
			return nil, domainErr(err)
		}
		targetHostID = &hostID
		targetHostName = host.Name
		sc, wsID, nsID = host.Scope, host.WorkspaceID, host.NamespaceID
	}

	plaintext, err := agentgw.GenerateJoinToken()
	if err != nil {
		return nil, apierrors.NewInternalError(err)
	}
	var createdBy *int64
	if userID, ok := oidc.UserIDFromContext(ctx); ok {
		createdBy = &userID
	}

	row, err := o.store.Create(ctx, agentgw.JoinTokenCreateInput{
		Name:      in.Metadata.Name,
		TokenHash: agentgw.HashToken(plaintext),
		Scope:     sc,
		// A bound token is single-use by construction (the table's
		// chk_join_token_bound_single_use enforces it): one host, one
		// machine. An unbound one is single-use by default; max uses
		// becomes a control when batch onboarding lands.
		WorkspaceID:  wsID,
		NamespaceID:  nsID,
		MaxUses:      1,
		ExpiresAt:    time.Now().Add(time.Duration(ttl) * time.Hour),
		CreatedBy:    createdBy,
		TargetHostID: targetHostID,
	})
	if err != nil {
		return nil, domainErr(err)
	}

	out := joinTokenToAPI(row, targetHostName)
	out.Spec.Token = plaintext
	out.Spec.TTLHours = ttl
	out.Spec.ServerURL = o.serverURL
	return &out, nil
}

// +openapi:summary=吊销 agent 接入令牌
// +openapi:summary.workspaces.agent-join-tokens=吊销工作空间下的 agent 接入令牌
// +openapi:summary.workspaces.namespaces.agent-join-tokens=吊销项目下的 agent 接入令牌
func (o joinTokenOps) delete(ctx apiserver.Ctx, id int64) error {
	return domainErr(o.store.Delete(ctx, id, scope.FromIDs(ctx.Scope.WorkspaceID, ctx.Scope.NamespaceID)))
}

func joinTokenToAPI(r *agentgw.JoinTokenRow, targetHostName string) AgentJoinToken {
	createdAt := r.CreatedAt
	expiresAt := r.ExpiresAt
	t := AgentJoinToken{
		Metadata: apiObjectMeta(r.ID, r.Name, &createdAt, nil),
		Spec: AgentJoinTokenSpec{
			Scope:          r.Scope,
			MaxUses:        r.MaxUses,
			UsedCount:      r.UsedCount,
			ExpiresAt:      &expiresAt,
			CreatedByName:  r.CreatorName,
			TargetHostName: targetHostName,
		},
	}
	if r.WorkspaceID != nil {
		t.Spec.WorkspaceID = strconv.FormatInt(*r.WorkspaceID, 10)
	}
	if r.NamespaceID != nil {
		t.Spec.NamespaceID = strconv.FormatInt(*r.NamespaceID, 10)
	}
	if r.TargetHostID != nil {
		t.Spec.TargetHostID = strconv.FormatInt(*r.TargetHostID, 10)
	}
	return t
}
