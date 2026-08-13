package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
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

// ConnectorResolver produces the connector for a host the registry does not
// already hold. delegate_to can name a host the play never listed, so a plain
// registry lookup cannot serve it.
type ConnectorResolver func(ctx context.Context, host string) (connector.Connector, error)

// TaskExecutor executes a single TaskSpec against hosts.
type TaskExecutor struct {
	variable   variable.Variable
	source     project.Source
	logOutput  io.Writer
	connectors *connectorRegistry // host -> connector, race-safe
	resolve    ConnectorResolver  // optional; nil means registry-only
}

// NewTaskExecutor creates a new task executor.
//
// conns is the same registry the PlaybookExecutor owns; tests can build
// one via newConnectorRegistry + Put. Worker goroutines spawned by
// Exec read it concurrently with PlaybookExecutor.Resize /
// SetLiveOutput, so the RWMutex inside the registry is required.
//
// resolve may be nil, in which case delegate_to can only target hosts that
// are already in the registry.
func NewTaskExecutor(v variable.Variable, src project.Source, conns *connectorRegistry, logOutput io.Writer, resolve ConnectorResolver) *TaskExecutor {
	return &TaskExecutor{
		variable:   v,
		source:     src,
		logOutput:  logOutput,
		connectors: conns,
		resolve:    resolve,
	}
}

// connectorFor returns the connector to run a task through, creating one via
// the resolver when the registry has no entry for host.
func (e *TaskExecutor) connectorFor(ctx context.Context, host string) (connector.Connector, error) {
	if conn := e.connectors.Get(host); conn != nil {
		return conn, nil
	}
	if e.resolve == nil {
		return nil, fmt.Errorf("no connector for host %q", host)
	}
	return e.resolve(ctx, host)
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

	// 2. Execute on each host in parallel. Loop items are resolved inside
	// each host's goroutine because a templated loop ("{{ .list }}") can
	// name a host-specific variable.
	var mu sync.Mutex
	results := make([]ansible.TaskResult, 0, len(task.Hosts))
	var wg sync.WaitGroup

	for _, host := range task.Hosts {
		wg.Add(1)
		go func(host string) {
			defer wg.Done()
			result := e.execTaskHost(ctx, task, host, moduleFn)
			mu.Lock()
			results = append(results, result)
			mu.Unlock()
		}(host)
	}
	wg.Wait()

	// 3. Register results if task.Register is set
	if task.Register != "" {
		e.registerResults(task, results)
	}

	return results
}

// execTaskHost executes task on a single host with loop/retry support.
func (e *TaskExecutor) execTaskHost(ctx context.Context, task ansible.TaskSpec, host string, moduleFn modules.ModuleExecFunc) ansible.TaskResult {
	result := ansible.TaskResult{Host: host, Status: ansible.TaskStatusOK}

	items, err := e.resolveLoop(task, e.hostVars(host))
	if err != nil {
		result.Status = ansible.TaskStatusFailed
		result.Error = err.Error()
		return result
	}

	allSkipped := true
	for idx, item := range items {
		loopResult := e.executeWithRetry(ctx, task, host, moduleFn, item, idx)

		result.Output = mergeOutput(result.Output, loopResult)

		if loopResult.Result.Changed {
			result.Changed = true
		}

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

	switch {
	case allSkipped:
		// An empty loop runs no iterations, which is a skip, not a success.
		result.Status = ansible.TaskStatusSkipped
	case result.Changed:
		result.Status = ansible.TaskStatusChanged
	}

	return result
}

// hostVars returns the merged variable map for host, or an empty map when the
// variable store holds something else.
func (e *TaskExecutor) hostVars(host string) map[string]any {
	if vars, ok := e.variable.Get(variable.GetAllVariable(host)).(map[string]any); ok {
		return vars
	}
	return make(map[string]any)
}

// executeWithRetry executes module with retry logic.
func (e *TaskExecutor) executeWithRetry(ctx context.Context, task ansible.TaskSpec, host string, moduleFn modules.ModuleExecFunc, item any, idx int) ansible.LoopResult {
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

		loopResult = e.executeModule(ctx, task, host, moduleFn, item, idx)

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
func (e *TaskExecutor) executeModule(ctx context.Context, task ansible.TaskSpec, host string, moduleFn modules.ModuleExecFunc, item any, idx int) ansible.LoopResult {
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

	// Set loop item variable (honouring loop_control's loop_var/index_var)
	if item != nil {
		loopVars := map[string]any{loopVarName(task): item}
		if iv := task.LoopControl.IndexVar; iv != "" {
			loopVars[iv] = idx
		}
		for k, v := range loopVars {
			vars[k] = v
		}
		// Also merge into runtime vars temporarily
		e.variable.Merge(variable.MergeHostRuntimeVars(host, loopVars))
		defer func() {
			// Clean up loop item variables after execution
			cleared := make(map[string]any, len(loopVars))
			for k := range loopVars {
				cleared[k] = nil
			}
			e.variable.Merge(variable.MergeHostRuntimeVars(host, cleared))
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

	// Execute module. With delegate_to the module runs through the
	// delegate's connection while the variables and the registered result
	// stay with the original host, which is Ansible's semantics.
	conn, connErr := e.taskConnector(ctx, task, host, vars)
	if connErr != nil {
		result.Result.Error = connErr.Error()
		result.Result.Status = ansible.TaskStatusFailed
		return result
	}

	stdout, stderr, err := moduleFn(ctx, modules.ExecOptions{
		Args:        args,
		Host:        host,
		Variable:    e.variable,
		Connector:   conn,
		Source:      e.source,
		LogOutput:   e.logOutput,
		Become:      task.Become,
		Environment: resolveEnvironment(task.Environment, vars),
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
	e.executeModuleApplyChanged(host, task, &result)

	return result
}

// taskConnector picks the connection a task runs over: the host's own, or the
// delegate_to target's when that directive is set. delegate_to is templated,
// so it is rendered against the original host's variables first.
func (e *TaskExecutor) taskConnector(ctx context.Context, task ansible.TaskSpec, host string, vars map[string]any) (connector.Connector, error) {
	if task.DelegateTo == "" {
		return e.connectorFor(ctx, host)
	}

	target, err := template.ParseString(vars, task.DelegateTo)
	if err != nil {
		return nil, fmt.Errorf("render delegate_to: %w", err)
	}
	target = strings.TrimSpace(target)
	if target == "" {
		return nil, fmt.Errorf("delegate_to %q rendered empty", task.DelegateTo)
	}

	conn, err := e.connectorFor(ctx, target)
	if err != nil {
		return nil, fmt.Errorf("delegate_to %q: %w", target, err)
	}
	return conn, nil
}

// executeModuleApplyChanged evaluates changed_when and records the outcome on
// the result. Without changed_when the engine has no per-module change
// reporting, so Changed stays false rather than being guessed at.
func (e *TaskExecutor) executeModuleApplyChanged(host string, task ansible.TaskSpec, result *ansible.LoopResult) {
	if len(task.ChangedWhen) == 0 ||
		result.Result.Status == ansible.TaskStatusFailed ||
		result.Result.Status == ansible.TaskStatusSkipped {
		return
	}

	vars := e.getVarsWithResult(host, *result)
	ok, err := template.ParseBool(vars, task.ChangedWhen...)
	if err != nil {
		result.Result.Error = fmt.Sprintf("evaluate changed_when: %v", err)
		result.Result.Status = ansible.TaskStatusFailed
		return
	}
	result.Result.Changed = ok
	if ok {
		result.Result.Status = ansible.TaskStatusChanged
	}
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

// resolveLoop resolves the loop items for one host. The loop value is rendered
// against that host's variables first, so "loop: {{ .list }}" iterates the
// list the variable holds instead of the string it renders to.
func (e *TaskExecutor) resolveLoop(task ansible.TaskSpec, vars map[string]any) ([]any, error) {
	if task.Loop == nil {
		return []any{nil}, nil // single execution, no loop
	}

	val := resolveTemplatedValue(task.Loop, vars)

	if task.LoopKind == ansible.LoopKindDict {
		return dictItems(val)
	}
	items := toItemSlice(val)
	if task.LoopKind == ansible.LoopKindItems {
		items = flattenOnce(items)
	}
	return items, nil
}

// toItemSlice normalises a resolved loop value into items. A nil value (an
// unset or empty variable) yields no items, which surfaces as a skipped task
// rather than one bogus run against a nil item.
func toItemSlice(v any) []any {
	switch x := v.(type) {
	case nil:
		return nil
	case []any:
		return x
	case []string:
		items := make([]any, len(x))
		for i, s := range x {
			items[i] = s
		}
		return items
	default:
		return []any{x}
	}
}

// flattenOnce implements with_items' one-level flattening.
func flattenOnce(items []any) []any {
	out := make([]any, 0, len(items))
	for _, it := range items {
		if nested, ok := it.([]any); ok {
			out = append(out, nested...)
			continue
		}
		out = append(out, it)
	}
	return out
}

// dictItems implements with_dict: a mapping becomes {key, value} items,
// ordered by key so runs are reproducible. A nil value (an unset variable)
// yields no items and surfaces as a skip; any other non-mapping is a playbook
// error and fails the task rather than silently skipping it.
func dictItems(v any) ([]any, error) {
	if v == nil {
		return nil, nil
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("with_dict requires a mapping, got %T", v)
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	out := make([]any, 0, len(keys))
	for _, k := range keys {
		out = append(out, map[string]any{"key": k, "value": m[k]})
	}
	return out, nil
}

// loopVarName returns the variable name a loop item is exposed under.
func loopVarName(task ansible.TaskSpec) string {
	if task.LoopControl.LoopVar != "" {
		return task.LoopControl.LoopVar
	}
	return "item"
}

// resolveTemplatedValue renders raw against vars, preserving the value's real
// type. A string that is exactly one "{{ ... }}" expression is evaluated
// through toJson so lists and maps survive: rendering them directly would
// stringify a list as "[a b c]", which cannot be turned back into items.
// Anything else renders as an ordinary string.
func resolveTemplatedValue(raw any, vars map[string]any) any {
	s, ok := raw.(string)
	if !ok {
		return raw
	}

	if inner, single := singleTmplExpr(s); single {
		out, err := template.ParseString(vars, "{{ toJson ("+inner+") }}")
		if err == nil {
			decoder := json.NewDecoder(strings.NewReader(out))
			decoder.UseNumber()
			var v any
			if decoder.Decode(&v) == nil {
				return normalizeJSONNumbers(v)
			}
		}
		return s
	}

	if rendered, err := template.ParseString(vars, s); err == nil {
		return rendered
	}
	return s
}

// singleTmplExpr reports whether s is exactly one "{{ ... }}" expression and
// returns its inner pipeline, with any whitespace-trim markers removed.
func singleTmplExpr(s string) (string, bool) {
	t := strings.TrimSpace(s)
	if !strings.HasPrefix(t, "{{") || !strings.HasSuffix(t, "}}") {
		return "", false
	}
	inner := t[2 : len(t)-2]
	if strings.Contains(inner, "{{") || strings.Contains(inner, "}}") {
		return "", false
	}
	// "{{- x -}}" trim markers are part of the delimiter, not the pipeline.
	inner = strings.TrimSuffix(strings.TrimPrefix(inner, "- "), " -")
	return inner, true
}

// resolveEnvironment renders a task's environment into concrete key/value
// pairs. It accepts a mapping, a list of mappings (merged in order), or a
// template resolving to either.
func resolveEnvironment(raw any, vars map[string]any) map[string]string {
	if raw == nil {
		return nil
	}

	env := make(map[string]string)
	switch val := resolveTemplatedValue(raw, vars).(type) {
	case map[string]any:
		mergeEnv(env, val, vars)
	case []any:
		for _, entry := range val {
			// An entry can itself be a template resolving to a mapping.
			if m, ok := resolveTemplatedValue(entry, vars).(map[string]any); ok {
				mergeEnv(env, m, vars)
			}
		}
	}

	if len(env) == 0 {
		return nil
	}
	return env
}

// mergeEnv renders each value of src and merges it into dst.
func mergeEnv(dst map[string]string, src map[string]any, vars map[string]any) {
	for k, v := range src {
		if s, ok := v.(string); ok {
			if rendered, err := template.ParseString(vars, s); err == nil {
				dst[k] = rendered
				continue
			}
		}
		dst[k] = fmt.Sprintf("%v", v)
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
		regVar["changed"] = result.Changed

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
