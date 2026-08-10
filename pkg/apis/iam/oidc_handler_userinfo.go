package iam

import (
	"encoding/json"
	"net/http"

	"vraxel.io/vraxel/lib/oidc"
)

// handleUserInfo 通过 Bearer Token 获取当前认证用户的信息。根据令牌中的 scope 返回相应的 claims。
// +openapi:endpoint
// +openapi:method=GET
// +openapi:path=/oidc/userinfo
// +openapi:summary=用户信息端点
// +openapi:description=通过 Bearer Token 获取当前认证用户的信息
// +openapi:tag=OIDC
// +openapi:operationId=getOIDCUserInfo
// +openapi:response.200.description=OK
// +openapi:response.200.contentType=application/json
// +openapi:response.200.schema=OIDCUserInfoResponse
// +openapi:response.401.description=令牌无效
// +openapi:response.401.contentType=application/json
// +openapi:response.401.schema=OIDCErrorResponse
func handleUserInfo(provider *oidc.Provider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(oidc.CookieAccessToken)
		if err != nil || c.Value == "" {
			oidcError(w, "invalid_token", "authentication required", http.StatusUnauthorized)
			return
		}

		userInfo, err := provider.UserInfoForToken(r.Context(), c.Value)
		if err != nil {
			oidcError(w, "invalid_token", "invalid or expired token", http.StatusUnauthorized)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(userInfo)
	}
}

// handleUserInfoPost 同 handleUserInfo，支持 POST 方法。
// +openapi:endpoint
// +openapi:method=POST
// +openapi:path=/oidc/userinfo
// +openapi:summary=用户信息端点（POST）
// +openapi:description=通过 Bearer Token 获取当前认证用户的信息（POST 方法）
// +openapi:tag=OIDC
// +openapi:operationId=postOIDCUserInfo
// +openapi:response.200.description=OK
// +openapi:response.200.contentType=application/json
// +openapi:response.200.schema=OIDCUserInfoResponse
// +openapi:response.401.description=令牌无效
// +openapi:response.401.contentType=application/json
// +openapi:response.401.schema=OIDCErrorResponse
func handleUserInfoPost() {} //nolint:unused // OpenAPI annotation target only
