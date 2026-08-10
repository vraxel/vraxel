package oidc

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"testing"

	"github.com/golang-jwt/jwt/v5"
)

func TestGenerateKeySet_EdDSA(t *testing.T) {
	ks, err := GenerateKeySet(AlgEdDSA)
	if err != nil {
		t.Fatalf("GenerateKeySet(EdDSA) error: %v", err)
	}
	if ks.Algorithm != AlgEdDSA {
		t.Errorf("Algorithm = %q, want %q", ks.Algorithm, AlgEdDSA)
	}
	if ks.KeyID == "" {
		t.Error("KeyID is empty")
	}
	if _, ok := ks.PrivateKey.(ed25519.PrivateKey); !ok {
		t.Errorf("PrivateKey type = %T, want ed25519.PrivateKey", ks.PrivateKey)
	}
	if _, ok := ks.PublicKey.(ed25519.PublicKey); !ok {
		t.Errorf("PublicKey type = %T, want ed25519.PublicKey", ks.PublicKey)
	}
}

func TestGenerateKeySet_ES256(t *testing.T) {
	ks, err := GenerateKeySet(AlgES256)
	if err != nil {
		t.Fatalf("GenerateKeySet(ES256) error: %v", err)
	}
	if ks.Algorithm != AlgES256 {
		t.Errorf("Algorithm = %q, want %q", ks.Algorithm, AlgES256)
	}
	if ks.KeyID == "" {
		t.Error("KeyID is empty")
	}
	if _, ok := ks.PrivateKey.(*ecdsa.PrivateKey); !ok {
		t.Errorf("PrivateKey type = %T, want *ecdsa.PrivateKey", ks.PrivateKey)
	}
	if _, ok := ks.PublicKey.(*ecdsa.PublicKey); !ok {
		t.Errorf("PublicKey type = %T, want *ecdsa.PublicKey", ks.PublicKey)
	}
}

func TestGenerateKeySet_RS256(t *testing.T) {
	ks, err := GenerateKeySet(AlgRS256)
	if err != nil {
		t.Fatalf("GenerateKeySet(RS256) error: %v", err)
	}
	if ks.Algorithm != AlgRS256 {
		t.Errorf("Algorithm = %q, want %q", ks.Algorithm, AlgRS256)
	}
	if ks.KeyID == "" {
		t.Error("KeyID is empty")
	}
	if _, ok := ks.PrivateKey.(*rsa.PrivateKey); !ok {
		t.Errorf("PrivateKey type = %T, want *rsa.PrivateKey", ks.PrivateKey)
	}
	if _, ok := ks.PublicKey.(*rsa.PublicKey); !ok {
		t.Errorf("PublicKey type = %T, want *rsa.PublicKey", ks.PublicKey)
	}
}

func TestGenerateKeySet_InvalidAlgorithm(t *testing.T) {
	_, err := GenerateKeySet("INVALID")
	if err == nil {
		t.Fatal("expected error for invalid algorithm, got nil")
	}
}

func TestPEMRoundTrip_EdDSA(t *testing.T) {
	ks, err := GenerateKeySet(AlgEdDSA)
	if err != nil {
		t.Fatalf("GenerateKeySet error: %v", err)
	}

	privPEM, pubPEM, err := MarshalKeySetPEM(ks)
	if err != nil {
		t.Fatalf("MarshalKeySetPEM error: %v", err)
	}

	parsed, err := ParseKeySetPEM(privPEM, pubPEM, AlgEdDSA)
	if err != nil {
		t.Fatalf("ParseKeySetPEM error: %v", err)
	}

	if parsed.KeyID != ks.KeyID {
		t.Errorf("KeyID = %q, want %q", parsed.KeyID, ks.KeyID)
	}
	if parsed.Algorithm != AlgEdDSA {
		t.Errorf("Algorithm = %q, want %q", parsed.Algorithm, AlgEdDSA)
	}
}

func TestPEMRoundTrip_ES256(t *testing.T) {
	ks, err := GenerateKeySet(AlgES256)
	if err != nil {
		t.Fatalf("GenerateKeySet error: %v", err)
	}

	privPEM, pubPEM, err := MarshalKeySetPEM(ks)
	if err != nil {
		t.Fatalf("MarshalKeySetPEM error: %v", err)
	}

	parsed, err := ParseKeySetPEM(privPEM, pubPEM, AlgES256)
	if err != nil {
		t.Fatalf("ParseKeySetPEM error: %v", err)
	}

	if parsed.KeyID != ks.KeyID {
		t.Errorf("KeyID = %q, want %q", parsed.KeyID, ks.KeyID)
	}
	if parsed.Algorithm != AlgES256 {
		t.Errorf("Algorithm = %q, want %q", parsed.Algorithm, AlgES256)
	}
}

func TestPEMRoundTrip_RS256(t *testing.T) {
	ks, err := GenerateKeySet(AlgRS256)
	if err != nil {
		t.Fatalf("GenerateKeySet error: %v", err)
	}

	privPEM, pubPEM, err := MarshalKeySetPEM(ks)
	if err != nil {
		t.Fatalf("MarshalKeySetPEM error: %v", err)
	}

	parsed, err := ParseKeySetPEM(privPEM, pubPEM, AlgRS256)
	if err != nil {
		t.Fatalf("ParseKeySetPEM error: %v", err)
	}

	if parsed.KeyID != ks.KeyID {
		t.Errorf("KeyID = %q, want %q", parsed.KeyID, ks.KeyID)
	}
	if parsed.Algorithm != AlgRS256 {
		t.Errorf("Algorithm = %q, want %q", parsed.Algorithm, AlgRS256)
	}
}

func TestComputeKeyID_Deterministic(t *testing.T) {
	ks, err := GenerateKeySet(AlgES256)
	if err != nil {
		t.Fatalf("GenerateKeySet error: %v", err)
	}

	kid1 := ComputeKeyID(ks.PublicKey)
	kid2 := ComputeKeyID(ks.PublicKey)

	if kid1 != kid2 {
		t.Errorf("ComputeKeyID not deterministic: %q != %q", kid1, kid2)
	}
	if kid1 == "" {
		t.Error("ComputeKeyID returned empty string")
	}
}

func TestSigningMethod(t *testing.T) {
	tests := []struct {
		alg  string
		want string
	}{
		{AlgEdDSA, "EdDSA"},
		{AlgES256, "ES256"},
		{AlgRS256, "RS256"},
	}

	for _, tt := range tests {
		t.Run(tt.alg, func(t *testing.T) {
			ks, err := GenerateKeySet(tt.alg)
			if err != nil {
				t.Fatalf("GenerateKeySet(%s) error: %v", tt.alg, err)
			}
			sm := ks.SigningMethod()
			if sm == nil {
				t.Fatal("SigningMethod() returned nil")
			}
			if got := sm.Alg(); got != tt.want {
				t.Errorf("SigningMethod().Alg() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSigningMethod_CorrectType(t *testing.T) {
	tests := []struct {
		alg      string
		wantType jwt.SigningMethod
	}{
		{AlgEdDSA, jwt.SigningMethodEdDSA},
		{AlgES256, jwt.SigningMethodES256},
		{AlgRS256, jwt.SigningMethodRS256},
	}

	for _, tt := range tests {
		t.Run(tt.alg, func(t *testing.T) {
			ks, err := GenerateKeySet(tt.alg)
			if err != nil {
				t.Fatalf("GenerateKeySet(%s) error: %v", tt.alg, err)
			}
			got := ks.SigningMethod()
			if got != tt.wantType {
				t.Errorf("SigningMethod() = %v, want %v", got, tt.wantType)
			}
		})
	}
}
