package hostinfo

import (
	"bufio"
	"bytes"
	"strconv"
	"strings"

	agenttypes "vraxel.io/vraxel/lib/agent/types"
)

// parseSSHDConfig reads the output of `sshd -T`.
//
// One "key value" line per setting, keys lowercased by sshd itself, and
// only the settings this platform has a question about are kept. A fixed
// set rather than the whole dump: sshd -T prints a hundred lines, most of
// them ciphers and timeouts nobody is here to read, and shipping all of
// them would be a hundred fields nobody chose.
func parseSSHDConfig(data []byte) *agenttypes.SSHDConfig {
	out := &agenttypes.SSHDConfig{}
	seen := false
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		key, value, ok := strings.Cut(strings.TrimSpace(sc.Text()), " ")
		if !ok {
			continue
		}
		seen = true
		switch key {
		case "port":
			out.Ports = append(out.Ports, int32(atoi(value)))
		case "permitrootlogin":
			// Four values, not a boolean: "prohibit-password" is the
			// common hardened setting and folding it into yes/no would
			// either raise a false alarm or hide a real one.
			out.PermitRootLogin = value
		case "passwordauthentication":
			out.PasswordAuth = value == "yes"
		case "kbdinteractiveauthentication":
			out.KbdInteractiveAuth = value == "yes"
		case "pubkeyauthentication":
			out.PubkeyAuth = value == "yes"
		case "permitemptypasswords":
			out.PermitEmptyPasswords = value == "yes"
		case "maxauthtries":
			out.MaxAuthTries = int32(atoi(value))
		// APPENDED, not assigned. sshd -T prints one line per entry --
		// "AllowGroups sudo adm" comes back as two allowgroups lines --
		// so assigning would keep only the last and report an access
		// list that lets one account in where the machine lets several.
		case "allowusers":
			out.AllowUsers = append(out.AllowUsers, strings.Fields(value)...)
		case "allowgroups":
			out.AllowGroups = append(out.AllowGroups, strings.Fields(value)...)
		case "denyusers":
			out.DenyUsers = append(out.DenyUsers, strings.Fields(value)...)
		case "denygroups":
			out.DenyGroups = append(out.DenyGroups, strings.Fields(value)...)
		}
	}
	if !seen {
		return nil
	}
	return out
}

// countMatchBlocks counts "Match" directives in a config file.
//
// Deliberately the only thing read out of sshd_config, and deliberately
// not interpreted. The summary above is what sshd -T computes WITHOUT
// evaluating Match, so a host with conditional overrides has exceptions
// this report does not describe. Counting them says so; parsing them
// would mean picking a user and address to evaluate them for, and any
// pick answers a question nobody asked.
func countMatchBlocks(data []byte) int32 {
	var n int32
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		// "Match" is case-insensitive in sshd_config, and the keyword is
		// only a Match block when something follows it.
		if kw, rest, ok := strings.Cut(line, " "); ok &&
			strings.EqualFold(kw, "match") && strings.TrimSpace(rest) != "" {
			n++
		}
	}
	return n
}

// includeGlobs returns the patterns an sshd_config pulls in.
func includeGlobs(data []byte) []string {
	var out []string
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		kw, rest, ok := strings.Cut(line, " ")
		if ok && strings.EqualFold(kw, "include") {
			out = append(out, strings.Fields(rest)...)
		}
	}
	return out
}

func atoi(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	return n
}
