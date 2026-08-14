package agentgw

import (
	"testing"
	"time"

	agenttypes "vraxel.io/vraxel/lib/agent/types"
	gwstore "vraxel.io/vraxel/pkg/apis/agentgw/store"
)

// The two machines this whole change came from: VMware clones whose
// machine-id and root filesystem are byte-identical and whose
// hypervisor-assigned identity is not.
const (
	sharedMachineID = "af96255a706d48428f2b20f494a89742"
	uuidNode12      = "8c1e4d56-f7c1-65c9-b323-12015fb893e2"
	uuidNode15      = "dda44d56-d328-87d9-8f09-6c2406b71177"
)

func fp(productUUID, machineID string) Fingerprint {
	return Fingerprint{ProductUUID: productUUID, MachineID: machineID}
}

func row(hostID int64, agentID, productUUID, machineID string, lastSeen *time.Time) gwstore.AgentRow {
	return gwstore.AgentRow{
		HostID: hostID, AgentID: agentID,
		ProductUUID: productUUID, MachineID: machineID,
		IdentitySource: gwstore.IdentitySourceProductUUID,
		LastSeenAt:     lastSeen,
	}
}

// The regression that motivated all of this: the second clone must not
// land on the first one's host row, however much of the disk they share.
func TestMatchMachineRefusesCloneWithIdenticalMachineID(t *testing.T) {
	seen := time.Now()
	existing := []gwstore.AgentRow{row(1, "agent-12", uuidNode12, sharedMachineID, &seen)}

	got := MatchMachine(fp(uuidNode15, sharedMachineID), existing, time.Now().Add(-time.Minute))

	if got.AgentID != "" || got.HostID != 0 {
		t.Fatalf("clone claimed host %d via agent %q; it must get its own row", got.HostID, got.AgentID)
	}
}

func TestMatchMachineClaimsSameMachine(t *testing.T) {
	seen := time.Now().Add(-time.Hour)
	existing := []gwstore.AgentRow{row(7, "agent-12", uuidNode12, sharedMachineID, &seen)}

	got := MatchMachine(fp(uuidNode12, sharedMachineID), existing, time.Now().Add(-time.Minute))

	if got.HostID != 7 || got.AgentID != "agent-12" {
		t.Fatalf("same machine did not reclaim its row: %+v", got)
	}
}

// A machine reinstalled from scratch keeps its hardware identity and
// arrives with a brand-new machine-id. It must land back on its own host
// rather than splitting off a second one -- the case the machine-id
// derivation used to get wrong in the other direction.
func TestMatchMachineClaimsAfterOSReinstall(t *testing.T) {
	seen := time.Now().Add(-24 * time.Hour)
	existing := []gwstore.AgentRow{row(7, "agent-12", uuidNode12, sharedMachineID, &seen)}

	got := MatchMachine(fp(uuidNode12, "0d5b3f0e5f2b4a2f8b7c1d9e6a3f0c11"), existing, time.Now().Add(-time.Minute))

	if got.HostID != 7 {
		t.Fatalf("reinstalled machine did not reclaim host 7: %+v", got)
	}
}

// Same SMBIOS UUID, different image, and the incumbent is still
// heartbeating: two live machines share this UUID, so it identifies
// firmware rather than a machine and must not claim anything.
func TestMatchMachineRefusesUUIDSharedByLiveMachines(t *testing.T) {
	seen := time.Now()
	live := row(1, "agent-a", uuidNode12, sharedMachineID, &seen)
	live.Status = agentStatusOnline
	existing := []gwstore.AgentRow{live}

	got := MatchMachine(fp(uuidNode12, "9f8e7d6c5b4a39281706f5e4d3c2b1a0"), existing, time.Now().Add(-time.Minute))

	if got.AgentID != "" {
		t.Fatalf("claimed a row whose UUID two live machines share: %+v", got)
	}
	if got.Source != gwstore.IdentitySourceNone {
		t.Fatalf("source = %q, want %q", got.Source, gwstore.IdentitySourceNone)
	}
}

// Found on real hardware: a machine that reset /etc/machine-id and
// re-onboarded -- the remedy printed for a cloned host -- came back as a
// SECOND host. install-agent.sh stops the agent before registering, so
// the row is offline with a heartbeat seconds old; reading that as
// "another machine is live with this UUID" refused the match.
func TestMatchMachineClaimsAfterMachineIDResetWithFreshHeartbeat(t *testing.T) {
	justStopped := time.Now().Add(-2 * time.Second)
	prev := row(2, "agent-15", uuidNode15, sharedMachineID, &justStopped)
	prev.Status = "offline"

	got := MatchMachine(fp(uuidNode15, "0dd778917e924eb4848e1a411d22a1d1"),
		[]gwstore.AgentRow{prev}, time.Now().Add(-time.Minute))

	if got.HostID != 2 {
		t.Fatalf("de-cloned machine did not reclaim host 2 (got %+v); our own remedy would duplicate the host", got)
	}
}

func TestMatchMachineRefusesUUIDClaimedBySeveralHosts(t *testing.T) {
	existing := []gwstore.AgentRow{
		row(1, "agent-a", uuidNode12, "aaaa", nil),
		row(2, "agent-b", uuidNode12, "bbbb", nil),
	}

	got := MatchMachine(fp(uuidNode12, "cccc"), existing, time.Now().Add(-time.Minute))

	if got.AgentID != "" {
		t.Fatalf("claimed one of several hosts sharing a batch UUID: %+v", got)
	}
}

// No usable hardware identity: a new row every time, never a claim on the
// disk image alone.
func TestMatchMachineWithoutProductUUIDNeverClaims(t *testing.T) {
	seen := time.Now().Add(-time.Hour)
	existing := []gwstore.AgentRow{row(1, "agent-a", "", sharedMachineID, &seen)}

	got := MatchMachine(fp("", sharedMachineID), existing, time.Now().Add(-time.Minute))

	if got.AgentID != "" || got.Source != gwstore.IdentitySourceNone {
		t.Fatalf("claimed on machine-id alone: %+v", got)
	}
	if len(got.ClaimUUIDs) != 0 {
		t.Fatalf("offered claim uuids with no A-class signal: %+v", got.ClaimUUIDs)
	}
}

// One machine, two spellings of its own UUID. A kernel or firmware
// upgrade can flip the byte order under a machine that has not changed,
// and treating that as a new machine would split hosts fleet-wide.
func TestMatchMachineToleratesSMBIOSByteOrderFlip(t *testing.T) {
	seen := time.Now().Add(-time.Hour)
	stored := swapUUIDByteOrder(uuidNode12)
	if stored == uuidNode12 {
		t.Fatal("byte-order swap produced the same string; the fixture is wrong")
	}
	existing := []gwstore.AgentRow{row(7, "agent-12", stored, sharedMachineID, &seen)}

	got := MatchMachine(fp(uuidNode12, sharedMachineID), existing, time.Now().Add(-time.Minute))

	if got.HostID != 7 {
		t.Fatalf("byte-order flip split the host: %+v", got)
	}
}

func TestNormaliseProductUUIDRejectsFirmwareDefaults(t *testing.T) {
	for _, v := range []string{
		"",
		"not-a-uuid",
		"00000000-0000-0000-0000-000000000000",
		"FFFFFFFF-FFFF-FFFF-FFFF-FFFFFFFFFFFF",
		"03000200-0400-0500-0006-000700080009",
	} {
		if got := normaliseProductUUID(v); got != "" {
			t.Errorf("normaliseProductUUID(%q) = %q, want empty", v, got)
		}
	}
	if got := normaliseProductUUID("  " + uuidNode12 + "  "); got != uuidNode12 {
		t.Errorf("normaliseProductUUID trimmed badly: %q", got)
	}
}

// A copied agent.json carries a valid credential; what it cannot carry is
// the machine. The original wins deterministically, on the copy's very
// first connection, rather than by whoever reconnected last.
func TestVerifyMachineRefusesCopiedCredential(t *testing.T) {
	r := row(7, "agent-12", uuidNode12, sharedMachineID, nil)

	if got := VerifyMachine(&r, fp(uuidNode15, sharedMachineID)); got != VerdictForeignMachine {
		t.Fatalf("verdict = %v, want VerdictForeignMachine", got)
	}
}

// The remedy we print for a cloned host is "reset /etc/machine-id". If
// that produced a refusal, our own instructions would be a dead end.
func TestVerifyMachineAdmitsMachineIDReset(t *testing.T) {
	r := row(7, "agent-12", uuidNode12, sharedMachineID, nil)

	if got := VerifyMachine(&r, fp(uuidNode12, "0d5b3f0e5f2b4a2f8b7c1d9e6a3f0c11")); got != VerdictMachineIDReset {
		t.Fatalf("verdict = %v, want VerdictMachineIDReset", got)
	}
}

func TestVerifyMachineAdmitsWhenUnverifiable(t *testing.T) {
	noUUIDStored := row(7, "agent-12", "", sharedMachineID, nil)
	if got := VerifyMachine(&noUUIDStored, fp(uuidNode12, sharedMachineID)); got != VerdictUnverifiable {
		t.Fatalf("verdict = %v, want VerdictUnverifiable", got)
	}

	stored := row(7, "agent-12", uuidNode12, sharedMachineID, nil)
	if got := VerifyMachine(&stored, fp("", sharedMachineID)); got != VerdictUnverifiable {
		t.Fatalf("agent reporting no fingerprint must be admitted, got %v", got)
	}
}

func TestVerifyMachineAdmitsSameMachine(t *testing.T) {
	r := row(7, "agent-12", uuidNode12, sharedMachineID, nil)

	if got := VerifyMachine(&r, fp(uuidNode12, sharedMachineID)); got != VerdictSameMachine {
		t.Fatalf("verdict = %v, want VerdictSameMachine", got)
	}
}

// Boot time is dated by the server's clock: a fresh VM whose own clock is
// years out must not report a boot instant to match.
func TestNewFingerprintDatesBootByServerClock(t *testing.T) {
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)

	got := NewFingerprint("", agenttypes.MachineFingerprint{
		MachineID:     sharedMachineID,
		ProductUUID:   uuidNode12,
		UptimeSeconds: 3600,
	}, now)

	if want := now.Add(-time.Hour); !got.BootAt.Equal(want) {
		t.Fatalf("BootAt = %v, want %v", got.BootAt, want)
	}
	if got.MachineID != sharedMachineID {
		t.Fatalf("MachineID = %q", got.MachineID)
	}
}

func TestNewFingerprintDropsImpossibleUptime(t *testing.T) {
	now := time.Now()
	for _, secs := range []int64{0, -1, 200 * 365 * 24 * 3600} {
		got := NewFingerprint("", agenttypes.MachineFingerprint{UptimeSeconds: secs}, now)
		if !got.BootAt.IsZero() {
			t.Errorf("uptime %d produced BootAt %v, want zero", secs, got.BootAt)
		}
	}
}

func TestNewFingerprintBoundsMACs(t *testing.T) {
	many := make([]string, maxFingerprintMACs+5)
	for i := range many {
		many[i] = "00:0c:29:b8:93:e2"
	}

	got := NewFingerprint("", agenttypes.MachineFingerprint{MACs: many}, time.Now())

	if len(got.MACs) != maxFingerprintMACs {
		t.Fatalf("kept %d MACs, want %d", len(got.MACs), maxFingerprintMACs)
	}
}

func at(t time.Time) *time.Time { return &t }

// A clone of a machine that is still running: proven, no question asked.
func TestClassifyImageGroupProvesCloneOfLiveMachine(t *testing.T) {
	now := time.Now()
	cutoff := now.Add(-time.Minute)
	self := row(2, "agent-15", uuidNode15, sharedMachineID, at(now))
	self.BootAt = at(now.Add(-10 * time.Minute))
	parent := row(1, "agent-12", uuidNode12, sharedMachineID, at(now))

	got, others := ClassifyImageGroup(&self, []gwstore.AgentRow{self, parent}, cutoff)

	if got != RelationClone {
		t.Fatalf("relation = %v, want RelationClone", got)
	}
	if len(others) != 1 || others[0].HostID != 1 {
		t.Fatalf("others = %+v, want just host 1", others)
	}
}

// The parent is long offline, but this machine was already up while the
// parent was still reporting: they coexisted, so no hardware swap can
// explain the pair.
func TestClassifyImageGroupProvesCloneByCoexistence(t *testing.T) {
	now := time.Now()
	cutoff := now.Add(-time.Minute)
	self := row(2, "agent-15", uuidNode15, sharedMachineID, at(now))
	self.BootAt = at(now.Add(-48 * time.Hour))
	parent := row(1, "agent-12", uuidNode12, sharedMachineID, at(now.Add(-24*time.Hour)))

	if got, _ := ClassifyImageGroup(&self, []gwstore.AgentRow{self, parent}, cutoff); got != RelationClone {
		t.Fatalf("relation = %v, want RelationClone", got)
	}
}

// A powered-off template's first clone: no time evidence either way, and
// both stories fit. This is the only case that may ask an operator.
func TestClassifyImageGroupLeavesFirstCloneOfDeadTemplateUnresolved(t *testing.T) {
	now := time.Now()
	cutoff := now.Add(-time.Minute)
	self := row(2, "agent-15", uuidNode15, sharedMachineID, at(now))
	self.BootAt = at(now.Add(-10 * time.Minute))
	template := row(1, "agent-12", uuidNode12, sharedMachineID, at(now.Add(-72*time.Hour)))

	if got, _ := ClassifyImageGroup(&self, []gwstore.AgentRow{self, template}, cutoff); got != RelationUnresolved {
		t.Fatalf("relation = %v, want RelationUnresolved", got)
	}
}

// ...and the moment a second clone shows up, the question answers itself:
// one machine cannot become two machines that coexist.
func TestClassifyImageGroupResolvesOnceThreeShareAnImage(t *testing.T) {
	now := time.Now()
	cutoff := now.Add(-time.Minute)
	self := row(2, "agent-15", uuidNode15, sharedMachineID, at(now))
	self.BootAt = at(now.Add(-10 * time.Minute))
	template := row(1, "agent-12", uuidNode12, sharedMachineID, at(now.Add(-72*time.Hour)))
	third := row(3, "agent-16", "5cf24d56-1111-2222-3333-444455556666", sharedMachineID, at(now.Add(-71*time.Hour)))

	got, others := ClassifyImageGroup(&self, []gwstore.AgentRow{self, template, third}, cutoff)

	if got != RelationTemplate {
		t.Fatalf("relation = %v, want RelationTemplate", got)
	}
	if len(others) != 2 {
		t.Fatalf("others = %d, want 2", len(others))
	}
}

func TestClassifyImageGroupIgnoresLoneHost(t *testing.T) {
	self := row(1, "agent-12", uuidNode12, sharedMachineID, at(time.Now()))

	if got, others := ClassifyImageGroup(&self, []gwstore.AgentRow{self}, time.Now().Add(-time.Minute)); got != RelationNone || others != nil {
		t.Fatalf("relation = %v, others = %+v; want RelationNone and nil", got, others)
	}
}
