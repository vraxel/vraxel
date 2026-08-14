package agentgw

import (
	"context"
	"encoding/hex"
	"regexp"
	"strings"
	"time"

	agenttypes "vraxel.io/vraxel/lib/agent/types"
	"vraxel.io/vraxel/lib/logger"
	gwstore "vraxel.io/vraxel/pkg/apis/agentgw/store"
)

// --- machine identity ---
//
// Which machine is this? The question has one honest answer and several
// tempting wrong ones.
//
// The wrong one this code replaced: derive an id from /etc/machine-id.
// That file lives on the disk, so a clone answers with its parent's
// identity and the two machines proceed to share one host row, each
// overwriting the other's facts and stealing the other's channel.
//
// The signals split by where they live, not by how much we like them:
//
//	A class  product_uuid, MACs -- assigned from outside the image. A
//	         hypervisor re-issues them when it copies a VM.
//	B class  machine-id -- carried inside the image. A copy inherits it.
//
// So A class answers "which machine", B class answers "which image". A
// weighted blend of the two is strictly worse than either: B-class
// signals are all just restatements of "the disk", so letting them vote
// lets a clone outvote the hypervisor.
//
// Everything here is evidence the machine reports about itself, and a
// root user on that machine can report whatever they like. This defends
// against cloning accidents, which are routine, not against a hostile
// root, which would need hardware attestation.

// maxFingerprintMACs bounds what a peer can make us store. A machine with
// more than this many physical NICs exists, but the tail beyond it adds
// nothing: MACs corroborate, they never claim.
const maxFingerprintMACs = 8

// coexistenceSlack is how much a boot_at / last_seen_at overlap must
// exceed before it counts as proof that two machines ran at once.
//
// boot_at is derived from an uptime counter that ticks in whole seconds
// and travels over the network, and last_seen_at lands up to a heartbeat
// late. The slack keeps that jitter from manufacturing an overlap; it
// costs nothing, because a real clone overlaps its parent by however long
// the parent kept running, never by a second and a half.
const coexistenceSlack = 30 * time.Second

// templateGroupSize is the number of machines sharing one machine-id that
// proves the image is a template.
//
// Two is ambiguous: one machine whose motherboard was replaced looks
// exactly like one machine that was cloned once. Three is not, because a
// machine cannot be swapped into two machines that then coexist. The
// third arrival therefore settles the second one's question too.
const templateGroupSize = 3

var uuidShape = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// junkProductUUIDs mirrors the agent-side list. Kept on both sides
// deliberately: the agent filters so it never reports one, and the server
// filters because it must survive an agent that does anyway -- an older
// build, or a machine whose firmware the agent's list did not yet know.
// Admitting one would merge an entire hardware batch into a single host.
var junkProductUUIDs = map[string]bool{
	"00000000-0000-0000-0000-000000000000": true,
	"ffffffff-ffff-ffff-ffff-ffffffffffff": true,
	"03000200-0400-0500-0006-000700080009": true,
}

// Fingerprint is the normalised evidence one machine presents.
type Fingerprint struct {
	// ProductUUID is "" when the machine has none worth trusting. That is
	// a normal state (containers, many ARM boards, whitebox firmware), and
	// it means this machine can never claim an existing row automatically.
	ProductUUID string
	MachineID   string
	MACs        []string
	// BootAt is when this machine booted, by OUR clock. Zero when the
	// agent reported no uptime.
	BootAt time.Time
}

// NewFingerprint normalises what a peer sent. now is the server's clock,
// which is what dates the boot -- see hostinfo.uptimeSeconds for why the
// machine's own clock is not usable for this.
func NewFingerprint(machineID string, fp agenttypes.MachineFingerprint, now time.Time) Fingerprint {
	// The fingerprint's own copy wins; machineID is the top-level
	// register field, which is where an agent older than this struct puts
	// it and the only place the control channel never had it.
	if fp.MachineID != "" {
		machineID = fp.MachineID
	}
	out := Fingerprint{
		MachineID:   clampIdentity(machineID),
		ProductUUID: normaliseProductUUID(fp.ProductUUID),
	}
	for _, m := range fp.MACs {
		if m = clampIdentity(strings.ToLower(m)); m != "" {
			out.MACs = append(out.MACs, m)
		}
		if len(out.MACs) == maxFingerprintMACs {
			break
		}
	}
	// A negative or absurd uptime is a broken agent, not a machine that
	// booted before the epoch; dropping it costs one piece of evidence,
	// while trusting it would date the boot somewhere that makes every
	// coexistence test lie.
	if fp.UptimeSeconds > 0 && fp.UptimeSeconds < 100*365*24*3600 {
		out.BootAt = now.Add(-time.Duration(fp.UptimeSeconds) * time.Second)
	}
	return out
}

// normaliseProductUUID lowercases, shape-checks and rejects firmware
// defaults. Anything that does not survive comes back "", which the rest
// of the system reads as "no A-class signal" rather than as a value.
func normaliseProductUUID(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	if !uuidShape.MatchString(v) || junkProductUUIDs[v] {
		return ""
	}
	return v
}

// clampIdentity bounds a reported identity string to what the columns
// hold. Truncation cannot create a collision that matters: two machines
// whose ids agree for 64 characters are already indistinguishable to us.
func clampIdentity(v string) string {
	v = strings.TrimSpace(v)
	if len(v) > 64 {
		return v[:64]
	}
	return v
}

// sameProductUUID reports whether two SMBIOS UUIDs name the same machine.
//
// Not string equality, because one machine can legitimately present two
// spellings of its own UUID. SMBIOS 2.6 declared the first three fields
// little-endian, and whether a given kernel/firmware pair honours that
// decides which way round they come out -- so a firmware update, a kernel
// upgrade or a move between hypervisor versions can flip the bytes under
// a machine that has not changed at all. Measured on the VM this was
// developed against, whose DMI reports both spellings at once:
//
//	product_uuid    8c1e4d56-f7c1-65c9-b323-12015fb893e2
//	product_serial  VMware-56 4d 1e 8c c1 f7 c9 65-...
//
// Treating that flip as a different machine would split hosts en masse on
// an unrelated upgrade, which is exactly the failure class this whole
// change exists to remove.
func sameProductUUID(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return a == b || a == swapUUIDByteOrder(b)
}

// swapUUIDByteOrder reverses the byte order of the first three UUID
// fields, converting between the two SMBIOS spellings. Returns the input
// unchanged if it is not a well-formed UUID.
func swapUUIDByteOrder(v string) string {
	parts := strings.Split(v, "-")
	if len(parts) != 5 {
		return v
	}
	for i := range 3 {
		b, err := hex.DecodeString(parts[i])
		if err != nil {
			return v
		}
		for l, r := 0, len(b)-1; l < r; l, r = l+1, r-1 {
			b[l], b[r] = b[r], b[l]
		}
		parts[i] = hex.EncodeToString(b)
	}
	return strings.Join(parts, "-")
}

// --- matching ---

// MatchOutcome is what the fingerprint says about a machine presenting a
// join token.
type MatchOutcome struct {
	// AgentID is the row to rebind, or "" to create a new host.
	AgentID string
	// HostID accompanies AgentID; 0 when creating.
	HostID int64
	// Source is what to record as this row's identity_source.
	Source string
	// ClaimUUIDs is handed to the store as its under-lock re-check. Non
	// empty only when claiming such a row would be correct.
	ClaimUUIDs []string
	// Why is the operator- and log-facing reason, in English, for why
	// this machine did or did not join an existing row.
	Why string
}

// MatchMachine decides which host row a registering machine belongs to.
//
// It claims on A-class evidence only. B-class agreement (same
// /etc/machine-id) never claims, because it cannot distinguish "the same
// machine" from "a copy of the same disk" -- and getting that wrong is
// how two machines end up sharing one host row, each overwriting the
// other's facts while jobs land on whichever holds the channel. Being
// wrong the other way costs a duplicate row an operator can merge.
//
// candidates are the rows already claiming this machine's SMBIOS UUID in
// either spelling; aliveCutoff is the instant before which a row's
// last_seen_at counts as stale.
func MatchMachine(fp Fingerprint, candidates []gwstore.AgentRow, aliveCutoff time.Time) MatchOutcome {
	if fp.ProductUUID == "" {
		return MatchOutcome{
			Source: gwstore.IdentitySourceNone,
			Why:    "machine reports no usable SMBIOS UUID; it cannot be recognised automatically",
		}
	}
	claim := []string{fp.ProductUUID, swapUUIDByteOrder(fp.ProductUUID)}

	matches := make([]gwstore.AgentRow, 0, 1)
	for _, c := range candidates {
		if sameProductUUID(fp.ProductUUID, c.ProductUUID) {
			matches = append(matches, c)
		}
	}

	switch len(matches) {
	case 0:
		return MatchOutcome{
			Source:     gwstore.IdentitySourceProductUUID,
			ClaimUUIDs: claim,
			Why:        "no host claims this machine's SMBIOS UUID",
		}
	case 1:
		// fall through
	default:
		// Several hosts already share this UUID, so it names a production
		// run rather than a machine -- whitebox firmware does this. Claiming
		// one of them would fold the whole batch into a single host.
		return MatchOutcome{
			Source: gwstore.IdentitySourceNone,
			Why:    "this SMBIOS UUID is already claimed by several hosts, so it identifies a hardware batch rather than a machine",
		}
	}

	prev := matches[0]
	// The candidate is held by a machine with a live channel, yet this
	// caller reports a different disk image. Two things are up holding one
	// hardware id, so the id is not distinguishing them -- the
	// batch-firmware case, seen one machine at a time.
	//
	// Liveness is the channel, NOT a recent last_seen_at. A machine
	// re-registering has just stopped its own agent (install-agent.sh does
	// that so the server will accept the registration), so its heartbeat
	// is seconds old while nothing is connected. Reading that as "another
	// machine is live" split a host in testing: reset /etc/machine-id and
	// re-onboard -- the exact remedy printed for a cloned host -- came
	// back as a second row instead of reclaiming the first.
	if prev.MachineID != "" && fp.MachineID != "" && prev.MachineID != fp.MachineID &&
		prev.Status == agentStatusOnline && prev.LastSeenAt != nil && prev.LastSeenAt.After(aliveCutoff) {
		return MatchOutcome{
			Source: gwstore.IdentitySourceNone,
			Why:    "another machine is online with the same SMBIOS UUID but a different /etc/machine-id, so the UUID is not unique here",
		}
	}

	return MatchOutcome{
		AgentID:    prev.AgentID,
		HostID:     prev.HostID,
		Source:     gwstore.IdentitySourceProductUUID,
		ClaimUUIDs: claim,
		Why:        "same SMBIOS UUID as this host",
	}
}

// NameSeed is the per-machine constant used to disambiguate host names
// and to label this machine in logs. Strongest available signal first.
func (f Fingerprint) NameSeed() string {
	switch {
	case f.ProductUUID != "":
		return f.ProductUUID
	default:
		return f.MachineID
	}
}

// ToStore is the persisted form, tagged with the source that claimed it.
func (f Fingerprint) ToStore(source string) gwstore.FingerprintInput {
	out := gwstore.FingerprintInput{
		ProductUUID: f.ProductUUID,
		MachineID:   f.MachineID,
		MACs:        f.MACs,
		Source:      source,
	}
	// Postgres takes NULL for "this machine did not say", which the
	// coexistence test reads as "no evidence" rather than as the epoch.
	if !f.BootAt.IsZero() {
		at := f.BootAt
		out.BootAt = &at
	}
	if out.MACs == nil {
		out.MACs = []string{}
	}
	return out
}

// findClaimCandidates fetches the rows that could be this machine.
func (h *protocolHandler) findClaimCandidates(ctx context.Context, fp Fingerprint) ([]gwstore.AgentRow, error) {
	if fp.ProductUUID == "" {
		return nil, nil
	}
	// Both spellings, because the row may have been written under the
	// other byte order (sameProductUUID).
	return h.agents.FindByProductUUID(ctx, []string{fp.ProductUUID, swapUUIDByteOrder(fp.ProductUUID)})
}

// --- reconnect verification ---

// MachineVerdict is what a reconnecting agent's fingerprint says about
// the credential it presented.
type MachineVerdict int

const (
	// VerdictSameMachine: the fingerprint matches the row.
	VerdictSameMachine MachineVerdict = iota
	// VerdictMachineIDReset: same hardware, new /etc/machine-id. This is
	// the fix we tell the operator of a cloned host to apply, so it must
	// be admitted -- refusing it would make our own remedy a dead end.
	VerdictMachineIDReset
	// VerdictForeignMachine: the credential is valid but the machine
	// holding it is not the one it was issued to. A copied agent.json,
	// or a disk moved into different hardware.
	VerdictForeignMachine
	// VerdictUnverifiable: not enough evidence on one side or the other.
	// Admitted, because the alternative is locking out every agent that
	// predates fingerprinting and every machine without DMI.
	VerdictUnverifiable
)

// VerifyMachine compares a reconnecting machine against the row its
// credential names.
//
// A valid credential is no longer sufficient on its own. It proves the
// bearer once completed a registration; it cannot prove the bearer is
// still the same machine, because everything it consists of sits in a
// file that a disk image copies. The fingerprint is the half that a copy
// cannot bring with it, so the two together answer a question neither
// answers alone -- and the answer is deterministic, unlike the old
// last-writer-wins race between two clones.
func VerifyMachine(row *gwstore.AgentRow, fp Fingerprint) MachineVerdict {
	if row.ProductUUID == "" || fp.ProductUUID == "" {
		return VerdictUnverifiable
	}
	if !sameProductUUID(fp.ProductUUID, row.ProductUUID) {
		return VerdictForeignMachine
	}
	if fp.MachineID != "" && row.MachineID != "" && fp.MachineID != row.MachineID {
		return VerdictMachineIDReset
	}
	return VerdictSameMachine
}

// --- image-group findings ---

// ImageRelation classifies a host against the other hosts built from the
// same disk image.
type ImageRelation int

const (
	// RelationNone: nothing else came from this image.
	RelationNone ImageRelation = iota
	// RelationClone: proven a different machine from the one it shares an
	// image with -- they ran at the same time, so no hardware change can
	// explain it.
	RelationClone
	// RelationTemplate: three or more machines share the image, which one
	// machine's history cannot produce.
	RelationTemplate
	// RelationUnresolved: one other host shares this image, they were
	// never seen together, and both stories fit -- a machine whose
	// hardware was replaced, or a clone of a machine that had already
	// been shut down. Only an operator knows which, so this is the one
	// state that asks.
	RelationUnresolved
)

// ClassifyImageGroup decides what to tell an operator about a host that
// shares its /etc/machine-id with others.
//
// This is the part of clone handling that keeps the common case out of
// anyone's lap. Sharing an image is normal and, on its own, ambiguous:
// a replaced motherboard and a fresh clone leave identical evidence. Time
// is what separates them -- two machines that were up at once are two
// machines, whatever their disks say -- so most of the fleet resolves
// itself and only a genuine one-in one-out swap ever reaches a human.
//
// group is every row sharing the machine-id, self included.
func ClassifyImageGroup(self *gwstore.AgentRow, group []gwstore.AgentRow, aliveCutoff time.Time) (ImageRelation, []gwstore.AgentRow) {
	others := make([]gwstore.AgentRow, 0, len(group))
	for _, r := range group {
		if r.HostID != self.HostID {
			others = append(others, r)
		}
	}
	if len(others) == 0 {
		return RelationNone, nil
	}
	// A machine cannot be swapped into two machines that then coexist, so
	// the third member settles the second one's question too.
	if len(group) >= templateGroupSize {
		return RelationTemplate, others
	}
	for _, other := range others {
		if coexisted(self, &other, aliveCutoff) {
			return RelationClone, others
		}
	}
	return RelationUnresolved, others
}

// coexisted reports whether two machines were provably up at the same
// time, which no hardware swap can produce.
func coexisted(self, other *gwstore.AgentRow, aliveCutoff time.Time) bool {
	// The other machine is still heartbeating. Whatever this one is, it is
	// not that machine with new hardware -- that machine is running.
	if other.LastSeenAt != nil && other.LastSeenAt.After(aliveCutoff) {
		return true
	}
	// This machine had already booted when the other was last heard from.
	if self.BootAt != nil && other.LastSeenAt != nil {
		return self.BootAt.Add(coexistenceSlack).Before(*other.LastSeenAt)
	}
	return false
}

// reportImageGroup logs what this machine's disk image says about it.
//
// Best-effort and after the fact: the registration has already succeeded,
// and a machine is never refused for sharing an image. Two hosts built
// from one template is a normal thing to do; the failure it can hide is
// an operator who thinks they are looking at one machine when they are
// looking at two, so it is worth saying out loud even before the host
// list learns to draw it.
func (h *protocolHandler) reportImageGroup(ctx context.Context, self *gwstore.AgentRow, now time.Time) {
	if self.MachineID == "" {
		return
	}
	group, err := h.agents.FindByMachineID(ctx, self.MachineID)
	if err != nil {
		logger.Warnf("agentgw register: look up image group for host %d: %v", self.HostID, err)
		return
	}
	relation, others := ClassifyImageGroup(self, group, now.Add(-agentStaleAfter))
	if relation == RelationNone {
		return
	}
	peers := make([]int64, 0, len(others))
	for _, o := range others {
		peers = append(peers, o.HostID)
	}
	switch relation {
	case RelationTemplate:
		logger.Warnf("agentgw register: host %d shares /etc/machine-id with hosts %v -- %d machines from one image. "+
			"The template still carries a machine id; run systemd-machine-id-setup in it so future clones arrive distinct.",
			self.HostID, peers, len(group))
	case RelationClone:
		logger.Warnf("agentgw register: host %d is a clone of hosts %v: same disk image, and they were up at the same time. "+
			"Distinct hosts, but reset /etc/machine-id on the copies to keep them apart everywhere else.",
			self.HostID, peers)
	case RelationUnresolved:
		logger.Infof("agentgw register: host %d shares its disk image with hosts %v and they were never seen together. "+
			"Either a clone of a machine already shut down, or that machine with replaced hardware -- merge them if it is the latter.",
			self.HostID, peers)
	}
}
