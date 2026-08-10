package oidc

import (
	"fmt"
	"testing"
)

// mockDBStore implements KeyStore using an in-memory map, simulating
// the load-or-generate pattern that DBKeyStore uses against PostgreSQL.
type mockDBStore struct {
	keys map[string]*KeySet
}

func newMockDBStore() *mockDBStore {
	return &mockDBStore{keys: make(map[string]*KeySet)}
}

func (m *mockDBStore) LoadOrGenerate(algorithm string) (*KeySet, error) {
	if ks, ok := m.keys[algorithm]; ok {
		return ks, nil
	}

	ks, err := GenerateKeySet(algorithm)
	if err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}

	// Simulate PEM round-trip (same as what DBKeyStore does via DB storage)
	privPEM, pubPEM, err := MarshalKeySetPEM(ks)
	if err != nil {
		return nil, fmt.Errorf("marshal key: %w", err)
	}

	parsed, err := ParseKeySetPEM(privPEM, pubPEM, algorithm)
	if err != nil {
		return nil, fmt.Errorf("parse key: %w", err)
	}

	m.keys[algorithm] = parsed
	return parsed, nil
}

// Compile-time interface check for DBKeyStore.
var _ KeyStore = (*DBKeyStore)(nil)

func TestMockKeyStore_FirstCallGenerates(t *testing.T) {
	store := newMockDBStore()

	ks, err := store.LoadOrGenerate(AlgEdDSA)
	if err != nil {
		t.Fatalf("LoadOrGenerate error: %v", err)
	}

	if ks == nil {
		t.Fatal("expected non-nil KeySet")
	}
	if ks.KeyID == "" {
		t.Error("KeyID is empty")
	}
	if ks.Algorithm != AlgEdDSA {
		t.Errorf("Algorithm = %q, want %q", ks.Algorithm, AlgEdDSA)
	}
	if ks.PrivateKey == nil {
		t.Error("PrivateKey is nil")
	}
	if ks.PublicKey == nil {
		t.Error("PublicKey is nil")
	}
}

func TestMockKeyStore_SecondCallLoads(t *testing.T) {
	store := newMockDBStore()

	ks1, err := store.LoadOrGenerate(AlgEdDSA)
	if err != nil {
		t.Fatalf("first LoadOrGenerate error: %v", err)
	}

	ks2, err := store.LoadOrGenerate(AlgEdDSA)
	if err != nil {
		t.Fatalf("second LoadOrGenerate error: %v", err)
	}

	if ks1.KeyID != ks2.KeyID {
		t.Errorf("KeyID mismatch: first=%q, second=%q", ks1.KeyID, ks2.KeyID)
	}
	if ks1 != ks2 {
		t.Error("expected same *KeySet pointer on second call")
	}
}
