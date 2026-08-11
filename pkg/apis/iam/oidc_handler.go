package iam

import (
	"encoding/json"
	"net/http"
	"strings"

	"vraxel.io/vraxel/lib/audit"
	"vraxel.io/vraxel/lib/config"
	"vraxel.io/vraxel/lib/logger"
	"vraxel.io/vraxel/lib/oidc"
	"vraxel.io/vraxel/pkg/apis/iam/store"
)

// NewOIDCMux 创建包含所有 OIDC 公开端点的 HTTP 路由。
// 这些端点不经过认证中间件，公开访问：
//   - GET  /.well-known/openid-configuration — OIDC 发现文档
//   - GET  /.well-known/jwks.json            — JSON Web Key Set（公钥集）
//   - GET  /oidc/authorize                   — 授权端点，发起 Authorization Code Flow
//   - POST /oidc/login                       — 用户登录（用户名+密码），完成授权并返回授权码
//   - POST /oidc/token                       — 令牌端点，用授权码或刷新令牌换取访问令牌
//   - POST /oidc/logout                      — 注销端点，清除 cookie 并撤销 refresh token
//   - GET  /oidc/userinfo                    — 用户信息端点，通过 Bearer Token 获取当前用户信息
//   - POST /oidc/userinfo                    — 同上（支持 POST 方法）
//   - GET  /oidc/config                      — 前端认证配置（自助注册开关 + 社交登录提供者）
//   - POST /oidc/register                    — 自助注册（仅在开启自助注册时挂载）
//   - GET  /oidc/social/{provider}/start     — 发起 GitHub/Google 登录
//   - GET  /oidc/social/{provider}/callback  — 外部提供者回调
func NewOIDCMux(provider *oidc.Provider, auditLogger audit.Logger, stores store.Stores, cfg *config.OIDCConfig, externalURL string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/openid-configuration", handleDiscovery(provider))
	mux.HandleFunc("GET /.well-known/jwks.json", handleJWKS(provider))
	mux.HandleFunc("GET /oidc/authorize", handleAuthorize(provider))
	mux.HandleFunc("POST /oidc/login", handleLogin(provider, auditLogger))
	mux.HandleFunc("POST /oidc/token", handleToken(provider, auditLogger))
	mux.HandleFunc("POST /oidc/logout", handleLogout(provider, auditLogger))
	mux.HandleFunc("GET /oidc/userinfo", handleUserInfo(provider))
	mux.HandleFunc("POST /oidc/userinfo", handleUserInfo(provider))

	extras := authExtras{
		registration:     stores.Registration,
		oauthState:       stores.OAuthState,
		social:           buildSocialProviders(cfg.Social),
		selfRegistration: cfg.SelfRegistrationEnabled(),
		defaultRole:      store.RolePlatformViewer,
		externalURL:      strings.TrimRight(externalURL, "/"),
	}
	mux.HandleFunc("GET /oidc/config", handleAuthConfig(extras))
	if extras.selfRegistration {
		mux.HandleFunc("POST /oidc/register", handleRegister(provider, extras, auditLogger))
	}
	mux.HandleFunc("GET /oidc/social/{provider}/start", handleSocialStart(extras))
	mux.HandleFunc("GET /oidc/social/{provider}/callback", handleSocialCallback(provider, extras, auditLogger))

	return mux
}

// oidcError 写入标准 OAuth2 错误响应（RFC 6749 Section 5.2）。
func oidcError(w http.ResponseWriter, errCode, description string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error":             errCode,
		"error_description": description,
	})
}

// writeTokenResponse emits the successful token exchange / refresh result as
// HttpOnly cookies (BFF pattern). The JSON body carries only the
// non-sensitive envelope.
func writeTokenResponse(w http.ResponseWriter, r *http.Request, provider *oidc.Provider, tokens *oidc.TokenPair) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")

	secure := oidc.IsSecureRequest(r)
	accessTTL := provider.AccessTokenTTL()
	refreshTTL := provider.RefreshTokenTTL()

	http.SetCookie(w, &http.Cookie{
		Name:     oidc.CookieAccessToken,
		Value:    tokens.AccessToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(accessTTL.Seconds()),
	})
	if tokens.RefreshToken != "" {
		http.SetCookie(w, &http.Cookie{
			Name:     oidc.CookieRefreshToken,
			Value:    tokens.RefreshToken,
			Path:     oidc.RefreshCookiePath,
			HttpOnly: true,
			Secure:   secure,
			SameSite: http.SameSiteStrictMode,
			MaxAge:   int(refreshTTL.Seconds()),
		})
	}

	csrfToken, err := oidc.GenerateCSRFToken()
	if err != nil {
		logger.Errorf("generate csrf token: %v", err)
		oidcError(w, "server_error", "failed to generate csrf token", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     oidc.CookieCSRFToken,
		Value:    csrfToken,
		Path:     "/",
		HttpOnly: false, // readable by JS -- echoed back in X-CSRF-Token header
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(refreshTTL.Seconds()),
	})

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"token_type": tokens.TokenType,
		"expires_in": tokens.ExpiresIn,
		"scope":      tokens.Scope,
	})
}

// clearAuthCookies overwrites all auth cookies with Max-Age=0 so the browser
// drops them. Paths must match the originals.
func clearAuthCookies(w http.ResponseWriter, r *http.Request) {
	secure := oidc.IsSecureRequest(r)
	expire := func(name, path string, sameSite http.SameSite, httpOnly bool) {
		http.SetCookie(w, &http.Cookie{
			Name:     name,
			Value:    "",
			Path:     path,
			HttpOnly: httpOnly,
			Secure:   secure,
			SameSite: sameSite,
			MaxAge:   -1,
		})
	}
	expire(oidc.CookieAccessToken, "/", http.SameSiteLaxMode, true)
	expire(oidc.CookieRefreshToken, oidc.RefreshCookiePath, http.SameSiteStrictMode, true)
	expire(oidc.CookieCSRFToken, "/", http.SameSiteLaxMode, false)
}
