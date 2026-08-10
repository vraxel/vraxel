package oidc

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"math/big"

	"github.com/golang-jwt/jwt/v5"
)

// Algorithm constants for supported signing algorithms.
const (
	AlgEdDSA = "EdDSA"
	AlgES256 = "ES256"
	AlgRS256 = "RS256"
)

// KeyStore is the interface for loading or generating key sets.
type KeyStore interface {
	LoadOrGenerate(algorithm string) (*KeySet, error)
}

// KeySet holds a signing key pair, its derived key ID, and the algorithm name.
type KeySet struct {
	PrivateKey crypto.Signer
	PublicKey  crypto.PublicKey
	KeyID      string
	Algorithm  string
}

// SigningMethod returns the jwt.SigningMethod for this key set's algorithm.
func (ks *KeySet) SigningMethod() jwt.SigningMethod {
	switch ks.Algorithm {
	case AlgEdDSA:
		return jwt.SigningMethodEdDSA
	case AlgES256:
		return jwt.SigningMethodES256
	case AlgRS256:
		return jwt.SigningMethodRS256
	default:
		return nil
	}
}

// JWKSet is a JSON Web Key Set.
type JWKSet struct {
	Keys []JWK `json:"keys"`
}

// JWK is a single JSON Web Key. Fields are conditional based on key type.
type JWK struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Use string `json:"use"`
	Alg string `json:"alg"`

	// EC fields (P-256)
	Crv string `json:"crv,omitempty"`
	X   string `json:"x,omitempty"`
	Y   string `json:"y,omitempty"`

	// RSA fields
	N string `json:"n,omitempty"`
	E string `json:"e,omitempty"`
}

// JWKSet returns the JWK Set representation of the public key.
func (ks *KeySet) JWKSet() *JWKSet {
	jwk := JWK{
		Kid: ks.KeyID,
		Use: "sig",
		Alg: ks.Algorithm,
	}

	switch pub := ks.PublicKey.(type) {
	case ed25519.PublicKey:
		jwk.Kty = "OKP"
		jwk.Crv = "Ed25519"
		jwk.X = base64URLEncode(pub)
	case *ecdsa.PublicKey:
		jwk.Kty = "EC"
		jwk.Crv = "P-256"
		byteLen := (pub.Curve.Params().BitSize + 7) / 8
		jwk.X = base64URLEncodeBigInt(pub.X, byteLen)
		jwk.Y = base64URLEncodeBigInt(pub.Y, byteLen)
	case *rsa.PublicKey:
		jwk.Kty = "RSA"
		jwk.N = base64URLEncodeBigInt(pub.N, 0)
		jwk.E = base64URLEncodeBigInt(big.NewInt(int64(pub.E)), 0)
	}

	return &JWKSet{Keys: []JWK{jwk}}
}

// GenerateKeySet generates a new key pair for the specified algorithm.
func GenerateKeySet(algorithm string) (*KeySet, error) {
	switch algorithm {
	case AlgEdDSA:
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, fmt.Errorf("generate Ed25519 key: %w", err)
		}
		kid := ComputeKeyID(pub)
		return &KeySet{
			PrivateKey: priv,
			PublicKey:  pub,
			KeyID:      kid,
			Algorithm:  AlgEdDSA,
		}, nil

	case AlgES256:
		priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return nil, fmt.Errorf("generate ECDSA P-256 key: %w", err)
		}
		kid := ComputeKeyID(&priv.PublicKey)
		return &KeySet{
			PrivateKey: priv,
			PublicKey:  &priv.PublicKey,
			KeyID:      kid,
			Algorithm:  AlgES256,
		}, nil

	case AlgRS256:
		priv, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			return nil, fmt.Errorf("generate RSA 2048 key: %w", err)
		}
		kid := ComputeKeyID(&priv.PublicKey)
		return &KeySet{
			PrivateKey: priv,
			PublicKey:  &priv.PublicKey,
			KeyID:      kid,
			Algorithm:  AlgRS256,
		}, nil

	default:
		return nil, fmt.Errorf("unsupported algorithm: %s", algorithm)
	}
}

// MarshalKeySetPEM encodes a KeySet to PEM-encoded private (PKCS8) and public (PKIX) key bytes.
func MarshalKeySetPEM(ks *KeySet) (privPEM, pubPEM []byte, err error) {
	privDER, err := x509.MarshalPKCS8PrivateKey(ks.PrivateKey)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal private key: %w", err)
	}
	privBlock := &pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privDER,
	}

	pubDER, err := x509.MarshalPKIXPublicKey(ks.PublicKey)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal public key: %w", err)
	}
	pubBlock := &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubDER,
	}

	return pem.EncodeToMemory(privBlock), pem.EncodeToMemory(pubBlock), nil
}

// ParseKeySetPEM decodes PEM-encoded private and public keys back into a KeySet.
func ParseKeySetPEM(privPEM, pubPEM []byte, algorithm string) (*KeySet, error) {
	privBlock, _ := pem.Decode(privPEM)
	if privBlock == nil {
		return nil, fmt.Errorf("no PEM block found in private key data")
	}
	privKey, err := x509.ParsePKCS8PrivateKey(privBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}

	pubBlock, _ := pem.Decode(pubPEM)
	if pubBlock == nil {
		return nil, fmt.Errorf("no PEM block found in public key data")
	}
	pubKey, err := x509.ParsePKIXPublicKey(pubBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse public key: %w", err)
	}

	signer, ok := privKey.(crypto.Signer)
	if !ok {
		return nil, fmt.Errorf("private key does not implement crypto.Signer")
	}

	kid := ComputeKeyID(pubKey)

	return &KeySet{
		PrivateKey: signer,
		PublicKey:  pubKey,
		KeyID:      kid,
		Algorithm:  algorithm,
	}, nil
}

// ComputeKeyID derives a key ID from the public key using a SHA-256 hash,
// base64url encoded, truncated to 16 bytes.
func ComputeKeyID(pub crypto.PublicKey) string {
	h := sha256.New()

	switch k := pub.(type) {
	case ed25519.PublicKey:
		h.Write(k)
	case *ecdsa.PublicKey:
		byteLen := (k.Curve.Params().BitSize + 7) / 8
		h.Write(padTo(k.X.Bytes(), byteLen))
		h.Write(padTo(k.Y.Bytes(), byteLen))
	case *rsa.PublicKey:
		h.Write(k.N.Bytes())
		h.Write(big.NewInt(int64(k.E)).Bytes())
	}

	return base64URLEncode(h.Sum(nil)[:16])
}

// padTo pads b with leading zeros to the given size.
func padTo(b []byte, size int) []byte {
	if len(b) >= size {
		return b
	}
	padded := make([]byte, size)
	copy(padded[size-len(b):], b)
	return padded
}

// base64URLEncode encodes data as base64url without padding.
func base64URLEncode(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}

// base64URLEncodeBigInt encodes a big.Int as base64url.
// If byteLen > 0, the value is left-padded with zeros to that length.
func base64URLEncodeBigInt(n *big.Int, byteLen int) string {
	b := n.Bytes()
	if byteLen > 0 {
		b = padTo(b, byteLen)
	}
	return base64URLEncode(b)
}
