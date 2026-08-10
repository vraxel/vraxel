package iam

import (
	"fmt"
	"strings"

	apierrors "vraxel.io/vraxel/lib/api/errors"
	modstore "vraxel.io/vraxel/pkg/apis/iam/store"
)

// blockingResourceError builds a 409 Conflict StatusError that lists the
// blocking child resources preventing a workspace / namespace from being
// deleted. Returns nil when rows is empty (i.e. scope is safe to delete).
//
// Message format is "cannot delete <scope>: <kind>(<count>), ...". The
// "cannot delete <scope>" prefix is the stable contract the frontend
// matches against messagePrefixMap for i18n; the comma-separated suffix
// is what extractMessageSuffix appends to the localized template.
func blockingResourceError(scope string, rows []modstore.BlockingResourceRow) error {
	if len(rows) == 0 {
		return nil
	}
	parts := make([]string, 0, len(rows))
	for _, r := range rows {
		parts = append(parts, fmt.Sprintf("%s(%d)", r.Kind, r.Count))
	}
	msg := fmt.Sprintf("cannot delete %s: %s", scope, strings.Join(parts, ", "))
	return apierrors.NewConflictMessage(msg)
}
