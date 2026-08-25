//go:build linux

package hostinfo

import (
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
	shadow := parseShadowStates(readFileBytes(shadowPath))
	rules, who := parseSudoers(sudoersFiles())

	accounts, groups := buildAccounts(users, groups, shadow, who, authorizedKeys)
	return agenttypes.HostAccounts{Users: accounts, Groups: groups, SudoRules: rules}
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

// uidNameTable maps uid to name for one process walk.
//
// Built per collection, never cached across them: the agent runs for
// weeks, and a cache would keep reporting the name a uid had when the
// process first started -- so an account created or renamed after that
// would be wrong until somebody restarted the agent, which is the kind of
// staleness nobody thinks to look for.
func uidNameTable() map[int64]string {
	out := map[int64]string{}
	for _, e := range parsePasswd(readFileBytes(passwdPath)) {
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
// The number is not a failed lookup to hide: a container process runs as
// a uid that exists only inside its image, and printing "1000" is honest
// where printing THIS machine's user 1000 would be a wrong answer that
// looks right.
func userName(table map[int64]string, uid int64) string {
	if n, ok := table[uid]; ok {
		return n
	}
	return strconv.FormatInt(uid, 10)
}
