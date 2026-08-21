package compute

import (
	"encoding/json"
	"slices"
	"testing"

	agenttypes "vraxel.io/vraxel/lib/agent/types"
	"vraxel.io/vraxel/lib/apiserver"
)

// logsAction finds the logs action on the hosts resource.
func logsAction(t *testing.T) apiserver.ActionDef {
	t.Helper()
	def := HostsDef(nil, nil, nil, nil, NewTerminalSessions(), NewAgentDialerHolder(),
		NewAgentLiveMetrics(NewAgentDialerHolder()))
	for _, a := range def.Actions {
		if a.Name == "logs" {
			return a
		}
	}
	t.Fatal("hosts declares no logs action")
	return apiserver.ActionDef{}
}

// TestLogsIsAudited pins the audit mark for the same reason the terminal
// pins it: drop MarkInteractive and the route keeps working while no
// record says who read a machine's auth log.
func TestLogsIsAudited(t *testing.T) {
	if !logsAction(t).Interactive {
		t.Fatal("the logs action is not marked Interactive, so reading logs is never audited")
	}
}

// TestLogsHasItsOwnPermission keeps the grant honest in both directions:
// not compute:hosts:get, because the journal carries auth logs and
// service output that "may read this host's details" should not cover;
// and not compute:hosts:terminal, because granting "may read logs" must
// not require granting a root shell.
func TestLogsHasItsOwnPermission(t *testing.T) {
	perms := logsAction(t).Permission
	if len(perms) != 1 || perms[0] != "compute:hosts:logs" {
		t.Fatalf("logs permission = %v, want [compute:hosts:logs]", perms)
	}
}

// TestLogsIsOnTheItem guards the URL shape: logs are read from one host.
func TestLogsIsOnTheItem(t *testing.T) {
	if !logsAction(t).OnItem {
		t.Fatal("the logs action is mounted on the collection, not on a host")
	}
}

// TestLogCommandFromParams pins the trust boundary of this feature: the
// browser sends a source name and a few bounded values, and everything
// that reaches a root process's argv is composed or validated here.
func TestLogCommandFromParams(t *testing.T) {
	tests := []struct {
		name    string
		params  map[string]string
		want    []string
		wantErr bool
	}{
		{
			name:   "journal defaults follow on",
			params: map[string]string{"source": "journal"},
			want:   []string{"journalctl", "--no-pager", "-o", "short-iso", "-n", "500", "-f"},
		},
		{
			name:   "journal without follow is a bounded read",
			params: map[string]string{"source": "journal", "follow": "false"},
			want:   []string{"journalctl", "--no-pager", "-o", "short-iso", "-n", "500"},
		},
		{
			name:   "journal unit and priority",
			params: map[string]string{"source": "journal", "unit": "nginx.service", "priority": "warning", "tail": "200"},
			want:   []string{"journalctl", "--no-pager", "-o", "short-iso", "-n", "200", "-u", "nginx.service", "-p", "warning", "-f"},
		},
		{
			name:   "unit globs pass through as one argv element",
			params: map[string]string{"source": "journal", "unit": "nginx*"},
			want:   []string{"journalctl", "--no-pager", "-o", "short-iso", "-n", "500", "-u", "nginx*", "-f"},
		},
		{
			name:   "kernel is dmesg",
			params: map[string]string{"source": "kernel"},
			want:   []string{"journalctl", "--no-pager", "-o", "short-iso", "-n", "500", "-k", "-f"},
		},
		{
			// The UI hides the unit field for the kernel source; a
			// hand-built URL that sends one anyway must not smuggle it in.
			name:   "kernel ignores a unit",
			params: map[string]string{"source": "kernel", "unit": "nginx.service"},
			want:   []string{"journalctl", "--no-pager", "-o", "short-iso", "-n", "500", "-k", "-f"},
		},
		{
			name:   "file follow survives rotation",
			params: map[string]string{"source": "file", "path": "/var/log/nginx/access.log"},
			want:   []string{"tail", "-F", "-n", "500", "--", "/var/log/nginx/access.log"},
		},
		{
			name:   "file without follow",
			params: map[string]string{"source": "file", "path": "/var/log/syslog", "follow": "false"},
			want:   []string{"tail", "-n", "500", "--", "/var/log/syslog"},
		},
		{
			name:   "tail clamps at the cap",
			params: map[string]string{"source": "journal", "tail": "99999"},
			want:   []string{"journalctl", "--no-pager", "-o", "short-iso", "-n", "10000", "-f"},
		},
		{
			name:   "tail garbage falls back",
			params: map[string]string{"source": "journal", "tail": "many"},
			want:   []string{"journalctl", "--no-pager", "-o", "short-iso", "-n", "500", "-f"},
		},
		{name: "no source", params: map[string]string{}, wantErr: true},
		{name: "unknown source", params: map[string]string{"source": "docker"}, wantErr: true},
		{
			// A leading dash could read as a flag to journalctl; nothing
			// with one is a unit name.
			name:    "unit with a leading dash",
			params:  map[string]string{"source": "journal", "unit": "-x"},
			wantErr: true,
		},
		{
			name:    "unit with spaces",
			params:  map[string]string{"source": "journal", "unit": "a b"},
			wantErr: true,
		},
		{
			name:    "priority outside the syslog set",
			params:  map[string]string{"source": "journal", "priority": "verbose"},
			wantErr: true,
		},
		{name: "file without a path", params: map[string]string{"source": "file"}, wantErr: true},
		{
			// The permission says "may read this host's logs"; a path
			// outside /var/log would make it "may read any file as root".
			name:    "path outside /var/log",
			params:  map[string]string{"source": "file", "path": "/etc/shadow"},
			wantErr: true,
		},
		{
			name:    "path escaping via dot-dot",
			params:  map[string]string{"source": "file", "path": "/var/log/../../etc/shadow"},
			wantErr: true,
		},
		{
			name:    "path with a newline",
			params:  map[string]string{"source": "file", "path": "/var/log/a\nb"},
			wantErr: true,
		},
		{
			name:    "the directory itself is not a log",
			params:  map[string]string{"source": "file", "path": "/var/log/"},
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := logCommandFromParams(tc.params)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("argv = %v, want an error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !slices.Equal(got, tc.want) {
				t.Fatalf("argv = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestLogEndMessage(t *testing.T) {
	mustJSON := func(v agenttypes.PTYExit) []byte {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		return b
	}
	tests := []struct {
		name    string
		payload []byte
		want    string
	}{
		// Code 0 is the ordinary end of a non-follow read.
		{"clean end", mustJSON(agenttypes.PTYExit{Code: 0}), "log stream ended"},
		{"nonzero code", mustJSON(agenttypes.PTYExit{Code: 1}), "log stream ended with code 1"},
		{"error wins over code", mustJSON(agenttypes.PTYExit{Code: 1, Error: "killed"}), "log stream ended: killed"},
		{"garbage still reports the end", []byte("{"), "log stream ended"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := logEndMessage(tc.payload); got != tc.want {
				t.Fatalf("logEndMessage = %q, want %q", got, tc.want)
			}
		})
	}
}
