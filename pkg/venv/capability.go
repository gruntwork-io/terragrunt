package venv

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// ErrUnknownCapability is returned when a capability name is not one the
// caller accepts.
var ErrUnknownCapability = errors.New("unknown capability")

// Capability is one thing a [Restrict] venv refuses until a caller grants it.
type Capability string

const (
	// CapabilityExec lets the venv run the commands Terragrunt runs itself,
	// such as tofu and the git that downloads a source. A program a
	// configuration names also needs an [AllowCmd].
	CapabilityExec Capability = "exec"
	// CapabilityHTTP lets the venv make the requests Terragrunt makes itself.
	CapabilityHTTP Capability = "http"
	// CapabilitySops lets the venv decrypt SOPS-encrypted files.
	CapabilitySops Capability = "sops"
	// CapabilityEnv lets the venv keep the environment variables of the venv
	// it narrows.
	CapabilityEnv Capability = "env"
)

// Capabilities is a set of granted capabilities.
type Capabilities map[Capability]struct{}

// Has reports whether c was granted.
func (caps Capabilities) Has(c Capability) bool {
	_, ok := caps[c]

	return ok
}

// RestrictCapabilities returns every capability [Restrict] governs, in the
// order an error for an unknown one lists them. A caller that grants more of
// its own appends them before passing the list to [ParseCapabilities].
func RestrictCapabilities() []Capability {
	return []Capability{CapabilityExec, CapabilityHTTP, CapabilitySops, CapabilityEnv}
}

// ParseCapabilities turns the values of an --allow flag into a set, accepting
// only the names in accepted. Surrounding space is ignored, so
// "--allow=exec, http" grants both.
//
// Returns [ErrUnknownCapability] for any other name, rather than ignoring it,
// because a typo that silently granted nothing would leave an operator
// believing they had opened something up.
func ParseCapabilities(names []string, accepted []Capability) (Capabilities, error) {
	caps := Capabilities{}

	for _, name := range names {
		c := Capability(strings.TrimSpace(name))

		if !slices.Contains(accepted, c) {
			return nil, fmt.Errorf(
				"%w %q: expected one of %s",
				ErrUnknownCapability,
				name,
				capabilityNames(accepted),
			)
		}

		caps[c] = struct{}{}
	}

	return caps, nil
}

// capabilityNames renders the accepted values for an error message.
func capabilityNames(accepted []Capability) string {
	names := make([]string, 0, len(accepted))
	for _, c := range accepted {
		names = append(names, string(c))
	}

	return strings.Join(names, ", ")
}
