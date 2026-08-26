package hostinfo

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"time"
)

// Running a program is the LAST resort for a collector, and this file is
// the only place in hostinfo that does it. Everything else here reads
// files, which cannot hang, cannot depend on PATH, and cannot change
// their output format between releases.
//
// It exists for sshd, which has no other honest source: the effective
// configuration is the result of Include expansion, Match evaluation and
// compiled-in defaults, and `sshd -T` is the only thing that computes it.
// Parsing sshd_config by hand would give a confident wrong answer, and on
// a question about who can log in that is worse than no answer.
//
// The three exec sites already in this repository -- the log tail, the
// terminal, the probe runner -- are all operator-initiated: somebody is
// watching and a failure is visible to the person who caused it. A
// collector is not. It runs unattended, on a loop, on every host, so each
// guard below covers a failure that would otherwise be silent.
const (
	// toolTimeout bounds one run. These are local, synchronous tools that
	// answer in milliseconds; anything near this has stopped answering.
	toolTimeout = 5 * time.Second
	// toolKillDelay is how long the pipes may stay open after the context
	// ends. exec.CommandContext kills only the DIRECT child: a grandchild
	// that inherited the pipe holds it open and the read blocks forever
	// with the child already dead. WaitDelay is what closes them.
	toolKillDelay = time.Second
	// toolMaxOutput caps what is read. Far above any configuration dump;
	// the point is that a tool which has gone wrong cannot make this
	// agent allocate without bound.
	toolMaxOutput = 1 << 20
)

// lookTool returns the first of the given ABSOLUTE paths that exists.
//
// Absolute and enumerated rather than resolved through PATH: PATH is
// inherited from whatever started the agent, and letting it choose which
// binary answers a security question is a way to be told what somebody
// else would like to be true.
func lookTool(paths ...string) string {
	for _, p := range paths {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

// runTool runs one known program and returns its stdout, or nil.
//
// Nil for every failure, a non-zero exit included: the contract is
// "report this if it can be read", and a tool that refused to answer has
// not given a partial answer worth reporting.
func runTool(path string, args ...string) []byte {
	ctx, cancel := context.WithTimeout(context.Background(), toolTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, path, args...)
	// A fixed environment, not the inherited one. LC_ALL and LANG because
	// several of these tools localise their output and a collector must
	// not parse a translation; PATH because some re-exec themselves and
	// an empty one breaks them. Nothing else is passed -- the agent's
	// environment is not this program's business.
	cmd.Env = []string{"LC_ALL=C", "LANG=C", "PATH=/usr/sbin:/usr/bin:/sbin:/bin"}
	// A nil Stdin is /dev/null. Left explicit because the alternative --
	// a tool waiting on input nobody will send -- hangs the collector,
	// and that is not obvious from the field's absence.
	cmd.Stdin = nil
	cmd.WaitDelay = toolKillDelay

	out := &capBuffer{max: toolMaxOutput}
	cmd.Stdout = out
	// Stderr is dropped rather than merged: this output gets parsed, and
	// a warning line interleaved into it would become a setting.
	cmd.Stderr = nil

	if err := cmd.Run(); err != nil || out.over {
		return nil
	}
	return out.buf.Bytes()
}

// capBuffer collects up to max bytes and remembers whether more arrived.
//
// It never returns an error, so an overrunning tool is stopped by the
// deadline rather than by a broken pipe, and the caller refuses the
// truncated result rather than parsing half a file as though it were
// whole.
type capBuffer struct {
	buf  bytes.Buffer
	max  int
	over bool
}

func (c *capBuffer) Write(p []byte) (int, error) {
	room := c.max - c.buf.Len()
	switch {
	case room >= len(p):
		c.buf.Write(p)
	case room > 0:
		c.buf.Write(p[:room])
		c.over = true
	case len(p) > 0:
		c.over = true
	}
	return len(p), nil
}
