package agentgw

import (
	"strings"
	"testing"
)

var testMasterKey = []byte("0123456789abcdef0123456789abcdef")

func TestTokenRoundTrip(t *testing.T) {
	s := NewTokenSigner(testMasterKey)
	want := AgentClaims{AgentID: "6f1d4b2e-9c3a-5f77-8d21-0a5e6b74c910", HostID: 42, TokenVersion: 3, IssuedAtUnix: 1770000000}

	tok, err := s.Issue(want)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if !strings.HasPrefix(tok, AgentTokenPrefix+".") {
		t.Fatalf("token %q lacks the %q prefix", tok, AgentTokenPrefix)
	}
	got, err := s.Parse(tok)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if *got != want {
		t.Fatalf("claims round-trip: got %+v want %+v", *got, want)
	}
}

func TestTokenRejectsForeignKey(t *testing.T) {
	issuer := NewTokenSigner(testMasterKey)
	other := NewTokenSigner([]byte("ffffffffffffffffffffffffffffffff"))

	tok, err := issuer.Issue(AgentClaims{AgentID: "a", HostID: 1, TokenVersion: 1})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if _, err := other.Parse(tok); err == nil {
		t.Fatal("a token signed under a different master key must not verify")
	}
}

func TestTokenRejectsTampering(t *testing.T) {
	s := NewTokenSigner(testMasterKey)
	tok, err := s.Issue(AgentClaims{AgentID: "agent-1", HostID: 7, TokenVersion: 1})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	parts := strings.Split(tok, ".")

	cases := map[string]string{
		// A forged payload with a valid-looking body: this is the attack
		// the signature exists to stop -- claiming another host's id.
		"payload swapped": AgentTokenPrefix + ".eyJhaWQiOiJhZ2VudC0xIiwiaGlkIjo5OTksInZlciI6MX0." + parts[2],
		"signature cut":   parts[0] + "." + parts[1] + "." + parts[2][:len(parts[2])-4],
		"prefix changed":  AgentTokenPrefix + "X." + parts[1] + "." + parts[2],
		"not a token":     "garbage",
		"empty":           "",
	}
	for name, bad := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := s.Parse(bad); err == nil {
				t.Fatalf("Parse(%q) succeeded, want rejection", bad)
			}
		})
	}
}

func TestTokenRejectsEmptyIdentity(t *testing.T) {
	s := NewTokenSigner(testMasterKey)
	// Correctly signed but semantically useless: a token with no agent id
	// or no host id would authorise a channel the gateway cannot bind to
	// any row, so Parse must reject it rather than leaving the handler to
	// notice.
	for _, claims := range []AgentClaims{
		{AgentID: "", HostID: 1, TokenVersion: 1},
		{AgentID: "agent-1", HostID: 0, TokenVersion: 1},
		{AgentID: "agent-1", HostID: -5, TokenVersion: 1},
	} {
		tok, err := s.Issue(claims)
		if err != nil {
			t.Fatalf("Issue(%+v): %v", claims, err)
		}
		if _, err := s.Parse(tok); err == nil {
			t.Fatalf("Parse accepted claims %+v", claims)
		}
	}
}

func TestAgentIDForMachineIsStableAndDistinct(t *testing.T) {
	// Stability is what makes re-running install-agent.sh idempotent: the
	// second registration must resolve to the same agent id, hit the
	// ON CONFLICT (agent_id) branch, and rebind the existing host row.
	const machine = "3f1a2b4c5d6e7f8091a2b3c4d5e6f708"
	first := AgentIDForMachine(machine)
	if first != AgentIDForMachine(machine) {
		t.Fatal("AgentIDForMachine is not deterministic")
	}
	if first == AgentIDForMachine(machine+"0") {
		t.Fatal("two different machine ids produced the same agent id")
	}
	if len(first) != len("6f1d4b2e-9c3a-5f77-8d21-0a5e6b74c910") {
		t.Fatalf("agent id %q is not uuid-shaped; the DB column is uuid", first)
	}
}

func TestNameSuffixForAgentIsStable(t *testing.T) {
	const agentID = "6f1d4b2e-9c3a-5f77-8d21-0a5e6b74c910"
	six := NameSuffixForAgent(agentID, 6)
	if len(six) != 6 {
		t.Fatalf("NameSuffixForAgent(_, 6) = %q, want 6 chars", six)
	}
	if six != NameSuffixForAgent(agentID, 6) {
		t.Fatal("suffix is not deterministic; a re-registration would pick a new name each time")
	}
	if twelve := NameSuffixForAgent(agentID, 12); !strings.HasPrefix(twelve, six) {
		t.Fatalf("longer suffix %q must extend the shorter one %q", twelve, six)
	}
	if NameSuffixForAgent(agentID, 6) == NameSuffixForAgent("other-agent", 6) {
		t.Fatal("two agents produced the same 6-char suffix")
	}
}

func TestHashTokenIsStableAndDistinct(t *testing.T) {
	a, err := GenerateJoinToken()
	if err != nil {
		t.Fatalf("GenerateJoinToken: %v", err)
	}
	b, err := GenerateJoinToken()
	if err != nil {
		t.Fatalf("GenerateJoinToken: %v", err)
	}
	if a == b {
		t.Fatal("two generated join tokens collided")
	}
	if string(HashToken(a)) != string(HashToken(a)) {
		t.Fatal("HashToken is not deterministic")
	}
	if string(HashToken(a)) == string(HashToken(b)) {
		t.Fatal("distinct tokens hashed to the same value")
	}
	if len(HashToken(a)) != 32 {
		t.Fatalf("HashToken returned %d bytes, want 32 (sha256)", len(HashToken(a)))
	}
}
