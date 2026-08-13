package project

import (
	"io/fs"
	"path"
)

// BuiltinSource reads project files from an embedded fs.FS.
type BuiltinSource struct {
	fsys   fs.FS
	prefix string
}

// NewBuiltinSource creates a BuiltinSource backed by fsys.
// prefix is prepended to all paths (e.g. "playbooks").
func NewBuiltinSource(fsys fs.FS, prefix string) *BuiltinSource {
	return &BuiltinSource{fsys: fsys, prefix: prefix}
}

// ReadFile reads a file's content from the embedded FS.
func (s *BuiltinSource) ReadFile(p string) ([]byte, error) {
	return fs.ReadFile(s.fsys, s.fullPath(p))
}

// ReadDir reads a directory's entries from the embedded FS.
func (s *BuiltinSource) ReadDir(p string) ([]fs.DirEntry, error) {
	return fs.ReadDir(s.fsys, s.fullPath(p))
}

// Stat returns file info from the embedded FS.
func (s *BuiltinSource) Stat(p string) (fs.FileInfo, error) {
	return fs.Stat(s.fsys, s.fullPath(p))
}

// fullPath joins prefix and path using forward-slash separators,
// which is the convention for fs.FS paths.
func (s *BuiltinSource) fullPath(p string) string {
	if s.prefix == "" {
		return p
	}
	return path.Join(s.prefix, p)
}
