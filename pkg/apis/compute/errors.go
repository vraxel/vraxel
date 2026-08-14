package compute

import (
	"strconv"
	"time"

	apierrors "vraxel.io/vraxel/lib/api/errors"
	apitypes "vraxel.io/vraxel/lib/api/types"
)

// domainErr maps a store-layer sentinel onto the HTTP status the REST
// layer returns. Anything unrecognised passes through and surfaces as a
// 500, which is the right answer for a fault nobody anticipated.
func domainErr(err error) error {
	if err == nil {
		return nil
	}
	if se := apierrors.FromDomain(err, "host"); se != nil {
		return se
	}
	return err
}

// apiObjectMeta builds the metadata envelope shared by every resource in
// this module.
func apiObjectMeta(id int64, name string, createdAt, updatedAt *time.Time) apitypes.ObjectMeta {
	return apitypes.ObjectMeta{
		ID:        strconv.FormatInt(id, 10),
		Name:      name,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}
}
