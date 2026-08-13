package service

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"strings"
	"testing"

	"vraxel.io/vraxel/lib/ansible/modules/internal"
)

// recordingConnector captures the systemctl commands the module issues; a
// test box has no unit to drive for real.
type recordingConnector struct {
	cmds []string
	fail bool
}

func (c *recordingConnector) Init(context.Context) error  { return nil }
func (c *recordingConnector) Close(context.Context) error { return nil }
func (c *recordingConnector) ExecuteCommand(_ context.Context, cmd string) ([]byte, []byte, error) {
	c.cmds = append(c.cmds, cmd)
	if c.fail {
		return nil, []byte("Unit not found"), fmt.Errorf("exit status 5")
	}
	return nil, nil, nil
}
func (c *recordingConnector) PutFile(context.Context, []byte, string, fs.FileMode) error { return nil }
func (c *recordingConnector) FetchFile(context.Context, string, io.Writer) error         { return nil }
func (c *recordingConnector) Resize(context.Context, uint16, uint16) error               { return nil }

func runService(t *testing.T, args map[string]any) (*recordingConnector, error) {
	t.Helper()
	conn := &recordingConnector{}
	_, _, err := ModuleService(context.Background(), internal.ExecOptions{Args: args, Connector: conn})
	return conn, err
}

func TestModuleService_StateVerbs(t *testing.T) {
	for state, want := range map[string]string{
		"started":   "systemctl start 'nginx'",
		"stopped":   "systemctl stop 'nginx'",
		"restarted": "systemctl restart 'nginx'",
		"reloaded":  "systemctl reload 'nginx'",
	} {
		t.Run(state, func(t *testing.T) {
			conn, err := runService(t, map[string]any{"name": "nginx", "state": state})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(conn.cmds) != 1 || conn.cmds[0] != want {
				t.Errorf("commands = %v, want [%s]", conn.cmds, want)
			}
		})
	}
}

func TestModuleService_EnableRunsBeforeStart(t *testing.T) {
	// A unit that is both enabled and started should have its boot-time
	// state in place before it comes up.
	conn, err := runService(t, map[string]any{"name": "nginx", "state": "started", "enabled": true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(conn.cmds) != 2 {
		t.Fatalf("expected enable then start, got %v", conn.cmds)
	}
	if !strings.Contains(conn.cmds[0], "enable") || !strings.Contains(conn.cmds[1], "start") {
		t.Errorf("expected enable before start, got %v", conn.cmds)
	}
}

func TestModuleService_DaemonReloadRunsFirst(t *testing.T) {
	conn, err := runService(t, map[string]any{"name": "app", "state": "restarted", "daemon_reload": true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(conn.cmds) != 2 || conn.cmds[0] != "systemctl daemon-reload" {
		t.Errorf("expected daemon-reload first, got %v", conn.cmds)
	}
}

func TestModuleService_EnabledFalseDisables(t *testing.T) {
	conn, err := runService(t, map[string]any{"name": "nginx", "enabled": false})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(conn.cmds) != 1 || !strings.Contains(conn.cmds[0], "disable") {
		t.Errorf("expected disable, got %v", conn.cmds)
	}
}

func TestModuleService_EnabledAcceptsRenderedStrings(t *testing.T) {
	// A templated value arrives as a string, not a YAML bool.
	conn, err := runService(t, map[string]any{"name": "nginx", "enabled": "true"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(conn.cmds) != 1 || !strings.Contains(conn.cmds[0], "enable") {
		t.Errorf("expected enable, got %v", conn.cmds)
	}
}

func TestModuleService_NameIsShellQuoted(t *testing.T) {
	conn, err := runService(t, map[string]any{"name": "a b'c", "state": "started"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(conn.cmds[0], `'a b'\''c'`) {
		t.Errorf("unit name not shell-escaped: %v", conn.cmds)
	}
}

func TestModuleService_ArgumentErrors(t *testing.T) {
	if _, err := runService(t, map[string]any{"state": "started"}); err == nil {
		t.Error("expected an error when name is missing")
	}
	if _, err := runService(t, map[string]any{"name": "nginx"}); err == nil {
		t.Error("expected an error when nothing was asked for")
	}
	if _, err := runService(t, map[string]any{"name": "nginx", "state": "bogus"}); err == nil {
		t.Error("expected an error for an unsupported state")
	}
}

func TestModuleService_SurfacesUnitFailure(t *testing.T) {
	conn := &recordingConnector{fail: true}
	_, _, err := ModuleService(context.Background(), internal.ExecOptions{
		Args:      map[string]any{"name": "nginx", "state": "started"},
		Connector: conn,
	})
	if err == nil {
		t.Fatal("expected the systemctl failure to surface")
	}
	if !strings.Contains(err.Error(), "Unit not found") {
		t.Errorf("expected stderr in the error, got %q", err.Error())
	}
}
