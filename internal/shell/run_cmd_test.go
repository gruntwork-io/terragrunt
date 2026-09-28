package shell_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/engine"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/shell"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestShellOptionsEngineEnabled(t *testing.T) {
	t.Parallel()

	enabled := experiment.NewExperiments()
	require.NoError(t, enabled.EnableExperiment(experiment.IacEngine))

	testCases := []struct {
		opts *shell.ShellOptions
		name string
		want bool
	}{
		{
			name: "no engine block",
			opts: shell.NewShellOptions(map[string]string{}).WithExperiments(enabled),
			want: false,
		},
		{
			name: "engine block with the experiment on",
			opts: shell.NewShellOptions(map[string]string{}).
				WithExperiments(enabled).
				WithEngine(&engine.EngineConfig{Source: "github.com/example/engine"}, new(engine.EngineOptions)),
			want: true,
		},
		{
			name: "engine block with the experiment off",
			opts: shell.NewShellOptions(map[string]string{}).
				WithExperiments(experiment.NewExperiments()).
				WithEngine(&engine.EngineConfig{Source: "github.com/example/engine"}, new(engine.EngineOptions)),
			want: false,
		},
		{
			name: "engine block disabled with --no-engine",
			opts: shell.NewShellOptions(map[string]string{}).
				WithExperiments(enabled).
				WithEngine(&engine.EngineConfig{Source: "github.com/example/engine"}, &engine.EngineOptions{NoEngine: true}),
			want: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, tc.opts.EngineEnabled())
		})
	}
}
