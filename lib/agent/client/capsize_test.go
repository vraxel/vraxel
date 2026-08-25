package client

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	agenttypes "vraxel.io/vraxel/lib/agent/types"
)

type capLog struct{ lines []string }

func (l *capLog) Infof(f string, a ...any) { l.lines = append(l.lines, fmt.Sprintf(f, a...)) }
func (l *capLog) Warnf(f string, a ...any) { l.lines = append(l.lines, fmt.Sprintf(f, a...)) }

// A list that already fits is returned untouched and says nothing: this
// runs on every sample, and a line per sample would be noise on a fleet.
func TestCapBySizeLeavesAFittingListAlone(t *testing.T) {
	log := &capLog{}
	items := []agenttypes.UserGroup{{Name: "sudo", GID: 27, Members: []string{"zly"}}}
	got := capBySize(items, 8*1024, log, "groups")
	if len(got) != 1 {
		t.Errorf("trimmed a fitting list to %d", len(got))
	}
	if len(log.lines) != 0 {
		t.Errorf("logged for a list that fits: %q", log.lines)
	}
}

// The case the count cap cannot catch: few entries, each enormous. A jump
// host's accounts each carry several authorized keys, so the frame limit
// is reached at an entry count nowhere near 128.
func TestCapBySizeTrimsFewLargeEntries(t *testing.T) {
	log := &capLog{}
	var users []agenttypes.Account
	for i := range 40 {
		u := agenttypes.Account{Name: fmt.Sprintf("user%02d", i), UID: int64(1000 + i)}
		for k := range 8 {
			u.SSHKeys = append(u.SSHKeys, agenttypes.SSHKey{
				Type:        "ssh-ed25519",
				Fingerprint: "SHA256:" + strings.Repeat("x", 43),
				Comment:     fmt.Sprintf("user%02d@laptop-%d", i, k),
			})
		}
		users = append(users, u)
	}
	// Well under the count cap, and far over any sane byte budget.
	if len(users) > maxFactsListEntries {
		t.Fatalf("fixture defeats its own purpose: %d entries", len(users))
	}
	const budget = 8 * 1024
	got := capBySize(users, budget, log, "accounts")

	if len(got) == 0 || len(got) >= len(users) {
		t.Fatalf("trimmed to %d of %d; want a nonempty prefix", len(got), len(users))
	}
	b, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) > budget {
		t.Errorf("result is %d bytes, over the %d budget", len(b), budget)
	}
	// Truncation must be visible. Silent capping reads as completeness,
	// which is the failure this whole mechanism exists to avoid.
	if len(log.lines) == 0 || !strings.Contains(log.lines[0], "accounts") {
		t.Errorf("truncation not logged: %q", log.lines)
	}
}

// The three account lists share one frame, so their budgets must leave
// room for the envelope inside MaxFrameBytes. Asserted rather than
// assumed: raising one of them later is exactly the change that would
// quietly push a real host's report over the limit and make it report
// nothing at all.
func TestAccountBudgetsFitOneFrame(t *testing.T) {
	total := accountsUsersBudget + accountsGroupsBudget + accountsRulesBudget
	if total > agenttypes.MaxFrameBytes {
		t.Fatalf("account budgets total %d, over the %d frame limit", total, agenttypes.MaxFrameBytes)
	}
	// Headroom for the JSON envelope around the lists.
	if headroom := agenttypes.MaxFrameBytes - total; headroom < 8*1024 {
		t.Errorf("only %d bytes left for the frame envelope", headroom)
	}
	if inventoryBudget > agenttypes.MaxFrameBytes {
		t.Errorf("inventoryBudget %d exceeds the frame limit", inventoryBudget)
	}
}

// A budget so small nothing fits must not loop forever, and must not
// return a list it knows is oversize.
func TestCapBySizeTerminatesOnAnImpossibleBudget(t *testing.T) {
	log := &capLog{}
	items := []agenttypes.UserGroup{{Name: strings.Repeat("g", 500)}, {Name: "b"}}
	got := capBySize(items, 4, log, "groups")
	if len(got) != 0 {
		t.Errorf("got %d entries under a 4-byte budget, want none", len(got))
	}
}
