package ansible

import (
	"fmt"
	"strings"
)

// This file rejects, at load time, the directives the YAML layer happily
// parses but the engine has no execution for.
//
// The alternative is what the engine used to do: accept them and do nothing.
// That produces a run which reports success while behaving differently from
// what the playbook says -- a task that never delegates, a loop that never
// loops, a handler that never fires. A directive the engine cannot honour is
// therefore an error, raised when the playbook is loaded rather than when a
// host is already half-configured.
//
// Directives that are merely inert (check_mode, diff) are not listed: the
// engine has no dry-run mode at all, so there is nothing for them to change
// and rejecting them would only break playbooks that set them defensively.

// unsupported builds the load-time error for a directive the engine parses but
// cannot execute. advice, when present, names the supported way to express it.
func unsupported(where, directive, advice string) error {
	msg := fmt.Sprintf("%s: %q is not implemented by this engine", where, directive)
	if advice != "" {
		msg += "; " + advice
	}
	return fmt.Errorf("%s", msg)
}

// describe names a play or task for an error message.
func describe(kind, name string) string {
	if name == "" {
		return kind
	}
	return fmt.Sprintf("%s %q", kind, name)
}

// ValidatePlay reports the first play-level directive the engine cannot honour.
func ValidatePlay(play Play) error {
	where := describe("play", play.Name)

	switch {
	case play.Strategy != "" && play.Strategy != "linear":
		return unsupported(where, "strategy: "+play.Strategy, "only the default linear strategy is available")
	case play.Order != "":
		return unsupported(where, "order", "hosts run in inventory order")
	case play.ForceHandlers:
		return unsupported(where, "force_handlers", "handlers are not implemented")
	case play.MaxFailPercentage > 0:
		return unsupported(where, "max_fail_percentage", "a failing host aborts the play")
	case len(play.Handlers) > 0:
		return unsupported(where, "handlers",
			"notify/handlers are not implemented; call the task directly or guard it with when")
	case play.Environment != nil:
		// Parsed but never inherited into tasks, which would make it a
		// silent no-op - the exact class this file exists to reject.
		return unsupported(where, "play-level environment", "set environment on each task")
	}

	return validateBase(where, play.Base)
}

// ValidateBlocks reports the first directive the engine cannot honour in
// blocks, descending into block/rescue/always. Every path that turns YAML into
// blocks -- playbook, role, include_tasks -- runs it, so a directive cannot
// slip in through a file the top-level playbook never mentions.
func ValidateBlocks(blocks []Block) error {
	for _, b := range blocks {
		if err := validateBlock(b); err != nil {
			return err
		}
	}
	return nil
}

func validateBlock(b Block) error {
	where := describe("task", b.Name)

	switch {
	case b.Notify != "":
		return unsupported(where, "notify",
			"handlers are not implemented; call the handler task directly or guard it with when")
	case b.DelegateFacts:
		return unsupported(where, "delegate_facts", "facts always belong to the original host")
	case b.AsyncVal > 0 || b.Poll > 0:
		return unsupported(where, "async/poll", "run the command in the background from the shell instead")
	}

	// The three loop directives share one execution slot; writing two on the
	// same task would silently drop one, so it is an error, as in Ansible.
	set := 0
	for _, v := range []any{b.Loop, b.WithItems, b.WithDict} {
		if v != nil {
			set++
		}
	}
	if set > 1 {
		return fmt.Errorf("%s: loop, with_items and with_dict are mutually exclusive", where)
	}

	// A block container's environment would not be inherited by the tasks
	// inside it - a silent no-op, so it is rejected like the play-level one.
	if len(b.BlockInfo.Block) > 0 && b.Environment != nil {
		return unsupported(where, "block-level environment", "set environment on each task")
	}

	if err := validateBase(where, b.Base); err != nil {
		return err
	}
	if err := validateUnknownFields(where, b.UnknownField); err != nil {
		return err
	}

	for _, nested := range [][]Block{b.BlockInfo.Block, b.BlockInfo.Rescue, b.BlockInfo.Always} {
		if err := ValidateBlocks(nested); err != nil {
			return err
		}
	}
	return nil
}

// validateBase covers the directives Play and Block share.
func validateBase(where string, b Base) error {
	switch {
	case b.AnyErrorsFatal:
		return unsupported(where, "any_errors_fatal", "a failing host already aborts the play")
	case b.Throttle > 0:
		return unsupported(where, "throttle", "use serial to limit how many hosts run at once")
	case b.Timeout > 0:
		return unsupported(where, "timeout", "bound the command itself, e.g. with timeout(1)")
	case b.Debugger != "":
		return unsupported(where, "debugger", "")
	}
	return nil
}

// unsupportedDirectives are keys that reach UnknownField because the engine
// has no field for them. Anything else there is taken to be a module name.
var unsupportedDirectives = map[string]string{
	"include_role":    "declare the role in the play's roles: list",
	"import_role":     "declare the role in the play's roles: list",
	"import_playbook": "the referenced playbook is not loaded; merge its plays into this file",
	"local_action":    "use delegate_to: localhost",
}

// validateUnknownFields flags directives hiding among the module keys. A
// with_* variant is the dangerous case: without a field for it the task simply
// runs once, so the loop vanishes without a word.
func validateUnknownFields(where string, fields map[string]any) error {
	for k := range fields {
		if advice, ok := unsupportedDirectives[k]; ok {
			return unsupported(where, k, advice)
		}
		if strings.HasPrefix(k, "with_") {
			return unsupported(where, k, "only with_items and with_dict are available; otherwise use loop")
		}
	}
	return nil
}
