package oidc

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// OIDCUser is a user representation for OIDC operations.
type OIDCUser struct {
	ID           int64
	Username     string
	Email        string
	DisplayName  string
	Phone        string
	PasswordHash string
	Status       string
}

// UserLookup provides user data to the OIDC provider.
type UserLookup interface {
	GetByIdentifier(ctx context.Context, identifier string) (*OIDCUser, error)
	GetByID(ctx context.Context, id int64) (*OIDCUser, error)
	UpdateLastLogin(ctx context.Context, id int64) error
}

// Provider is the main OIDC orchestrator.
type Provider struct {
	config   *ProviderConfig
	keySet   *KeySet
	tokens   *TokenService
	codes    AuthCodeStore
	sessions SessionStore
	users    UserLookup
	refresh  RefreshTokenStore
	password *PasswordService

	clientsMu sync.RWMutex
	clients   map[string]*Client

	// pendingAuthorize stores pending authorization requests by request_id
	pendingAuthorize PendingStore

	// loginThrottle counts failed logins for brute-force lockout; nil
	// disables throttling (see SetLoginThrottle).
	loginThrottle LoginThrottleStore
}

// NewProvider creates a new OIDC provider.
// The caller must supply all store implementations (use PG stores for production,
// in-memory stores for tests).
func NewProvider(cfg *ProviderConfig, keySet *KeySet, users UserLookup, refresh RefreshTokenStore,
	sessions SessionStore, codes AuthCodeStore, pending PendingStore) *Provider {
	return &Provider{
		config:           cfg,
		keySet:           keySet,
		tokens:           NewTokenService(keySet, cfg.Issuer, cfg.AccessTokenTTL),
		codes:            codes,
		sessions:         sessions,
		users:            users,
		refresh:          refresh,
		password:         NewPasswordService(0),
		pendingAuthorize: pending,
	}
}

// SetClients sets the registered OAuth2 clients (thread-safe for hot-reload).
func (p *Provider) SetClients(clients map[string]*Client) {
	p.clientsMu.Lock()
	p.clients = clients
	p.clientsMu.Unlock()
}

// LoginURL returns the configured login URL.
func (p *Provider) LoginURL() string { return p.config.LoginURL }

// PasswordService returns the password service for external use (e.g., user creation).
func (p *Provider) PasswordService() *PasswordService {
	return p.password
}

// Login authenticates a user and creates a session.
func (p *Provider) Login(ctx context.Context, identifier, password string) (*Session, *OIDCUser, error) {
	user, err := p.users.GetByIdentifier(ctx, identifier)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid credentials")
	}

	if user.Status != "active" {
		return nil, nil, fmt.Errorf("account is not active")
	}

	if user.PasswordHash == "" {
		return nil, nil, fmt.Errorf("invalid credentials")
	}

	if err := p.password.Verify(password, user.PasswordHash); err != nil {
		return nil, nil, fmt.Errorf("invalid credentials")
	}

	sessionID, err := GenerateSessionID()
	if err != nil {
		return nil, nil, fmt.Errorf("generate session: %w", err)
	}

	now := time.Now()
	session := &Session{
		SessionID: sessionID,
		UserID:    user.ID,
		AuthTime:  now,
		ExpiresAt: now.Add(24 * time.Hour),
	}
	p.sessions.Create(ctx, session)

	_ = p.users.UpdateLastLogin(ctx, user.ID)

	return session, user, nil
}

// StorePendingAuthorize stores a pending authorization request and returns a request ID.
func (p *Provider) StorePendingAuthorize(ctx context.Context, req *AuthorizeRequest) (string, error) {
	return p.pendingAuthorize.Store(ctx, req)
}

// GetPendingAuthorize retrieves and removes a pending authorization request.
func (p *Provider) GetPendingAuthorize(ctx context.Context, requestID string) (*AuthorizeRequest, error) {
	return p.pendingAuthorize.Consume(ctx, requestID)
}

// Authorize generates an authorization code and returns a redirect URL.
func (p *Provider) Authorize(ctx context.Context, req *AuthorizeRequest, userID int64, authTime time.Time) (string, error) {
	codeStr, err := GenerateCode()
	if err != nil {
		return "", fmt.Errorf("generate code: %w", err)
	}

	scopes := strings.Split(req.Scope, " ")
	code := &AuthorizationCode{
		Code:                codeStr,
		ClientID:            req.ClientID,
		UserID:              userID,
		RedirectURI:         req.RedirectURI,
		Scopes:              scopes,
		Nonce:               req.Nonce,
		CodeChallenge:       req.CodeChallenge,
		CodeChallengeMethod: req.CodeChallengeMethod,
		AuthTime:            authTime,
		ExpiresAt:           time.Now().Add(p.config.AuthCodeTTL),
	}
	p.codes.Store(ctx, code)

	redirectURL, err := url.Parse(req.RedirectURI)
	if err != nil {
		return "", fmt.Errorf("parse redirect URI: %w", err)
	}
	q := redirectURL.Query()
	q.Set("code", codeStr)
	if req.State != "" {
		q.Set("state", req.State)
	}
	redirectURL.RawQuery = q.Encode()
	return redirectURL.String(), nil
}

// ExchangeCode exchanges an authorization code for tokens.
func (p *Provider) ExchangeCode(ctx context.Context, req *CodeExchangeRequest) (*TokenPair, error) {
	code, err := p.codes.Consume(ctx, req.Code)
	if err != nil {
		return nil, fmt.Errorf("invalid authorization code: %w", err)
	}

	if code.ClientID != req.ClientID {
		return nil, errors.New("client_id mismatch")
	}
	if req.RedirectURI != "" && code.RedirectURI != req.RedirectURI {
		return nil, errors.New("redirect_uri mismatch")
	}

	// Verify PKCE
	if code.CodeChallenge != "" {
		if req.CodeVerifier == "" {
			return nil, errors.New("code_verifier required")
		}
		if !verifyPKCE(code.CodeChallenge, code.CodeChallengeMethod, req.CodeVerifier) {
			return nil, errors.New("PKCE verification failed")
		}
	}

	// Issue tokens
	return p.issueTokens(ctx, code.UserID, code.ClientID, code.Scopes, code.Nonce, code.AuthTime)
}

// RefreshTokens exchanges a refresh token for new tokens.
func (p *Provider) RefreshTokens(ctx context.Context, req *RefreshRequest) (*TokenPair, error) {
	rtData, err := p.refresh.Consume(ctx, req.RefreshToken)
	if err != nil {
		return nil, fmt.Errorf("invalid refresh token: %w", err)
	}

	if rtData.ClientID != req.ClientID {
		return nil, errors.New("client_id mismatch")
	}

	// Block inactive users from refreshing tokens
	if _, err := p.CheckUserActive(ctx, rtData.UserID); err != nil {
		return nil, fmt.Errorf("account is not active")
	}

	scopes := strings.Split(rtData.Scope, " ")
	return p.issueTokens(ctx, rtData.UserID, rtData.ClientID, scopes, "", time.Now())
}

// UserInfoForToken validates a bearer token and returns user info.
func (p *Provider) UserInfoForToken(ctx context.Context, accessToken string) (*UserInfo, error) {
	claims, err := p.tokens.VerifyAccessToken(accessToken)
	if err != nil {
		return nil, err
	}

	userID, err := strconv.ParseInt(claims.Subject, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid subject: %w", err)
	}

	user, err := p.users.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	scopeSet := make(map[string]bool)
	for _, s := range strings.Split(claims.Scope, " ") {
		scopeSet[s] = true
	}

	info := &UserInfo{
		Sub: claims.Subject,
	}
	if scopeSet["profile"] {
		info.Name = user.DisplayName
	}
	if scopeSet["email"] {
		info.Email = user.Email
	}
	if scopeSet["phone"] {
		info.PhoneNumber = user.Phone
	}
	return info, nil
}

// VerifyBearerToken validates a bearer token and returns the user ID.
func (p *Provider) VerifyBearerToken(tokenStr string) (int64, error) {
	claims, err := p.tokens.VerifyAccessToken(tokenStr)
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(claims.Subject, 10, 64)
}

// RevokeUserRefreshTokens revokes all refresh tokens belonging to the user.
// Used by the logout endpoint to invalidate the server-side session.
func (p *Provider) RevokeUserRefreshTokens(ctx context.Context, userID int64) error {
	return p.refresh.RevokeByUser(ctx, userID)
}

// AccessTokenTTL returns the configured access token lifetime.
func (p *Provider) AccessTokenTTL() time.Duration { return p.config.AccessTokenTTL }

// RefreshTokenTTL returns the configured refresh token lifetime.
func (p *Provider) RefreshTokenTTL() time.Duration { return p.config.RefreshTokenTTL }

// CheckUserActive verifies that the user is still in active status.
// Returns the username on success.
func (p *Provider) CheckUserActive(ctx context.Context, userID int64) (string, error) {
	user, err := p.users.GetByID(ctx, userID)
	if err != nil {
		return "", fmt.Errorf("user not found")
	}
	if user.Status != "active" {
		return "", fmt.Errorf("account is not active")
	}
	return user.Username, nil
}

// DiscoveryDocument returns the OIDC discovery metadata.
func (p *Provider) DiscoveryDocument() *DiscoveryDocument {
	return &DiscoveryDocument{
		Issuer:                           p.config.Issuer,
		AuthorizationEndpoint:            p.config.Issuer + "/oidc/authorize",
		TokenEndpoint:                    p.config.Issuer + "/oidc/token",
		UserinfoEndpoint:                 p.config.Issuer + "/oidc/userinfo",
		JwksURI:                          p.config.Issuer + "/.well-known/jwks.json",
		ResponseTypesSupported:           []string{"code"},
		SubjectTypesSupported:            []string{"public"},
		IDTokenSigningAlgValuesSupported: []string{p.keySet.SigningMethod().Alg()},
		ScopesSupported:                  []string{"openid", "profile", "email", "phone"},
		TokenEndpointAuthMethodsSupp:     []string{"client_secret_post", "none"},
		ClaimsSupported:                  []string{"sub", "iss", "aud", "exp", "iat", "nonce", "auth_time", "at_hash", "name", "email", "phone_number"},
		GrantTypesSupported:              []string{"authorization_code", "refresh_token"},
		CodeChallengeMethodsSupported:    []string{"S256"},
	}
}

// JWKSet returns the JSON Web Key Set.
func (p *Provider) JWKSet() *JWKSet {
	return p.keySet.JWKSet()
}

// GetClient returns a registered client by ID (thread-safe).
func (p *Provider) GetClient(clientID string) (*Client, bool) {
	p.clientsMu.RLock()
	c, ok := p.clients[clientID]
	p.clientsMu.RUnlock()
	return c, ok
}

// ValidateRedirectURI checks if the URI is registered for the client.
func (p *Provider) ValidateRedirectURI(client *Client, uri string) bool {
	for _, allowed := range client.RedirectURIs {
		if allowed == uri {
			return true
		}
	}
	return false
}

func (p *Provider) issueTokens(ctx context.Context, userID int64, clientID string, scopes []string, nonce string, authTime time.Time) (*TokenPair, error) {
	accessToken, err := p.tokens.IssueAccessToken(userID, clientID, scopes)
	if err != nil {
		return nil, fmt.Errorf("issue access token: %w", err)
	}

	user, err := p.users.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user for id token: %w", err)
	}

	userInfo := &UserInfo{
		Sub:         strconv.FormatInt(userID, 10),
		Name:        user.DisplayName,
		Email:       user.Email,
		PhoneNumber: user.Phone,
	}

	var idToken string
	hasOpenID := false
	for _, s := range scopes {
		if s == "openid" {
			hasOpenID = true
			break
		}
	}
	if hasOpenID {
		idToken, err = p.tokens.IssueIDToken(userID, clientID, nonce, authTime, accessToken, userInfo, scopes)
		if err != nil {
			return nil, fmt.Errorf("issue id token: %w", err)
		}
	}

	// Issue refresh token
	rawRT, err := GenerateRefreshToken()
	if err != nil {
		return nil, fmt.Errorf("generate refresh token: %w", err)
	}
	rtData := &RefreshTokenData{
		TokenHash: HashToken(rawRT),
		UserID:    userID,
		ClientID:  clientID,
		Scope:     strings.Join(scopes, " "),
		ExpiresAt: time.Now().Add(p.config.RefreshTokenTTL),
	}
	if err := p.refresh.Store(ctx, rtData); err != nil {
		return nil, fmt.Errorf("store refresh token: %w", err)
	}

	return &TokenPair{
		AccessToken:  accessToken,
		IDToken:      idToken,
		RefreshToken: rawRT,
		TokenType:    "Bearer",
		ExpiresIn:    int64(p.config.AccessTokenTTL.Seconds()),
		Scope:        strings.Join(scopes, " "),
	}, nil
}

// verifyPKCE verifies a PKCE code_verifier against a code_challenge.
func verifyPKCE(challenge, method, verifier string) bool {
	if method != "S256" {
		return false
	}
	h := sha256.Sum256([]byte(verifier))
	computed := base64.RawURLEncoding.EncodeToString(h[:])
	return subtle.ConstantTimeCompare([]byte(computed), []byte(challenge)) == 1
}

// memPendingStore is an in-memory implementation of PendingStore.
type memPendingStore struct {
	mu       sync.Mutex
	requests map[string]*memPendingRequest
}

type memPendingRequest struct {
	req       *AuthorizeRequest
	expiresAt time.Time
}

// NewMemPendingStore creates an in-memory pending authorization store (useful for tests).
func NewMemPendingStore() PendingStore {
	return &memPendingStore{
		requests: make(map[string]*memPendingRequest),
	}
}

func (ps *memPendingStore) Store(_ context.Context, req *AuthorizeRequest) (string, error) {
	id, err := GenerateCode()
	if err != nil {
		return "", err
	}
	ps.mu.Lock()
	defer ps.mu.Unlock()
	// Lazy cleanup
	now := time.Now()
	for k, v := range ps.requests {
		if now.After(v.expiresAt) {
			delete(ps.requests, k)
		}
	}
	ps.requests[id] = &memPendingRequest{
		req:       req,
		expiresAt: now.Add(30 * time.Minute),
	}
	return id, nil
}

func (ps *memPendingStore) Consume(_ context.Context, id string) (*AuthorizeRequest, error) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	pr, ok := ps.requests[id]
	if !ok {
		return nil, errors.New("pending request not found")
	}
	delete(ps.requests, id)
	if time.Now().After(pr.expiresAt) {
		return nil, errors.New("pending request expired")
	}
	return pr.req, nil
}
