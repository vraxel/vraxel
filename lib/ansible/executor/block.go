package executor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"vraxel.io/vraxel/lib/ansible"
	"vraxel.io/vraxel/lib/ansible/converter"
	"vraxel.io/vraxel/lib/ansible/modules"
	"vraxel.io/vraxel/lib/ansible/project"
	"vraxel.io/vraxel/lib/ansible/variable"
)

// noLogRedacted replaces a no_log task's captured output in every sink that
// outlives the run (job log, WS stream, returned error). A placeholder rather
// than an empty string so an operator reading the log knows output existed and
// why it is missing.
const noLogRedacted = "<output withheld: task is marked no_log>"

// BlockExecutor executes a list of blocks.
type BlockExecutor struct {
	taskExecutor *TaskExecutor
	variable     variable.Variable
	source       project.Source
	logOutput    io.Writer
	connectors   *connectorRegistry
	hosts        []string
	ignoreErrors *bool
	when         []string     // inherited when conditions
	tags         ansible.Tags // inherited tags
	playTags     []string     // playbook-level --tags
	playSkipTags []string     // playbook-level --skip-tags
	role         string       // current role name (for logging)
	callback     EventCallback
	taskIndex    *int // shared counter across all blocks in a play
	taskTotal    int  // total tasks in the play
	playBecome   bool // inherited play-level become

	// skipRunOnce drops run_once blocks entirely instead of running them
	// on the first host. Set only on the agent, where a play has been
	// localized to one host per job: without it every agent in a batch
	// would see itself as "the first host" and run the run_once block,
	// turning a once-per-batch step into once-per-host. The server picks
	// the single batch leader (planner.PlayPlan.RunOnceTarget) and sets
	// this on every other job.
	skipRunOnce bool

	// includeStack tracks the chain of include_tasks files currently
	// being executed in this call stack. dealIncludeTasks pushes its
	// file before recursing into the sub-BlockExecutor and the sub
	// executor inherits the stack. If the file is already in the
	// stack, that is a cycle (direct A->A or indirect A->B->A) and
	// dealIncludeTasks fails immediately with a recursive-include
	// error. Scoped to the call stack, not the lifetime history, so
	// re-including the same helper file at separate sequential steps
	// stays legal — only nested re-entry trips the check.
	includeStack []string
}

// NewBlockExecutor creates a new block executor.
func NewBlockExecutor(
	taskExec *TaskExecutor,
	v variable.Variable,
	src project.Source,
	conns *connectorRegistry,
	hosts []string,
	logOutput io.Writer,
) *BlockExecutor {
	return &BlockExecutor{
		taskExecutor: taskExec,
		variable:     v,
		source:       src,
		logOutput:    logOutput,
		connectors:   conns,
		hosts:        hosts,
	}
}

// WithRole sets the role name for logging.
func (e *BlockExecutor) WithRole(role string) *BlockExecutor {
	e.role = role
	return e
}

// WithIgnoreErrors sets the inherited ignore_errors flag.
func (e *BlockExecutor) WithPlayBecome(become bool) *BlockExecutor {
	e.playBecome = become
	return e
}

func (e *BlockExecutor) WithIgnoreErrors(ie *bool) *BlockExecutor {
	e.ignoreErrors = ie
	return e
}

// WithSkipRunOnce drops run_once blocks instead of running them on the
// first host.
func (e *BlockExecutor) WithSkipRunOnce(skip bool) *BlockExecutor {
	e.skipRunOnce = skip
	return e
}

// WithWhen sets the inherited when conditions.
func (e *BlockExecutor) WithWhen(when []string) *BlockExecutor {
	e.when = when
	return e
}

// WithTags sets the inherited tags.
func (e *BlockExecutor) WithTags(tags ansible.Tags) *BlockExecutor {
	e.tags = tags
	return e
}

// WithPlayTags sets the playbook-level --tags filter.
func (e *BlockExecutor) WithPlayTags(tags []string) *BlockExecutor {
	e.playTags = tags
	return e
}

// WithPlaySkipTags sets the playbook-level --skip-tags filter.
func (e *BlockExecutor) WithPlaySkipTags(tags []string) *BlockExecutor {
	e.playSkipTags = tags
	return e
}

// WithTaskProgress sets the shared task counter and total for progress tracking.
func (e *BlockExecutor) WithTaskProgress(index *int, total int) *BlockExecutor {
	e.taskIndex = index
	e.taskTotal = total
	return e
}

// Exec executes all blocks sequentially.
func (e *BlockExecutor) Exec(ctx context.Context, blocks []ansible.Block) error {
	for _, block := range blocks {
		if err := e.execBlock(ctx, block); err != nil {
			return err
		}
	}
	return nil
}

func (e *BlockExecutor) execBlock(ctx context.Context, block ansible.Block) error {
	// 1. Determine effective hosts (respect run_once)
	hosts := e.hosts
	if block.RunOnce {
		if e.skipRunOnce {
			// Advance past the tasks that will not run. countBlocks
			// included them in the play's total, so returning without
			// this leaves the progress stuck short of the end on every
			// host that is not the batch leader.
			if e.taskIndex != nil {
				*e.taskIndex += countBlocks([]ansible.Block{block})
			}
			return nil
		}
		if len(hosts) > 0 {
			hosts = hosts[:1]
		}
	}

	// 2. Merge block-level when conditions with inherited when
	when := mergeWhen(e.when, block.When.Data)

	// 3. Merge tags and check if enabled
	tags := e.tags.JoinTag(block.Tags)

	// 4. Determine ignore_errors (inherit from parent if not set)
	ignoreErrors := block.IgnoreErrors
	if ignoreErrors == nil {
		ignoreErrors = e.ignoreErrors
	}

	// 5. Merge block variables into runtime vars
	if len(block.Vars.Nodes) > 0 {
		e.variable.Merge(variable.MergeRuntimeVariable(block.Vars.Nodes, hosts...))
	}

	// 6. Handle block/rescue/always structure
	if len(block.BlockInfo.Block) > 0 {
		return e.dealBlockRescueAlways(ctx, block, hosts, when, tags, ignoreErrors)
	}

	// 7. Handle include_tasks
	if block.IncludeTasks != "" {
		// Check tags before including
		if !tags.IsEnabled(e.playTags, e.playSkipTags) {
			return nil
		}
		return e.dealIncludeTasks(ctx, block.IncludeTasks, hosts, when, tags, ignoreErrors)
	}

	// 8. Check tags for regular tasks
	if !tags.IsEnabled(e.playTags, e.playSkipTags) {
		return nil // skip this block (tags filtered out)
	}

	// 9. Convert to TaskSpec and execute via TaskExecutor
	return e.dealTask(ctx, block, hosts, when, ignoreErrors)
}

// dealBlockRescueAlways handles block/rescue/always structure.
func (e *BlockExecutor) dealBlockRescueAlways(ctx context.Context, block ansible.Block, hosts []string, when []string, tags ansible.Tags, ignoreErrors *bool) error {
	var errs error

	// Execute main block
	blockErr := e.execSubBlocks(ctx, block.BlockInfo.Block, hosts, when, tags, ignoreErrors)

	if blockErr != nil {
		// If main block failed and rescue exists, execute rescue
		if len(block.BlockInfo.Rescue) > 0 {
			rescueErr := e.execSubBlocks(ctx, block.BlockInfo.Rescue, hosts, when, tags, ignoreErrors)
			if rescueErr != nil {
				errs = errors.Join(errs, blockErr, rescueErr)
			}
			// rescue succeeded: clear block error
		} else {
			// No rescue block: propagate block error
			errs = errors.Join(errs, blockErr)
		}
	}

	// Always block executes regardless
	if len(block.BlockInfo.Always) > 0 {
		alwaysErr := e.execSubBlocks(ctx, block.BlockInfo.Always, hosts, when, tags, ignoreErrors)
		if alwaysErr != nil {
			errs = errors.Join(errs, alwaysErr)
		}
	}

	return errs
}

// execSubBlocks creates a child BlockExecutor and executes blocks.
func (e *BlockExecutor) execSubBlocks(ctx context.Context, blocks []ansible.Block, hosts []string, when []string, tags ansible.Tags, ignoreErrors *bool) error {
	sub := &BlockExecutor{
		taskExecutor: e.taskExecutor,
		variable:     e.variable,
		source:       e.source,
		logOutput:    e.logOutput,
		connectors:   e.connectors,
		hosts:        hosts,
		ignoreErrors: ignoreErrors,
		when:         when,
		tags:         tags,
		playTags:     e.playTags,
		playSkipTags: e.playSkipTags,
		role:         e.role,
		callback:     e.callback,
		taskIndex:    e.taskIndex,
		taskTotal:    e.taskTotal,
		playBecome:   e.playBecome,
		skipRunOnce:  e.skipRunOnce,
	}
	return sub.Exec(ctx, blocks)
}

// dealTask converts a block to TaskSpec and executes it.
func (e *BlockExecutor) dealTask(ctx context.Context, block ansible.Block, hosts []string, when []string, ignoreErrors *bool) error {
	spec := converter.BlockToTaskSpec(block, hosts, e.role, modules.IsModule)
	spec.When = when
	spec.IgnoreErrors = ignoreErrors
	// Inherit play-level become if task doesn't explicitly set it
	if !spec.Become && e.playBecome {
		spec.Become = true
	}

	e.dealTaskLogName(block)

	index := e.dealTaskEmitStart(block)

	taskStart := time.Now()
	results := e.taskExecutor.Exec(ctx, spec)

	// Determine aggregate status
	status, errMsg, output := aggregateResults(results)

	// Log SKIPPED status after execution
	if e.logOutput != nil && status == string(ansible.TaskStatusSkipped) {
		fmt.Fprintf(e.logOutput, "  => SKIPPED\n")
	}

	// Emit task_end. no_log suppresses the captured output and error text:
	// both end up in the job log and the WS stream, and a task marked no_log
	// is one whose command line was built by interpolating a secret -- a
	// shell syntax error or `set -u` abort echoes the offending line, secret
	// included. Status/duration stay so the task is still visible.
	if e.callback != nil {
		evOutput, evErr := output, errMsg
		if block.NoLog {
			evOutput = nil
			if evErr != "" {
				evErr = noLogRedacted
			}
		}
		e.callback(Event{
			Type:      EventTaskEnd,
			Timestamp: time.Now(),
			Task:      block.Name,
			Index:     index,
			Total:     e.taskTotal,
			Status:    status,
			Output:    evOutput,
			Error:     evErr,
			Duration:  time.Since(taskStart).Milliseconds(),
		})
	}

	return e.dealTaskCheckFailure(block, results, ignoreErrors)
}

// dealTaskLogName writes the task's name (with optional role prefix) to the
// log output. Extracted from dealTask to cut its cognitive complexity.
func (e *BlockExecutor) dealTaskLogName(block ansible.Block) {
	if e.logOutput != nil {
		rolePart := ""
		if e.role != "" {
			rolePart = fmt.Sprintf("[%s] ", e.role)
		}
		fmt.Fprintf(e.logOutput, "%s%s\n", rolePart, block.Name)
	}
}

// dealTaskEmitStart advances the shared task counter and emits the task_start
// event. Extracted from dealTask to cut its cognitive complexity. Returns the
// 1-based task index (0 when no shared counter is set).
func (e *BlockExecutor) dealTaskEmitStart(block ansible.Block) int {
	index := 0
	if e.taskIndex != nil {
		*e.taskIndex++
		index = *e.taskIndex
	}
	if e.callback != nil {
		e.callback(Event{
			Type:      EventTaskStart,
			Timestamp: time.Now(),
			Task:      block.Name,
			Index:     index,
			Total:     e.taskTotal,
		})
	}
	return index
}

// dealTaskCheckFailure scans task results for the first non-ignored failure
// and returns a diagnostic error (including stdout/stderr) for it. Extracted
// from dealTask to cut its cognitive complexity.
func (e *BlockExecutor) dealTaskCheckFailure(block ansible.Block, results []ansible.TaskResult, ignoreErrors *bool) error {
	for _, r := range results {
		if r.Status == ansible.TaskStatusFailed {
			if ignoreErrors != nil && *ignoreErrors {
				continue
			}
			// Include stdout/stderr in error message for diagnostics --
			// unless the task is no_log, in which case that output is the
			// exact thing we must not persist (see dealTask).
			if block.NoLog {
				return fmt.Errorf("task %q failed on host %q: %s", block.Name, r.Host, noLogRedacted)
			}
			detail := r.Error
			if out, ok := r.Output["stdout"].(string); ok && out != "" {
				detail += "\n--- stdout ---\n" + out
			}
			if errOut, ok := r.Output["stderr"].(string); ok && errOut != "" {
				detail += "\n--- stderr ---\n" + errOut
			}
			return fmt.Errorf("task %q failed on host %q: %s", block.Name, r.Host, detail)
		}
	}
	return nil
}

// aggregateResults picks the most significant status from task results.
func aggregateResults(results []ansible.TaskResult) (status string, errMsg string, output map[string]any) {
	status = string(ansible.TaskStatusOK)
	for _, r := range results {
		switch r.Status {
		case ansible.TaskStatusFailed:
			return string(ansible.TaskStatusFailed), r.Error, r.Output
		case ansible.TaskStatusChanged:
			status = string(ansible.TaskStatusChanged)
		case ansible.TaskStatusSkipped:
			if status == string(ansible.TaskStatusOK) {
				status = string(ansible.TaskStatusSkipped)
			}
		}
		if output == nil {
			output = r.Output
		}
	}
	return
}

// dealIncludeTasks loads and executes tasks from an included file.
func (e *BlockExecutor) dealIncludeTasks(ctx context.Context, file string, hosts []string, when []string, tags ansible.Tags, ignoreErrors *bool) error {
	// Cycle check: bail before reading / parsing / spawning a sub
	// executor if `file` is already on the active include stack. This
	// covers direct self-reference (A -> A) and indirect cycles of
	// any depth (A -> B -> ... -> A) with the same mechanism.
	for _, prev := range e.includeStack {
		if prev == file {
			chain := append([]string(nil), e.includeStack...)
			chain = append(chain, file)
			return fmt.Errorf("include_tasks: recursive include detected: %s", strings.Join(chain, " -> "))
		}
	}

	data, err := e.source.ReadFile(file)
	if err != nil {
		return fmt.Errorf("include_tasks: read %s: %w", file, err)
	}
	var blocks []ansible.Block
	if err := yaml.Unmarshal(data, &blocks); err != nil {
		return fmt.Errorf("include_tasks: parse %s: %w", file, err)
	}

	// Build the sub-executor's include stack: parent's stack + current
	// file. Allocate a fresh slice instead of appending in place — the
	// parent's stack must stay unchanged so sibling include_tasks
	// blocks (sequential, not nested) start from the parent's view.
	subStack := make([]string, 0, len(e.includeStack)+1)
	subStack = append(subStack, e.includeStack...)
	subStack = append(subStack, file)

	sub := &BlockExecutor{
		taskExecutor: e.taskExecutor,
		variable:     e.variable,
		source:       e.source,
		logOutput:    e.logOutput,
		connectors:   e.connectors,
		hosts:        hosts,
		ignoreErrors: ignoreErrors,
		when:         when,
		tags:         tags,
		playTags:     e.playTags,
		playSkipTags: e.playSkipTags,
		role:         e.role,
		callback:     e.callback,
		taskIndex:    e.taskIndex,
		taskTotal:    e.taskTotal,
		playBecome:   e.playBecome,
		skipRunOnce:  e.skipRunOnce,
		includeStack: subStack,
	}
	return sub.Exec(ctx, blocks)
}

// mergeWhen combines parent and child when conditions, deduplicating entries.
func mergeWhen(parent, child []string) []string {
	if len(child) == 0 {
		return parent
	}
	if len(parent) == 0 {
		result := make([]string, len(child))
		copy(result, child)
		return result
	}

	result := make([]string, len(parent))
	copy(result, parent)
	for _, c := range child {
		if !slices.Contains(result, c) {
			result = append(result, c)
		}
	}
	return result
}
