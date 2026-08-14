package compute

import (
	"context"
	"testing"

	"vraxel.io/vraxel/pkg/apis/agentgw"
	modstore "vraxel.io/vraxel/pkg/apis/compute/store"
	"vraxel.io/vraxel/pkg/apis/shared/scope"
	"vraxel.io/vraxel/pkg/db/pgerrors"
)

func ptr(v int64) *int64 { return &v }

// fakeAgentHostStore drives the registrar's branch selection. Only the
// error shape matters, so each method is a canned answer.
type fakeAgentHostStore struct {
	scopeErr  error
	updateErr error
	created   bool
}

func (f *fakeAgentHostStore) Create(context.Context, modstore.AgentHostCreateInput) (int64, error) {
	f.created = true
	return 42, nil
}

func (f *fakeAgentHostStore) UpdateFacts(context.Context, int64, modstore.AgentHostFactsInput) error {
	return f.updateErr
}

func (f *fakeAgentHostStore) GetScope(context.Context, int64) (*modstore.AgentHostScope, error) {
	if f.scopeErr != nil {
		return nil, f.scopeErr
	}
	return &modstore.AgentHostScope{Scope: scope.Platform}, nil
}

func (f *fakeAgentHostStore) Delete(context.Context, int64) error { return nil }

// TestRegisterAgentHostReportsWhetherItCreated pins the flag the
// gateway's rollback is gated on. Getting it wrong is destructive in
// both directions: false when the row was created leaks an orphan host
// that holds one of the three candidate names forever, and true when the
// row was adopted deletes a host the registration merely attached to.
//
// The last two cases are the ones no caller-side inference can get
// right. ExistingHostID is set, so "existing id means we adopted" says
// adopt, yet both fall through to the create branch.
func TestRegisterAgentHostReportsWhetherItCreated(t *testing.T) {
	tests := []struct {
		name        string
		existing    int64
		store       *fakeAgentHostStore
		wantCreated bool
		wantHostID  int64
	}{
		{
			name:       "known agent updates its row",
			existing:   7,
			store:      &fakeAgentHostStore{},
			wantHostID: 7,
		},
		{
			name:        "new agent creates a row",
			store:       &fakeAgentHostStore{},
			wantCreated: true,
			wantHostID:  42,
		},
		{
			name:        "host vanished before the scope read",
			existing:    7,
			store:       &fakeAgentHostStore{scopeErr: pgerrors.ErrNotFound},
			wantCreated: true,
			wantHostID:  42,
		},
		{
			name:        "host vanished between the scope read and the update",
			existing:    7,
			store:       &fakeAgentHostStore{updateErr: pgerrors.ErrNotFound},
			wantCreated: true,
			wantHostID:  42,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := &agentHostRegistrar{store: tc.store}
			hostID, created, err := r.RegisterAgentHost(context.Background(), agentgw.AgentHostSpec{
				ExistingHostID: tc.existing,
				AgentID:        "9b91082a-6555-5db2-bd8e-be12f80602f8",
				Hostname:       "node-1",
				Scope:          scope.Platform,
			})
			if err != nil {
				t.Fatalf("RegisterAgentHost() error = %v", err)
			}
			if hostID != tc.wantHostID || created != tc.wantCreated {
				t.Errorf("RegisterAgentHost() = (%d, %v), want (%d, %v)",
					hostID, created, tc.wantHostID, tc.wantCreated)
			}
			if tc.store.created != tc.wantCreated {
				t.Errorf("store.Create called = %v, want %v", tc.store.created, tc.wantCreated)
			}
		})
	}
}

// TestRebindAuthorised pins the check that stops a join token from any
// scope taking over any host whose machine id the caller knows. Losing it
// means a workspace operator can evict a platform host's agent and
// inherit its jobs.
func TestRebindAuthorised(t *testing.T) {
	platformHost := &modstore.AgentHostScope{Scope: scope.Platform}
	nsHost := &modstore.AgentHostScope{Scope: scope.Namespace, WorkspaceID: ptr(1), NamespaceID: ptr(7)}

	tests := []struct {
		name string
		spec agentgw.AgentHostSpec
		cur  *modstore.AgentHostScope
		want bool
	}{
		{
			name: "namespace token cannot rebind a platform host",
			spec: agentgw.AgentHostSpec{Scope: scope.Namespace, WorkspaceID: ptr(1), NamespaceID: ptr(7)},
			cur:  platformHost,
		},
		{
			name: "namespace token cannot rebind another namespace's host",
			spec: agentgw.AgentHostSpec{Scope: scope.Namespace, WorkspaceID: ptr(1), NamespaceID: ptr(8)},
			cur:  nsHost,
		},
		{
			name: "workspace token cannot rebind a host in its namespace",
			spec: agentgw.AgentHostSpec{Scope: scope.Workspace, WorkspaceID: ptr(1)},
			cur:  nsHost,
		},
		{
			name: "same namespace rebinds",
			spec: agentgw.AgentHostSpec{Scope: scope.Namespace, WorkspaceID: ptr(1), NamespaceID: ptr(7)},
			cur:  nsHost,
			want: true,
		},
		{
			name: "platform token rebinds anything",
			spec: agentgw.AgentHostSpec{Scope: scope.Platform},
			cur:  nsHost,
			want: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := rebindAuthorised(tc.spec, tc.cur); got != tc.want {
				t.Errorf("rebindAuthorised() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestAgentHostNameFitsColumn pins that every candidate name stays inside
// the 3-50 character host-name rule, including the longest one.
func TestAgentHostNameFitsColumn(t *testing.T) {
	const agentID = "9b91082a-6555-5db2-bd8e-be12f80602f8"
	for _, hostname := range []string{
		"a",
		"node-1",
		"this-hostname-is-far-longer-than-any-column-would-ever-allow.example.internal",
	} {
		for _, name := range agentHostNameCandidates(agentHostName(hostname), agentID) {
			if len(name) < 3 || len(name) > 50 {
				t.Errorf("hostname %q produced candidate %q of length %d, want 3..50",
					hostname, name, len(name))
			}
		}
	}
}
