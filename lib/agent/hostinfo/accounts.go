package hostinfo

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"sort"
	"strconv"
	"strings"

	agenttypes "vraxel.io/vraxel/lib/agent/types"
)

// passwdEntry is one line of /etc/passwd.
type passwdEntry struct {
	name  string
	uid   int64
	gid   int64
	home  string
	shell string
}

// parsePasswd reads /etc/passwd. Malformed lines are skipped rather than
// failing the file: a single bad entry must not cost the inventory of
// every other account on the machine.
func parsePasswd(data []byte) []passwdEntry {
	var out []passwdEntry
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		// name:passwd:uid:gid:gecos:home:shell
		f := strings.Split(sc.Text(), ":")
		if len(f) < 7 || f[0] == "" || strings.HasPrefix(f[0], "#") {
			continue
		}
		uid, err1 := strconv.ParseInt(f[2], 10, 64)
		gid, err2 := strconv.ParseInt(f[3], 10, 64)
		if err1 != nil || err2 != nil {
			continue
		}
		out = append(out, passwdEntry{name: f[0], uid: uid, gid: gid, home: f[5], shell: f[6]})
	}
	return out
}

// parseGroup reads /etc/group.
func parseGroup(data []byte) []agenttypes.UserGroup {
	var out []agenttypes.UserGroup
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		// name:passwd:gid:member,member
		f := strings.Split(sc.Text(), ":")
		if len(f) < 4 || f[0] == "" || strings.HasPrefix(f[0], "#") {
			continue
		}
		gid, err := strconv.ParseInt(f[2], 10, 64)
		if err != nil {
			continue
		}
		g := agenttypes.UserGroup{Name: f[0], GID: gid}
		for _, m := range strings.Split(f[3], ",") {
			if m = strings.TrimSpace(m); m != "" {
				g.Members = append(g.Members, m)
			}
		}
		out = append(out, g)
	}
	return out
}

// parseShadowStates maps each account to the SHAPE of its password field.
//
// The hash is read and immediately discarded. It never enters a struct,
// never reaches a frame, and never reaches the database -- what leaves
// this function is one of four words.
func parseShadowStates(data []byte) map[string]string {
	out := map[string]string{}
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		name, rest, ok := strings.Cut(sc.Text(), ":")
		if !ok || name == "" {
			continue
		}
		field, _, _ := strings.Cut(rest, ":")
		out[name] = passwordState(field)
	}
	return out
}

// passwordState classifies one shadow password field.
func passwordState(field string) string {
	switch {
	case field == "":
		// Not "no password set" -- no password REQUIRED. Anyone at a
		// console or an sshd configured to permit it logs straight in.
		return agenttypes.PwEmpty
	case strings.HasPrefix(field, "!"):
		// passwd -l prefixes the existing hash rather than removing it, so
		// this is a disabled account that can be restored as it was.
		return agenttypes.PwLocked
	case field == "*" || field == "x":
		return agenttypes.PwDisabled
	default:
		return agenttypes.PwSet
	}
}

// nonLoginShells are the shells that exist to refuse a login. Matched on
// the basename because the path varies (/usr/sbin/nologin,
// /sbin/nologin, /usr/bin/false).
var nonLoginShells = map[string]struct{}{
	"nologin": {}, "false": {}, "true": {}, "sync": {}, "shutdown": {}, "halt": {},
}

// canLogin reports whether a shell would give somebody a session.
//
// An empty shell field is NOT a refusal: the kernel falls back to
// /bin/sh, so an account with no shell listed is an account with a shell.
func canLogin(shell string) bool {
	if shell == "" {
		return true
	}
	base := shell
	if i := strings.LastIndex(base, "/"); i >= 0 {
		base = base[i+1:]
	}
	_, blocked := nonLoginShells[base]
	return !blocked
}

// rootEquivalentGroups maps a group name to the privilege its membership
// confers.
//
// docker and lxd are here for a reason that is invisible in /etc/sudoers:
// a member can start a container with the host's root filesystem bind
// mounted and write to any file on it. It is unrestricted root by a route
// no sudo audit would ever show. disk and raw grant the block devices
// under every filesystem, which is the same power one layer down.
var rootEquivalentGroups = map[string]string{
	"docker": agenttypes.PrivDockerGroup,
	"lxd":    agenttypes.PrivDockerGroup,
	"podman": agenttypes.PrivDockerGroup,
	"disk":   agenttypes.PrivDiskGroup,
	"raw":    agenttypes.PrivDiskGroup,
}

// sudoersWho is the set of users and groups that any sudoers rule names.
type sudoersWho struct {
	users  map[string]struct{}
	groups map[string]struct{}
}

// parseSudoers extracts the rule lines and WHO each one names.
//
// Only the who field is interpreted. The rest of the line -- host spec,
// Runas list, NOPASSWD tags, the command set -- is returned verbatim for
// a human to read, because deciding from it whether a rule "grants root"
// means implementing sudo's grammar, and a parser that gets it 90% right
// produces a confident wrong answer about the one account that matters.
//
// User_Alias IS expanded, because it is the only alias kind that changes
// who a rule applies to: without it, "ADMINS ALL=(ALL) ALL" would flag a
// nonexistent user named ADMINS and silently miss every real member.
// Host_Alias, Cmnd_Alias and Runas_Alias are ignored on purpose.
func parseSudoers(files map[string][]byte, order []string) (rules []string, who sudoersWho) {
	who = sudoersWho{users: map[string]struct{}{}, groups: map[string]struct{}{}}
	aliases := map[string][]string{}

	// Two passes: an alias may be defined in a file included after the
	// rule that uses it, and sudo resolves them across the whole set.
	for _, name := range order {
		for _, line := range sudoersLines(files[name]) {
			if rest, ok := strings.CutPrefix(line, "User_Alias"); ok {
				if n, members, found := strings.Cut(rest, "="); found {
					aliases[strings.TrimSpace(n)] = splitList(members)
				}
			}
		}
	}
	for _, name := range order {
		for _, line := range sudoersLines(files[name]) {
			if isSudoersDirective(line) {
				continue
			}
			rules = append(rules, line)
			for _, w := range sudoersRuleWho(line) {
				addSudoersWho(&who, aliases, w, 0)
			}
		}
	}
	return rules, who
}

// sudoersRuleWho returns the users and groups a rule line applies to.
//
// A rule is "who host = (runas) commands", and the who part is what this
// has to isolate. It cannot be "everything before the first space": the
// distro default is tab-separated ("root\tALL=(ALL:ALL) ALL") and its
// FIRST space falls inside the runas list, so a space-split hands back
// "root\tALL=(ALL:ALL)" -- a token matching no account, which is how root
// itself came out looking like it had no sudo rule.
//
// It cannot be "the first whitespace token" either, because the who part
// is a comma-separated LIST that is allowed spaces after the commas
// ("alice, bob ALL=(ALL) ALL").
//
// What is unambiguous is the "=": everything before it is the who list
// plus exactly one host spec, so dropping that last token leaves the who
// list however it was spaced.
func sudoersRuleWho(line string) []string {
	left, _, found := strings.Cut(line, "=")
	if !found {
		return nil
	}
	fields := strings.Fields(left)
	if len(fields) < 2 {
		return nil
	}
	return splitList(strings.Join(fields[:len(fields)-1], " "))
}

// addSudoersWho records one who-token, expanding a User_Alias.
//
// depth bounds alias recursion: an alias may name another alias, and a
// hand-edited file can name itself. sudo would reject the cycle; this
// just stops descending rather than looping forever on a machine nobody
// is watching.
func addSudoersWho(who *sudoersWho, aliases map[string][]string, token string, depth int) {
	token = strings.TrimSpace(token)
	switch {
	case token == "":
		return
	case strings.HasPrefix(token, "%#"), strings.HasPrefix(token, "+"):
		// A gid-by-number spec and a netgroup. Neither resolves to a name
		// without more lookups than this is worth.
		return
	case strings.HasPrefix(token, "%"):
		who.groups[strings.TrimPrefix(token, "%")] = struct{}{}
	default:
		if members, ok := aliases[token]; ok {
			if depth >= 4 {
				return
			}
			for _, m := range members {
				addSudoersWho(who, aliases, m, depth+1)
			}
			return
		}
		who.users[token] = struct{}{}
	}
}

// isSudoersDirective reports whether a line configures sudo rather than
// granting anybody anything.
func isSudoersDirective(line string) bool {
	for _, p := range []string{"Defaults", "User_Alias", "Host_Alias", "Cmnd_Alias", "Runas_Alias"} {
		if strings.HasPrefix(line, p) {
			return true
		}
	}
	return strings.HasPrefix(line, "@include") || strings.HasPrefix(line, "#include")
}

// sudoersLines returns the meaningful lines of one sudoers file.
//
// A leading "#" is only a comment when it is not an include: sudo spells
// that directive "#includedir", which is a comment everywhere else in
// Unix and would be stripped by a naive comment filter.
func sudoersLines(data []byte) []string {
	var out []string
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "#include") {
			continue
		}
		out = append(out, line)
	}
	return out
}

// sudoersIncludes returns the directories a sudoers file pulls in.
func sudoersIncludes(data []byte) []string {
	var dirs []string
	for _, line := range sudoersLines(data) {
		for _, p := range []string{"#includedir", "@includedir"} {
			if rest, ok := strings.CutPrefix(line, p); ok {
				if d := strings.TrimSpace(rest); d != "" {
					dirs = append(dirs, d)
				}
			}
		}
	}
	return dirs
}

// splitList splits a comma-separated sudoers list.
func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// keyTypePrefixes are how an authorized_keys line names its algorithm.
var keyTypePrefixes = []string{"ssh-", "ecdsa-", "sk-ssh-", "sk-ecdsa-", "rsa-sha2-"}

// parseAuthorizedKeys reduces one authorized_keys file to fingerprints.
//
// The key body is read only to hash it and is not reported: a public key
// is not a secret, but a fingerprint is what anybody actually compares
// against, and it is 50 characters instead of 400.
//
// The type is found by scanning for the first field that looks like an
// algorithm rather than by taking field 0, because a line may begin with
// an options list (command="...",no-pty) that itself contains spaces.
func parseAuthorizedKeys(data []byte) []agenttypes.SSHKey {
	var out []agenttypes.SSHKey
	sc := bufio.NewScanner(bytes.NewReader(data))
	// A single key line can exceed bufio's default 64 KiB budget once
	// options are involved; the file as a whole stays small.
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		i := indexOfKeyType(fields)
		if i < 0 || i+1 >= len(fields) {
			continue
		}
		fp, ok := sshFingerprint(fields[i+1])
		if !ok {
			continue
		}
		out = append(out, agenttypes.SSHKey{
			Type:        fields[i],
			Fingerprint: fp,
			Comment:     strings.Join(fields[i+2:], " "),
		})
	}
	return out
}

func indexOfKeyType(fields []string) int {
	for i, f := range fields {
		for _, p := range keyTypePrefixes {
			if strings.HasPrefix(f, p) {
				return i
			}
		}
	}
	return -1
}

// sshFingerprint computes the OpenSSH SHA256 fingerprint of a base64 key
// blob: the SHA-256 of the raw key, base64 without padding. Same value
// ssh-keygen -l prints, computed here rather than shelled out.
func sshFingerprint(blob string) (string, bool) {
	raw, err := base64.StdEncoding.DecodeString(blob)
	if err != nil || len(raw) == 0 {
		return "", false
	}
	sum := sha256.Sum256(raw)
	return "SHA256:" + base64.RawStdEncoding.EncodeToString(sum[:]), true
}

// buildAccounts assembles the reported inventory from the parsed files.
//
// Split out of the file reading so it can be tested against a whole
// synthetic machine: the interesting behaviour here is how the four
// sources combine, not how any one of them is read.
func buildAccounts(
	users []passwdEntry,
	groups []agenttypes.UserGroup,
	shadow map[string]string,
	who sudoersWho,
	keysOf func(home string) []agenttypes.SSHKey,
) ([]agenttypes.Account, []agenttypes.UserGroup) {
	byGID := make(map[int64]string, len(groups))
	supplementary := map[string][]string{}
	for _, g := range groups {
		byGID[g.GID] = g.Name
		for _, m := range g.Members {
			supplementary[m] = append(supplementary[m], g.Name)
		}
	}

	out := make([]agenttypes.Account, 0, len(users))
	for _, u := range users {
		a := agenttypes.Account{
			Name:     u.name,
			UID:      u.uid,
			GID:      u.gid,
			Group:    byGID[u.gid],
			Home:     u.home,
			Shell:    u.shell,
			CanLogin: canLogin(u.shell),
			Password: shadow[u.name],
			Groups:   supplementary[u.name],
		}
		sort.Strings(a.Groups)
		a.Privileges = privilegesOf(a, who)
		if keysOf != nil {
			a.SSHKeys = keysOf(u.home)
		}
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UID < out[j].UID })
	sort.Slice(groups, func(i, j int) bool { return groups[i].GID < groups[j].GID })
	return out, groups
}

// privilegesOf lists every route this account has to root.
//
// The primary group counts as membership. It is not in /etc/group's
// member list -- that field holds only supplementary members -- so an
// account whose PRIMARY group is docker would otherwise look unprivileged
// while being exactly as privileged as one listed there.
func privilegesOf(a agenttypes.Account, who sudoersWho) []string {
	var out []string
	if a.UID == 0 {
		out = append(out, agenttypes.PrivRoot)
	}
	if _, ok := who.users[a.Name]; ok {
		out = append(out, agenttypes.PrivSudo)
	} else {
		for _, g := range allGroupsOf(a) {
			if _, ok := who.groups[g]; ok {
				out = append(out, agenttypes.PrivSudo)
				break
			}
		}
	}
	seen := map[string]struct{}{}
	for _, g := range allGroupsOf(a) {
		p, ok := rootEquivalentGroups[g]
		if !ok {
			continue
		}
		if _, dup := seen[p]; dup {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}

func allGroupsOf(a agenttypes.Account) []string {
	if a.Group == "" {
		return a.Groups
	}
	return append([]string{a.Group}, a.Groups...)
}
