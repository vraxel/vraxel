//go:build linux

package hostinfo

import (
	"path/filepath"

	agenttypes "vraxel.io/vraxel/lib/agent/types"
)

// sshdPaths are where the daemon is installed, in the order they are
// tried. Enumerated rather than found on PATH -- see lookTool.
var sshdPaths = []string{"/usr/sbin/sshd", "/sbin/sshd", "/usr/local/sbin/sshd"}

// sshdConfigPath is where the Match count is read from. Only this file
// and whatever it Includes: the count is a warning that exceptions exist,
// not an attempt to describe them.
const sshdConfigPath = "/etc/ssh/sshd_config"

// sshdConfig asks the daemon for its effective configuration.
//
// Nil when sshd is not installed, or refused to answer. Both are the same
// answer for this report -- there is nothing to say about a daemon that
// is not there or would not speak -- and neither is an error worth
// failing the whole account inventory over.
//
// Note this needs root. `sshd -T` opens the host keys, and as any other
// user it exits with "no hostkeys available"; the agent runs as root, and
// a build that did not would simply report nothing here.
func sshdConfig() *agenttypes.SSHDConfig {
	path := lookTool(sshdPaths...)
	if path == "" {
		return nil
	}
	cfg := parseSSHDConfig(runTool(path, "-T"))
	if cfg == nil {
		return nil
	}
	cfg.MatchBlocks = matchBlockCount()
	return cfg
}

// matchBlockCount counts Match directives across sshd_config and one
// level of its Includes.
//
// One level because that is the shape every distribution ships
// (Include /etc/ssh/sshd_config.d/*.conf) and because this is a warning
// counter, not a parser: a nested include that adds a block makes the
// count low, which understates the warning rather than inventing one.
func matchBlockCount() int32 {
	main := readFileLimit(sshdConfigPath, toolMaxOutput)
	n := countMatchBlocks(main)
	for _, pattern := range includeGlobs(main) {
		if !filepath.IsAbs(pattern) {
			pattern = filepath.Join(filepath.Dir(sshdConfigPath), pattern)
		}
		files, err := filepath.Glob(pattern)
		if err != nil {
			continue
		}
		for _, f := range files {
			n += countMatchBlocks(readFileLimit(f, toolMaxOutput))
		}
	}
	return n
}
