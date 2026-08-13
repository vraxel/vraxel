package project

import "io/fs"

// Source provides access to playbook files.
// Implementations include LocalSource (filesystem) and BuiltinSource (embedded FS).
type Source interface {
	// ReadFile reads a file's content.
	ReadFile(path string) ([]byte, error)
	// ReadDir reads a directory's entries.
	ReadDir(path string) ([]fs.DirEntry, error)
	// Stat returns file info.
	Stat(path string) (fs.FileInfo, error)
}
