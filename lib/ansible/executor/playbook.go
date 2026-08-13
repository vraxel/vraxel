package executor

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"vraxel.io/vraxel/lib/ansible"
	"vraxel.io/vraxel/lib/ansible/connector"
	"vraxel.io/vraxel/lib/ansible/modules"
	"vraxel.io/vraxel/lib/ansible/planner"
	"vraxel.io/vraxel/lib/ansible/project"
	"vraxel.io/vraxel/lib/ansible/variable"
)

// PlaybookExecutor orchestrates playbook execution.
type PlaybookExecutor struct {
	inventory ansible.Inventory
	source    project.Source
	logOutput io.Writer
	callback  EventCallback
	tags      []string
	skipTags  []string
	// skipRunOnce drops run_once blocks; see WithSkipRunOnce.
	skipRunOnce bool
	// newConnector builds a host's connector. Defaults to the local-only
	// factory; callers that need SSH pass connector/ssh's, which is what
	// keeps the SSH client and SFTP library out of the agent binary.
	newConnector ConnectorFactory
	connectors   *connectorRegistry
	variable     variable.Variable
	planner      *planner.Planner

	// liveOutput is sticky so connectors created mid-play inherit it
	// (attachLiveOutput in initConnectors handles per-connector wiring).
	// Guarded by liveMu — separate from the connectorRegistry's lock so
	// SetLiveOutput's full sequence (write field + iterate connectors)
	// stays atomic without contending with map writes.
	liveMu     sync.Mutex
	liveOutput io.Writer

	// delegateMu serialises on-demand connector creation for delegate_to
	// targets. Task workers run in parallel, so without it two hosts
	// delegating to the same third host would each build a connector and
	// one would be orphaned in the registry.
	delegateMu sync.Mutex
}

// Resize forwards a window-size change to every connector that has
// declared a Resize method (currently SSHConnector with PTY enabled).
// Safe to call concurrently with Execute — the registry snapshot is
// race-free, and each connector serialises its own resize channel.
func (e *PlaybookExecutor) Resize(ctx context.Context, rows, cols uint16) error {
	var firstErr error
	for _, c := range e.connectors.Snapshot() {
		if err := c.Resize(ctx, rows, cols); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// SetLiveOutput attaches a streaming byte sink to every connector
// (existing and future) that implements LiveOutputter. Connectors
// created later in the play inherit the writer. Pass nil to disable.
func (e *PlaybookExecutor) SetLiveOutput(w io.Writer) {
	e.liveMu.Lock()
	e.liveOutput = w
	e.liveMu.Unlock()
	for _, c := range e.connectors.Snapshot() {
		if lo, ok := c.(connector.LiveOutputter); ok {
			lo.SetLiveOutput(w)
		}
	}
}

// attachLiveOutput propagates the saved liveOutput onto a freshly
// created connector. Called from initConnectors after each
// connector.Init so per-host PTY mode picks up the live writer
// without the caller having to re-call SetLiveOutput.
func (e *PlaybookExecutor) attachLiveOutput(c connector.Connector) {
	e.liveMu.Lock()
	w := e.liveOutput
	e.liveMu.Unlock()
	if w == nil {
		return
	}
	if lo, ok := c.(connector.LiveOutputter); ok {
		lo.SetLiveOutput(w)
	}
}

// ConnectorFactory builds the connector for one host from its vars.
type ConnectorFactory func(host string, vars map[string]any) (connector.Connector, error)

// Option configures the executor.
type Option func(*PlaybookExecutor)

// WithConnectorFactory replaces the connector factory.
//
// The default handles local execution only. Anything that runs plays
// over SSH must pass ssh.NewConnector; asking the default for an SSH
// connection returns an error naming this option, so a missing wire-up
// is a clear message rather than a puzzling failure further along.
func WithConnectorFactory(f ConnectorFactory) Option {
	return func(e *PlaybookExecutor) { e.newConnector = f }
}

// WithLogOutput sets the log output writer.
func WithLogOutput(w io.Writer) Option { return func(e *PlaybookExecutor) { e.logOutput = w } }

// WithTags sets the playbook-level --tags filter.
func WithTags(tags []string) Option { return func(e *PlaybookExecutor) { e.tags = tags } }

// WithSkipTags sets the playbook-level --skip-tags filter.
func WithSkipTags(tags []string) Option { return func(e *PlaybookExecutor) { e.skipTags = tags } }

// WithSkipRunOnce drops run_once blocks rather than running them on the
// play's first host. Used by the agent, where each job is one play on one
// host and only the server-chosen batch leader may run them.
func WithSkipRunOnce() Option {
	return func(e *PlaybookExecutor) { e.skipRunOnce = true }
}

// WithEventCallback sets the event callback for structured progress events.
func WithEventCallback(cb EventCallback) Option {
	return func(e *PlaybookExecutor) { e.callback = cb }
}

// NewPlaybookExecutor creates a new executor.
func NewPlaybookExecutor(inv ansible.Inventory, source project.Source, opts ...Option) *PlaybookExecutor {
	e := &PlaybookExecutor{
		inventory:    inv,
		source:       source,
		logOutput:    io.Discard,
		connectors:   newConnectorRegistry(),
		newConnector: connector.NewConnector,
	}
	for _, opt := range opts {
		opt(e)
	}
	e.variable = variable.New(inv)
	e.planner = planner.New(inv)
	return e
}

// emit sends an event to the callback if one is set.
func (e *PlaybookExecutor) emit(ev Event) {
	if e.callback != nil {
		ev.Timestamp = time.Now()
		e.callback(ev)
	}
}

// Execute runs a playbook and returns the result.
func (e *PlaybookExecutor) Execute(ctx context.Context, playbook *ansible.Playbook) (*ansible.PlaybookResult, error) {
	result := &ansible.PlaybookResult{
		StartTime: time.Now(),
		Stats:     ansible.NewPlaybookStats(),
		Success:   true,
	}

	// Process each play
	for i, play := range playbook.Play {
		if e.logOutput != nil {
			fmt.Fprintf(e.logOutput, "\nPLAY [%s] ***\n", play.Name)
		}
		e.emit(Event{Type: EventPlayStart, Play: play.Name})

		if err := e.execPlay(ctx, i, play, result); err != nil {
			e.emit(Event{Type: EventPlayEnd, Play: play.Name, Error: err.Error()})
			result.Success = false
			result.Error = fmt.Sprintf("play %d '%s': %v", i+1, play.Name, err)
			result.EndTime = time.Now()
			return result, fmt.Errorf("play %d '%s': %w", i+1, play.Name, err)
		}
		e.emit(Event{Type: EventPlayEnd, Play: play.Name})
	}

	result.EndTime = time.Now()
	return result, nil
}

// execPlay executes a single play.
func (e *PlaybookExecutor) execPlay(ctx context.Context, playIndex int, play ansible.Play, result *ansible.PlaybookResult) error {
	// 1. Plan the play: host resolution and serial batching live in the
	//    planner (lib/ansible/planner); the executor only consumes its output.
	plan, err := e.planner.PlanPlay(playIndex, play)
	if err != nil {
		return err
	}
	if len(plan) == 0 {
		// Standard ansible semantics: a play that matches no hosts is SKIPPED
		// (a warning), not a fatal error. Treating it as fatal breaks any
		// playbook with a conditional/optional play whose host group can be
		// empty -- e.g. add-nodes.yml's "Join new master nodes" when only
		// workers are being added (new_masters group empty), or install.yml's
		// "Join worker nodes" on a master-only topology (workers group empty).
		// Skip the play and continue; the rest of the playbook runs normally.
		if e.logOutput != nil {
			fmt.Fprintf(e.logOutput, "skipping: no hosts matched\n")
		}
		return nil
	}

	// Batches are contiguous slices of the play's resolved host list, so
	// concatenating them reproduces it for connector/vars/facts setup.
	var hosts []string
	for _, pp := range plan {
		hosts = append(hosts, pp.Hosts...)
	}

	// 2. Initialize connectors for all hosts
	if err := e.initConnectors(ctx, hosts, play); err != nil {
		return err
	}
	defer e.closeConnectors(ctx)

	// 3. Merge play-level variables
	if len(play.Vars.Nodes) > 0 {
		e.variable.Merge(variable.MergeRuntimeVariable(play.Vars.Nodes, hosts...))
	}

	// 4. Load vars_files
	for _, vf := range play.VarsFiles {
		e.loadVarsFile(vf, hosts)
	}

	// 5. Gather facts if enabled
	if play.GatherFacts {
		if err := e.gatherFacts(ctx, hosts); err != nil {
			return fmt.Errorf("gather_facts: %w", err)
		}
	}

	// 6. Execute each serial batch in plan order
	for _, pp := range plan {
		if err := e.execBatch(ctx, play, pp.Hosts, result); err != nil {
			return err
		}
	}

	return nil
}

// countBlocks counts the number of executable tasks in a block list.
func countBlocks(blocks []ansible.Block) int {
	count := 0
	for _, b := range blocks {
		if len(b.BlockInfo.Block) > 0 {
			count += countBlocks(b.BlockInfo.Block)
			count += countBlocks(b.BlockInfo.Rescue)
			count += countBlocks(b.BlockInfo.Always)
		} else {
			count++
		}
	}
	return count
}

// execBatch executes pre_tasks -> roles -> tasks -> post_tasks for a host batch.
func (e *PlaybookExecutor) execBatch(ctx context.Context, play ansible.Play, hosts []string, result *ansible.PlaybookResult) error {
	taskExec := NewTaskExecutor(e.variable, e.source, e.connectors, e.logOutput,
		func(ctx context.Context, host string) (connector.Connector, error) {
			return e.connectorForDelegate(ctx, host, play)
		})

	// Count total tasks for progress tracking (best-effort for inline tasks)
	total := countBlocks(play.PreTasks) + countBlocks(play.Tasks) + countBlocks(play.PostTasks)
	for _, role := range play.Roles {
		total += countBlocks(role.Blocks)
	}
	taskIndex := 0

	makeBlockExec := func() *BlockExecutor {
		be := NewBlockExecutor(taskExec, e.variable, e.source, e.connectors, hosts, e.logOutput)
		be.WithPlayTags(e.tags)
		be.WithPlaySkipTags(e.skipTags)
		be.WithIgnoreErrors(play.IgnoreErrors)
		be.WithPlayBecome(play.Become)
		be.callback = e.callback
		be.WithTaskProgress(&taskIndex, total)
		be.WithSkipRunOnce(e.skipRunOnce)
		return be
	}

	if err := e.execBatchTaskPhase(ctx, "\nPRE TASKS ***", "pre_tasks", makeBlockExec, play.PreTasks); err != nil {
		return err
	}

	if err := e.execBatchRoles(ctx, play, hosts, makeBlockExec); err != nil {
		return err
	}

	if err := e.execBatchTaskPhase(ctx, "\nTASKS ***", "tasks", makeBlockExec, play.Tasks); err != nil {
		return err
	}

	if err := e.execBatchTaskPhase(ctx, "\nPOST TASKS ***", "post_tasks", makeBlockExec, play.PostTasks); err != nil {
		return err
	}

	return nil
}

// execBatchTaskPhase runs one of the pre_tasks / tasks / post_tasks phases:
// logs the section header and executes the blocks, wrapping any error with
// errPrefix. Extracted from execBatch to cut its cognitive complexity. No-op
// when blocks is empty (preserving the original len()>0 guard).
func (e *PlaybookExecutor) execBatchTaskPhase(ctx context.Context, header, errPrefix string, makeBlockExec func() *BlockExecutor, blocks []ansible.Block) error {
	if len(blocks) > 0 {
		if e.logOutput != nil {
			fmt.Fprintln(e.logOutput, header)
		}
		if err := makeBlockExec().Exec(ctx, blocks); err != nil {
			return fmt.Errorf("%s: %w", errPrefix, err)
		}
	}
	return nil
}

// execBatchRoles runs the play's roles in order, each via its own
// RoleExecutor. Extracted from execBatch to cut its cognitive complexity.
func (e *PlaybookExecutor) execBatchRoles(ctx context.Context, play ansible.Play, hosts []string, makeBlockExec func() *BlockExecutor) error {
	for _, role := range play.Roles {
		if e.logOutput != nil {
			fmt.Fprintf(e.logOutput, "\nROLE [%s] ***\n", role.Role)
		}
		roleExec := NewRoleExecutor(makeBlockExec(), e.source, e.variable, hosts, e.logOutput)
		if err := roleExec.Exec(ctx, role); err != nil {
			return fmt.Errorf("role '%s': %w", role.Role, err)
		}
	}
	return nil
}

// initConnectors creates connectors for hosts.
// Play-level become settings are applied as defaults when the inventory
// does not already specify them for a host.
func (e *PlaybookExecutor) initConnectors(ctx context.Context, hosts []string, play ansible.Play) error {
	for _, host := range hosts {
		if e.connectors.Has(host) {
			continue
		}
		if err := e.initConnector(ctx, host, play); err != nil {
			return err
		}
	}
	return nil
}

// initConnector creates, initialises and registers a single host's connector.
// Extracted from initConnectors to cut its cognitive complexity.
func (e *PlaybookExecutor) initConnector(ctx context.Context, host string, play ansible.Play) error {
	// Get host variables for connection config
	rawVars := e.variable.Get(variable.GetAllVariable(host))
	vars, ok := rawVars.(map[string]any)
	if !ok {
		vars = make(map[string]any)
	}

	initConnectorBecomeDefaults(vars, play)

	conn, err := e.newConnector(host, vars)
	if err != nil {
		return fmt.Errorf("create connector for %s: %w", host, err)
	}
	if err := conn.Init(ctx); err != nil {
		return fmt.Errorf("init connector for %s: %w", host, err)
	}
	e.attachLiveOutput(conn)
	e.connectors.Put(host, conn)
	return nil
}

// initConnectorBecomeDefaults merges play-level become settings into a host's
// connection vars, leaving any inventory-provided values untouched. Extracted
// from initConnectors to cut its cognitive complexity.
func initConnectorBecomeDefaults(vars map[string]any, play ansible.Play) {
	// Merge play-level become into vars (if not already set by inventory)
	if play.Become {
		if _, exists := vars["become"]; !exists {
			vars["become"] = true
		}
	}
	if play.BecomeUser != "" {
		if _, exists := vars["become_user"]; !exists {
			vars["become_user"] = play.BecomeUser
		}
	}
}

// closeConnectors closes every connector the play opened. It drains the
// registry rather than walking the play's host list so that connectors made
// on demand for delegate_to targets are closed too.
func (e *PlaybookExecutor) closeConnectors(ctx context.Context) {
	for _, conn := range e.connectors.Drain() {
		_ = conn.Close(ctx)
	}
}

// connectorForDelegate returns the connector for a delegate_to target,
// creating and registering it on first use. A delegate can be a host the play
// never listed, so the play's initConnectors pass will not have covered it.
func (e *PlaybookExecutor) connectorForDelegate(ctx context.Context, host string, play ansible.Play) (connector.Connector, error) {
	e.delegateMu.Lock()
	defer e.delegateMu.Unlock()

	// Re-check under the lock: a parallel task worker may have just made it.
	if conn := e.connectors.Get(host); conn != nil {
		return conn, nil
	}
	if err := e.initConnector(ctx, host, play); err != nil {
		return nil, err
	}
	return e.connectors.Get(host), nil
}

// gatherFacts runs setup module on all hosts.
func (e *PlaybookExecutor) gatherFacts(ctx context.Context, hosts []string) error {
	setupFn := modules.FindModule("setup")
	if setupFn == nil {
		return fmt.Errorf("setup module not registered")
	}
	for _, host := range hosts {
		_, _, err := setupFn(ctx, modules.ExecOptions{
			Host:      host,
			Variable:  e.variable,
			Connector: e.connectors.Get(host),
			LogOutput: e.logOutput,
		})
		if err != nil {
			return fmt.Errorf("host %s: %w", host, err)
		}
	}
	return nil
}

// loadVarsFile loads a YAML variables file and merges it into runtime vars for the given hosts.
// If the file does not exist or fails to parse, it is silently ignored.
func (e *PlaybookExecutor) loadVarsFile(path string, hosts []string) {
	if e.source == nil {
		return
	}
	data, err := e.source.ReadFile(path)
	if err != nil {
		return // file not found or read error — silently skip
	}
	var vars map[string]any
	if err := yaml.Unmarshal(data, &vars); err != nil {
		return // parse error — silently skip
	}
	if len(vars) == 0 {
		return
	}
	for _, host := range hosts {
		e.variable.Merge(variable.MergeHostRuntimeVars(host, vars))
	}
}
