package main

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/hashicorp/go-version"
	"github.com/stretchr/testify/assert"
)

func TestVersionString(t *testing.T) {
	t.Parallel()

	assert.Empty(t, versionString(nil))
	assert.Empty(t, versionString(&options.TerragruntOptions{}))

	opts := &options.TerragruntOptions{
		TerragruntVersion: version.Must(version.NewVersion("1.7.9")),
	}
	assert.Equal(t, "1.7.9", versionString(opts))
}
