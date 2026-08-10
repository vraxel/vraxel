package iam

import (
	"context"
	"fmt"
	"slices"
	"strconv"

	apierrors "vraxel.io/vraxel/lib/api/errors"
	"vraxel.io/vraxel/lib/rest"
	modstore "vraxel.io/vraxel/pkg/apis/iam/store"
)

// mapKeys returns the keys of a map as a sorted slice.
func mapKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// filterString reads a string filter value from a list.Query filter map
// ("" when absent or non-string) — the typed-Ops replacement for reading
// v1's map[string]string ListOptions.Filters directly.
func filterString(filters map[string]any, key string) string {
	if v, ok := filters[key].(string); ok {
		return v
	}
	return ""
}

// ensureUserExists verifies a user exists by ID, returning a BadRequest error if not.
func ensureUserExists(ctx context.Context, store modstore.UserStore, id int64) error {
	if _, err := store.GetByID(ctx, id); err != nil {
		return apierrors.NewBadRequest(fmt.Sprintf("user %d not found", id), nil)
	}
	return nil
}

// batchAddUsers validates and adds users via the provided addFn.
// Returns the count of successfully added users.
func batchAddUsers(ctx context.Context, ids []string, userStore modstore.UserStore, addFn func(ctx context.Context, uid int64) (bool, error)) (int, error) {
	added := 0
	for _, idStr := range ids {
		uid, err := parseID(idStr)
		if err != nil {
			return 0, apierrors.NewBadRequest(fmt.Sprintf("invalid user ID: %s", idStr), nil)
		}
		if err := ensureUserExists(ctx, userStore, uid); err != nil {
			return 0, err
		}
		ok, err := addFn(ctx, uid)
		if err != nil {
			return 0, err
		}
		if ok {
			added++
		}
	}
	return added, nil
}

// batchRemoveUsers removes users via the provided removeFn. IDs arrive
// pre-parsed from the batch-delete wrapper; per-id removal failures are
// reported in failedIDs rather than aborting the batch.
func batchRemoveUsers(ctx context.Context, ids []int64, removeFn func(ctx context.Context, uid int64) error) (successCount int, failedIDs []string, err error) {
	for _, uid := range ids {
		if removeErr := removeFn(ctx, uid); removeErr != nil {
			failedIDs = append(failedIDs, strconv.FormatInt(uid, 10))
			continue
		}
		successCount++
	}
	return successCount, failedIDs, nil
}

var parseID = rest.ParseID
