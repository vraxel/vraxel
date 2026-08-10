package audit

import (
	"encoding/json"

	"vraxel.io/vraxel/lib/runtime"
)

// AuditLog represents an audit log entry in the API layer.
// +openapi:description=审计日志：记录 API 写操作和认证事件
type AuditLog struct {
	runtime.TypeMeta `json:",inline"`
	Spec             AuditLogSpec `json:"spec"`
}

func (a *AuditLog) GetTypeMeta() *runtime.TypeMeta { return &a.TypeMeta }

// AuditLogSpec contains audit log fields.
// +openapi:description=审计日志属性
type AuditLogSpec struct {
	// +openapi:description=日志 ID
	ID string `json:"id"`
	// +openapi:description=操作用户 ID
	UserID *string `json:"userId,omitempty"`
	// +openapi:description=操作用户名
	Username string `json:"username"`
	// +openapi:description=事件类型：api_operation 或 authentication
	// +openapi:enum=api_operation,authentication
	EventType string `json:"eventType"`
	// +openapi:description=操作动作
	Action string `json:"action"`
	// +openapi:description=资源类型
	ResourceType string `json:"resourceType,omitempty"`
	// +openapi:description=资源 ID
	ResourceID string `json:"resourceId,omitempty"`
	// +openapi:description=所属模块
	Module string `json:"module,omitempty"`
	// +openapi:description=作用域
	// +openapi:enum=platform,workspace,namespace
	Scope string `json:"scope"`
	// +openapi:description=租户 ID
	WorkspaceID *string `json:"workspaceId,omitempty"`
	// +openapi:description=项目 ID
	NamespaceID *string `json:"namespaceId,omitempty"`
	// +openapi:description=HTTP 方法
	HTTPMethod string `json:"httpMethod,omitempty"`
	// +openapi:description=HTTP 路径
	HTTPPath string `json:"httpPath,omitempty"`
	// +openapi:description=HTTP 状态码
	StatusCode int `json:"statusCode,omitempty"`
	// +openapi:description=客户端 IP
	ClientIP string `json:"clientIp,omitempty"`
	// +openapi:description=User-Agent
	UserAgent string `json:"userAgent,omitempty"`
	// +openapi:description=请求耗时（毫秒）
	DurationMs int `json:"durationMs,omitempty"`
	// +openapi:description=是否成功
	Success bool `json:"success"`
	// +openapi:description=请求体（JSON）
	Detail json.RawMessage `json:"detail,omitempty"`
	// +openapi:description=响应体（JSON）
	ResponseDetail json.RawMessage `json:"responseDetail,omitempty"`
	// +openapi:description=创建时间
	CreatedAt string `json:"createdAt"`
}

// AuditLogList is a paginated list of audit logs.
// +openapi:description=审计日志列表
type AuditLogList struct {
	runtime.TypeMeta `json:",inline"`
	Items            []AuditLog `json:"items"`
	TotalCount       int64      `json:"totalCount"`
}

func (a *AuditLogList) GetTypeMeta() *runtime.TypeMeta { return &a.TypeMeta }
