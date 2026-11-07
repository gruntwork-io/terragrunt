package global_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/flags/global"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDependencyStackGenerationRequiresOptIn(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		args    []string
		env     map[string]string
		enabled bool
	}{
		{name: "default", env: map[string]string{}},
		{name: "CLI", args: []string{"--experiment", "dependency-stack-generation"}, env: map[string]string{}, enabled: true},
		{name: "environment", env: map[string]string{"TG_EXPERIMENT": "dependency-stack-generation"}, enabled: true},
		{name: "all experiments CLI", args: []string{"--experiment-mode"}, env: map[string]string{}, enabled: true},
		{name: "all experiments environment", env: map[string]string{"TG_EXPERIMENT_MODE": "true"}, enabled: true},
		{name: "unrelated experiment", args: []string{"--experiment", "stack-dependencies"}, env: map[string]string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			opts := options.NewTerragruntOptions(vexec.NewOSExec())
			require.NoError(t, global.NewFlags(logger.CreateLogger(), opts, nil).Parse(tc.args, tc.env))

			registered := opts.Experiments.Find(experiment.DependencyStackGeneration)
			require.NotNil(t, registered)
			assert.Equal(t, experiment.StatusOngoing, registered.Status)
			assert.Equal(t, tc.enabled, opts.Experiments.Evaluate(experiment.DependencyStackGeneration))
		})
	}
}
