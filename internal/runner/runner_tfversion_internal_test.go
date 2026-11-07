package runner

import (
	"testing"

	"github.com/stretchr/testify/assert"

	semver "github.com/gruntwork-io/terragrunt/internal/semver"
	"github.com/gruntwork-io/terragrunt/internal/tfimpl"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// TestApplyUnitTFVersion_ClearsInheritedVersionForUnknownEngine pins that a unit whose version
// check detected no version (e.g. an engine-enabled unit) does not keep the version it inherited
// from the stack options.
func TestApplyUnitTFVersion_ClearsInheritedVersionForUnknownEngine(t *testing.T) {
	t.Parallel()

	rnr := &Runner{}
	rnr.recordUnitTFVersion("/units/engine", nil, tfimpl.Unknown)

	unitOpts := &options.TerragruntOptions{
		TerraformVersion:   semver.MustParse("1.9.0"),
		TofuImplementation: tfimpl.OpenTofu,
	}

	rnr.applyUnitTFVersion("/units/engine", unitOpts)

	assert.Nil(t, unitOpts.TerraformVersion)
	assert.Equal(t, tfimpl.Unknown, unitOpts.TofuImplementation)
}
