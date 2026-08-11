package config

import (
	"fmt"
	"net/netip"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"gopkg.in/yaml.v3"
)

// Config is the top-level configuration structure.
type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	Logger   LoggerConfig   `yaml:"logger"`
	OIDC     OIDCConfig     `yaml:"oidc"`
	Admin    AdminConfig    `yaml:"admin"`
}

// ServerConfig holds server-level configuration.
type ServerConfig struct {
	// ExternalURL is the externally accessible URL of this Vraxel server.
	// Example: "http://vraxel.local:9099"
	ExternalURL string `yaml:"externalUrl"`
	// Name uniquely identifies this Vraxel deployment. Emitted as the `server`
	// label on every metric target and log stream so a single metrics/logs
	// backend can ingest from multiple Vraxel deployments and queries can
	// filter by `server="vraxel-dev"` etc. Default "vraxel"; multi-deployment
	// setups MUST set distinct values (e.g. "vraxel-dev" / "vraxel-test" /
	// "vraxel-prod"). Once set it should not be changed - changing it
	// disconnects historical series and log streams from the new ones.
	Name string `yaml:"name"`
	// TrustedProxies lists the peer CIDRs (or bare IPs) allowed to set
	// X-Forwarded-For / X-Real-IP. Empty (the default) makes the server
	// ignore those headers and use the socket peer address.
	//
	// This is a security control, not a convenience: the client IP keys
	// the login brute-force throttle, and any client can forge a header.
	// Set this to your ingress / load-balancer range when running behind
	// one, and to nothing when the server is directly exposed.
	TrustedProxies []string `yaml:"trustedProxies"`
}

// ParseTrustedProxies converts the configured CIDRs (bare IPs allowed)
// into prefixes.
func (c *ServerConfig) ParseTrustedProxies() ([]netip.Prefix, error) {
	out := make([]netip.Prefix, 0, len(c.TrustedProxies))
	for _, raw := range c.TrustedProxies {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if p, err := netip.ParsePrefix(raw); err == nil {
			out = append(out, p)
			continue
		}
		addr, err := netip.ParseAddr(raw)
		if err != nil {
			return nil, fmt.Errorf("server.trustedProxies %q: not an IP or CIDR", raw)
		}
		out = append(out, netip.PrefixFrom(addr, addr.BitLen()))
	}
	return out, nil
}

// serverNamePattern restricts Name to characters that are safe in
// Prometheus / VictoriaLogs label values and in dashboard/regex queries.
var serverNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,32}$`)

// Validate checks ServerConfig invariants.
func (c *ServerConfig) Validate() error {
	if !serverNamePattern.MatchString(c.Name) {
		return fmt.Errorf("server.name %q invalid: must match %s", c.Name, serverNamePattern.String())
	}
	if _, err := c.ParseTrustedProxies(); err != nil {
		return err
	}
	return nil
}

// AdminConfig holds the initial admin user configuration.
type AdminConfig struct {
	Username    string `yaml:"username"`
	Password    string `yaml:"password"`
	Email       string `yaml:"email"`
	Phone       string `yaml:"phone"`
	DisplayName string `yaml:"displayName"`
}

// OIDCConfig holds OIDC provider configuration.
type OIDCConfig struct {
	Issuer          string         `yaml:"issuer"`
	Algorithm       string         `yaml:"algorithm"`
	AccessTokenTTL  string         `yaml:"accessTokenTTL"`
	RefreshTokenTTL string         `yaml:"refreshTokenTTL"`
	AuthCodeTTL     string         `yaml:"authCodeTTL"`
	LoginURL        string         `yaml:"loginUrl"`
	Clients         []ClientConfig `yaml:"clients"`
}

// ClientConfig holds OAuth2 client configuration.
type ClientConfig struct {
	ID           string   `yaml:"id"`
	Secret       string   `yaml:"secret"`
	RedirectURIs []string `yaml:"redirectUris"`
	Scopes       []string `yaml:"scopes"`
	Public       bool     `yaml:"public"`
}

// DatabaseConfig holds PostgreSQL connection parameters.
type DatabaseConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	DBName   string `yaml:"dbName"`
	SSLMode  string `yaml:"sslMode"`
	MaxConns int32  `yaml:"maxConns"`
}

// LoggerConfig holds logging configuration.
type LoggerConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

var globalConfig atomic.Pointer[Config]

// Get returns the current global Config. May return nil if not yet set.
func Get() *Config {
	return globalConfig.Load()
}

// Set atomically replaces the global Config and triggers registered callbacks.
func Set(cfg *Config) {
	globalConfig.Store(cfg)
	callbacksMu.RLock()
	cbs := make([]func(*Config), len(callbacks))
	copy(cbs, callbacks)
	callbacksMu.RUnlock()
	for _, fn := range cbs {
		fn(cfg)
	}
}

var (
	callbacks   []func(*Config)
	callbacksMu sync.RWMutex
)

// RegisterReloadCallback registers a function to be called when configuration is reloaded.
func RegisterReloadCallback(fn func(*Config)) {
	callbacksMu.Lock()
	callbacks = append(callbacks, fn)
	callbacksMu.Unlock()
}

// SetDefaults fills zero-value fields with sensible defaults. Must run
// AFTER every override layer (file / env / CLI) so cross-field
// derivations (OIDC issuer + redirect URIs from server.externalUrl) use
// the effective values.
func SetDefaults(cfg *Config) {
	setDefaultsServer(cfg)
	setDefaultsDatabase(cfg)
	setDefaultsLogger(cfg)
	setDefaultsOIDC(cfg)
	setDefaultsAdmin(cfg)
}

func setDefaultsServer(cfg *Config) {
	if cfg.Server.ExternalURL == "" {
		cfg.Server.ExternalURL = "http://localhost:9099"
	}
	if cfg.Server.Name == "" {
		cfg.Server.Name = "vraxel"
	}
}

func setDefaultsDatabase(cfg *Config) {
	if cfg.Database.Host == "" {
		cfg.Database.Host = "localhost"
	}
	if cfg.Database.Port == 0 {
		cfg.Database.Port = 5432
	}
	if cfg.Database.User == "" {
		cfg.Database.User = "vraxel"
	}
	if cfg.Database.Password == "" {
		cfg.Database.Password = "vraxel"
	}
	if cfg.Database.DBName == "" {
		cfg.Database.DBName = "vraxel"
	}
	if cfg.Database.SSLMode == "" {
		cfg.Database.SSLMode = "disable"
	}
	if cfg.Database.MaxConns == 0 {
		cfg.Database.MaxConns = 10
	}
}

func setDefaultsLogger(cfg *Config) {
	if cfg.Logger.Level == "" {
		cfg.Logger.Level = "INFO"
	}
	if cfg.Logger.Format == "" {
		cfg.Logger.Format = "default"
	}
}

func setDefaultsOIDC(cfg *Config) {
	if cfg.OIDC.AccessTokenTTL == "" {
		cfg.OIDC.AccessTokenTTL = "1h"
	}
	if cfg.OIDC.RefreshTokenTTL == "" {
		cfg.OIDC.RefreshTokenTTL = "168h"
	}
	if cfg.OIDC.AuthCodeTTL == "" {
		cfg.OIDC.AuthCodeTTL = "5m"
	}
	if cfg.OIDC.LoginURL == "" {
		cfg.OIDC.LoginURL = "/login"
	}
	if cfg.OIDC.Algorithm == "" {
		cfg.OIDC.Algorithm = "EdDSA"
	}
	// Derive the issuer from the server's external URL so a config with
	// no oidc: section still boots with authentication ON. There is no
	// "auth disabled" mode: an empty issuer would silently serve every
	// /api route unauthenticated.
	if cfg.OIDC.Issuer == "" {
		cfg.OIDC.Issuer = cfg.Server.ExternalURL
	}
	if len(cfg.OIDC.Clients) == 0 {
		cfg.OIDC.Clients = []ClientConfig{{
			ID:     "vraxel-ui",
			Public: true,
			// Exact-match validated (lib/oidc ValidateRedirectURI), so
			// defaults must be full URLs: the embedded frontend on the
			// external URL plus the vite dev server.
			RedirectURIs: []string{
				strings.TrimRight(cfg.Server.ExternalURL, "/") + "/auth/callback",
				"http://localhost:5199/auth/callback",
			},
			Scopes: []string{"openid", "profile", "email", "phone"},
		}}
	}
}

func setDefaultsAdmin(cfg *Config) {
	if cfg.Admin.Username == "" {
		cfg.Admin.Username = "admin"
	}
	if cfg.Admin.Password == "" {
		cfg.Admin.Password = "Admin123!"
	}
	if cfg.Admin.Email == "" {
		cfg.Admin.Email = "admin@vraxel.io"
	}
	if cfg.Admin.Phone == "" {
		cfg.Admin.Phone = "13800000000"
	}
	if cfg.Admin.DisplayName == "" {
		cfg.Admin.DisplayName = "Admin"
	}
}

// LoadFromFile reads and parses a YAML configuration file. A missing
// file yields an empty Config (zero-config boot is supported). Defaults
// are NOT applied here: callers layer file < env < CLI first and then
// call SetDefaults, so derived defaults (e.g. the OIDC issuer from
// server.externalUrl) see the final override values.
func LoadFromFile(path string) (*Config, error) {
	cfg := &Config{}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, fmt.Errorf("read config file %q: %w", path, err)
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config file %q: %w", path, err)
	}
	return cfg, nil
}

// ApplyEnvOverrides overrides Config fields with environment variable values when set.
func ApplyEnvOverrides(cfg *Config) {
	applyEnvDatabase(cfg)
	applyEnvServer(cfg)
	applyEnvOIDC(cfg)
	applyEnvAdmin(cfg)
}

func applyEnvDatabase(cfg *Config) {
	if v := os.Getenv("DB_HOST"); v != "" {
		cfg.Database.Host = v
	}
	if v := os.Getenv("DB_PORT"); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			cfg.Database.Port = i
		}
	}
	if v := os.Getenv("DB_USER"); v != "" {
		cfg.Database.User = v
	}
	if v := os.Getenv("DB_PASSWORD"); v != "" {
		cfg.Database.Password = v
	}
	if v := os.Getenv("DB_NAME"); v != "" {
		cfg.Database.DBName = v
	}
	if v := os.Getenv("DB_SSL_MODE"); v != "" {
		cfg.Database.SSLMode = v
	}
	if v := os.Getenv("DB_MAX_CONNS"); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			cfg.Database.MaxConns = int32(i)
		}
	}
}

func applyEnvServer(cfg *Config) {
	if v := os.Getenv("SERVER_EXTERNAL_URL"); v != "" {
		cfg.Server.ExternalURL = v
	}
	if v := os.Getenv("SERVER_NAME"); v != "" {
		cfg.Server.Name = v
	}
}

func applyEnvOIDC(cfg *Config) {
	if v := os.Getenv("OIDC_ISSUER"); v != "" {
		cfg.OIDC.Issuer = v
	}
	if v := os.Getenv("OIDC_ALGORITHM"); v != "" {
		cfg.OIDC.Algorithm = v
	}
	if v := os.Getenv("OIDC_LOGIN_URL"); v != "" {
		cfg.OIDC.LoginURL = v
	}
}

func applyEnvAdmin(cfg *Config) {
	if v := os.Getenv("ADMIN_USERNAME"); v != "" {
		cfg.Admin.Username = v
	}
	if v := os.Getenv("ADMIN_PASSWORD"); v != "" {
		cfg.Admin.Password = v
	}
	if v := os.Getenv("ADMIN_EMAIL"); v != "" {
		cfg.Admin.Email = v
	}
	if v := os.Getenv("ADMIN_PHONE"); v != "" {
		cfg.Admin.Phone = v
	}
	if v := os.Getenv("ADMIN_DISPLAY_NAME"); v != "" {
		cfg.Admin.DisplayName = v
	}
}
