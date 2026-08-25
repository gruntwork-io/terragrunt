package mcp

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// ErrUnknownCapability is returned when --allow names something the server
// does not grant.
var ErrUnknownCapability = errors.New("unknown capability")

// Capability is one thing the server may do that it denies until an
// operator asks for it.
type Capability string

const (
	// CapabilityExec lets the server run the commands Terragrunt runs itself.
	CapabilityExec Capability = "exec"
	// CapabilityHTTP lets the server make the requests Terragrunt makes itself.
	CapabilityHTTP Capability = "http"
	// CapabilitySops lets the server decrypt SOPS-encrypted files.
	CapabilitySops Capability = "sops"
)

// allCapabilities is every capability --allow accepts, in the order the error
// for an unknown one lists them.
var allCapabilities = []Capability{CapabilityExec, CapabilityHTTP, CapabilitySops}

// Capabilities is the set an operator granted.
type Capabilities map[Capability]bool

// Has reports whether c was granted.
func (caps Capabilities) Has(c Capability) bool {
	return caps[c]
}

// ParseCapabilities turns the values of --allow into a set. An unknown name is
// an error rather than a no-op, because a typo that silently granted nothing
// would leave an operator believing they had opened something up.
func ParseCapabilities(names []string) (Capabilities, error) {
	caps := Capabilities{}

	for _, name := range names {
		c := Capability(strings.TrimSpace(name))

		if !slices.Contains(allCapabilities, c) {
			return nil, fmt.Errorf(
				"%w %q: expected one of %s",
				ErrUnknownCapability,
				name,
				capabilityNames(),
			)
		}

		caps[c] = true
	}

	return caps, nil
}

// capabilityNames renders the accepted values for an error message.
func capabilityNames() string {
	names := make([]string, 0, len(allCapabilities))
	for _, c := range allCapabilities {
		names = append(names, string(c))
	}

	return strings.Join(names, ", ")
}
