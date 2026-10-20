package runner

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// New discovers all Terragrunt units under the working directory and assembles them
// into a [Runner] that can apply or destroy them.
func New(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	opts *options.TerragruntOptions,
	runnerOpts ...Option,
) (*Runner, error) {
	discovered, err := discoverWithRetry(ctx, l, v, opts, runnerOpts...)
	if err != nil {
		return nil, err
	}

	rnr, err := createRunner(ctx, l, opts, discovered)
	if err != nil {
		return nil, err
	}

	setUnitPathLabels(l, opts, rnr.GetStack().Units)

	if err := checkVersionConstraints(ctx, l, v, opts, rnr.GetStack().Units); err != nil {
		return nil, err
	}

	return rnr, nil
}

// setUnitPathLabels passes the labels of the run's units to the log formatter, so the `basename`
// path format of `%prefix` can tell the units apart from the first log line on.
func setUnitPathLabels(l log.Logger, opts *options.TerragruntOptions, units []*component.Unit) {
	formatter := l.Formatter()
	if formatter == nil {
		return
	}

	paths := make([]string, 0, len(units))
	for _, unit := range units {
		paths = append(paths, unit.Path())
	}

	formatter.SetPathLabels(UnitPathLabels(resolveWorkingDir(opts), paths))
}
