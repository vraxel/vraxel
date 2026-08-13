package stat

import (
	"context"
	"fmt"
	"strings"

	"vraxel.io/vraxel/lib/ansible/modules/internal"
)

// ModuleStat reports what a path on the remote host is.
//
// Args:
//
//	path: target path (required)
//
// It prints a JSON object with exists / isdir / isreg / islnk / size / mode,
// so pair it with register_type: json to index into the result:
//
//   - stat: {path: /etc/app.conf}
//     register: cfg
//     register_type: json
//   - shell: "echo present"
//     when: '{{ .cfg.stdout.exists }}'
//
// A missing path is not a failure: exists is false and the task succeeds,
// which is what makes stat usable as a condition.
func ModuleStat(ctx context.Context, opts internal.ExecOptions) (string, string, error) {
	path := internal.StringArg(opts.Args, "path")
	if path == "" {
		return "", "", fmt.Errorf("stat: path is required")
	}

	stdout, stderr, err := opts.Connector.ExecuteCommand(ctx, statScript(path))
	if err != nil {
		return "", string(stderr), fmt.Errorf("stat: %s: %w", path, err)
	}
	return strings.TrimSpace(string(stdout)), "", nil
}

// statScript emits the JSON describing path. Mode is read through GNU stat
// with a BSD fallback, since the two spell the format differently and a
// target can be either.
func statScript(path string) string {
	qp := internal.ShellQuote(path)

	return fmt.Sprintf(`p=%s
if [ -e "$p" ] || [ -L "$p" ]; then
  [ -d "$p" ] && isdir=true || isdir=false
  [ -f "$p" ] && isreg=true || isreg=false
  [ -L "$p" ] && islnk=true || islnk=false
  size=$(wc -c < "$p" 2>/dev/null | tr -d ' ')
  [ -n "$size" ] || size=0
  mode=$(stat -c '%%a' "$p" 2>/dev/null || stat -f '%%Lp' "$p" 2>/dev/null || echo "")
  printf '{"exists":true,"isdir":%%s,"isreg":%%s,"islnk":%%s,"size":%%s,"mode":"%%s"}' \
    "$isdir" "$isreg" "$islnk" "$size" "$mode"
else
  printf '{"exists":false,"isdir":false,"isreg":false,"islnk":false,"size":0,"mode":""}'
fi`, qp)
}
