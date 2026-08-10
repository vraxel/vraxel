package iam

import (
	"encoding/json"
	"net/http"

	"vraxel.io/vraxel/lib/oidc"
)

// handleDiscovery 返回 OIDC 发现文档（RFC 8414），包含授权、令牌、JWKS 等端点地址。
// +openapi:endpoint
// +openapi:method=GET
// +openapi:path=/.well-known/openid-configuration
// +openapi:summary=OIDC 发现文档
// +openapi:description=返回 OpenID Connect 发现文档，包含授权、令牌、JWKS 等端点地址
// +openapi:tag=OIDC
// +openapi:operationId=getOIDCDiscovery
// +openapi:response.200.description=OK
// +openapi:response.200.contentType=application/json
// +openapi:response.200.schema=OIDCDiscoveryResponse
func handleDiscovery(provider *oidc.Provider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "max-age=3600")
		_ = json.NewEncoder(w).Encode(provider.DiscoveryDocument())
	}
}

// handleJWKS 返回 JSON Web Key Set，包含用于验证 JWT 签名的 ECDSA 公钥。
// +openapi:endpoint
// +openapi:method=GET
// +openapi:path=/.well-known/jwks.json
// +openapi:summary=JSON Web Key Set
// +openapi:description=返回用于验证 JWT 签名的 ECDSA 公钥集
// +openapi:tag=OIDC
// +openapi:operationId=getJWKS
// +openapi:response.200.description=OK
// +openapi:response.200.contentType=application/json
func handleJWKS(provider *oidc.Provider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "max-age=3600")
		_ = json.NewEncoder(w).Encode(provider.JWKSet())
	}
}
