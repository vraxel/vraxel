package iam

import (
	"vraxel.io/vraxel/lib/apiserver"
	"vraxel.io/vraxel/lib/list"
	modstore "vraxel.io/vraxel/pkg/apis/iam/store"
)

// --- Permission Ops (read-only) ---

type permissionOps struct {
	permStore modstore.PermissionStore
}

// PermissionsDef declares the read-only platform permissions resource.
func PermissionsDef(permStore modstore.PermissionStore) apiserver.ResourceDef[Permission] {
	o := permissionOps{permStore: permStore}
	return apiserver.ResourceDef[Permission]{
		Group: "iam", Name: "permissions",
		Ops: apiserver.Ops[Permission]{
			List: o.list,
		},
	}
}

// +openapi:summary=获取权限列表
func (o permissionOps) list(ctx apiserver.Ctx, q list.Query) (*list.Result[Permission], error) {
	query := list.Query{
		Filters: make(map[string]any),
		Pagination: list.Pagination{
			Page:      q.Page,
			PageSize:  q.PageSize,
			SortBy:    q.SortBy,
			SortOrder: q.SortOrder,
		},
	}
	if module := filterString(q.Filters, "module"); module != "" {
		query.Filters["module_prefix"] = module + ":"
	}
	if search := filterString(q.Filters, "search"); search != "" {
		query.Filters["search"] = search
	}
	if scope := filterString(q.Filters, "scope"); scope != "" {
		query.Filters["scope"] = scope
	}

	result, err := o.permStore.List(ctx, query)
	if err != nil {
		return nil, err
	}

	items := make([]Permission, len(result.Items))
	for i, item := range result.Items {
		items[i] = *permissionToAPI(&item)
	}

	return &list.Result[Permission]{Items: items, TotalCount: result.TotalCount}, nil
}
