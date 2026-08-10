package oidc

import "time"

// AuthorizationCode represents an issued authorization code.
type AuthorizationCode struct {
	Code                string
	ClientID            string
	UserID              int64
	RedirectURI         string
	Scopes              []string
	Nonce               string
	CodeChallenge       string
	CodeChallengeMethod string
	AuthTime            time.Time
	ExpiresAt           time.Time
	Consumed            bool
}

// TokenPair is the response from a token exchange.
type TokenPair struct {
	AccessToken  string `json:"access_token"`
	IDToken      string `json:"id_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	Scope        string `json:"scope,omitempty"`
}

// UserInfo is the OIDC userinfo response.
type UserInfo struct {
	Sub         string `json:"sub"`
	Name        string `json:"name,omitempty"`
	Email       string `json:"email,omitempty"`
	PhoneNumber string `json:"phone_number,omitempty"`
}

// DiscoveryDocument is the OIDC discovery metadata (RFC 8414).
type DiscoveryDocument struct {
	Issuer                           string   `json:"issuer"`
	AuthorizationEndpoint            string   `json:"authorization_endpoint"`
	TokenEndpoint                    string   `json:"token_endpoint"`
	UserinfoEndpoint                 string   `json:"userinfo_endpoint"`
	JwksURI                          string   `json:"jwks_uri"`
	ResponseTypesSupported           []string `json:"response_types_supported"`
	SubjectTypesSupported            []string `json:"subject_types_supported"`
	IDTokenSigningAlgValuesSupported []string `json:"id_token_signing_alg_values_supported"`
	ScopesSupported                  []string `json:"scopes_supported"`
	TokenEndpointAuthMethodsSupp     []string `json:"token_endpoint_auth_methods_supported"`
	ClaimsSupported                  []string `json:"claims_supported"`
	GrantTypesSupported              []string `json:"grant_types_supported"`
	CodeChallengeMethodsSupported    []string `json:"code_challenge_methods_supported"`
}

// AuthorizeRequest captures parameters from GET /oidc/authorize.
type AuthorizeRequest struct {
	ResponseType        string
	ClientID            string
	RedirectURI         string
	Scope               string
	State               string
	Nonce               string
	CodeChallenge       string
	CodeChallengeMethod string
}

// CodeExchangeRequest captures parameters from POST /oidc/token (authorization_code).
type CodeExchangeRequest struct {
	Code         string
	RedirectURI  string
	ClientID     string
	CodeVerifier string
}

// RefreshRequest captures parameters from POST /oidc/token (refresh_token).
type RefreshRequest struct {
	RefreshToken string
	ClientID     string
	Scope        string
}
