//go:build linux

package hostinfo

import (
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	agenttypes "vraxel.io/vraxel/lib/agent/types"
)

const (
	passwdPath     = "/etc/passwd"
	groupPath      = "/etc/group"
	shadowPath     = "/etc/shadow"
	sudoersPath    = "/etc/sudoers"
	lastlogPath    = "/var/log/lastlog"
	maxKeyFileSize = 256 << 10
)

// Accounts collects who can use this machine and what they can do on it.
//
// /etc/shadow is read, and the agent runs as root so it can be. Only the
// first character of each password field survives the read -- see
// parseShadowStates. Nothing in the returned structure can be used to
// authenticate as anybody.
func Accounts() agenttypes.HostAccounts {
	users := parsePasswd(readFileBytes(passwdPath))
	groups := parseGroup(readFileBytes(groupPath))
	shadow := parseShadow(readFileBytes(shadowPath))
	rules, who := parseSudoers(sudoersFiles())
	logins := lastLogins()

	accounts, groups := buildAccounts(users, groups, shadow, who, authorizedKeys,
		func(uid int64) int64 { return logins[uid] })
	return agenttypes.HostAccounts{Users: accounts, Groups: groups, SudoRules: rules, SSHD: sshdConfig()}
}

// sudoersFiles reads /etc/sudoers and every file it includes.
//
// Returned as a map plus an explicit order rather than as a slice,
// because parseSudoers walks the set twice (aliases first, then rules)
// and the second pass must see the same files in the same order --
// otherwise which rule wins depends on map iteration, which changes per
// run and would make the report look different every time.
func sudoersFiles() (map[string][]byte, []string) {
	files := map[string][]byte{}
	var order []string

	root, err := os.ReadFile(sudoersPath)
	if err != nil {
		// Not an error worth reporting: a container image routinely has no
		// sudo installed, and "no rules" is the truthful answer.
		return files, order
	}
	files[sudoersPath] = root
	order = append(order, sudoersPath)

	for _, dir := range sudoersIncludes(root) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		var names []string
		for _, e := range entries {
			// sudo's own rules for an included directory: editor backups
			// and anything with a dot are ignored, which is what keeps a
			// half-saved file from granting anybody anything.
			if e.IsDir() || strings.HasSuffix(e.Name(), "~") || strings.Contains(e.Name(), ".") {
				continue
			}
			names = append(names, e.Name())
		}
		// ReadDir already sorts, and sudo reads an included directory in
		// lexical order, so this matches what the machine actually does.
		for _, n := range names {
			p := filepath.Join(dir, n)
			b, err := os.ReadFile(p)
			if err != nil {
				continue
			}
			files[p] = b
			order = append(order, p)
		}
	}
	return files, order
}

// sshdAuthorizedKeysFiles is where sshd looks, as its own default
// spells it. AuthorizedKeysFile in sshd_config can move this, which is
// why a machine that has moved it reports nothing rather than something
// wrong -- there is no second guess here that would be better than none.
var sshdAuthorizedKeysFiles = []string{".ssh/authorized_keys", ".ssh/authorized_keys2"}

// authorizedKeys reads the keys that let somebody in as this account.
func authorizedKeys(home string) []agenttypes.SSHKey {
	if home == "" || home == "/" || home == "/nonexistent" {
		return nil
	}
	var out []agenttypes.SSHKey
	for _, rel := range sshdAuthorizedKeysFiles {
		path := filepath.Join(home, rel)
		st, err := os.Stat(path)
		if err != nil || !st.Mode().IsRegular() {
			continue
		}
		// A key file is a few hundred bytes. Something multi-megabyte at
		// this path is not one, and reading it into memory on a timer is
		// how a mislabelled file becomes the agent's memory problem.
		if st.Size() > maxKeyFileSize {
			continue
		}
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		out = append(out, parseAuthorizedKeys(b)...)
	}
	return out
}

// maxPasswdBytes bounds one passwd read.
//
// The host's own file needs no bound; a container's does. Since the
// process walk resolves uids against the passwd file inside each mount
// namespace, the bytes read are chosen by whatever is running in the
// container, and an unbounded os.ReadFile there is a way for one to make
// the agent allocate as much as it likes. Far above any real passwd file,
// including the LDAP-flattened ones.
const maxPasswdBytes = 4 << 20

// uidNameTable maps uid to name, read from one passwd file.
//
// Built per collection, never cached across them: the agent runs for
// weeks, and a cache would keep reporting the name a uid had when the
// process first started -- so an account created or renamed after that
// would be wrong until somebody restarted the agent, which is the kind of
// staleness nobody thinks to look for.
func uidNameTable(path string) map[int64]string {
	out := map[int64]string{}
	for _, e := range parsePasswd(readFileLimit(path, maxPasswdBytes)) {
		// First entry wins. Two names sharing a uid is legal and is how a
		// second root is spelled; the process list shows the first, and
		// the account list shows both.
		if _, dup := out[e.uid]; !dup {
			out[e.uid] = e.name
		}
	}
	return out
}

// userName resolves a uid for the process list, falling back to the
// number.
//
// The fallback is now rare: uids are resolved against the passwd file of
// the process's OWN mount namespace, so a container's uid 999 reads
// "postgres" from the image rather than as a bare number. What is left is
// a uid with no passwd entry anywhere, and the number is the honest answer
// there -- printing THIS machine's user 999 for a container that has no
// such user would be a wrong answer that looks right.
func userName(table map[int64]string, uid int64) string {
	if n, ok := table[uid]; ok {
		return n
	}
	return strconv.FormatInt(uid, 10)
}

// lastlogRecordSize is sizeof(struct lastlog): a 32-bit time followed by
// a 32-byte tty and a 256-byte host. The file is a flat array of these
// INDEXED BY UID -- record N starts at N*292 -- which is why it is sparse
// and why a machine whose highest uid ever to log in is 0 has a 292-byte
// file.
const lastlogRecordSize = 292

// maxLastlogBytes bounds the read. A machine with a uid in the billions
// (some LDAP mappings) has a nominally enormous file; it is sparse on
// disk, but reading it whole would not be.
const maxLastlogBytes = 8 << 20

// lastLogins maps uid to its last login, truncated to the day.
//
// Truncated because this rides a report that is sent only when its
// content changed: the exact second changes on every login, so a jump
// host would re-send its whole account inventory every time anybody
// connected. To the day it changes at most once per account per day.
//
// wtmp would give richer history and is the wrong file for this: it is
// an append-only log of every login ever, rotated and often gigabytes,
// and answering "when did uid N last log in" from it means scanning the
// whole thing. lastlog is the kernel-maintained answer to exactly this
// question, at a fixed offset.
func lastLogins() map[int64]int64 {
	out := map[int64]int64{}
	f, err := os.Open(lastlogPath)
	if err != nil {
		// No lastlog is normal: a container image has none, and some
		// distros have moved to lastlog2. Nothing reported beats a wrong
		// answer, and every other account field still arrives.
		return out
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil || st.Size() == 0 {
		return out
	}
	size := st.Size()
	if size > maxLastlogBytes {
		size = maxLastlogBytes
	}
	buf := make([]byte, size-size%lastlogRecordSize)
	n, err := io.ReadFull(f, buf)
	if err != nil && n == 0 {
		return out
	}
	for off := 0; off+lastlogRecordSize <= n; off += lastlogRecordSize {
		// Little-endian int32 seconds. A zero record is an account that
		// has never logged in, which is most of the file.
		secs := int64(int32(binary.LittleEndian.Uint32(buf[off : off+4])))
		if secs <= 0 {
			continue
		}
		uid := int64(off / lastlogRecordSize)
		out[uid] = secs / 86400 * 86400 * 1000
	}
	return out
}
