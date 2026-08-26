package types

// This file is the wire contract for what the machine is DOING, as
// opposed to HostFacts, which is what the machine IS.
//
// The distinction is load-bearing, and it is the reason these are
// separate frames rather than more fields on HostFacts. Facts sit still:
// a CPU model does not change, so the agent can compare a report against
// the last one it sent and stay silent, and the whole struct is built
// around that gate. Runtime data does change -- and the design problem
// for every field below was making it change ONLY when something an
// operator would call a change actually happened.

// HostProcesses is the machine's workload.
//
// Grouped, never per-process, and carrying no PID. Both follow from the
// reporting cadence: this arrives minutes apart, so a PID printed in a
// table is near-certainly stale by the time somebody reads it -- the same
// reason HostFacts carries no uptime. Dropping it is not a loss of
// detail, it is the removal of a field that could only ever mislead.
//
// Dropping it also makes the send-only-when-changed gate work at all.
// With PIDs, restarting one service rewrites every row that follows it
// and the agent would ship the whole table; grouped by identity, a
// restart produces a byte-identical report and nothing is sent. It bounds
// the payload too: the size follows the number of DISTINCT workloads, not
// the process count, so a machine running 400 nginx workers reports one
// row rather than four hundred.
type HostProcesses struct {
	Groups []ProcessGroup `json:"groups,omitempty"`
}

// ProcessGroup is one workload: every process sharing an identity,
// counted.
type ProcessGroup struct {
	// Name is the executable name (/proc/pid/comm), truncated to 15
	// characters by the kernel -- "victoria-metric", not
	// "victoria-metrics". Passed through as the kernel reports it rather
	// than reconstructed from the command line, which is not collected.
	Name string `json:"name"`
	// User is the effective user's name, or its numeric uid when the
	// machine has no passwd entry for it (every container process whose
	// uid exists only inside the image).
	User  string `json:"user,omitempty"`
	Count int32  `json:"count"`
	// Unit is the systemd unit this workload belongs to, from its cgroup
	// path. This is where a workload's IDENTITY comes from, and taking it
	// from the supervisor rather than from the command line is deliberate:
	// a command line routinely carries credentials (mysql -pSECRET, a
	// --token= flag), and a CMDB table that many people can read must not
	// be able to leak one. The cost is that two bare-shell java processes
	// are indistinguishable; production does not run that way.
	Unit string `json:"unit,omitempty"`
	// Container is true when the cgroup names a container scope rather
	// than a unit. The container's NAME is not here: the cgroup carries
	// only its 64-hex id, and resolving that means either talking to a
	// container daemon or reading its private state files.
	Container bool `json:"container,omitempty"`
	// Ports are the sockets this workload listens on -- the answer to
	// "what does this machine serve", which nothing else in the platform
	// can give. Empty for the workloads that only make outbound
	// connections, which is most of them and is exactly why the process
	// list is not filtered down to listeners.
	Ports []ListenPort `json:"ports,omitempty"`
	// Exe is the resolved path of the running binary. It carries no
	// credentials -- unlike the command line, which is why that one is not
	// collected -- and it answers a question Name cannot: /usr/sbin/nginx
	// and /tmp/nginx report the same comm.
	Exe string `json:"exe,omitempty"`
	// StartedAt is when the OLDEST member of this group started, in unix
	// milliseconds, dated from the machine's boot time rather than read
	// off a clock.
	//
	// A stable field despite describing a moment: it changes when the
	// workload restarts, and a restart is exactly the event this report
	// should surface. The oldest member because a prefork server replaces
	// workers continuously while the service itself has been up for
	// months.
	StartedAtMs int64 `json:"startedAtMs,omitempty"`

	// --- live only ---
	// CPUPct and RSSBytes are filled in ONLY on the process-stats stream,
	// never on the host.processes frame. Both change every time they are
	// read, and the frame is sent only when its content changes; carrying
	// them there would defeat that gate on every sample. Zero means "not
	// measured", which is what the stored inventory always says.
	//
	// RSSBytes counts each member's private pages plus ONE copy of what
	// they share, not the sum of their VmRSS -- see hostinfo.parseStatusMem
	// for why the naive sum reported 311 MiB for a postgres whose real
	// footprint was 130.
	CPUPct   float64 `json:"cpuPct,omitempty"`
	RSSBytes int64   `json:"rssBytes,omitempty"`
}

// ListenPort is one listening socket.
type ListenPort struct {
	// Proto is "tcp" or "udp". The v4/v6 split is not here because Addr
	// already carries it: 0.0.0.0 and :: are different bindings, and a
	// separate "tcp6" would say the same thing twice.
	Proto string `json:"proto"`
	Addr  string `json:"addr,omitempty"`
	Port  int32  `json:"port"`
}

// HostAccounts is who can use the machine, and what they can do on it.
//
// Nothing here is a secret and nothing here may become one. The shadow
// file is read to learn the SHAPE of a password field -- set, locked,
// absent -- and the hash itself never leaves the machine. Authorized keys
// are reported as fingerprints, which is the form anyone actually
// compares against.
type HostAccounts struct {
	Users  []Account   `json:"users,omitempty"`
	Groups []UserGroup `json:"groups,omitempty"`
	// SudoRules are the non-comment, non-Defaults lines of /etc/sudoers
	// and whatever it includes, verbatim.
	//
	// Verbatim because sudoers has a real grammar -- aliases, host specs,
	// Runas lists, NOPASSWD, command sets -- and a parser that understands
	// most of it produces confident wrong answers about who can become
	// root, which is worse than no answer. What IS extracted is the who
	// field, which is unambiguous, and it lands in Account.Privileges. The
	// rest is put in front of a human unaltered.
	SudoRules []string `json:"sudoRules,omitempty"`
	// SSHD is the daemon's side of the same question: the accounts above
	// exist, and this says which of them can actually get in.
	SSHD *SSHDConfig `json:"sshd,omitempty"`
}

// Account is one entry of /etc/passwd, with what the machine knows about
// what it can do.
type Account struct {
	Name string `json:"name"`
	// int64, not int32: a uid is an unsigned 32-bit value and the
	// conventional "nobody" on several systems is 4294967294, which
	// overflows a signed 32-bit field into a negative user id.
	UID   int64  `json:"uid"`
	GID   int64  `json:"gid"`
	Group string `json:"group,omitempty"`
	Home  string `json:"home,omitempty"`
	Shell string `json:"shell,omitempty"`
	// CanLogin is false for the nologin / false shells that most of a
	// passwd file carries. It is what separates the two or three accounts
	// a person could log into from the twenty-odd a package manager made.
	CanLogin bool `json:"canLogin,omitempty"`
	// Password is one of the Pw* constants: the shape of the shadow
	// field, never its content.
	Password string `json:"password,omitempty"`
	// Groups are the supplementary groups naming this account.
	Groups []string `json:"groups,omitempty"`
	// Privileges names every route this account has to root, empty for an
	// ordinary one. See the Priv* constants -- there is more than one
	// route, and only the first is visible in /etc/passwd.
	Privileges []string `json:"privileges,omitempty"`
	SSHKeys    []SSHKey `json:"sshKeys,omitempty"`
	// FullName is the first GECOS field. The passwd comment is
	// comma-separated ("zly,,,") and only the first part is the name;
	// the rest is office and phone that nobody has filled in since 1985.
	FullName string `json:"fullName,omitempty"`
	// PasswordChangedAtMs and ExpiresAtMs come from the shadow entry's
	// day counters, so they land on midnight UTC and no finer. Both are
	// the raw fields, not a judgement: how old is too old is a policy
	// this agent does not hold.
	PasswordChangedAtMs int64 `json:"passwordChangedAtMs,omitempty"`
	ExpiresAtMs         int64 `json:"expiresAtMs,omitempty"`
	// LastLoginAtMs is when this account last logged in, TRUNCATED TO THE
	// DAY.
	//
	// Truncated on purpose. The precise value changes on every login,
	// and this rides a report that is sent only when its content changes
	// -- so a jump host would re-send its whole account inventory every
	// time anybody connected. To the day it changes at most once per
	// account per day, which costs nothing and still answers the question
	// worth asking: "has anybody used this account in two years".
	//
	// Zero means never logged in, which for a service account is the
	// expected answer and for a person's account is a finding.
	LastLoginAtMs int64 `json:"lastLoginAtMs,omitempty"`
}

// SSHDConfig is the ssh daemon's EFFECTIVE configuration: the settings
// that decide who can reach this machine and how.
//
// The account inventory beside it answers half the question -- root has a
// password, this user has keys -- and this answers the other half.
// Neither is a finding alone: root having a password matters only if the
// daemon accepts passwords and permits root, and those two live here.
//
// Read from `sshd -T` and not from sshd_config. The file is not the
// configuration: Include pulls in a directory, compiled-in defaults fill
// what nobody wrote, and only sshd knows which is which. A hand parse
// would produce a confident wrong answer about who can log in.
type SSHDConfig struct {
	// Ports is every port the daemon listens on. A list because sshd -T
	// prints one line per Port directive, and a machine reachable on 22
	// and 2222 described as "22" is an answer somebody would act on.
	Ports           []int32 `json:"ports,omitempty"`
	PermitRootLogin string  `json:"permitRootLogin,omitempty"`
	PasswordAuth    bool    `json:"passwordAuth"`
	// KbdInteractiveAuth is the OTHER password path, through PAM.
	// Reported beside PasswordAuth because turning that one off and
	// leaving this one on is a machine that still takes passwords while
	// its configuration looks like it does not.
	KbdInteractiveAuth   bool  `json:"kbdInteractiveAuth"`
	PubkeyAuth           bool  `json:"pubkeyAuth"`
	PermitEmptyPasswords bool  `json:"permitEmptyPasswords"`
	MaxAuthTries         int32 `json:"maxAuthTries,omitempty"`
	// The four access lists, empty when unset -- sshd -T prints them only
	// when configured, and "unset" means no restriction rather than an
	// empty allowlist. Together with the account inventory they answer
	// "of the accounts that exist, which can actually come in over ssh".
	AllowUsers  []string `json:"allowUsers,omitempty"`
	AllowGroups []string `json:"allowGroups,omitempty"`
	DenyUsers   []string `json:"denyUsers,omitempty"`
	DenyGroups  []string `json:"denyGroups,omitempty"`
	// MatchBlocks counts the conditional blocks in the configuration, and
	// exists because everything above it is the GLOBAL answer.
	//
	// `sshd -T` without -C does not evaluate Match, so a host with
	// "PasswordAuthentication no" globally and a Match block turning it
	// back on for one group reports "no" here. Evaluating them would mean
	// choosing a user and address to evaluate them FOR, and any choice
	// would be an answer to a question nobody asked. Counting them is the
	// honest middle: the reader is told this summary has exceptions
	// without being told a wrong one.
	MatchBlocks int32 `json:"matchBlocks,omitempty"`
}

// UserGroup is one entry of /etc/group.
type UserGroup struct {
	Name string `json:"name"`
	GID  int64  `json:"gid"`
	// Members are the SUPPLEMENTARY members listed in /etc/group. A user
	// whose PRIMARY group this is does not appear here -- that membership
	// lives in the passwd entry -- so this list is not the full answer to
	// "who is in this group" and Account.Groups is not built from it
	// alone.
	Members []string `json:"members,omitempty"`
}

// SSHKey is one line of an authorized_keys file, reduced to what
// identifies it.
type SSHKey struct {
	Type string `json:"type"`
	// Fingerprint is the OpenSSH SHA256 form ("SHA256:base64"), which is
	// what ssh-keygen -l prints and what anyone would compare against.
	Fingerprint string `json:"fingerprint"`
	Comment     string `json:"comment,omitempty"`
}

// Password field shapes, from the shadow entry's first character. Four
// rather than a can-log-in boolean, because they are four different
// situations:
//
// PwSet is an ordinary account. PwLocked is one somebody disabled with
// passwd -l, and the hash is still there to be restored -- on a human's
// account that is a record of an offboarding. PwDisabled is the "*" that
// every packaged system account ships with and nobody ever set. And
// PwEmpty is an account with NO password that can still be logged into,
// which is not a quieter version of disabled: it is the one value here
// that is a finding.
const (
	PwSet      = "set"
	PwLocked   = "locked"
	PwDisabled = "disabled"
	PwEmpty    = "empty"
)

// Routes to root. Listed separately rather than collapsed to a boolean
// because they are removed in different places: a sudoers rule is edited
// in /etc/sudoers, a group membership with gpasswd, and uid 0 by deleting
// the account. "This user is privileged" without saying how sends an
// operator looking in the wrong file.
const (
	// PrivRoot is uid 0. Any account with it IS root, whatever it is
	// called -- a second uid-0 entry is the oldest backdoor there is.
	PrivRoot = "root"
	// PrivSudo means a sudoers rule names this account, directly or
	// through a group or a User_Alias. It does NOT mean the rule grants
	// full root: the rule text is reported alongside so a human can read
	// what it actually allows.
	PrivSudo = "sudo"
	// PrivDockerGroup is membership of docker / lxd. Root by a route that
	// does not appear anywhere in sudoers: a member can start a container
	// with the host filesystem mounted and write to any file on it.
	PrivDockerGroup = "docker-group"
	// PrivDiskGroup is membership of disk / raw device groups: read and
	// write to the block devices under every filesystem's permissions.
	PrivDiskGroup = "disk-group"
)
