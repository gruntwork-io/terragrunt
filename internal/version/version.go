// Package version exposes the Terragrunt binary version string.
// Version is set at build time via:
//
//	-ldflags "-X github.com/gruntwork-io/terragrunt/internal/version.Version=<tag>"
package version

// Version is the Terragrunt binary version. It defaults to "latest" for
// `go run`/local builds and is overwritten by release builds via ldflags.
var Version = "latest"

// GetVersion returns the current Terragrunt version string.
func GetVersion() string {
	return Version
}
