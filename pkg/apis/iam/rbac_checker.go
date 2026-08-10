package iam

import modstore "vraxel.io/vraxel/pkg/apis/iam/store"
import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

const rbacCacheTTL = 5 * time.Second

// cacheEntry holds a cached UserPermissionEntry with an expiry timestamp.
type cacheEntry struct {
	entry     *UserPermissionEntry
	expiresAt time.Time
}

// PermissionChecker defines the interface for RBAC permission checking.
type PermissionChecker interface {
	// CheckPermission checks whether a user has the given permission at the specified scope.
	CheckPermission(ctx context.Context, userID int64, permCode string, scope string, workspaceID, namespaceID int64) (bool, error)
	// IsPlatformAdmin checks whether a user has the platform-admin super permission (*:*).
	IsPlatformAdmin(ctx context.Context, userID int64) (bool, error)
	// GetAccessibleWorkspaceIDs returns workspace IDs the user has any role binding for.
	GetAccessibleWorkspaceIDs(ctx context.Context, userID int64) ([]int64, error)
	// GetAccessibleNamespaceIDs returns namespace IDs the user has any role binding for.
	GetAccessibleNamespaceIDs(ctx context.Context, userID int64) ([]int64, error)
}

// UserPermissionEntry holds permission rules for a single user,
// organized by scope level for efficient matching.
type UserPermissionEntry struct {
	IsPlatformAdmin bool               // true if platformRules contains "*:*" (fast short-circuit)
	PlatformRules   []string           // patterns from platform-scoped bindings
	WorkspaceRules  map[int64][]string // workspaceID → patterns
	NamespaceRules  map[int64][]string // namespaceID → patterns
}

// HasPermission checks whether this entry grants the given permission code
// at the specified scope/resource level, following scope chain inheritance:
// platform rules apply everywhere, workspace rules apply to workspace and its namespaces.
func (e *UserPermissionEntry) HasPermission(code, scope string, wsID, nsID int64) bool {
	// 1. Platform-level rules apply to all scopes
	if hasPermissionMatchRules(e.PlatformRules, code) {
		return true
	}
	// 2. Workspace-level rules apply to workspace and namespace scopes
	if (scope == modstore.ScopeWorkspace || scope == modstore.ScopeNamespace) && wsID > 0 {
		if hasPermissionMatchRules(e.WorkspaceRules[wsID], code) {
			return true
		}
	}
	// 3. Namespace-level rules apply to namespace scope only
	if scope == modstore.ScopeNamespace && nsID > 0 {
		if hasPermissionMatchRules(e.NamespaceRules[nsID], code) {
			return true
		}
	}
	return false
}

// hasPermissionMatchRules reports whether any pattern in rules matches code.
func hasPermissionMatchRules(rules []string, code string) bool {
	for _, pattern := range rules {
		if MatchPermission(pattern, code) {
			return true
		}
	}
	return false
}

// HasAnyPermission checks whether this entry grants any of the given permission codes.
// Uses bidirectional matching to support wildcard targets (e.g. "infra:hosts:*"):
//   - Forward: does a user's rule pattern match the target as a code?
//   - Reverse: does the target as a pattern match a user's rule as a code?
//
// This allows PermissionTargets like "infra:hosts:*" to be satisfied by a user
// who has any specific infra:hosts permission (e.g. "infra:hosts:create"),
// or by a broader wildcard rule (e.g. "infra:*").
func (e *UserPermissionEntry) HasAnyPermission(targets []string, scope string, wsID, nsID int64) bool {
	for _, target := range targets {
		if e.hasAnyPermissionTarget(target, scope, wsID, nsID) {
			return true
		}
	}
	return false
}

// hasAnyPermissionTarget reports whether a single target is satisfied by this
// entry's scope-chain rules, using bidirectional matching (see HasAnyPermission).
func (e *UserPermissionEntry) hasAnyPermissionTarget(target, scope string, wsID, nsID int64) bool {
	// 1. Platform-level rules apply to all scopes
	if hasAnyPermissionMatchRules(e.PlatformRules, target) {
		return true
	}
	// 2. Workspace-level rules apply to workspace and namespace scopes
	if (scope == modstore.ScopeWorkspace || scope == modstore.ScopeNamespace) && wsID > 0 {
		if hasAnyPermissionMatchRules(e.WorkspaceRules[wsID], target) {
			return true
		}
	}
	// 3. Namespace-level rules apply to namespace scope only
	if scope == modstore.ScopeNamespace && nsID > 0 {
		if hasAnyPermissionMatchRules(e.NamespaceRules[nsID], target) {
			return true
		}
	}
	return false
}

// hasAnyPermissionMatchRules reports whether any rule matches target
// bidirectionally (rule-as-pattern vs target, or target-as-pattern vs rule).
func hasAnyPermissionMatchRules(rules []string, target string) bool {
	for _, rule := range rules {
		if MatchPermission(rule, target) || MatchPermission(target, rule) {
			return true
		}
	}
	return false
}

// RBACChecker implements PermissionChecker backed by modstore.RoleBindingStore.
// Uses a short-lived TTL cache + singleflight to reduce DB queries per request.
type RBACChecker struct {
	rbStore modstore.RoleBindingStore
	sfGroup singleflight.Group
	cache   sync.Map // map[int64]cacheEntry
}

// NewRBACChecker creates a new checker with the given store.
func NewRBACChecker(rbStore modstore.RoleBindingStore) *RBACChecker {
	return &RBACChecker{rbStore: rbStore}
}

// InvalidateUser removes the cached permissions for a user,
// forcing a fresh database load on the next permission check.
func (c *RBACChecker) InvalidateUser(userID int64) {
	c.cache.Delete(userID)
}

func (c *RBACChecker) CheckPermission(ctx context.Context, userID int64, permCode string, scope string, workspaceID, namespaceID int64) (bool, error) {
	entry, err := c.loadEntry(ctx, userID)
	if err != nil {
		return false, err
	}
	return entry.HasPermission(permCode, scope, workspaceID, namespaceID), nil
}

func (c *RBACChecker) CheckAnyPermission(ctx context.Context, userID int64, permCodes []string, scope string, workspaceID, namespaceID int64) (bool, error) {
	entry, err := c.loadEntry(ctx, userID)
	if err != nil {
		return false, err
	}
	return entry.HasAnyPermission(permCodes, scope, workspaceID, namespaceID), nil
}

func (c *RBACChecker) IsPlatformAdmin(ctx context.Context, userID int64) (bool, error) {
	entry, err := c.loadEntry(ctx, userID)
	if err != nil {
		return false, err
	}
	return entry.IsPlatformAdmin, nil
}

func (c *RBACChecker) GetAccessibleWorkspaceIDs(ctx context.Context, userID int64) ([]int64, error) {
	return c.rbStore.GetAccessibleWorkspaceIDs(ctx, userID)
}

func (c *RBACChecker) GetAccessibleNamespaceIDs(ctx context.Context, userID int64) ([]int64, error) {
	return c.rbStore.GetAccessibleNamespaceIDs(ctx, userID)
}

// loadEntry loads permission rules for a user, using a short-lived TTL cache
// to avoid hitting the database on every request. On cache miss, singleflight
// deduplicates concurrent loads for the same user.
func (c *RBACChecker) loadEntry(ctx context.Context, userID int64) (*UserPermissionEntry, error) {
	// Check cache first
	if v, ok := c.cache.Load(userID); ok {
		ce := v.(cacheEntry)
		if time.Now().Before(ce.expiresAt) {
			return ce.entry, nil
		}
		// Expired — fall through to reload
	}

	key := strconv.FormatInt(userID, 10)
	v, err, _ := c.sfGroup.Do(key, func() (any, error) {
		entry, loadErr := c.loadUserEntry(ctx, userID)
		if loadErr != nil {
			return nil, loadErr
		}
		c.cache.Store(userID, cacheEntry{
			entry:     entry,
			expiresAt: time.Now().Add(rbacCacheTTL),
		})
		return entry, nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*UserPermissionEntry), nil
}

// loadUserEntry loads all permission rules for a user from the database
// and organizes them into a UserPermissionEntry by scope.
func (c *RBACChecker) loadUserEntry(ctx context.Context, userID int64) (*UserPermissionEntry, error) {
	rows, err := c.rbStore.LoadUserPermissionRules(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("load permission rules for user %d: %w", userID, err)
	}

	entry := &UserPermissionEntry{
		WorkspaceRules: make(map[int64][]string),
		NamespaceRules: make(map[int64][]string),
	}

	for _, row := range rows {
		switch row.Scope {
		case modstore.ScopePlatform:
			entry.PlatformRules = append(entry.PlatformRules, row.Pattern)
			if row.Pattern == "*:*" {
				entry.IsPlatformAdmin = true
			}
		case modstore.ScopeWorkspace:
			if row.WorkspaceID != nil {
				entry.WorkspaceRules[*row.WorkspaceID] = append(entry.WorkspaceRules[*row.WorkspaceID], row.Pattern)
			}
		case modstore.ScopeNamespace:
			if row.NamespaceID != nil {
				entry.NamespaceRules[*row.NamespaceID] = append(entry.NamespaceRules[*row.NamespaceID], row.Pattern)
			}
		}
	}

	return entry, nil
}
