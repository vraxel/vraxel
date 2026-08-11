package iam

import (
	"encoding/json"
	"net/http"
	"net/mail"
	"strconv"
	"time"
	"unicode/utf8"

	"vraxel.io/vraxel/lib/audit"
	"vraxel.io/vraxel/lib/logger"
	"vraxel.io/vraxel/lib/oidc"
	"vraxel.io/vraxel/pkg/apis/iam/store"
)

// handleRegister 处理自助注册请求（公开端点，无需认证）。校验用户名/邮箱/密码后创建激活用户，
// 绑定默认平台角色，然后走与密码登录一致的路径完成登录（带 requestId 则完成授权流程）。
// +openapi:endpoint
// +openapi:method=POST
// +openapi:path=/oidc/register
// +openapi:summary=用户自助注册
// +openapi:description=创建新用户并直接登录；未开启自助注册时该端点不存在
// +openapi:tag=OIDC
// +openapi:operationId=oidcRegister
// +openapi:requestBody.contentType=application/json
// +openapi:response.200.description=注册并登录成功
// +openapi:response.400.description=参数校验失败
// +openapi:response.409.description=用户名或邮箱已存在
func handleRegister(provider *oidc.Provider, extras authExtras, auditLogger audit.Logger) http.HandlerFunc {
	type registerRequest struct {
		Username    string `json:"username"`
		Email       string `json:"email"`
		Password    string `json:"password"`
		DisplayName string `json:"displayName"`
		RequestID   string `json:"requestId"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		var req registerRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			oidcError(w, "invalid_request", "invalid request body", http.StatusBadRequest)
			return
		}

		// Per-IP registration throttle BEFORE any expensive work: a locked
		// source is rejected without burning a bcrypt hash. Every attempt
		// counts (not just failures) -- successful spam registrations are
		// the abuse being bounded.
		clientIP := audit.ClientIP(r)
		if retryAfter, locked := provider.RegisterLocked(r.Context(), clientIP); locked {
			registerHandleThrottled(w, r, auditLogger, req.Username, retryAfter)
			return
		}
		provider.NoteRegisterAttempt(r.Context(), clientIP)

		if msg := validateRegistration(req.Username, req.Email, req.Password, req.DisplayName); msg != "" {
			oidcError(w, "invalid_request", msg, http.StatusBadRequest)
			return
		}

		hash, err := HashPassword(req.Password)
		if err != nil {
			oidcError(w, "server_error", "failed to hash password", http.StatusInternalServerError)
			return
		}

		displayName := req.DisplayName
		if displayName == "" {
			displayName = req.Username
		}

		user, err := extras.registration.RegisterLocal(r.Context(), store.RegisterLocalInput{
			Username:        req.Username,
			Email:           req.Email,
			DisplayName:     displayName,
			PasswordHash:    hash,
			DefaultRoleName: extras.defaultRole,
		})
		if err != nil {
			if store.IsConflict(err) {
				oidcError(w, "conflict", "username or email already exists", http.StatusConflict)
				return
			}
			logger.Errorf("register user %q failed: %v", req.Username, err)
			oidcError(w, "server_error", "failed to register", http.StatusInternalServerError)
			return
		}

		registerAuditSuccess(r, auditLogger, user.Username, user.ID)

		// Register-and-login: reuse the exact password-login path so the
		// session + authorization-code flow behave identically to /oidc/login.
		session, _, err := provider.Login(r.Context(), req.Username, req.Password)
		if err != nil {
			logger.Errorf("auto-login after register failed for %q: %v", req.Username, err)
			oidcError(w, "server_error", "registered but auto-login failed", http.StatusInternalServerError)
			return
		}

		if req.RequestID != "" {
			loginCompleteAuthorize(w, r, provider, req.RequestID, session)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"sessionId": session.SessionID,
			"userId":    session.UserID,
		})
	}
}

// validateRegistration checks the public signup fields, reusing the same rules
// as admin user creation (usernameRegexp, email, displayName length,
// ValidatePassword). Returns an empty string when valid, else a single
// human-readable message.
func validateRegistration(username, email, password, displayName string) string {
	if username == "" || !usernameRegexp.MatchString(username) {
		return "username must be 3-50 alphanumeric characters, underscores, or hyphens"
	}
	if email == "" || len(email) > 255 {
		return "email is required and must be at most 255 characters"
	}
	if _, err := mail.ParseAddress(email); err != nil {
		return "email is not a valid email address"
	}
	if utf8.RuneCountInString(displayName) > 128 {
		return "displayName must be at most 128 characters"
	}
	if errs := ValidatePassword(password); len(errs) > 0 {
		return errs[0].Message
	}
	return ""
}

// registerHandleThrottled 处理被限流拒绝的注册：写审计事件并返回 429 + Retry-After。
// error_description 是固定字符串（前端按 error code 匹配做本地化），等待秒数放在
// Retry-After 头给程序化客户端。
func registerHandleThrottled(w http.ResponseWriter, r *http.Request, auditLogger audit.Logger, username string, retryAfter time.Duration) {
	logger.Infof("register throttled from %s (retry in %s)", audit.ClientIP(r), retryAfter.Round(time.Second))
	if auditLogger != nil {
		auditLogger.Log(audit.Event{
			Username:   username,
			EventType:  "authentication",
			Action:     "register_throttled",
			HTTPMethod: r.Method,
			HTTPPath:   r.URL.Path,
			StatusCode: http.StatusTooManyRequests,
			ClientIP:   audit.ClientIP(r),
			UserAgent:  r.UserAgent(),
			Success:    false,
			Detail:     audit.JSONString("too many registration attempts"),
			CreatedAt:  time.Now(),
		})
	}
	w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())+1))
	oidcError(w, "rate_limited", "too many registration attempts", http.StatusTooManyRequests)
}

// registerAuditSuccess writes a successful-registration audit event.
func registerAuditSuccess(r *http.Request, auditLogger audit.Logger, username string, userID int64) {
	if auditLogger == nil {
		return
	}
	id := userID
	auditLogger.Log(audit.Event{
		UserID:     &id,
		Username:   username,
		EventType:  "authentication",
		Action:     "register",
		HTTPMethod: r.Method,
		HTTPPath:   r.URL.Path,
		StatusCode: http.StatusOK,
		ClientIP:   audit.ClientIP(r),
		UserAgent:  r.UserAgent(),
		Success:    true,
		CreatedAt:  time.Now(),
	})
}
