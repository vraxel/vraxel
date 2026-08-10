# OIDC Provider — Vraxel

## Overview

Vraxel includes a built-in OpenID Connect (OIDC) provider that supports the Authorization Code Flow with PKCE. This enables:

- User authentication via username + password
- Standard JWT token issuance (ID Token + Access Token + Refresh Token)
- Bearer token-based API protection
- Third-party application integration via standard OIDC protocols

## Phase 1 Features

| Feature | Status |
|---------|--------|
| Authorization Code Flow | Supported |
| PKCE (S256) | Supported |
| JWT signing (ES256) | Supported |
| Discovery endpoint | Supported |
| JWKS endpoint | Supported |
| Token endpoint | Supported |
| UserInfo endpoint | Supported |
| Refresh tokens | Supported |
| Password change | Supported |

## Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/.well-known/openid-configuration` | OIDC Discovery Document |
| GET | `/.well-known/jwks.json` | JSON Web Key Set |
| GET | `/oidc/authorize` | Authorization endpoint |
| POST | `/oidc/login` | Login (username + password) |
| POST | `/oidc/token` | Token exchange |
| GET/POST | `/oidc/userinfo` | User information |
| POST | `/api/v1/users/{userId}/change-password` | Change password |

## Authorization Code Flow

```
1. Client → GET /oidc/authorize
     ?response_type=code
     &client_id=vraxel-ui
     &redirect_uri=http://localhost:5173/auth/callback
     &scope=openid+profile+email
     &state=<random>
     &nonce=<random>
     &code_challenge=<S256>
     &code_challenge_method=S256

2. Server validates → stores pending request → 302 to /login?request_id=<id>

3. Login page → POST /oidc/login
     {"username": "...", "password": "...", "requestId": "..."}

4. Server validates password → generates auth code → returns
     {"redirectUri": "http://localhost:5173/auth/callback?code=...&state=..."}

5. Client redirects to callback URL

6. Callback → POST /oidc/token (application/x-www-form-urlencoded)
     grant_type=authorization_code
     &code=...
     &redirect_uri=...
     &client_id=vraxel-ui
     &code_verifier=<PKCE_verifier>

7. Server: validates code + PKCE → issues tokens
     {access_token, id_token, refresh_token, token_type, expires_in, scope}
```

## Token Format

### Access Token (JWT ES256)
Claims: `iss`, `sub` (user ID), `aud` (client ID), `exp`, `iat`, `scope`

### ID Token (JWT ES256)
Claims: `iss`, `sub`, `aud`, `exp`, `iat`, `nonce`, `auth_time`, `at_hash`, `name`, `email`, `phone_number` (based on requested scopes)

### Refresh Token
Opaque string (32 random bytes, hex-encoded). SHA-256 hash stored in database.

## Scopes

| Scope | Claims |
|-------|--------|
| openid | sub |
| profile | name |
| email | email |
| phone | phone_number |

## Client Configuration

Clients are configured in `config.yaml`:

```yaml
oidc:
  clients:
    - id: "vraxel-ui"
      public: true          # No client_secret, PKCE required
      redirectUris:
        - "http://localhost:5173/auth/callback"
      scopes: ["openid", "profile", "email", "phone"]
```

## Key Management

Signing keys are auto-generated at startup and stored in PostgreSQL (table `oidc_signing_keys`). No manual key file management is needed.

Configure the signing algorithm in `config.yaml`:
```yaml
oidc:
  algorithm: "EdDSA"   # Supported: EdDSA (default), ES256, RS256
```

## Security

- Passwords: bcrypt hashed (cost 10)
- password_hash never appears in standard API queries
- Refresh tokens: only SHA-256 hash stored in DB
- Authorization codes: single-use, 5-minute TTL, atomic consumption
- Public clients must use PKCE
- OIDC endpoints are public; API endpoints require Bearer token authentication
- Token TTLs: access 1h, refresh 7d, auth code 5m
