package internal

import (
	"fmt"
	"io/fs"
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

// ReadSource reads a file from the Source, returning its contents.
// Returns an error if Source is nil or the file cannot be read.
func ReadSource(source Source, path string) ([]byte, error) {
	if source == nil {
		return nil, fmt.Errorf("no source available")
	}
	return source.ReadFile(path)
}
