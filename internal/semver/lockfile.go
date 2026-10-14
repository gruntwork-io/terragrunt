package semver

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/apparentlymart/go-versions/versions"
	"github.com/apparentlymart/go-versions/versions/constraints"
	goversion "github.com/hashicorp/go-version"
)

// LockFileConstraint returns the `constraints` value tofu writes to a
// dependency lock file for a provider whose `version` argument is s. Each
// term appears once with three version parts, or two for a `~>` declared with
// one or two, sorted by version and then by operator.
//
// Returns an error when tofu rejects s as a provider version constraint.
func LockFileConstraint(s string) (string, error) {
	declared, err := goversion.NewConstraint(s)
	if err != nil {
		return "", fmt.Errorf("invalid constraint %s: %w", s, err)
	}

	spec, err := constraints.ParseRubyStyleMulti(declared.String())
	if err != nil {
		return "", fmt.Errorf("invalid constraint %s: %w", s, err)
	}

	terms := make([]lockFileTerm, 0, len(spec))

	for i := range spec {
		terms = append(terms, newLockFileTerm(&spec[i]))
	}

	slices.SortFunc(terms, func(a, b lockFileTerm) int {
		return cmp.Or(compareLockFileVersions(a.version, b.version), cmp.Compare(a.rank, b.rank))
	})

	terms = slices.CompactFunc(terms, func(a, b lockFileTerm) bool {
		return a.text == b.text
	})

	texts := make([]string, 0, len(terms))
	for _, term := range terms {
		texts = append(texts, term.text)
	}

	return strings.Join(texts, ", "), nil
}

// lockFileOperator is an operator and the prefix tofu writes before its
// version in a lock file.
type lockFileOperator struct {
	prefix   string
	operator constraints.SelectionOp
}

// lockFileOperators lists every operator tofu writes to a lock file, in the
// order it sorts terms that share a version.
var lockFileOperators = []lockFileOperator{
	{prefix: "> ", operator: constraints.OpGreaterThan},
	{prefix: ">= ", operator: constraints.OpGreaterThanOrEqual},
	{prefix: "", operator: constraints.OpEqual},
	{prefix: "~> ", operator: constraints.OpGreaterThanOrEqualPatchOnly},
	{prefix: "~> ", operator: constraints.OpGreaterThanOrEqualMinorOnly},
	{prefix: "<= ", operator: constraints.OpLessThanOrEqual},
	{prefix: "< ", operator: constraints.OpLessThan},
	{prefix: "!= ", operator: constraints.OpNotEqual},
}

// lockFileTerm is one term of a lock file constraint.
type lockFileTerm struct {
	text    string
	version versions.Version
	rank    int
}

// newLockFileTerm writes selection the way tofu writes it to a lock file, with
// omitted version parts as zero.
//
// Panics when the operator of selection is not in lockFileOperators.
// [LockFileConstraint] gets selection from [constraints.ParseRubyStyleMulti],
// which returns an error for any other operator.
func newLockFileTerm(selection *constraints.SelectionSpec) lockFileTerm {
	rank := slices.IndexFunc(lockFileOperators, func(o lockFileOperator) bool {
		return o.operator == selection.Operator
	})
	if rank == -1 {
		panic(fmt.Sprintf("semver: no lock file operator for %s", selection.Operator))
	}

	boundary := selection.Boundary.ConstrainToZero()
	version := versions.Version{
		Major:      boundary.Major.Num,
		Minor:      boundary.Minor.Num,
		Patch:      boundary.Patch.Num,
		Prerelease: versions.VersionExtra(boundary.Prerelease),
		Metadata:   versions.VersionExtra(boundary.Metadata),
	}

	text := fmt.Sprintf("%d.%d.%d", version.Major, version.Minor, version.Patch)
	if selection.Operator == constraints.OpGreaterThanOrEqualMinorOnly {
		text = fmt.Sprintf("%d.%d", version.Major, version.Minor)
	}

	if version.Prerelease != "" {
		text += "-" + version.Prerelease.Raw()
	}

	if version.Metadata != "" {
		text += "+" + version.Metadata.Raw()
	}

	return lockFileTerm{
		version: version,
		text:    lockFileOperators[rank].prefix + text,
		rank:    rank,
	}
}

// compareLockFileVersions orders a and b by precedence, then by build
// metadata.
func compareLockFileVersions(a, b versions.Version) int {
	switch {
	case a.LessThan(b):
		return -1
	case b.LessThan(a):
		return 1
	case a.Metadata == b.Metadata:
		return 0
	case a.Metadata.LessThan(b.Metadata):
		return -1
	}

	return 1
}
