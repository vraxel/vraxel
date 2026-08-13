package project

import (
	"io/fs"
	"os"
	"path/filepath"
)

// LocalSource reads project files from the local filesystem.
type LocalSource struct {
	basePath string
}

// NewLocalSource creates a LocalSource rooted at basePath.
func NewLocalSource(basePath string) *LocalSource {
	return &LocalSource{basePath: basePath}
}

// ReadFile reads a file's content relative to the base path.
func (s *LocalSource) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(filepath.Join(s.basePath, path))
}

// ReadDir reads a directory's entries relative to the base path.
func (s *LocalSource) ReadDir(path string) ([]fs.DirEntry, error) {
	return os.ReadDir(filepath.Join(s.basePath, path))
}

// Stat returns file info for a path relative to the base path.
func (s *LocalSource) Stat(path string) (fs.FileInfo, error) {
	return os.Stat(filepath.Join(s.basePath, path))
}
