// Package semver consolidates Terragrunt's version handling behind a single
// API so callers pick a function that matches their use case rather than
// picking a library.
//
// # When to use what
//
// Use [Parse] for a version string written by a tool: an `OpenTofu v1.9.0`
// banner, a provider version in a lock file, a `git version` line. It is the
// lenient reading, and it accepts shapes the semantic versioning spec does
// not, such as a two-part `1.9` or a prerelease with no separator. Most call
// sites want this, because most of these strings are not under Terragrunt's
// control.
//
// Use [IsExact] to decide whether a string names one specific release rather
// than a moving target. It holds only for a full three-part version, so `v1`
// and `v1.2` are rejected. Ask it of a Git tag or a module version before
// treating what it points at as fixed: a maintainer moves `v1` and does
// not move `v1.2.3`.
//
// Use [ParseConstraint] and [Constraints.Check] to test a version against a
// range such as `>= 1.2, < 2.0`.
//
// Use [CheckConstraint] for the semver_check HCL function, which reads its
// version argument more strictly than [Parse] does.
//
// # Backing libraries
//
// [Parse], [MustParse] and [ParseConstraint] are [go-version], which is
// forgiving about partial and unseparated versions. [IsExact] is
// [Masterminds/semver], whose strict parser is the only one of the two that
// tells `v1` from `v1.0.0`: go-version pads the missing parts and does not
// expose how many were given.
//
// [go-version]: https://pkg.go.dev/github.com/hashicorp/go-version
// [Masterminds/semver]: https://pkg.go.dev/github.com/Masterminds/semver/v3
package semver

import (
	"fmt"
	"strings"

	masterminds "github.com/Masterminds/semver/v3"
	goversion "github.com/hashicorp/go-version"
)

// Version is one parsed version. Compare two with [Version.Compare],
// [Version.LessThan] and their siblings.
type Version = goversion.Version

// Constraints is a parsed version range, such as the `>= 1.2, < 2.0` a
// terraform_version_constraint carries. Test a version with
// [Constraints.Check].
type Constraints = goversion.Constraints

// Parse reads s as a version, accepting the partial and unseparated shapes
// tools write: `1.9`, `v1.9.0`, and `1.9.0beta` all parse. Use [IsExact]
// instead when a partial version must not pass.
func Parse(s string) (*Version, error) {
	return goversion.NewVersion(s)
}

// MustParse is [Parse] for a version string fixed in the source, and panics
// when it does not parse.
func MustParse(s string) *Version {
	return goversion.Must(goversion.NewVersion(s))
}

// ParseConstraint reads s as a version range such as `>= 1.2, < 2.0`.
func ParseConstraint(s string) (Constraints, error) {
	return goversion.NewConstraint(s)
}

// IsExact reports whether s names one specific release: three dot-separated
// numbers, with an optional leading `v` and optional pre-release and build
// suffixes. A partial version such as `v1` or `v1.2` does not qualify, nor
// does a range, a constraint, or a number with a leading zero.
//
// The leading `v` is trimmed before parsing because a Git tag conventionally
// carries one and a semantic version does not.
func IsExact(s string) bool {
	_, err := masterminds.StrictNewVersion(strings.TrimPrefix(s, "v"))

	return err == nil
}

// CheckConstraint reports whether version satisfies constraint, reading
// version strictly enough to require a separator before any pre-release, so
// `1.2.3beta` is rejected where [Parse] accepts it. This backs the
// semver_check HCL function, whose accepted input is user-facing.
func CheckConstraint(version, constraint string) (bool, error) {
	v, err := goversion.NewSemver(version)
	if err != nil {
		return false, fmt.Errorf("invalid version %s: %w", version, err)
	}

	c, err := goversion.NewConstraint(constraint)
	if err != nil {
		return false, fmt.Errorf("invalid constraint %s: %w", constraint, err)
	}

	return c.Check(v), nil
}
