package iam

import (
	"encoding/json"
	"net/http"
	"time"

	"vraxel.io/vraxel/lib/audit"
	"vraxel.io/vraxel/lib/logger"
	"vraxel.io/vraxel/lib/oidc"
)

// handleAuthorize 处理 OAuth2 授权请求。验证 client_id、redirect_uri 和 PKCE 参数后，
// 将请求存储为待处理状态，并重定向用户到登录页面。
// +openapi:endpoint
// +openapi:method=GET
// +openapi:path=/oidc/authorize
// +openapi:summary=授权端点
// +openapi:description=发起 OAuth2 Authorization Code Flow，验证参数后重定向到登录页面
// +openapi:tag=OIDC
// +openapi:operationId=oidcAuthorize
// +openapi:response.302.description=重定向到登录页面
func handleAuthorize(provider *oidc.Provider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		req, ok := authorizeValidateRequest(w, r, provider)
		if !ok {
			return
		}

		requestID, err := provider.StorePendingAuthorize(r.Context(), req)
		if err != nil {
			http.Redirect(w, r, "/error?status=500", http.StatusFound)
			return
		}

		// Redirect to login page with request_id
		loginURL := provider.LoginURL() + "?request_id=" + requestID
		http.Redirect(w, r, loginURL, http.StatusFound)
	}
}

// authorizeValidateRequest 校验 OAuth2 授权请求的 query 参数（response_type / client /
// redirect_uri / PKCE），通过则返回构造好的 AuthorizeRequest 与 true；任一校验失败时已写出
// 对应 /error?status=... 重定向并返回 false，调用方应直接返回。重定向状态码与原内联逻辑逐字一致。
func authorizeValidateRequest(w http.ResponseWriter, r *http.Request, provider *oidc.Provider) (*oidc.AuthorizeRequest, bool) {
	q := r.URL.Query()
	responseType := q.Get("response_type")
	clientID := q.Get("client_id")
	redirectURI := q.Get("redirect_uri")
	scope := q.Get("scope")
	state := q.Get("state")
	nonce := q.Get("nonce")
	codeChallenge := q.Get("code_challenge")
	codeChallengeMethod := q.Get("code_challenge_method")

	if responseType != "code" {
		http.Redirect(w, r, "/error?status=400", http.StatusFound)
		return nil, false
	}

	client, ok := provider.GetClient(clientID)
	if !ok {
		http.Redirect(w, r, "/error?status=400", http.StatusFound)
		return nil, false
	}

	if redirectURI == "" {
		if len(client.RedirectURIs) > 0 {
			redirectURI = client.RedirectURIs[0]
		} else {
			http.Redirect(w, r, "/error?status=400", http.StatusFound)
			return nil, false
		}
	}

	if !provider.ValidateRedirectURI(client, redirectURI) {
		http.Redirect(w, r, "/error?status=403", http.StatusFound)
		return nil, false
	}

	// Public clients must use PKCE
	if client.Public && codeChallenge == "" {
		http.Redirect(w, r, "/error?status=400", http.StatusFound)
		return nil, false
	}

	if codeChallenge != "" && codeChallengeMethod != "S256" {
		http.Redirect(w, r, "/error?status=400", http.StatusFound)
		return nil, false
	}

	return &oidc.AuthorizeRequest{
		ResponseType:        responseType,
		ClientID:            clientID,
		RedirectURI:         redirectURI,
		Scope:               scope,
		State:               state,
		Nonce:               nonce,
		CodeChallenge:       codeChallenge,
		CodeChallengeMethod: codeChallengeMethod,
	}, true
}

// handleLogin 处理用户登录请求。验证用户名和密码后：
// - 如果提供了 requestId，完成 OIDC 授权流程，生成授权码并返回回调 URL；
// - 如果未提供 requestId，执行直接登录，返回会话信息。
// +openapi:endpoint
// +openapi:method=POST
// +openapi:path=/oidc/login
// +openapi:summary=用户登录
// +openapi:description=验证用户名和密码，完成授权流程或直接登录
// +openapi:tag=OIDC
// +openapi:operationId=oidcLogin
// +openapi:requestBody.contentType=application/json
// +openapi:requestBody.schema=OIDCLoginRequest
// +openapi:response.200.description=登录成功
// +openapi:response.200.contentType=application/json
// +openapi:response.200.schema=OIDCLoginResponse
// +openapi:response.401.description=认证失败
// +openapi:response.401.contentType=application/json
// +openapi:response.401.schema=OIDCErrorResponse
func handleLogin(provider *oidc.Provider, auditLogger audit.Logger) http.HandlerFunc {
	type loginRequest struct {
		Username  string `json:"username"`
		Password  string `json:"password"`
		RequestID string `json:"requestId"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		var req loginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			oidcError(w, "invalid_request", "invalid request body", http.StatusBadRequest)
			return
		}

		if req.Username == "" || req.Password == "" {
			oidcError(w, "invalid_request", "username and password are required", http.StatusBadRequest)
			return
		}

		session, _, err := provider.Login(r.Context(), req.Username, req.Password)
		if err != nil {
			loginHandleFailure(w, r, auditLogger, req.Username, err)
			return
		}

		loginAuditSuccess(r, auditLogger, req.Username, session.UserID)

		// If requestId provided, complete the authorization flow
		if req.RequestID != "" {
			loginCompleteAuthorize(w, r, provider, req.RequestID, session)
			return
		}

		// Direct login without OIDC flow — just return session info
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"sessionId": session.SessionID,
			"userId":    session.UserID,
		})
	}
}

// loginHandleFailure 处理登录失败：记录日志、按错误区分描述、写审计事件，并写出 401 OIDC 错误。
// 描述判定与审计字段与原内联逻辑逐字一致。
func loginHandleFailure(w http.ResponseWriter, r *http.Request, auditLogger audit.Logger, username string, err error) {
	logger.Infof("login failed for user %q: %v", username, err)
	description := "invalid credentials"
	if err.Error() == "account is not active" {
		description = "account is not active"
	}
	if auditLogger != nil {
		auditLogger.Log(audit.Event{
			Username:   username,
			EventType:  "authentication",
			Action:     "login_failed",
			HTTPMethod: r.Method,
			HTTPPath:   r.URL.Path,
			StatusCode: http.StatusUnauthorized,
			ClientIP:   audit.ClientIP(r),
			UserAgent:  r.UserAgent(),
			Success:    false,
			Detail:     audit.JSONString(description),
			CreatedAt:  time.Now(),
		})
	}
	oidcError(w, "invalid_grant", description, http.StatusUnauthorized)
}

// loginAuditSuccess 在登录成功后写入审计事件（auditLogger 为 nil 时跳过）。
func loginAuditSuccess(r *http.Request, auditLogger audit.Logger, username string, sessionUserID int64) {
	if auditLogger != nil {
		userID := sessionUserID
		auditLogger.Log(audit.Event{
			UserID:     &userID,
			Username:   username,
			EventType:  "authentication",
			Action:     "login",
			HTTPMethod: r.Method,
			HTTPPath:   r.URL.Path,
			StatusCode: http.StatusOK,
			ClientIP:   audit.ClientIP(r),
			UserAgent:  r.UserAgent(),
			Success:    true,
			CreatedAt:  time.Now(),
		})
	}
}

// loginCompleteAuthorize 在提供 requestId 时完成 OIDC 授权流程：取回待处理授权、生成授权码，
// 成功时返回 {"redirectUri": ...}；任一步失败已写出对应 OIDC 错误响应。
func loginCompleteAuthorize(w http.ResponseWriter, r *http.Request, provider *oidc.Provider, requestID string, session *oidc.Session) {
	authReq, err := provider.GetPendingAuthorize(r.Context(), requestID)
	if err != nil {
		oidcError(w, "invalid_request", "invalid or expired request_id", http.StatusBadRequest)
		return
	}

	redirectURL, err := provider.Authorize(r.Context(), authReq, session.UserID, session.AuthTime)
	if err != nil {
		oidcError(w, "server_error", "failed to generate authorization code", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(struct {
		RedirectURI string `json:"redirectUri"`
	}{RedirectURI: redirectURL})
}
