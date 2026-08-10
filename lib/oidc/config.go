package oidc

import (
	"fmt"
	"time"

	"vraxel.io/vraxel/lib/config"
)

// ProviderConfig holds parsed runtime configuration for the OIDC provider.
type ProviderConfig struct {
	Issuer          string
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
	AuthCodeTTL     time.Duration
	LoginURL        string
}

// Client represents a registered OAuth2 client at runtime.
type Client struct {
	ID           string
	Secret       string
	RedirectURIs []string
	Scopes       []string
	Public       bool
}

// ParseConfig converts a config.OIDCConfig into a ProviderConfig.
func ParseConfig(cfg *config.OIDCConfig) (*ProviderConfig, error) {
	accessTTL, err := time.ParseDuration(cfg.AccessTokenTTL)
	if err != nil {
		return nil, fmt.Errorf("parse accessTokenTTL %q: %w", cfg.AccessTokenTTL, err)
	}
	refreshTTL, err := time.ParseDuration(cfg.RefreshTokenTTL)
	if err != nil {
		return nil, fmt.Errorf("parse refreshTokenTTL %q: %w", cfg.RefreshTokenTTL, err)
	}
	authCodeTTL, err := time.ParseDuration(cfg.AuthCodeTTL)
	if err != nil {
		return nil, fmt.Errorf("parse authCodeTTL %q: %w", cfg.AuthCodeTTL, err)
	}
	return &ProviderConfig{
		Issuer:          cfg.Issuer,
		AccessTokenTTL:  accessTTL,
		RefreshTokenTTL: refreshTTL,
		AuthCodeTTL:     authCodeTTL,
		LoginURL:        cfg.LoginURL,
	}, nil
}

// ParseClients converts config.ClientConfig slice into a map of Client by ID.
func ParseClients(cfgs []config.ClientConfig) map[string]*Client {
	clients := make(map[string]*Client, len(cfgs))
	for _, c := range cfgs {
		clients[c.ID] = &Client{
			ID:           c.ID,
			Secret:       c.Secret,
			RedirectURIs: c.RedirectURIs,
			Scopes:       c.Scopes,
			Public:       c.Public,
		}
	}
	return clients
}
