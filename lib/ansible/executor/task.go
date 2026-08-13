package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"errors"

	"gopkg.in/yaml.v3"

	"vraxel.io/vraxel/lib/ansible"
	"vraxel.io/vraxel/lib/ansible/connector"
	"vraxel.io/vraxel/lib/ansible/modules"
	"vraxel.io/vraxel/lib/ansible/project"
	"vraxel.io/vraxel/lib/ansible/template"
	"vraxel.io/vraxel/lib/ansible/variable"
)

// TaskExecutor executes a single TaskSpec against hosts.
type TaskExecutor struct {
	variable   variable.Variable
	source     project.Source
	logOutput  io.Writer
	connectors *connectorRegistry // host -> connector, race-safe
}

// NewTaskExecutor creates a new task executor.
//
// conns is the same registry the PlaybookExecutor owns; tests can build
// one via newConnectorRegistry + Put. Worker goroutines spawned by
// Exec read it concurrently with PlaybookExecutor.Resize /
// SetLiveOutput, so the RWMutex inside the registry is required.
func NewTaskExecutor(v variable.Variable, src project.Source, conns *connectorRegistry, logOutput io.Writer) *TaskExecutor {
	return &TaskExecutor{
		variable:   v,
		source:     src,
		logOutput:  logOutput,
		connectors: conns,
	}
}

// Exec executes a task and returns results per host.
func (e *TaskExecutor) Exec(ctx context.Context, task ansible.TaskSpec) []ansible.TaskResult {
	// 1. Find module
	moduleFn := modules.FindModule(task.Module.Name)
	if moduleFn == nil {
		// Return error result for all hosts
		results := make([]ansible.TaskResult, len(task.Hosts))
		for i, host := range task.Hosts {
			results[i] = ansible.TaskResult{
				Host:   host,
				Status: ansible.TaskStatusFailed,
				Error:  fmt.Sprintf("module %q not found", task.Module.Name),
			}
		}
		return results
	}

	// 2. Resolve loop items
	items := e.resolveLoop(task)

	// 3. Execute on each host in parallel
	var mu sync.Mutex
	results := make([]ansible.TaskResult, 0, len(task.Hosts))
	var wg sync.WaitGroup

	for _, host := range task.Hosts {
		wg.Add(1)
		go func(host string) {
			defer wg.Done()
			result := e.execTaskHost(ctx, task, host, moduleFn, items)
			mu.Lock()
			results = append(results, result)
			mu.Unlock()
		}(host)
	}
	wg.Wait()

	// 4. Register results if task.Register is set
	if task.Register != "" {
		e.registerResults(task, results)
	}

	return results
}

// execTaskHost executes task on a single host with loop/retry support.
func (e *TaskExecutor) execTaskHost(ctx context.Context, task ansible.TaskSpec, host string, moduleFn modules.ModuleExecFunc, items []any) ansible.TaskResult {
	result := ansible.TaskResult{Host: host, Status: ansible.TaskStatusOK}

	allSkipped := true
	for _, item := range items {
		loopResult := e.executeWithRetry(ctx, task, host, moduleFn, item)

		result.Output = mergeOutput(result.Output, loopResult)

		if loopResult.Result.Status != ansible.TaskStatusSkipped {
			allSkipped = false
		}

		if loopResult.Result.Error != "" {
			if task.IgnoreErrors != nil && *task.IgnoreErrors {
				// Preserve error for register, but don't stop execution
				if result.Output == nil {
					result.Output = make(map[string]any)
				}
				result.Output["_ignored_error"] = loopResult.Result.Error
				allSkipped = false
			} else {
				result.Status = ansible.TaskStatusFailed
				result.Error = loopResult.Result.Error
				return result // stop loop on first failure
			}
		}
	}

	if allSkipped {
		result.Status = ansible.TaskStatusSkipped
	}

	return result
}

// executeWithRetry executes module with retry logic.
func (e *TaskExecutor) executeWithRetry(ctx context.Context, task ansible.TaskSpec, host string, moduleFn modules.ModuleExecFunc, item any) ansible.LoopResult {
	retries := task.Retries
	if retries == 0 {
		retries = 1 // at least one attempt
	}
	delay := task.Delay

	var loopResult ansible.LoopResult

	for attempt := 0; attempt < retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				loopResult.Result.Error = ctx.Err().Error()
				return loopResult
			case <-time.After(time.Duration(delay) * time.Second):
			}
		}

		loopResult = e.executeModule(ctx, task, host, moduleFn, item)

		if e.executeWithRetryShouldStop(host, attempt, retries, task, &loopResult) {
			break
		}
	}

	return loopResult
}

// executeWithRetryShouldStop evaluates the post-attempt until/success logic
// for executeWithRetry and reports whether the retry loop should stop.
// Extracted to cut executeWithRetry's cognitive complexity. It may mutate
// loopResult.Result.Error (clear on until-satisfied, or set the
// not-met-after-retries marker), so loopResult is passed by pointer.
func (e *TaskExecutor) executeWithRetryShouldStop(host string, attempt, retries int, task ansible.TaskSpec, loopResult *ansible.LoopResult) bool {
	// Check until conditions
	if len(task.Until) > 0 {
		vars := e.getVarsWithResult(host, *loopResult)
		ok, _ := template.ParseBool(vars, task.Until...)
		if ok {
			loopResult.Result.Error = "" // until satisfied, clear error
			return true
		}
		// If until not satisfied and this is the last attempt, mark as failure
		if attempt == retries-1 && loopResult.Result.Error == "" {
			loopResult.Result.Error = "until condition not met after retries"
		}
		return false
	}
	if loopResult.Result.Error == "" {
		return true // success, no need to retry
	}
	return false
}

// executeModule executes the module once, handling when/failed_when.
func (e *TaskExecutor) executeModule(ctx context.Context, task ansible.TaskSpec, host string, moduleFn modules.ModuleExecFunc, item any) ansible.LoopResult {
	result := ansible.LoopResult{
		Item: item,
		Result: ansible.TaskResult{
			Host:   host,
			Status: ansible.TaskStatusOK,
		},
	}

	// Get host variables
	rawVars := e.variable.Get(variable.GetAllVariable(host))
	vars, ok := rawVars.(map[string]any)
	if !ok {
		result.Result.Error = fmt.Sprintf("host %s: variables are not a map", host)
		result.Result.Status = ansible.TaskStatusFailed
		return result
	}

	// Set loop item variable
	if item != nil {
		vars["item"] = item
		// Also merge into runtime vars temporarily
		e.variable.Merge(variable.MergeHostRuntimeVars(host, map[string]any{"item": item}))
		defer func() {
			// Clean up loop item variable after execution
			e.variable.Merge(variable.MergeHostRuntimeVars(host, map[string]any{"item": nil}))
		}()
	}

	// Check when conditions
	if len(task.When) > 0 {
		ok, err := template.ParseBool(vars, task.When...)
		if err != nil {
			result.Result.Error = fmt.Sprintf("evaluate when: %v", err)
			result.Result.Status = ansible.TaskStatusFailed
			return result
		}
		if !ok {
			result.Result.Status = ansible.TaskStatusSkipped
			return result // skipped, not an error
		}
	}

	// Render module args with template
	args := e.toArgsMap(task.Module.Args)
	args = e.renderArgs(args, vars)

	// Execute module
	conn := e.connectors.Get(host)
	stdout, stderr, err := moduleFn(ctx, modules.ExecOptions{
		Args:      args,
		Host:      host,
		Variable:  e.variable,
		Connector: conn,
		Source:    e.source,
		LogOutput: e.logOutput,
		Become:    task.Become,
	})

	rc := 0
	if err != nil {
		var exitErr *connector.ExitError
		if errors.As(err, &exitErr) {
			rc = exitErr.Code
		}
	}

	result.Result.Output = map[string]any{
		"stdout": strings.TrimRight(stdout, "\n"),
		"stderr": strings.TrimRight(stderr, "\n"),
		"rc":     rc,
	}

	e.executeModuleApplyFailure(host, task, err, &result)

	return result
}

// executeModuleApplyFailure applies the pass/fail decision after a module
// run. Extracted from executeModule to cut its cognitive complexity. It
// mutates result.Result (Error/Status) so result is passed by pointer.
//
// failed_when overrides the default rc-based pass/fail decision per
// Ansible semantics. The previous logic short-circuited on err != nil
// before evaluating failed_when, so writing `failed_when: false` on a
// shell task with grep -c (which exits 1 on no match) still surfaced
// as a hard failure - the exact opposite of what the directive
// promises. Run failed_when first when set; only fall back to the
// rc-based default when failed_when is absent.
func (e *TaskExecutor) executeModuleApplyFailure(host string, task ansible.TaskSpec, err error, result *ansible.LoopResult) {
	if len(task.FailedWhen) > 0 {
		failVars := e.getVarsWithResult(host, *result)
		ok, parseErr := template.ParseBool(failVars, task.FailedWhen...)
		switch {
		case parseErr != nil:
			result.Result.Error = fmt.Sprintf("evaluate failed_when: %v", parseErr)
			result.Result.Status = ansible.TaskStatusFailed
		case ok:
			result.Result.Error = "failed_when condition met"
			if err != nil {
				result.Result.Error += ": " + err.Error()
			}
			result.Result.Status = ansible.TaskStatusFailed
		}
		// else: failed_when says NOT failed - keep status OK regardless of rc.
	} else if err != nil {
		result.Result.Error = err.Error()
		result.Result.Status = ansible.TaskStatusFailed
	}
}

// resolveLoop resolves loop items from task.
func (e *TaskExecutor) resolveLoop(task ansible.TaskSpec) []any {
	if task.Loop == nil {
		return []any{nil} // single execution, no loop
	}

	switch v := task.Loop.(type) {
	case []any:
		return v
	case []string:
		items := make([]any, len(v))
		for i, s := range v {
			items[i] = s
		}
		return items
	default:
		// If it's some other type, wrap it as a single-item loop
		return []any{v}
	}
}

// renderArgs renders template syntax in module args.
func (e *TaskExecutor) renderArgs(args map[string]any, vars map[string]any) map[string]any {
	rendered := make(map[string]any, len(args))
	for k, v := range args {
		switch val := v.(type) {
		case string:
			if result, err := template.ParseString(vars, val); err == nil {
				rendered[k] = result
			} else {
				rendered[k] = val
			}
		case map[string]any:
			rendered[k] = e.renderArgs(val, vars)
		default:
			rendered[k] = v
		}
	}
	return rendered
}

// toArgsMap converts the module Args (typed as any) to map[string]any.
func (e *TaskExecutor) toArgsMap(args any) map[string]any {
	if args == nil {
		return make(map[string]any)
	}
	if m, ok := args.(map[string]any); ok {
		return m
	}
	// Fallback: wrap as a single value under "raw"
	return map[string]any{"raw": args}
}

// registerResults registers task results as variables.
func (e *TaskExecutor) registerResults(task ansible.TaskSpec, results []ansible.TaskResult) {
	for _, result := range results {
		regVar := make(map[string]any)

		// Collect loop-level outputs from result.Output
		stdout := extractString(result.Output, "stdout")
		stderr := extractString(result.Output, "stderr")

		regVar["stdout"] = parseRegisterStdout(task.RegisterType, stdout)
		regVar["stderr"] = stderr
		regVar["rc"] = extractInt(result.Output, "rc")
		regVar["failed"] = result.Error != "" || extractString(result.Output, "_ignored_error") != ""
		regVar["skipped"] = result.Status == ansible.TaskStatusSkipped

		e.variable.Merge(variable.MergeHostRuntimeVars(result.Host, map[string]any{task.Register: regVar}))
	}
}

// parseRegisterStdout interprets the captured stdout according to the task's
// register_type. For "json"/"yaml" the stdout string is decoded into a
// structured value so downstream templates can index into it; on decode
// failure it falls back to the raw string. Default keeps the raw string.
func parseRegisterStdout(registerType, s string) any {
	switch registerType {
	case "json":
		// Use UseNumber() to keep large-integer precision, then normalize
		// json.Number back to int64/float64 so values are not later
		// re-marshaled as strings.
		decoder := json.NewDecoder(strings.NewReader(s))
		decoder.UseNumber()
		var out any
		if err := decoder.Decode(&out); err != nil {
			return s
		}
		return normalizeJSONNumbers(out)
	case "yaml", "yml":
		var out any
		if err := yaml.Unmarshal([]byte(s), &out); err != nil {
			return s
		}
		return out
	default:
		return s
	}
}

// normalizeJSONNumbers recursively converts json.Number values (produced by a
// decoder with UseNumber) to int64 or float64, preserving numeric types while
// avoiding precision loss for large integers.
func normalizeJSONNumbers(v any) any {
	switch x := v.(type) {
	case json.Number:
		if i, err := x.Int64(); err == nil {
			return i
		}
		if f, err := x.Float64(); err == nil {
			return f
		}
		return x.String()
	case map[string]any:
		for k, vv := range x {
			x[k] = normalizeJSONNumbers(vv)
		}
		return x
	case []any:
		for i, vv := range x {
			x[i] = normalizeJSONNumbers(vv)
		}
		return x
	default:
		return v
	}
}

// getVarsWithResult returns host variables merged with the current loop result,
// so that until/failed_when conditions can reference stdout/stderr/etc.
func (e *TaskExecutor) getVarsWithResult(host string, lr ansible.LoopResult) map[string]any {
	rawVars := e.variable.Get(variable.GetAllVariable(host))
	vars, ok := rawVars.(map[string]any)
	if !ok {
		vars = make(map[string]any)
	}

	// Add result fields so conditions can reference them
	stdout := extractString(lr.Result.Output, "stdout")
	stderr := extractString(lr.Result.Output, "stderr")
	vars["stdout"] = stdout
	vars["stderr"] = stderr
	vars["failed"] = lr.Result.Error != ""

	return vars
}

// mergeOutput merges loop result output into the overall result output map.
func mergeOutput(existing map[string]any, lr ansible.LoopResult) map[string]any {
	if existing == nil {
		return lr.Result.Output
	}
	// For multiple loop iterations, keep the last output
	if lr.Result.Output != nil {
		return lr.Result.Output
	}
	return existing
}

// extractInt extracts an int value from a map, returning 0 if not found.
func extractInt(m map[string]any, key string) int {
	if m == nil {
		return 0
	}
	v, ok := m[key]
	if !ok {
		return 0
	}
	if i, ok := v.(int); ok {
		return i
	}
	return 0
}

// extractString extracts a string value from a map, returning "" if not found.
func extractString(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return fmt.Sprintf("%v", v)
	}
	return s
}
