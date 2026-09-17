package shell_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/shell"
	"github.com/stretchr/testify/assert"
)

func TestLastReleaseTag(t *testing.T) {
	t.Parallel()

	var tags = []string{
		"refs/tags/v0.0.1",
		"refs/tags/v0.0.2",
		"refs/tags/v0.10.0",
		"refs/tags/v20.0.1",
		"refs/tags/v0.3.1",
		"refs/tags/v20.1.2",
		"refs/tags/v0.5.1",
	}

	lastTag := shell.LastReleaseTag(tags)
	assert.NotEmpty(t, lastTag)
	assert.Equal(t, "v20.1.2", lastTag)
}
