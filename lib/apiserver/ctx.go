package apiserver

import (
	"context"
	"net/http"
	"net/url"
	"strconv"

	"vraxel.io/vraxel/lib/oidc"
	"vraxel.io/vraxel/lib/rest/filters"
)

// UserInfo is the authenticated caller, extracted once by the route
// wrapper from the context values the global authentication middleware
// populated.
type UserInfo struct {
	ID       int64
	Username string
}

// Ctx is the typed request context handed to every Ops / Action / Verb
// handler. It replaces v1's map[string]string PathParams: IDs arrive
// parsed, scope arrives resolved from the matched route pattern, and the
// embedded context.Context flows into store calls unchanged.
type Ctx struct {
	context.Context

	User  UserInfo
	Scope ScopeInfo

	// ID is the item id on item routes ({id} path segment); 0 on
	// collection routes. Typed Action handlers read the operation
	// target from here.
	ID int64

	// Name is the raw item segment on string-keyed routes (NamedDef);
	// empty elsewhere.
	Name string

	// Parents holds the parent resource ID chain for nested resources,
	// ordered outermost → innermost. The order is statically determined
	// by the ResourceDef.Parent declaration, e.g. for
	// /hosts/{hostId}/nics/{nicId}/ips the ips handler sees
	// Parents[0]=hostId, Parents[1]=nicId.
	Parents []int64

	// Query carries the raw URL query values. List wrappers additionally
	// parse them into list.Query; Get/Delete/Action handlers that accept
	// caller options (?preserveVM=true, ?file=cert.pem) read them here.
	Query url.Values

	// DryRun mirrors v1's ?dryRun=true handling: validate, don't persist.
	// The wrapper passes it through; handlers decide what it means.
	DryRun bool

	// Access is the list-visibility filter for non-admin users, injected
	// by the authorization middleware for resources that declare an
	// AccessScope hook (iam workspaces/namespaces). Nil = unrestricted.
	Access *filters.AccessFilter
}

// buildCtx assembles the typed Ctx from an authenticated request matched
// to a route with the given metadata.
func buildCtx(r *http.Request, meta *routeMeta) (Ctx, error) {
	c := Ctx{
		Context: r.Context(),
		Query:   r.URL.Query(),
		DryRun:  r.URL.Query().Get("dryRun") == "true",
		Access:  filters.AccessFilterFromContext(r.Context()),
	}

	if uid, ok := oidc.UserIDFromContext(r.Context()); ok {
		c.User.ID = uid
	}
	c.User.Username = oidc.UsernameFromContext(r.Context())

	c.Scope.Level = meta.ScopeLevel
	if meta.ScopeLevel&(ScopeWorkspace|ScopeNamespace) != 0 {
		ws, err := parsePathID(r, "workspaceId")
		if err != nil {
			return c, err
		}
		c.Scope.WorkspaceID = ws
	}
	if meta.ScopeLevel&ScopeNamespace != 0 {
		ns, err := parsePathID(r, "namespaceId")
		if err != nil {
			return c, err
		}
		c.Scope.NamespaceID = ns
	}

	for _, p := range meta.ParentParams {
		id, err := parsePathID(r, p)
		if err != nil {
			return c, err
		}
		c.Parents = append(c.Parents, id)
	}

	if meta.IDParam != "" {
		raw := pathValue(r, meta.IDParam)
		if meta.StringID {
			c.Name = raw
		} else {
			id, err := strconv.ParseInt(raw, 10, 64)
			if err != nil {
				return c, invalidIDError(meta.IDParam, raw)
			}
			c.ID = id
		}
	}
	return c, nil
}

// parsePathID reads one int64 path value from the matched pattern.
func parsePathID(r *http.Request, name string) (int64, error) {
	raw := pathValue(r, name)
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, invalidIDError(name, raw)
	}
	return id, nil
}
