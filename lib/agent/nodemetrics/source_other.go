//go:build !linux

package nodemetrics

import "errors"

// errUnsupported is why nothing is collected off Linux.
//
// The embedded collector set is compiled for Linux only, the same
// arrangement hostinfo uses: the production agent is linux/amd64 and
// linux/arm64, and building node_exporter's darwin collectors just so a
// dev laptop can chart itself is dependency weight with no user. The
// stub keeps the package building and unit-testing on macOS -- the ring,
// the derivation and the gather conversion are platform-neutral and
// carry the tests.
var errUnsupported = errors.New("host metrics are collected on Linux hosts only")

type source struct{}

func newSource(Logger) (*source, error) { return nil, errUnsupported }

func (s *source) collect(atMs int64) Sample { return Sample{AtMs: atMs} }
