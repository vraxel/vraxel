package compute

import (
	"math"
	"slices"
	"strconv"
	"strings"

	apierrors "vraxel.io/vraxel/lib/api/errors"
	"vraxel.io/vraxel/lib/apiserver"
	"vraxel.io/vraxel/lib/list"
	"vraxel.io/vraxel/lib/oidc"
	"vraxel.io/vraxel/pkg/apis/agentgw"
	modstore "vraxel.io/vraxel/pkg/apis/compute/store"
	"vraxel.io/vraxel/pkg/apis/shared/scope"
)

// alertOps serves the host-alert-rules resource: what an operator may
// say about thresholds. Evaluation lives in the agent gateway, which
// reads the same table on its own store -- these routes never see a
// heartbeat.
type alertOps struct {
	store modstore.AlertRuleStore
}

// AlertRulesDef declares the host-alert-rules resource.
func AlertRulesDef(store modstore.AlertRuleStore) apiserver.ResourceDef[HostAlertRule] {
	o := alertOps{store: store}
	return apiserver.ResourceDef[HostAlertRule]{
		Group: "compute", Name: "host-alert-rules",
		Scopes: apiserver.ScopeAll,
		Ops: apiserver.Ops[HostAlertRule]{
			List:   o.list,
			Get:    o.get,
			Create: o.create,
			Update: o.update,
			Delete: o.delete,
		},
	}
}

// +openapi:summary=获取主机告警规则列表
// +openapi:summary.workspaces.host-alert-rules=获取工作空间下主机告警规则列表
// +openapi:summary.workspaces.namespaces.host-alert-rules=获取项目下主机告警规则列表
func (o alertOps) list(ctx apiserver.Ctx, q list.Query) (*list.Result[HostAlertRule], error) {
	if q.Filters == nil {
		q.Filters = map[string]any{}
	}
	applyHostScopeFilters(&q, ctx.Scope)
	res, err := o.store.List(ctx, q)
	if err != nil {
		return nil, domainErr(err)
	}
	items := make([]HostAlertRule, len(res.Items))
	for i := range res.Items {
		items[i] = alertRuleToAPI(&res.Items[i])
	}
	return &list.Result[HostAlertRule]{Items: items, TotalCount: res.TotalCount}, nil
}

// +openapi:summary=获取主机告警规则详情
func (o alertOps) get(ctx apiserver.Ctx, id int64) (*HostAlertRule, error) {
	sf := scope.FromIDs(ctx.Scope.WorkspaceID, ctx.Scope.NamespaceID)
	r, err := o.store.GetByID(ctx, id, sf)
	if err != nil {
		return nil, domainErr(err)
	}
	out := alertRuleToAPI(r)
	return &out, nil
}

// +openapi:summary=创建主机告警规则
func (o alertOps) create(ctx apiserver.Ctx, in *HostAlertRule) (*HostAlertRule, error) {
	input, err := alertRuleInput(in)
	if err != nil {
		return nil, err
	}
	sf := scope.FromIDs(ctx.Scope.WorkspaceID, ctx.Scope.NamespaceID)
	sc, wsID, nsID := sf.PartsForCreate()
	var createdBy *int64
	if userID, ok := oidc.UserIDFromContext(ctx); ok {
		createdBy = &userID
	}
	id, err := o.store.Create(ctx, sc, wsID, nsID, createdBy, input)
	if err != nil {
		return nil, domainErr(err)
	}
	return o.get(ctx, id)
}

// +openapi:summary=更新主机告警规则
func (o alertOps) update(ctx apiserver.Ctx, id int64, in *HostAlertRule) (*HostAlertRule, error) {
	input, err := alertRuleInput(in)
	if err != nil {
		return nil, err
	}
	sf := scope.FromIDs(ctx.Scope.WorkspaceID, ctx.Scope.NamespaceID)
	if err := o.store.Update(ctx, id, sf, input); err != nil {
		return nil, domainErr(err)
	}
	return o.get(ctx, id)
}

// +openapi:summary=删除主机告警规则
func (o alertOps) delete(ctx apiserver.Ctx, id int64) error {
	sf := scope.FromIDs(ctx.Scope.WorkspaceID, ctx.Scope.NamespaceID)
	if err := o.store.Delete(ctx, id, sf); err != nil {
		return domainErr(err)
	}
	return nil
}

var (
	alertComparators = []string{"gt", "ge", "lt", "le"}
	alertSeverities  = []string{"info", "warning", "critical"}
	alertMaxForSecs  = int32(24 * 3600)
	alertNameMaxLen  = 255
	alertDescMaxLen  = 1000
	alertDefaultSecs = int32(60)
)

// alertRuleInput validates the writable fields once for create and
// update. The metric whitelist is the gateway's own list, so a rule
// that passes here is by construction one the evaluator can read.
func alertRuleInput(in *HostAlertRule) (modstore.AlertRuleInput, error) {
	var out modstore.AlertRuleInput
	name := strings.TrimSpace(in.Metadata.Name)
	if name == "" || len(name) > alertNameMaxLen {
		return out, apierrors.NewBadRequest("name is required and at most 255 characters", nil)
	}
	if len(in.Spec.Description) > alertDescMaxLen {
		return out, apierrors.NewBadRequest("description is at most 1000 characters", nil)
	}
	if !slices.Contains(agentgw.AlertMetrics, in.Spec.Metric) {
		return out, apierrors.NewBadRequest("metric must be one of "+strings.Join(agentgw.AlertMetrics, ", "), nil)
	}
	if !slices.Contains(alertComparators, in.Spec.Op) {
		return out, apierrors.NewBadRequest("op must be one of gt, ge, lt, le", nil)
	}
	if !slices.Contains(alertSeverities, in.Spec.Severity) {
		return out, apierrors.NewBadRequest("severity must be one of info, warning, critical", nil)
	}
	forSecs := in.Spec.ForSeconds
	if forSecs == 0 {
		forSecs = alertDefaultSecs
	}
	if forSecs < 0 || forSecs > alertMaxForSecs {
		return out, apierrors.NewBadRequest("forSeconds must be between 0 and 86400", nil)
	}
	out = modstore.AlertRuleInput{
		Name:        name,
		Description: strings.TrimSpace(in.Spec.Description),
		Metric:      in.Spec.Metric,
		Op:          in.Spec.Op,
		Threshold:   in.Spec.Threshold,
		ForSeconds:  forSecs,
		Severity:    in.Spec.Severity,
		Enabled:     in.Spec.Enabled == nil || *in.Spec.Enabled,
	}
	return out, nil
}

func alertRuleToAPI(r *modstore.AlertRuleRow) HostAlertRule {
	createdAt, updatedAt := r.CreatedAt, r.UpdatedAt
	enabled := r.Enabled
	// The column is real; widening 85.3 through float32 prints as
	// 85.30000305. Thresholds are human-typed numbers with at most a few
	// decimals, so round-tripping through 1e4 restores what was typed.
	threshold := math.Round(r.Threshold*10000) / 10000
	out := HostAlertRule{
		Metadata: apiObjectMeta(r.ID, r.Name, &createdAt, &updatedAt),
		Spec: HostAlertRuleSpec{
			Description:   r.Description,
			Scope:         r.Scope,
			WorkspaceName: r.WorkspaceName,
			NamespaceName: r.NamespaceName,
			Metric:        r.Metric,
			Op:            r.Op,
			Threshold:     threshold,
			ForSeconds:    r.ForSeconds,
			Severity:      r.Severity,
			Enabled:       &enabled,
			FiringCount:   r.FiringCount,
			CreatedByName: r.CreatorName,
		},
	}
	if r.WorkspaceID != nil {
		out.Spec.WorkspaceID = strconv.FormatInt(*r.WorkspaceID, 10)
	}
	if r.NamespaceID != nil {
		out.Spec.NamespaceID = strconv.FormatInt(*r.NamespaceID, 10)
	}
	return out
}
