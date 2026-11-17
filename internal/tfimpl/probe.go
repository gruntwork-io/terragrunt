package tfimpl

import "github.com/gruntwork-io/terragrunt/internal/semver"

// Probe is what the version command of an OpenTofu/Terraform binary reported.
type Probe struct {
	// Version is nil when no probe ran, or when an engine runs the commands.
	Version *semver.Version
	// Type is [Unknown] when no probe ran, when an engine runs the commands,
	// or when the version output names neither tool.
	Type Type
}

// Unprobed returns the [Probe] of a binary whose version command never ran.
func Unprobed() Probe {
	return Probe{Type: Unknown}
}
