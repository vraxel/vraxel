package ssh

import (
	"vraxel.io/vraxel/lib/ansible/connector"
	"vraxel.io/vraxel/lib/clients/sshclient"
)

// NewConnector is the full connector factory: everything the local-only
// factory in the parent package handles, plus SSH.
//
// Server-side callers pass this to executor.WithConnectorFactory. The
// agent does not, which is what keeps the SSH client and SFTP library
// out of its binary.
//
// Recognised variables:
//
//	connection, become, become_user, password,
//	port, remote_user, private_key, private_key_content,
//	pty (bool, default true for ssh -- allocates a PTY of pty_rows /
//	     pty_cols so command output is properly column-aligned for
//	     xterm.js),
//	pty_rows, pty_cols
func NewConnector(host string, vars map[string]any) (connector.Connector, error) {
	connType, _ := vars["connection"].(string)
	become, _ := vars["become"].(bool)
	becomeUser, _ := vars["become_user"].(string)
	password, _ := vars["password"].(string)

	switch connType {
	case "ssh":
		// Explicit ssh always means ssh, even for a loopback address.
		return newSSHConnectorFromVars(host, vars, become, becomeUser, password), nil
	case "":
		if !connector.IsLocalHost(host) {
			return newSSHConnectorFromVars(host, vars, become, becomeUser, password), nil
		}
	}
	// "local", implicit-local, and unsupported types are the parent
	// package's business; keeping one implementation of that answer
	// stops the two factories from drifting.
	return connector.NewConnector(host, vars)
}

// newSSHConnectorFromVars builds an SSHConnector from the recognised vars,
// applying PTY defaults unless the caller disables them.
func newSSHConnectorFromVars(host string, vars map[string]any, become bool, becomeUser, password string) connector.Connector {
	config := sshclient.Config{
		Host:     host,
		Password: password,
	}
	if port, ok := vars["port"].(int); ok {
		config.Port = port
	}
	if user, ok := vars["remote_user"].(string); ok {
		config.User = user
	}
	if key, ok := vars["private_key"].(string); ok {
		config.PrivateKey = key
	}
	if keyContent, ok := vars["private_key_content"].(string); ok {
		config.PrivateKeyContent = keyContent
	}
	conn := NewSSHConnector(config, become, becomeUser)
	// PTY mode is on by default -- every modern xterm.js consumer
	// expects properly TTY-formatted output. Callers that pump the
	// connector's output into a non-terminal sink can disable.
	usePty := true
	if v, ok := vars["pty"].(bool); ok {
		usePty = v
	}
	if usePty {
		rows, cols := sshConnectorPtySize(vars)
		conn.EnablePty(rows, cols)
	}
	return conn
}

// sshConnectorPtySize resolves the PTY dimensions, defaulting to 40x120
// and overriding from positive pty_rows / pty_cols vars.
func sshConnectorPtySize(vars map[string]any) (rows, cols uint16) {
	rows = uint16(40)
	cols = uint16(120)
	if v, ok := vars["pty_rows"].(int); ok && v > 0 {
		rows = uint16(v)
	}
	if v, ok := vars["pty_cols"].(int); ok && v > 0 {
		cols = uint16(v)
	}
	return rows, cols
}
