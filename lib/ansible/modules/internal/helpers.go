package internal

import (
	"context"
	"fmt"
	"io/fs"
	"strings"
	"time"
)

// StringArg extracts a string value from module args by key.
// Returns empty string if the key is missing or not a string.
func StringArg(args map[string]any, key string) string {
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// FileModeArg extracts a file mode from module args by key.
// Returns the provided default if the key is missing or cannot be converted.
// Supports integer values (e.g., 0644) and string values (e.g., "0644").
func FileModeArg(args map[string]any, key string, defaultMode fs.FileMode) fs.FileMode {
	v, ok := args[key]
	if !ok {
		return defaultMode
	}

	switch m := v.(type) {
	case fs.FileMode:
		return m
	case int:
		return fs.FileMode(m)
	case int64:
		return fs.FileMode(m)
	case uint32:
		return fs.FileMode(m)
	case float64:
		return fs.FileMode(int(m))
	case string:
		var mode uint32
		if _, err := fmt.Sscanf(m, "%o", &mode); err == nil {
			return fs.FileMode(mode)
		}
	}

	return defaultMode
}

// ShellQuote wraps s in single quotes and escapes any embedded single quotes,
// producing a token that a POSIX shell treats literally. Use it on every
// caller-controlled path interpolated into an ExecuteCommand string to prevent
// shell injection and breakage on paths with spaces or metacharacters.
func ShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// WriteFile writes data to dest on the host. Without Become it uploads
// directly. With Become it uploads to a temp file first, then moves it into
// place via a shell command (SFTP as a non-root user often cannot write
// privileged destinations directly). dest is shell-escaped.
func WriteFile(ctx context.Context, opts ExecOptions, data []byte, dest string, mode fs.FileMode) error {
	if !opts.Become {
		if err := opts.Connector.PutFile(ctx, data, dest, mode); err != nil {
			return fmt.Errorf("put file %s: %w", dest, err)
		}
		return nil
	}

	tmpPath := fmt.Sprintf("/tmp/.ansible_tmp_%d", time.Now().UnixNano())
	if err := opts.Connector.PutFile(ctx, data, tmpPath, mode); err != nil {
		return fmt.Errorf("write temp %s: %w", tmpPath, err)
	}
	qd := ShellQuote(dest)
	cmd := fmt.Sprintf("mv %s %s && chmod %04o %s", tmpPath, qd, mode, qd)
	if _, _, err := opts.Connector.ExecuteCommand(ctx, cmd); err != nil {
		return fmt.Errorf("move to %s: %w", dest, err)
	}
	return nil
}

// ChownArg builds the "owner:group" argument for chown, or "" if neither is set.
func ChownArg(owner, group string) string {
	switch {
	case owner != "" && group != "":
		return owner + ":" + group
	case owner != "":
		return owner
	case group != "":
		return ":" + group
	default:
		return ""
	}
}

// ReadSource reads a file from the Source, returning its contents.
// Returns an error if Source is nil or the file cannot be read.
func ReadSource(source Source, path string) ([]byte, error) {
	if source == nil {
		return nil, fmt.Errorf("no source available")
	}
	return source.ReadFile(path)
}
