package compute

import (
	"fmt"
	"net"
	"regexp"
	"strings"

	apierrors "vraxel.io/vraxel/lib/api/errors"
)

// nameRegexp is the host-name rule: alphanumerics, underscore and
// hyphen, 3-50 characters, alphanumeric at both ends. Shared with the
// agent registrar, which normalises reported hostnames to satisfy it.
var nameRegexp = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9_-]{1,48}[a-zA-Z0-9])?$`)

const (
	maxDisplayNameLen = 128
	maxDescriptionLen = 1000
)

// validateHostCreate checks a hand-entered host.
//
// The address is optional on purpose: a host that will only ever be
// reached through an outbound agent has no address anything here would
// dial, and requiring one would make operators invent a value that no
// code reads. When present it must parse, because a malformed address is
// a typo rather than a choice.
func validateHostCreate(h *Host) error {
	name := strings.TrimSpace(h.Metadata.Name)
	if name == "" {
		return apierrors.NewBadRequest("name is required", nil)
	}
	if !nameRegexp.MatchString(name) {
		return apierrors.NewBadRequest(
			"name must be 3-50 characters of letters, digits, underscore or hyphen, starting and ending alphanumeric", nil)
	}
	if len(h.Spec.DisplayName) > maxDisplayNameLen {
		return apierrors.NewBadRequest(fmt.Sprintf("displayName must be at most %d characters", maxDisplayNameLen), nil)
	}
	if len(h.Spec.Description) > maxDescriptionLen {
		return apierrors.NewBadRequest(fmt.Sprintf("description must be at most %d characters", maxDescriptionLen), nil)
	}
	if ip := strings.TrimSpace(h.Spec.IP); ip != "" && net.ParseIP(ip) == nil {
		return apierrors.NewBadRequest(fmt.Sprintf("ip %q is not a valid address", ip), nil)
	}
	if h.Spec.SSHPort != 0 && (h.Spec.SSHPort < 1 || h.Spec.SSHPort > 65535) {
		return apierrors.NewBadRequest("sshPort must be between 1 and 65535", nil)
	}
	return nil
}

// validateHostUpdate checks the two fields an operator owns. The name is
// not among them: it is the host's stable identifier, and renaming it
// would break every reference held elsewhere for a cosmetic gain that
// displayName already provides.
func validateHostUpdate(h *Host) error {
	if len(h.Spec.DisplayName) > maxDisplayNameLen {
		return apierrors.NewBadRequest(fmt.Sprintf("displayName must be at most %d characters", maxDisplayNameLen), nil)
	}
	if len(h.Spec.Description) > maxDescriptionLen {
		return apierrors.NewBadRequest(fmt.Sprintf("description must be at most %d characters", maxDescriptionLen), nil)
	}
	return nil
}
