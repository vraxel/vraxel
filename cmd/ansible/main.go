// Command ansible runs a playbook against real hosts through lib/ansible.
//
// It exists so the engine can be exercised end to end against a real machine:
// SSH transport, sudo, systemd and the shell quoting of every module path are
// things no in-process test can cover, because the test machine is not the
// target machine. The engine's own test suite covers everything reachable
// through the local connector; this covers the rest.
//
// The defaults point at the engine's own E2E suite, so the common case is:
//
//	go run ./cmd/ansible -host 10.1.1.10 -user root -password secret
//
// Key-based auth, non-default port:
//
//	go run ./cmd/ansible -host 10.1.1.10 -port 2222 -user ops -key ~/.ssh/id_ed25519
//
// Multiple hosts, from a JSON inventory:
//
//	go run ./cmd/ansible -inventory hosts.json
//
// Any other playbook:
//
//	go run ./cmd/ansible -dir path/to/project -playbook site.yml -host 10.1.1.10
//
// The playbook path is relative to -dir, which is also where roles/ is
// looked up.
//
// Without a remote host, everything except the tasks tagged remote-only runs
// through the local connector:
//
//	go run ./cmd/ansible -host localhost -var connection=local \
//	  -become=false -skip-tag remote-only
//
// See README.md for what the suite covers.
//
// Inventory JSON:
//
//	{
//	  "hosts": {
//	    "10.1.1.12": {"node_role": "primary"},
//	    "10.1.1.13": {"node_role": "replica"}
//	  },
//	  "groups": {"db": {"hosts": ["10.1.1.12", "10.1.1.13"]}},
//	  "vars": {"remote_user": "root", "password": "secret"}
//	}
//
// Host vars win over inventory vars. connection=ssh and become=true are
// filled in when the inventory does not set them.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"vraxel.io/vraxel/lib/ansible"
	"vraxel.io/vraxel/lib/ansible/connector/ssh"
	"vraxel.io/vraxel/lib/ansible/converter"
	"vraxel.io/vraxel/lib/ansible/executor"
	"vraxel.io/vraxel/lib/ansible/project"

	// Register the built-in modules.
	_ "vraxel.io/vraxel/lib/ansible/modules"
)

func main() {
	opts := parseFlags()

	source := project.NewLocalSource(opts.dir)

	data, err := source.ReadFile(opts.playbook)
	if err != nil {
		exitf("read playbook %s: %v", opts.playbook, err)
	}
	playbook, err := converter.ParsePlaybook(data)
	if err != nil {
		exitf("parse playbook: %v", err)
	}

	inv := buildInventory(opts)

	execOpts := []executor.Option{
		executor.WithLogOutput(os.Stdout),
		// Without this the executor only knows how to talk to localhost;
		// the whole point here is to reach a real host.
		executor.WithConnectorFactory(ssh.NewConnector),
	}
	if len(opts.tags) > 0 {
		execOpts = append(execOpts, executor.WithTags(opts.tags))
	}
	if len(opts.skipTags) > 0 {
		execOpts = append(execOpts, executor.WithSkipTags(opts.skipTags))
	}

	result, err := executor.NewPlaybookExecutor(inv, source, execOpts...).
		Execute(context.Background(), playbook)
	if err != nil {
		exitf("execute: %v", err)
	}

	fmt.Printf("\n=== Result ===\n")
	fmt.Printf("Success:  %v\n", result.Success)
	fmt.Printf("Duration: %s\n", result.EndTime.Sub(result.StartTime))
	if result.Error != "" {
		fmt.Printf("Error:    %s\n", result.Error)
	}
	if !result.Success {
		os.Exit(1)
	}
}

// options holds the parsed command line.
type options struct {
	dir       string
	playbook  string
	inventory string
	host      string
	port      int
	user      string
	password  string
	key       string
	become    bool
	extraVars stringSlice
	tags      stringSlice
	skipTags  stringSlice
}

func parseFlags() options {
	var o options

	flag.StringVar(&o.dir, "dir", "cmd/ansible/e2e", "directory holding the playbook and roles/")
	flag.StringVar(&o.playbook, "playbook", "site.yml", "playbook path relative to -dir")
	flag.StringVar(&o.inventory, "inventory", "", "JSON inventory file (multi-host)")
	flag.StringVar(&o.host, "host", "", "target host (single-host mode)")
	flag.IntVar(&o.port, "port", 22, "SSH port")
	flag.StringVar(&o.user, "user", "root", "SSH user")
	flag.StringVar(&o.password, "password", "", "SSH password, also used for sudo")
	flag.StringVar(&o.key, "key", "", "SSH private key file")
	flag.BoolVar(&o.become, "become", true, "run tasks through sudo")
	flag.Var(&o.extraVars, "var", "inventory variable as key=value (repeatable)")
	flag.Var(&o.tags, "tag", "only run tasks with this tag (repeatable)")
	flag.Var(&o.skipTags, "skip-tag", "skip tasks with this tag (repeatable)")

	flag.Parse()

	if o.playbook == "" || (o.host == "" && o.inventory == "") {
		fmt.Fprintf(os.Stderr, "Usage:\n")
		fmt.Fprintf(os.Stderr, "  ansible -playbook <path> -host <ip> [options]\n")
		fmt.Fprintf(os.Stderr, "  ansible -playbook <path> -inventory <file.json>\n\n")
		flag.PrintDefaults()
		os.Exit(1)
	}
	return o
}

// buildInventory loads a JSON inventory or builds a single-host one from the
// flags, then applies -var overrides.
func buildInventory(o options) ansible.Inventory {
	inv := ansible.Inventory{}

	if o.inventory != "" {
		var err error
		if inv, err = loadInventory(o.inventory); err != nil {
			exitf("load inventory: %v", err)
		}
		if inv.Vars == nil {
			inv.Vars = make(map[string]any)
		}
		for k, v := range parseVars(o.extraVars) {
			inv.Vars[k] = v
		}
		return inv
	}

	hostVars := map[string]any{
		"connection":  "ssh",
		"port":        o.port,
		"remote_user": o.user,
		"become":      o.become,
		// The E2E playbook collects output into a buffer rather than a
		// terminal, and the PTY path drops output for short commands
		// without a live writer attached.
		"pty": false,
	}
	if o.password != "" {
		hostVars["password"] = o.password
	}
	if o.key != "" {
		data, err := os.ReadFile(o.key)
		if err != nil {
			exitf("read private key: %v", err)
		}
		hostVars["private_key_content"] = string(data)
	}
	for k, v := range parseVars(o.extraVars) {
		hostVars[k] = v
	}

	inv.Hosts = map[string]map[string]any{o.host: hostVars}
	return inv
}

// inventoryJSON is the on-disk shape of an inventory file.
type inventoryJSON struct {
	Hosts  map[string]map[string]any         `json:"hosts"`
	Groups map[string]ansible.InventoryGroup `json:"groups,omitempty"`
	Vars   map[string]any                    `json:"vars,omitempty"`
}

// loadInventory reads a JSON inventory, filling in the connection defaults and
// pushing inventory vars down into each host so single- and multi-host runs
// see the same variables.
func loadInventory(path string) (ansible.Inventory, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ansible.Inventory{}, fmt.Errorf("read %s: %w", path, err)
	}

	var raw inventoryJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return ansible.Inventory{}, fmt.Errorf("parse %s: %w", path, err)
	}
	if len(raw.Hosts) == 0 {
		return ansible.Inventory{}, fmt.Errorf("inventory has no hosts")
	}

	if raw.Vars == nil {
		raw.Vars = make(map[string]any)
	}
	for k, v := range map[string]any{"connection": "ssh", "become": true, "pty": false} {
		if _, ok := raw.Vars[k]; !ok {
			raw.Vars[k] = v
		}
	}

	for host, hostVars := range raw.Hosts {
		if hostVars == nil {
			hostVars = make(map[string]any)
		}
		for k, v := range raw.Vars {
			if _, exists := hostVars[k]; !exists {
				hostVars[k] = v
			}
		}
		raw.Hosts[host] = hostVars
	}

	return ansible.Inventory{Hosts: raw.Hosts, Groups: raw.Groups, Vars: raw.Vars}, nil
}

// parseVars turns repeated key=value flags into a map.
func parseVars(pairs stringSlice) map[string]any {
	out := make(map[string]any, len(pairs))
	for _, p := range pairs {
		if k, v, ok := strings.Cut(p, "="); ok {
			out[k] = v
		}
	}
	return out
}

func exitf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

// stringSlice implements flag.Value for repeatable flags.
type stringSlice []string

func (s *stringSlice) String() string { return strings.Join(*s, ", ") }
func (s *stringSlice) Set(v string) error {
	*s = append(*s, v)
	return nil
}
