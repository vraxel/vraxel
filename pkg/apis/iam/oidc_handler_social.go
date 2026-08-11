package iam

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	"vraxel.io/vraxel/lib/audit"
	"vraxel.io/vraxel/lib/config"
	"vraxel.io/vraxel/lib/logger"
	"vraxel.io/vraxel/lib/oidc"
	"vraxel.io/vraxel/lib/socialauth"
	"vraxel.io/vraxel/pkg/apis/iam/store"
)

// oauthStateTTL bounds how long a social-login round trip may take.
const oauthStateTTL = 15 * time.Minute

// authExtras bundles the dependencies for the public registration and
// social-login endpoints layered onto the base OIDC mux.
type authExtras struct {
	registration     store.RegistrationStore
	oauthState       store.OAuthStateStore
	social           map[string]socialauth.Provider
	selfRegistration bool
	defaultRole      string
	externalURL      string
}

// socialRedirectURI is the provider callback registered with the external
// OAuth app; it must match exactly on both the authorize and exchange calls.
func (e authExtras) socialRedirectURI(provider string) string {
	return e.externalURL + "/oidc/social/" + provider + "/callback"
}

// buildSocialProviders constructs the enabled social providers from config.
// A provider with no clientId/secret is omitted, so its endpoints 404 and the
// frontend hides its button.
func buildSocialProviders(cfg config.SocialConfig) map[string]socialauth.Provider {
	providers := map[string]socialauth.Provider{}
	if cfg.GitHub.Enabled() {
		providers["github"] = socialauth.NewGitHub(cfg.GitHub.ClientID, cfg.GitHub.Secret, cfg.GitHub.Scopes)
	}
	if cfg.Google.Enabled() {
		providers["google"] = socialauth.NewGoogle(cfg.Google.ClientID, cfg.Google.Secret, cfg.Google.Scopes)
	}
	return providers
}

// handleSocialStart 发起社交登录：生成 state（携带 vraxel 的 request_id）后重定向到外部
// 提供者的授权页。
func handleSocialStart(extras authExtras) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("provider")
		provider, ok := extras.social[name]
		if !ok {
			http.Redirect(w, r, "/error?status=404", http.StatusFound)
			return
		}

		state, err := oidc.GenerateCode()
		if err != nil {
			http.Redirect(w, r, "/error?status=500", http.StatusFound)
			return
		}
		requestID := r.URL.Query().Get("request_id")
		if err := extras.oauthState.Create(r.Context(), state, name, requestID, oauthStateTTL); err != nil {
			logger.Errorf("store oauth state: %v", err)
			http.Redirect(w, r, "/error?status=500", http.StatusFound)
			return
		}

		http.Redirect(w, r, provider.AuthCodeURL(state, extras.socialRedirectURI(name)), http.StatusFound)
	}
}

// handleSocialCallback 处理外部提供者回调：校验 state、换取并验证用户资料、find-or-create
// 本地用户，然后复用现有授权码流程重定向回 SPA 回调（后续 code 换 cookie 与本地登录完全一致）。
func handleSocialCallback(provider *oidc.Provider, extras authExtras, auditLogger audit.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("provider")
		social, ok := extras.social[name]
		if !ok {
			http.Redirect(w, r, "/error?status=404", http.StatusFound)
			return
		}

		q := r.URL.Query()
		if q.Get("error") != "" {
			http.Redirect(w, r, "/error?status=403", http.StatusFound)
			return
		}
		code, state := q.Get("code"), q.Get("state")
		if code == "" || state == "" {
			http.Redirect(w, r, "/error?status=400", http.StatusFound)
			return
		}

		st, err := extras.oauthState.Consume(r.Context(), state)
		if err != nil || st.Provider != name {
			http.Redirect(w, r, "/error?status=400", http.StatusFound)
			return
		}

		profile, err := social.Authenticate(r.Context(), code, extras.socialRedirectURI(name))
		if err != nil {
			logger.Infof("social login (%s) failed: %v", name, err)
			http.Redirect(w, r, "/error?status=403", http.StatusFound)
			return
		}

		user, err := extras.registration.FindOrCreateSocial(r.Context(), store.SocialLoginInput{
			Provider:        name,
			Subject:         profile.Subject,
			Email:           profile.Email,
			Username:        socialUsername(name, profile.Subject),
			DisplayName:     profile.Name,
			AvatarURL:       profile.AvatarURL,
			DefaultRoleName: extras.defaultRole,
			AllowCreate:     extras.selfRegistration,
		})
		if err != nil {
			// Policy refusals (inactive account, builtin-link, signups disabled)
			// are a 403, not a server error.
			if store.IsForbidden(err) {
				logger.Infof("social login refused (%s): %v", name, err)
				http.Redirect(w, r, "/error?status=403", http.StatusFound)
				return
			}
			logger.Errorf("provision social user (%s): %v", name, err)
			http.Redirect(w, r, "/error?status=500", http.StatusFound)
			return
		}

		loginAuditSuccess(r, auditLogger, user.Username, user.ID)

		// Resume the pending vraxel authorization the login page started, so
		// the SPA's /auth/callback exchanges the code for cookies exactly as
		// it does after a password login.
		authReq, err := provider.GetPendingAuthorize(r.Context(), st.RequestID)
		if err != nil {
			http.Redirect(w, r, "/error?status=400", http.StatusFound)
			return
		}
		redirectURL, err := provider.Authorize(r.Context(), authReq, user.ID, time.Now())
		if err != nil {
			http.Redirect(w, r, "/error?status=500", http.StatusFound)
			return
		}
		http.Redirect(w, r, redirectURL, http.StatusFound)
	}
}

// handleAuthConfig 暴露前端渲染登录/注册页所需的开关：是否开放自助注册、启用了哪些社交登录。
// +openapi:endpoint
// +openapi:method=GET
// +openapi:path=/oidc/config
// +openapi:summary=前端认证配置
// +openapi:description=返回是否开放自助注册及可用的社交登录提供者
// +openapi:tag=OIDC
// +openapi:operationId=oidcAuthConfig
// +openapi:response.200.description=配置
func handleAuthConfig(extras authExtras) http.HandlerFunc {
	providers := make([]string, 0, len(extras.social))
	for name := range extras.social {
		providers = append(providers, name)
	}
	sort.Strings(providers)
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"selfRegistration": extras.selfRegistration,
			"socialProviders":  providers,
		})
	}
}

// socialUsername derives a deterministic, unique local username from the
// provider and its stable subject id (e.g. "github_12345678"). Using the
// subject rather than a display name avoids collisions between social users
// and keeps the value within the username charset/length limits.
func socialUsername(provider, subject string) string {
	u := provider + "_" + subject
	u = strings.Map(func(rr rune) rune {
		switch {
		case rr >= 'a' && rr <= 'z', rr >= 'A' && rr <= 'Z', rr >= '0' && rr <= '9', rr == '_', rr == '-':
			return rr
		default:
			return '-'
		}
	}, u)
	if len(u) > 50 {
		u = u[:50]
	}
	return u
}
