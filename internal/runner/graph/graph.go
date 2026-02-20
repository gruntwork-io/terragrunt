// Package graph implements the logic for running commands against the
// graph of dependencies for the unit in the current working directory.
package graph

import (
	"context"
	"errors"
	"fmt"

	"github.com/gruntwork-io/terragrunt/internal/configbridge"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/runner"
	"github.com/gruntwork-io/terragrunt/internal/runner/common"
	"github.com/gruntwork-io/terragrunt/internal/runner/run/creds"
	"github.com/gruntwork-io/terragrunt/internal/runner/runall"

	"github.com/gruntwork-io/terragrunt/internal/os/stdout"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"

	"github.com/gruntwork-io/terragrunt/internal/shell"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

func Run(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) error {
	// Get credentials BEFORE config parsing — sops_decrypt_file() and
	// get_aws_account_id() in locals need auth-provider credentials
	// available in opts.Env during HCL evaluation.
	// *Getter discarded: graph.Run only needs creds in opts.Env for initial config parse.
	// Per-unit creds are re-fetched in runnerpool task (intentional: each unit may have
	// different opts after clone).
	if _, err := creds.ObtainCredsForParsing(ctx, l, opts.AuthProviderCmd, opts.Env, configbridge.ShellRunOptsFromOpts(opts)); err != nil {
		return err
	}

	ctx, pctx := configbridge.NewParsingContext(ctx, l, opts)

	cfg, err := config.ReadTerragruntConfig(ctx, l, pctx, pctx.ParserOptions)
	if err != nil {
		return err
	}

	if cfg == nil {
		return errors.New("terragrunt was not able to render the config as json because it received no config. This is almost certainly a bug in Terragrunt. Please open an issue on github.com/gruntwork-io/terragrunt with this message and the contents of your terragrunt.hcl")
	}
	// consider root for graph identification passed destroy-graph-root argument
	rootDir := opts.GraphRoot

	// if destroy-graph-root is empty, use git to find top level dir.
	// may cause issues if in the same repo exist unrelated modules which will generate errors when scanning.
	if rootDir == "" {
		gitRoot, gitRootErr := shell.GitTopLevelDir(ctx, l, opts.Env, opts.WorkingDir)
		if gitRootErr != nil {
			return gitRootErr
		}

		rootDir = gitRoot
	}

	// Clone options and set RootWorkingDir to rootDir so discovery starts from the graph root
	// This allows discovering all modules including dependents (modules that depend on the working dir)
	graphOpts := opts.Clone()
	graphOpts.RootWorkingDir = rootDir

	runnerOpts := make([]common.Option, 0, 1)

	r := report.NewReport().WithWorkingDir(opts.WorkingDir)

	if l.Formatter().DisabledColors() || stdout.IsRedirected() {
		r.WithDisableColor()
	}

	if opts.ReportFormat != "" {
		r.WithFormat(opts.ReportFormat)
	}

	if opts.SummaryPerUnit {
		r.WithShowUnitLevelSummary()
	}

	// Limit graph to the working directory and its dependents.
	// The prefix ellipsis means "include dependents"; target is included by default.
	pathExpr, err := filter.NewPathFilter(opts.WorkingDir)
	if err != nil {
		return fmt.Errorf("failed to create path filter for %s: %w", opts.WorkingDir, err)
	}

	graphExpr := filter.NewGraphExpression(pathExpr).WithDependents()
	graphOpts.Filters = filter.Filters{filter.NewFilter(graphExpr, graphExpr.String())}

	if opts.ReportSchemaFile != "" {
		defer r.WriteSchemaToFile(opts.ReportSchemaFile) //nolint:errcheck
	}

	if opts.ReportFile != "" {
		defer r.WriteToFile(opts.ReportFile) //nolint:errcheck
	}

	if !opts.SummaryDisable {
		defer r.WriteSummary(opts.Writer) //nolint:errcheck
	}

	rnr, err := runner.NewStackRunner(ctx, l, graphOpts, runnerOpts...)
	if err != nil {
		return err
	}

	return runall.RunAllOnStack(ctx, l, graphOpts, rnr, r)
}
