package oidc

import (
	"crypto/sha256"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// TokenService handles JWT issuance and verification.
type TokenService struct {
	keySet *KeySet
	issuer string
	atTTL  time.Duration
}

// NewTokenService creates a new token service.
func NewTokenService(keySet *KeySet, issuer string, accessTokenTTL time.Duration) *TokenService {
	return &TokenService{
		keySet: keySet,
		issuer: issuer,
		atTTL:  accessTokenTTL,
	}
}

// StandardClaims are the claims in an access token.
type StandardClaims struct {
	jwt.RegisteredClaims
	Scope string `json:"scope,omitempty"`
}

// IDTokenClaims are the claims in an ID token.
type IDTokenClaims struct {
	jwt.RegisteredClaims
	Nonce       string `json:"nonce,omitempty"`
	AuthTime    int64  `json:"auth_time,omitempty"`
	AtHash      string `json:"at_hash,omitempty"`
	Name        string `json:"name,omitempty"`
	Email       string `json:"email,omitempty"`
	PhoneNumber string `json:"phone_number,omitempty"`
}

// IssueAccessToken creates a signed JWT access token.
func (ts *TokenService) IssueAccessToken(userID int64, clientID string, scopes []string) (string, error) {
	now := time.Now()
	claims := StandardClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    ts.issuer,
			Subject:   strconv.FormatInt(userID, 10),
			Audience:  jwt.ClaimStrings{clientID},
			ExpiresAt: jwt.NewNumericDate(now.Add(ts.atTTL)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
		Scope: strings.Join(scopes, " "),
	}

	token := jwt.NewWithClaims(ts.keySet.SigningMethod(), claims)
	token.Header["kid"] = ts.keySet.KeyID
	return token.SignedString(ts.keySet.PrivateKey)
}

// IssueIDToken creates a signed JWT ID token.
func (ts *TokenService) IssueIDToken(userID int64, clientID string, nonce string, authTime time.Time, accessToken string, user *UserInfo, scopes []string) (string, error) {
	now := time.Now()
	claims := IDTokenClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    ts.issuer,
			Subject:   strconv.FormatInt(userID, 10),
			Audience:  jwt.ClaimStrings{clientID},
			ExpiresAt: jwt.NewNumericDate(now.Add(ts.atTTL)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
		Nonce:    nonce,
		AuthTime: authTime.Unix(),
		AtHash:   computeAtHash(accessToken),
	}

	scopeSet := make(map[string]bool, len(scopes))
	for _, s := range scopes {
		scopeSet[s] = true
	}
	if user != nil {
		if scopeSet["profile"] {
			claims.Name = user.Name
		}
		if scopeSet["email"] {
			claims.Email = user.Email
		}
		if scopeSet["phone"] {
			claims.PhoneNumber = user.PhoneNumber
		}
	}

	token := jwt.NewWithClaims(ts.keySet.SigningMethod(), claims)
	token.Header["kid"] = ts.keySet.KeyID
	return token.SignedString(ts.keySet.PrivateKey)
}

// VerifyAccessToken parses and validates an access token, returning its claims.
func (ts *TokenService) VerifyAccessToken(tokenStr string) (*StandardClaims, error) {
	claims := &StandardClaims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
		if t.Method.Alg() != ts.keySet.SigningMethod().Alg() {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return ts.keySet.PublicKey, nil
	}, jwt.WithIssuer(ts.issuer))
	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}
	if !token.Valid {
		return nil, fmt.Errorf("token is not valid")
	}
	return claims, nil
}

// computeAtHash computes the at_hash claim per OIDC spec (left half of SHA-256).
func computeAtHash(accessToken string) string {
	h := sha256.Sum256([]byte(accessToken))
	return base64URLEncode(h[:16])
}
