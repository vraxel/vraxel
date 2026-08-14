package compute

import (
	"errors"
	"net/http"
	"testing"
	"time"

	apierrors "vraxel.io/vraxel/lib/api/errors"
	"vraxel.io/vraxel/lib/apiserver"
	"vraxel.io/vraxel/lib/list"
	modstore "vraxel.io/vraxel/pkg/apis/compute/store"
)

// TestValidateHostCreate pins the rules a hand-entered host has to meet,
// and the one it deliberately does not: an address is optional, because
// a host reached only through an outbound agent has none worth
// recording. Requiring it would make operators invent a value.
func TestValidateHostCreate(t *testing.T) {
	valid := func() *Host {
		return &Host{Metadata: apiObjectMeta(0, "node-1", nil, nil)}
	}

	tests := []struct {
		name    string
		mutate  func(*Host)
		wantErr bool
	}{
		{name: "plain name", mutate: func(*Host) {}},
		{name: "no address is fine", mutate: func(h *Host) { h.Spec.IP = "" }},
		{name: "address parses", mutate: func(h *Host) { h.Spec.IP = "10.1.1.12" }},
		{name: "ipv6 parses", mutate: func(h *Host) { h.Spec.IP = "fd00::1" }},
		{name: "empty name", mutate: func(h *Host) { h.Metadata.Name = "" }, wantErr: true},
		{name: "name too short", mutate: func(h *Host) { h.Metadata.Name = "ab" }, wantErr: true},
		{name: "name starts with hyphen", mutate: func(h *Host) { h.Metadata.Name = "-node" }, wantErr: true},
		{name: "name ends with hyphen", mutate: func(h *Host) { h.Metadata.Name = "node-" }, wantErr: true},
		{name: "name has a dot", mutate: func(h *Host) { h.Metadata.Name = "node.1" }, wantErr: true},
		{name: "garbage address", mutate: func(h *Host) { h.Spec.IP = "10.1.1" }, wantErr: true},
		{name: "port out of range", mutate: func(h *Host) { h.Spec.SSHPort = 70000 }, wantErr: true},
		{
			name:    "description too long",
			mutate:  func(h *Host) { h.Spec.Description = string(make([]byte, maxDescriptionLen+1)) },
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := valid()
			tc.mutate(h)
			err := validateHostCreate(h)
			if tc.wantErr != (err != nil) {
				t.Fatalf("validateHostCreate() error = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantErr {
				var se *apierrors.StatusError
				if !errors.As(err, &se) || se.Status != http.StatusBadRequest {
					t.Errorf("want a 400, got %v", err)
				}
			}
		})
	}
}

// TestApplyHostScopeFilters pins that a scoped URL narrows the list.
// Without it every scope would answer with every host, and a workspace
// operator would see other tenants' machines.
func TestApplyHostScopeFilters(t *testing.T) {
	tests := []struct {
		name  string
		scope apiserver.ScopeInfo
		want  map[string]any
	}{
		{
			name:  "platform lists everything",
			scope: apiserver.ScopeInfo{Level: apiserver.ScopePlatform},
			want:  map[string]any{},
		},
		{
			name:  "workspace narrows to its own",
			scope: apiserver.ScopeInfo{Level: apiserver.ScopeWorkspace, WorkspaceID: 3},
			want:  map[string]any{"workspace_id": int64(3)},
		},
		{
			name:  "namespace narrows to its own",
			scope: apiserver.ScopeInfo{Level: apiserver.ScopeNamespace, WorkspaceID: 3, NamespaceID: 7},
			want:  map[string]any{"namespace_id": int64(7)},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			q := list.Query{Filters: map[string]any{}}
			applyHostScopeFilters(&q, tc.scope)
			if len(q.Filters) != len(tc.want) {
				t.Fatalf("filters = %v, want %v", q.Filters, tc.want)
			}
			for k, v := range tc.want {
				if q.Filters[k] != v {
					t.Errorf("filter %q = %v, want %v", k, q.Filters[k], v)
				}
			}
		})
	}
}

// TestHostToAPIKeepsTheNeverInstalledState pins the three-way reading of
// agent status. Flattening a nil to "offline" would erase the difference
// between a host waiting for an install and one whose machine stopped
// answering -- the first needs an operator, the second needs the machine
// back, and the list is where that is told apart.
func TestHostToAPIKeepsTheNeverInstalledState(t *testing.T) {
	online, offline := "online", "offline"
	version := "v0.4.1"
	seen := time.Now()

	t.Run("no agent ever bound", func(t *testing.T) {
		got := hostToAPI(&modstore.HostRow{ID: 1, Name: "imported"})
		if got.Spec.AgentStatus != "" {
			t.Errorf("agentStatus = %q, want empty", got.Spec.AgentStatus)
		}
		if got.Spec.AgentVersion != "" {
			t.Errorf("agentVersion = %q, want empty", got.Spec.AgentVersion)
		}
	})

	t.Run("agent bound and answering", func(t *testing.T) {
		got := hostToAPI(&modstore.HostRow{
			ID: 2, Name: "node-1",
			AgentStatus: &online, AgentVersion: &version, AgentLastSeenAt: &seen,
		})
		if got.Spec.AgentStatus != "online" || got.Spec.AgentVersion != version {
			t.Errorf("got %q / %q", got.Spec.AgentStatus, got.Spec.AgentVersion)
		}
	})

	t.Run("agent bound but silent", func(t *testing.T) {
		got := hostToAPI(&modstore.HostRow{ID: 3, Name: "node-2", AgentStatus: &offline})
		if got.Spec.AgentStatus != "offline" {
			t.Errorf("agentStatus = %q, want offline", got.Spec.AgentStatus)
		}
	})
}
