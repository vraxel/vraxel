package result

import (
	"context"
	"fmt"

	"vraxel.io/vraxel/lib/ansible/modules/internal"
	"vraxel.io/vraxel/lib/ansible/variable"
)

// ModuleResult stores key-value pairs in global result variables.
// Args: arbitrary key-value pairs to merge into PlaybookResult.Detail.
func ModuleResult(ctx context.Context, opts internal.ExecOptions) (string, string, error) {
	if len(opts.Args) == 0 {
		return "", "", fmt.Errorf("result: no arguments provided")
	}

	opts.Variable.Merge(variable.MergeResultVariable(opts.Args))

	return "result stored", "", nil
}
