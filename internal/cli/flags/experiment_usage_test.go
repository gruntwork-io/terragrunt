package flags_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli/flags/shared"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const gatedUsage = "Do the thing."

func TestExperimentUsageWhileOngoing(t *testing.T) {
	t.Parallel()

	exps := experiment.Experiments{{Name: "an-experiment", Status: experiment.StatusOngoing}}

	assert.Equal(t,
		gatedUsage+" Requires the 'an-experiment' experiment.",
		flags.ExperimentUsage(exps, "an-experiment", gatedUsage))
}

// TestExperimentUsageDropsNoteOnGraduation pins the reason the note is built
// rather than written: moving the experiment to completed is the only edit
// graduation needs, so no flag is left advertising a gate that is gone.
func TestExperimentUsageDropsNoteOnGraduation(t *testing.T) {
	t.Parallel()

	exps := experiment.Experiments{{Name: "an-experiment", Status: experiment.StatusCompleted}}

	assert.Equal(t, gatedUsage, flags.ExperimentUsage(exps, "an-experiment", gatedUsage))
}

func TestExperimentUsageUnknownExperiment(t *testing.T) {
	t.Parallel()

	assert.Equal(t, gatedUsage, flags.ExperimentUsage(experiment.Experiments{}, "absent", gatedUsage))
}

// TestGatedFlagUsageFollowsExperimentStatus pins that the flags actually
// gated on offline-cas build their usage through [flags.ExperimentUsage],
// so the note tracks the experiment instead of a hand-written string.
func TestGatedFlagUsageFollowsExperimentStatus(t *testing.T) {
	t.Parallel()

	gated := []string{
		shared.CASOfflineFlagName,
		shared.CASRefreshFlagName,
		shared.CASProbeTTLFlagName,
	}

	t.Run("ongoing", func(t *testing.T) {
		t.Parallel()

		opts := options.NewTerragruntOptions(vexec.NewOSExec())
		casFlags := shared.NewCASFlags(opts, flags.Prefix{})

		for _, name := range gated {
			f := casFlags.Get(name)
			require.NotNil(t, f, name)
			assert.Contains(t, f.GetUsage(), "Requires the 'offline-cas' experiment.", name)
		}
	})

	t.Run("completed", func(t *testing.T) {
		t.Parallel()

		opts := options.NewTerragruntOptions(vexec.NewOSExec())

		exp := opts.Experiments.Find(experiment.OfflineCAS)
		require.NotNil(t, exp)
		exp.Status = experiment.StatusCompleted

		casFlags := shared.NewCASFlags(opts, flags.Prefix{})

		for _, name := range gated {
			f := casFlags.Get(name)
			require.NotNil(t, f, name)
			assert.NotContains(t, strings.ToLower(f.GetUsage()), "experiment", name)
		}
	})
}
