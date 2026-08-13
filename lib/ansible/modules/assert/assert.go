package assert

import (
	"context"
	"fmt"

	"vraxel.io/vraxel/lib/ansible/modules/internal"
	"vraxel.io/vraxel/lib/ansible/template"
)

// ModuleAssert evaluates conditions and fails if any are false.
// Args:
//
//	that: list of conditions (template expressions)
//	fail_msg / msg: message on failure (optional; msg is a synonym for fail_msg, matching upstream Ansible)
//	success_msg: message on success (optional)
func ModuleAssert(ctx context.Context, opts internal.ExecOptions) (string, string, error) {
	that := opts.Args["that"]
	failMsg, _ := opts.Args["fail_msg"].(string)
	if failMsg == "" {
		// Upstream Ansible accepts `msg` as a synonym for `fail_msg`;
		// every playbook in this repo uses `msg`, so without this
		// fallback every assertion would surface as a bare
		// "assertion failed" with no context.
		failMsg, _ = opts.Args["msg"].(string)
	}
	successMsg, _ := opts.Args["success_msg"].(string)

	if failMsg == "" {
		failMsg = "assertion failed"
	}

	vars := opts.GetAllVariables()

	// Convert "that" to string slice.
	conditions := ToStringSlice(that)
	if len(conditions) == 0 {
		return "", "", fmt.Errorf("assert: 'that' argument required")
	}

	// Evaluate each condition.
	ok, err := template.ParseBool(vars, conditions...)
	if err != nil {
		return "", "", fmt.Errorf("assert: evaluate conditions: %w", err)
	}
	if !ok {
		return "", "", fmt.Errorf("assert: %s", failMsg)
	}

	if successMsg != "" {
		return successMsg, "", nil
	}
	return "all assertions passed", "", nil
}

// ToStringSlice converts an interface value to a string slice.
// It handles []any (from YAML parsing), []string, and single string values.
func ToStringSlice(v any) []string {
	switch val := v.(type) {
	case []any:
		result := make([]string, 0, len(val))
		for _, item := range val {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	case []string:
		return val
	case string:
		return []string{val}
	default:
		return nil
	}
}
