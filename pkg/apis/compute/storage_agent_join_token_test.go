package compute

import (
	"context"
	"testing"

	"vraxel.io/vraxel/lib/apiserver"
	"vraxel.io/vraxel/lib/list"
	"vraxel.io/vraxel/pkg/apis/agentgw"
	modstore "vraxel.io/vraxel/pkg/apis/compute/store"
	"vraxel.io/vraxel/pkg/apis/shared/scope"
	"vraxel.io/vraxel/pkg/db/pgerrors"
)

func ptrI64(v int64) *int64 { return &v }

// fakeHostStore answers GetByID subject to the same scope filter the SQL
// applies, so a test can tell "the URL could not reach this host" apart
// from "the host does not exist".
type fakeHostStore struct {
	rows map[int64]*modstore.HostRow
}

func (f *fakeHostStore) GetByID(_ context.Context, id int64, sf scope.Filter) (*modstore.HostRow, error) {
	row, ok := f.rows[id]
	if !ok {
		return nil, pgerrors.ErrNotFound
	}
	if sf.WorkspaceID != nil && (row.WorkspaceID == nil || *row.WorkspaceID != *sf.WorkspaceID) {
		return nil, pgerrors.ErrNotFound
	}
	if sf.NamespaceID != nil && (row.NamespaceID == nil || *row.NamespaceID != *sf.NamespaceID) {
		return nil, pgerrors.ErrNotFound
	}
	return row, nil
}

func (f *fakeHostStore) List(context.Context, list.Query) (*list.Result[modstore.HostRow], error) {
	return &list.Result[modstore.HostRow]{}, nil
}
func (f *fakeHostStore) Create(context.Context, modstore.HostCreateInput) (int64, error) {
	return 0, nil
}
func (f *fakeHostStore) Update(context.Context, int64, scope.Filter, modstore.HostUpdateInput) error {
	return nil
}
func (f *fakeHostStore) Delete(context.Context, int64, scope.Filter) error { return nil }
func (f *fakeHostStore) GetScope(context.Context, int64) (*modstore.HostScope, error) {
	return nil, nil
}

// fakeTokenStore records what Create was asked to persist.
type fakeTokenStore struct {
	got agentgw.JoinTokenCreateInput
}

func (f *fakeTokenStore) Create(_ context.Context, in agentgw.JoinTokenCreateInput) (*agentgw.JoinTokenRow, error) {
	f.got = in
	return &agentgw.JoinTokenRow{
		ID: 1, Scope: in.Scope, WorkspaceID: in.WorkspaceID, NamespaceID: in.NamespaceID,
		MaxUses: in.MaxUses, ExpiresAt: in.ExpiresAt, TargetHostID: in.TargetHostID,
	}, nil
}

func (f *fakeTokenStore) GetByID(context.Context, int64, scope.Filter) (*agentgw.JoinTokenRow, error) {
	return nil, pgerrors.ErrNotFound
}
func (f *fakeTokenStore) List(context.Context, list.Query) (*list.Result[agentgw.JoinTokenRow], error) {
	return &list.Result[agentgw.JoinTokenRow]{}, nil
}
func (f *fakeTokenStore) Delete(context.Context, int64, scope.Filter) error { return nil }
func (f *fakeTokenStore) Peek(context.Context, []byte) (*agentgw.JoinTokenRow, error) {
	return nil, pgerrors.ErrNotFound
}
func (f *fakeTokenStore) Consume(context.Context, []byte) (*agentgw.JoinTokenRow, error) {
	return nil, pgerrors.ErrNotFound
}
func (f *fakeTokenStore) Refund(context.Context, int64) error            { return nil }
func (f *fakeTokenStore) BindTarget(context.Context, int64, int64) error { return nil }

// TestJoinTokenScopeComesFromTheHostNotTheCaller pins the rule that
// stops a token from onboarding a machine into the wrong tenant.
//
// For an unbound token the scope is the URL's, so whichever scope the
// operator was standing in is where the machine lands. For a bound one
// it is read from the target host instead: the two agree for any request
// the scope filter lets through, and reading it from the host makes a
// mismatch impossible rather than merely unlikely.
func TestJoinTokenScopeComesFromTheHostNotTheCaller(t *testing.T) {
	hosts := &fakeHostStore{rows: map[int64]*modstore.HostRow{
		7: {ID: 7, Name: "ws-host", Scope: scope.Workspace, WorkspaceID: ptrI64(3)},
	}}

	t.Run("unbound token takes the URL scope", func(t *testing.T) {
		store := &fakeTokenStore{}
		o := joinTokenOps{store: store, hostStore: hosts}
		ctx := apiserver.Ctx{Context: context.Background(),
			Scope: apiserver.ScopeInfo{Level: apiserver.ScopeWorkspace, WorkspaceID: 3}}

		out, err := o.create(ctx, &AgentJoinToken{})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if store.got.Scope != scope.Workspace || store.got.WorkspaceID == nil || *store.got.WorkspaceID != 3 {
			t.Errorf("persisted scope = %q/%v, want workspace/3", store.got.Scope, store.got.WorkspaceID)
		}
		if store.got.TargetHostID != nil {
			t.Errorf("target = %v, want none", store.got.TargetHostID)
		}
		if out.Spec.Token == "" {
			t.Error("create returned no plaintext; it is the only copy there will ever be")
		}
	})

	t.Run("bound token takes the host's scope", func(t *testing.T) {
		store := &fakeTokenStore{}
		o := joinTokenOps{store: store, hostStore: hosts}
		ctx := apiserver.Ctx{Context: context.Background(),
			Scope: apiserver.ScopeInfo{Level: apiserver.ScopeWorkspace, WorkspaceID: 3}}

		out, err := o.create(ctx, &AgentJoinToken{Spec: AgentJoinTokenSpec{TargetHostID: "7"}})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if store.got.Scope != scope.Workspace || *store.got.WorkspaceID != 3 {
			t.Errorf("persisted scope = %q/%v, want the host's workspace/3", store.got.Scope, store.got.WorkspaceID)
		}
		if store.got.TargetHostID == nil || *store.got.TargetHostID != 7 {
			t.Fatalf("target = %v, want host 7", store.got.TargetHostID)
		}
		if store.got.MaxUses != 1 {
			t.Errorf("maxUses = %d, want 1: a bound token is one host, one machine", store.got.MaxUses)
		}
		if out.Spec.TargetHostName != "ws-host" {
			t.Errorf("targetHostName = %q, want ws-host", out.Spec.TargetHostName)
		}
	})

	// The discriminating case: caller and host disagree. A platform URL
	// applies no scope filter, so it can reach a workspace host -- and
	// the token must record the HOST's scope, not the caller's. Taking
	// the caller's would file a workspace machine under platform, and
	// the tenancy of everything that machine later carries follows the
	// token.
	t.Run("a platform caller binding a workspace host records the workspace", func(t *testing.T) {
		store := &fakeTokenStore{}
		o := joinTokenOps{store: store, hostStore: hosts}
		ctx := apiserver.Ctx{Context: context.Background(),
			Scope: apiserver.ScopeInfo{Level: apiserver.ScopePlatform}}

		if _, err := o.create(ctx, &AgentJoinToken{Spec: AgentJoinTokenSpec{TargetHostID: "7"}}); err != nil {
			t.Fatalf("create: %v", err)
		}
		if store.got.Scope != scope.Workspace {
			t.Errorf("persisted scope = %q, want workspace (the host's, not the caller's)", store.got.Scope)
		}
		if store.got.WorkspaceID == nil || *store.got.WorkspaceID != 3 {
			t.Errorf("persisted workspace = %v, want 3", store.got.WorkspaceID)
		}
	})

	t.Run("a host the URL cannot reach is not bindable", func(t *testing.T) {
		store := &fakeTokenStore{}
		o := joinTokenOps{store: store, hostStore: hosts}
		// Platform URL, so no scope filter -- but the operator names a
		// host in a workspace they addressed as workspace 9.
		ctx := apiserver.Ctx{Context: context.Background(),
			Scope: apiserver.ScopeInfo{Level: apiserver.ScopeWorkspace, WorkspaceID: 9}}

		if _, err := o.create(ctx, &AgentJoinToken{Spec: AgentJoinTokenSpec{TargetHostID: "7"}}); err == nil {
			t.Fatal("create succeeded; a token must not bind a host outside the caller's scope")
		}
	})

	t.Run("garbage target is rejected", func(t *testing.T) {
		o := joinTokenOps{store: &fakeTokenStore{}, hostStore: hosts}
		ctx := apiserver.Ctx{Context: context.Background()}
		if _, err := o.create(ctx, &AgentJoinToken{Spec: AgentJoinTokenSpec{TargetHostID: "not-an-id"}}); err == nil {
			t.Fatal("create succeeded on a non-numeric targetHostId")
		}
	})
}
