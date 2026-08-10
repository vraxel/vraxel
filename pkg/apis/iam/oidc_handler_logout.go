package iam

import (
	"net/http"
	"time"

	"vraxel.io/vraxel/lib/audit"
	"vraxel.io/vraxel/lib/logger"
	"vraxel.io/vraxel/lib/oidc"
)

// handleLogout 注销当前会话。若带有有效 access token cookie，撤销该用户的所有
// refresh token；无论是否有效，始终清理浏览器端的认证 cookie。
// +openapi:endpoint
// +openapi:method=POST
// +openapi:path=/oidc/logout
// +openapi:summary=注销
// +openapi:description=清除认证 cookie 并撤销 refresh token
// +openapi:tag=OIDC
// +openapi:operationId=oidcLogout
// +openapi:response.204.description=注销成功
func handleLogout(provider *oidc.Provider, auditLogger audit.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, username := resolveLogoutUser(provider, r)

		clearAuthCookies(w, r)

		if auditLogger != nil && userID > 0 {
			auditLogger.Log(audit.Event{
				UserID:     &userID,
				Username:   username,
				EventType:  "authentication",
				Action:     "logout",
				HTTPMethod: r.Method,
				HTTPPath:   r.URL.Path,
				StatusCode: http.StatusNoContent,
				ClientIP:   audit.ClientIP(r),
				UserAgent:  r.UserAgent(),
				Success:    true,
				CreatedAt:  time.Now(),
			})
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// resolveLogoutUser extracts the authenticated user from the access token cookie
// (if present and valid) and, as a side effect, revokes that user's refresh
// tokens. Returns the zero user when no valid token is present.
func resolveLogoutUser(provider *oidc.Provider, r *http.Request) (userID int64, username string) {
	if c, err := r.Cookie(oidc.CookieAccessToken); err == nil && c.Value != "" {
		if uid, verr := provider.VerifyBearerToken(c.Value); verr == nil {
			userID = uid
			if u, cerr := provider.CheckUserActive(r.Context(), uid); cerr == nil {
				username = u
			}
			if rerr := provider.RevokeUserRefreshTokens(r.Context(), uid); rerr != nil {
				logger.Warnf("revoke refresh tokens for user %d: %v", uid, rerr)
			}
		}
	}
	return userID, username
}
