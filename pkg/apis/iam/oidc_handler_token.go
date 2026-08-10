package iam

import (
	"crypto/subtle"
	"net/http"
	"strings"
	"time"

	"vraxel.io/vraxel/lib/audit"
	"vraxel.io/vraxel/lib/logger"
	"vraxel.io/vraxel/lib/oidc"
)

// handleToken 处理令牌请求，支持两种授权类型：
// - authorization_code：用授权码换取访问令牌、ID 令牌和刷新令牌；
// - refresh_token：用刷新令牌获取新的令牌对（令牌轮换）。
// +openapi:endpoint
// +openapi:method=POST
// +openapi:path=/oidc/token
// +openapi:summary=令牌端点
// +openapi:description=用授权码或刷新令牌换取访问令牌
// +openapi:tag=OIDC
// +openapi:operationId=oidcToken
// +openapi:requestBody.contentType=application/x-www-form-urlencoded
// +openapi:requestBody.schema=OIDCTokenRequest
// +openapi:response.200.description=令牌签发成功
// +openapi:response.200.contentType=application/json
// +openapi:response.200.schema=OIDCTokenResponse
// +openapi:response.400.description=请求无效
// +openapi:response.400.contentType=application/json
// +openapi:response.400.schema=OIDCErrorResponse
func handleToken(provider *oidc.Provider, auditLogger audit.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			oidcError(w, "invalid_request", "invalid form data", http.StatusBadRequest)
			return
		}

		grantType := r.FormValue("grant_type")
		switch grantType {
		case "authorization_code":
			handleCodeExchange(w, r, provider)
		case "refresh_token":
			handleRefreshToken(w, r, provider, auditLogger)
		default:
			oidcError(w, "unsupported_grant_type", "only authorization_code and refresh_token are supported", http.StatusBadRequest)
		}
	}
}

// handleCodeExchange 处理授权码换取令牌。验证客户端身份（机密客户端需要 client_secret），
// 验证 PKCE，然后签发 access_token、id_token 和 refresh_token。
func handleCodeExchange(w http.ResponseWriter, r *http.Request, provider *oidc.Provider) {
	req := &oidc.CodeExchangeRequest{
		Code:         r.FormValue("code"),
		RedirectURI:  r.FormValue("redirect_uri"),
		ClientID:     r.FormValue("client_id"),
		CodeVerifier: r.FormValue("code_verifier"),
	}

	if req.Code == "" || req.ClientID == "" {
		oidcError(w, "invalid_request", "code and client_id are required", http.StatusBadRequest)
		return
	}

	// Authenticate confidential clients
	client, ok := provider.GetClient(req.ClientID)
	if !ok {
		oidcError(w, "invalid_client", "unknown client_id", http.StatusUnauthorized)
		return
	}
	if !client.Public {
		clientSecret := r.FormValue("client_secret")
		if clientSecret == "" || subtle.ConstantTimeCompare([]byte(clientSecret), []byte(client.Secret)) != 1 {
			oidcError(w, "invalid_client", "invalid client credentials", http.StatusUnauthorized)
			return
		}
	}

	tokenPair, err := provider.ExchangeCode(r.Context(), req)
	if err != nil {
		logger.Infof("code exchange failed: %v", err)
		oidcError(w, "invalid_grant", "authorization code is invalid or expired", http.StatusBadRequest)
		return
	}

	writeTokenResponse(w, r, provider, tokenPair)
}

// handleRefreshToken 处理刷新令牌请求。原子消费旧刷新令牌（单次使用），签发新的令牌对。
// refresh_token 只从 vraxel_rt cookie 读取（BFF 模式）。
func handleRefreshToken(w http.ResponseWriter, r *http.Request, provider *oidc.Provider, auditLogger audit.Logger) {
	req := &oidc.RefreshRequest{
		ClientID: r.FormValue("client_id"),
		Scope:    r.FormValue("scope"),
	}
	if c, err := r.Cookie(oidc.CookieRefreshToken); err == nil {
		req.RefreshToken = c.Value
	}

	if req.RefreshToken == "" || req.ClientID == "" {
		oidcError(w, "invalid_request", "refresh_token and client_id are required", http.StatusBadRequest)
		return
	}

	// Authenticate confidential clients
	client, ok := provider.GetClient(req.ClientID)
	if !ok {
		oidcError(w, "invalid_client", "unknown client_id", http.StatusUnauthorized)
		return
	}
	if !client.Public {
		clientSecret := r.FormValue("client_secret")
		if clientSecret == "" || subtle.ConstantTimeCompare([]byte(clientSecret), []byte(client.Secret)) != 1 {
			oidcError(w, "invalid_client", "invalid client credentials", http.StatusUnauthorized)
			return
		}
	}

	tokenPair, err := provider.RefreshTokens(r.Context(), req)
	if err != nil {
		logger.Infof("token refresh failed: %v", err)
		if auditLogger != nil {
			action := "token_refresh_failed"
			detail := "refresh token is invalid or expired"
			if strings.Contains(err.Error(), "not active") {
				action = "token_refresh_blocked"
				detail = "account is not active"
			}
			auditLogger.Log(audit.Event{
				EventType:  "authentication",
				Action:     action,
				HTTPMethod: r.Method,
				HTTPPath:   r.URL.Path,
				StatusCode: http.StatusBadRequest,
				ClientIP:   audit.ClientIP(r),
				UserAgent:  r.UserAgent(),
				Success:    false,
				Detail:     audit.JSONString(detail),
				CreatedAt:  time.Now(),
			})
		}
		oidcError(w, "invalid_grant", "refresh token is invalid or expired", http.StatusBadRequest)
		return
	}

	if auditLogger != nil {
		auditLogger.Log(audit.Event{
			EventType:  "authentication",
			Action:     "token_refresh",
			HTTPMethod: r.Method,
			HTTPPath:   r.URL.Path,
			StatusCode: http.StatusOK,
			ClientIP:   audit.ClientIP(r),
			UserAgent:  r.UserAgent(),
			Success:    true,
			CreatedAt:  time.Now(),
		})
	}

	writeTokenResponse(w, r, provider, tokenPair)
}
