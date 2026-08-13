package compute

import (
	"testing"

	"vraxel.io/vraxel/pkg/apis/agentgw"
	modstore "vraxel.io/vraxel/pkg/apis/compute/store"
	"vraxel.io/vraxel/pkg/apis/shared/scope"
)

func ptr(v int64) *int64 { return &v }

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
