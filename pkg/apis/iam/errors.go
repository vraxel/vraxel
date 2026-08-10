package iam

import apierrors "vraxel.io/vraxel/lib/api/errors"

// domainErr converts a store-layer pgerrors sentinel into its HTTP
// StatusError. Business-layer callers invoke it at REST boundaries so
// "row not found" / "constraint violation" / etc. surface as 404 / 409
// instead of leaking through as 500. Unknown errors pass through
// unchanged so the REST framework still handles them.
//
// Mirrors the per-module pattern in pkg/apis/{db,mq,mw,...}/errors.go.
// iam's stores wrap sentinels with messages like
// "namespace 12: not found" / "role 7: not found", so the empty
// resource string is fine for the not-found path; the resource arg
// only enriches pg-error branches that iam's stores rarely surface.
func domainErr(err error) error {
	if err == nil {
		return nil
	}
	if se := apierrors.FromDomain(err, ""); se != nil {
		return se
	}
	return err
}
