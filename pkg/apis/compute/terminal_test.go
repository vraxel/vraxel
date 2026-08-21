package compute

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	agenttypes "vraxel.io/vraxel/lib/agent/types"
	"vraxel.io/vraxel/lib/apiserver"
	ws "vraxel.io/vraxel/lib/websocket"
	"vraxel.io/vraxel/pkg/apis/agentgw"
)

// terminalAction finds the terminal action on the hosts resource.
func terminalAction(t *testing.T) apiserver.ActionDef {
	t.Helper()
	def := HostsDef(nil, nil, nil, nil, NewTerminalSessions(), NewAgentDialerHolder(),
		NewAgentLiveMetrics(NewAgentDialerHolder()))
	for _, a := range def.Actions {
		if a.Name == "terminal" {
			return a
		}
	}
	t.Fatal("hosts declares no terminal action")
	return apiserver.ActionDef{}
}

// TestTerminalIsAudited pins the one property of this route that fails
// silently. A GET reaches the audit log only when its action is marked
// Interactive; drop that mark and the terminal keeps working perfectly
// while leaving no record of who opened a root shell on which machine --
// and an audit log with nothing in it looks exactly like a route nobody
// used.
func TestTerminalIsAudited(t *testing.T) {
	if !terminalAction(t).Interactive {
		t.Fatal("the terminal action is not marked Interactive, so opening one is never audited")
	}
}

// TestTerminalHasItsOwnPermission keeps the terminal from drifting onto a
// read permission. watch may borrow compute:hosts:list because it shows
// what listing shows; this runs arbitrary commands as root.
func TestTerminalHasItsOwnPermission(t *testing.T) {
	perms := terminalAction(t).Permission
	if len(perms) != 1 || perms[0] != "compute:hosts:terminal" {
		t.Fatalf("terminal permission = %v, want [compute:hosts:terminal]", perms)
	}
}

// TestTerminalIsOnTheItem guards the URL shape: a terminal is opened on
// one host, so the route must carry an id. On the collection it would be
// a shell on nothing in particular.
func TestTerminalIsOnTheItem(t *testing.T) {
	if !terminalAction(t).OnItem {
		t.Fatal("the terminal action is mounted on the collection, not on a host")
	}
}

func TestParseTerminalSize(t *testing.T) {
	tests := []struct {
		name       string
		cols, rows string
		wantC      int
		wantR      int
	}{
		{"absent falls back", "", "", ws.DefaultCols, ws.DefaultRows},
		{"normal", "100", "30", 100, 30},
		{"garbage falls back", "wide", "tall", ws.DefaultCols, ws.DefaultRows},
		// Out of range is a fallback, not a clamp: a client asking for
		// 100000 columns is malfunctioning, and honouring half its request
		// hides that better than ignoring it.
		{"too big falls back", "100000", "99999", ws.DefaultCols, ws.DefaultRows},
		{"too small falls back", "1", "1", ws.DefaultCols, ws.DefaultRows},
		{"negative falls back", "-80", "-24", ws.DefaultCols, ws.DefaultRows},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, r := parseTerminalSize(tc.cols, tc.rows)
			if c != tc.wantC || r != tc.wantR {
				t.Fatalf("size = %dx%d, want %dx%d", c, r, tc.wantC, tc.wantR)
			}
		})
	}
}

func TestExitMessage(t *testing.T) {
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
		{"clean exit", mustJSON(agenttypes.PTYExit{Code: 0}), "shell exited normally"},
		{"nonzero code", mustJSON(agenttypes.PTYExit{Code: 130}), "shell exited with code 130"},
		{"error wins over code", mustJSON(agenttypes.PTYExit{Code: 1, Error: "no shell"}), "shell exited: no shell"},
		// Undecodable is not an error path worth surfacing: the shell did
		// end, and that is what the operator needs told.
		{"garbage still reports the exit", []byte("{"), "shell exited"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := exitMessage(tc.payload); got != tc.want {
				t.Fatalf("exitMessage = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestAgentDialerHolderStartsEmpty covers the assembly order: compute's
// routes are built before the gateway exists, so the holder must answer
// nil rather than panic until it is filled.
func TestAgentDialerHolderStartsEmpty(t *testing.T) {
	if NewAgentDialerHolder().Get() != nil {
		t.Fatal("a fresh holder returned a dialer")
	}
	var nilHolder *AgentDialerHolder
	if nilHolder.Get() != nil {
		t.Fatal("a nil holder returned a dialer")
	}
}

// TestValidTerminalSizeRejectsUint16Wrap pins the resize bound. The value
// crosses the wire as a uint16 and lands in pty.Setsize on a managed
// machine, where nothing checks it -- so 65536 silently becomes a
// zero-column PTY and an unusable shell. The opening size was always
// bounded; this is the same value arriving by the other route.
func TestValidTerminalSizeRejectsUint16Wrap(t *testing.T) {
	tests := []struct {
		name       string
		cols, rows int
		want       bool
	}{
		{"ordinary", 120, 40, true},
		{"at the bounds", maxTerminalCols, maxTerminalRows, true},
		{"wraps to zero columns", 65536, 40, false},
		{"wraps to zero rows", 120, 65536, false},
		{"zero", 0, 0, false},
		{"negative", -1, -1, false},
		{"below the floor", 9, 4, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := validTerminalSize(tc.cols, tc.rows); got != tc.want {
				t.Fatalf("validTerminalSize(%d, %d) = %v, want %v", tc.cols, tc.rows, got, tc.want)
			}
		})
	}
}

// TestOpenFailureReasonSeparatesTheCauses pins that the five ways a
// terminal fails to open reach the operator as five different sentences.
// They call for different actions -- install an agent, wait, retry,
// report a bug -- and one shared message makes the screen useless for
// telling them apart, which is exactly the state this replaced.
func TestOpenFailureReasonSeparatesTheCauses(t *testing.T) {
	reasons := map[string]string{
		"offline":   openFailureReason(agentgw.ErrHostUnreachable),
		"elsewhere": openFailureReason(fmt.Errorf("wrapped: %w", agentgw.ErrHostOnAnotherInstance)),
		"timed out": openFailureReason(context.DeadlineExceeded),
		"agent said no": openFailureReason(&agentgw.StreamRejected{
			Code: agenttypes.StreamErrTargetNotAllowed, Message: "not loopback",
		}),
		"anything else": openFailureReason(errors.New("tunnel broke")),
	}

	seen := map[string]string{}
	for name, msg := range reasons {
		if msg == "" {
			t.Fatalf("%s produced an empty message", name)
		}
		if other, dup := seen[msg]; dup {
			t.Fatalf("%s and %s both say %q; the operator cannot tell them apart", name, other, msg)
		}
		seen[msg] = name
	}

	// The agent's own words have to survive: "not loopback" is the only
	// thing that says which rule the request broke.
	if !strings.Contains(reasons["agent said no"], "not loopback") {
		t.Fatalf("the agent's reason was dropped: %q", reasons["agent said no"])
	}
}
