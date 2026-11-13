package stack

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	rewrite "github.com/gruntwork-io/terragrunt/internal/config/rewrite"
	"github.com/gruntwork-io/terragrunt/internal/configbridge"
	"github.com/gruntwork-io/terragrunt/internal/telemetry"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"
	"golang.org/x/text/unicode/norm"

	"github.com/gruntwork-io/terragrunt/internal/runner/runall"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"

	"github.com/gruntwork-io/terragrunt/internal/stacks/clean"
	"github.com/gruntwork-io/terragrunt/internal/stacks/generate"
	"github.com/gruntwork-io/terragrunt/internal/stacks/output"
	"github.com/gruntwork-io/terragrunt/internal/tips"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/worktrees"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// RunGenerate runs the stack command.
func RunGenerate(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *rewrite.Store,
	opts *options.TerragruntOptions,
) error {
	opts.TerragruntStackConfigPath = filepath.Join(opts.WorkingDir, config.DefaultStackFile)

	if opts.NoStackGenerate {
		l.Debugf("Skipping stack generation for %s", opts.TerragruntStackConfigPath)
		return nil
	}

	tips.GiveStackTargetTip(l, v.FS, opts.WorkingDir, opts.Filters, opts.Tips)

	opts.StackAction = "generate"

	// Clean stack folders before calling `generate` when the `--source-update` flag is passed
	if opts.SourceUpdate {
		err := telemetry.TelemeterFromContext(ctx).Collect(ctx, l, "stack_clean", map[string]any{
			"stack_config_path": opts.TerragruntStackConfigPath,
			"working_dir":       opts.WorkingDir,
		}, func(ctx context.Context, l log.Logger) error {
			l.Debugf("Running stack clean for %s, as part of generate command", opts.WorkingDir)
			return clean.CleanStacks(l, v.FS, opts)
		})
		if err != nil {
			return fmt.Errorf(
				"failed to clean stack directories under %q: %w",
				opts.WorkingDir,
				err,
			)
		}
	}

	filters := opts.Filters

	gitFilters := filters.UniqueGitFilters()

	// Only create worktrees when git filter expressions are present
	var wts *worktrees.Worktrees

	if len(gitFilters) > 0 {
		var err error

		wts, err = worktrees.NewWorktrees(ctx, l, v, worktrees.WorktreeOpts{
			WorkingDir:     opts.WorkingDir,
			GitExpressions: gitFilters,
			Experiments:    opts.Experiments,
		})
		if err != nil {
			return fmt.Errorf("failed to create worktrees: %w", err)
		}

		defer func() {
			cleanupErr := wts.Cleanup(ctx, l, v)
			if cleanupErr != nil {
				l.Errorf("failed to cleanup worktrees: %v", cleanupErr)
			}
		}()
	}

	gen := generate.NewGenerator()

	err := telemetry.TelemeterFromContext(ctx).Collect(ctx, l, "stack_generate", map[string]any{
		"stack_config_path": opts.TerragruntStackConfigPath,
		"working_dir":       opts.WorkingDir,
	}, func(ctx context.Context, l log.Logger) error {
		return gen.GenerateStacks(ctx, l, v, s, opts, wts)
	})
	if err != nil {
		return err
	}

	// After generation, hint when a literal stack filter left nested stacks ungenerated.
	funcsFor := configbridge.StackFuncFactory(ctx, l, v, opts)
	tips.GiveStackNestedGenerateTip(
		ctx,
		l,
		v.FS,
		funcsFor,
		opts.WorkingDir,
		opts.Filters,
		opts.Tips,
	)

	return nil
}

// Run executes the stack command.
func Run(ctx context.Context, l log.Logger, v *venv.Venv, s *rewrite.Store, opts *options.TerragruntOptions) error {
	opts.StackAction = "run"

	err := telemetry.TelemeterFromContext(ctx).Collect(ctx, l, "stack_run", map[string]any{
		"stack_config_path": opts.TerragruntStackConfigPath,
		"working_dir":       opts.WorkingDir,
	}, func(ctx context.Context, l log.Logger) error {
		return RunGenerate(ctx, l, v, s, opts)
	})
	if err != nil {
		return err
	}

	return runall.Run(ctx, l, v, s, opts)
}

// RunOutput stack output.
func RunOutput(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *rewrite.Store,
	opts *options.TerragruntOptions,
	index string,
) error {
	opts.StackAction = "output"
	opts.TofuCommand = "output" // required for discovery exclude action matching in StackOutput

	var outputs cty.Value

	// collect outputs
	err := telemetry.TelemeterFromContext(ctx).Collect(ctx, l, "stack_output", map[string]any{
		"stack_config_path": opts.TerragruntStackConfigPath,
		"working_dir":       opts.WorkingDir,
	}, func(ctx context.Context, l log.Logger) error {
		stackOutputs, err := output.StackOutput(ctx, l, v, s, opts)
		outputs = stackOutputs

		return err
	})
	if err != nil {
		return err
	}

	filteredOutputs, err := FilterOutputs(outputs, index)
	if err != nil {
		return err
	}

	// render outputs

	switch opts.StackOutputFormat {
	default:
		if err := PrintOutputs(v.Writers.Writer, filteredOutputs); err != nil {
			return err
		}

	case rawOutputFormat:
		if err := PrintRawOutputs(opts, v.Writers.Writer, filteredOutputs); err != nil {
			return err
		}

	case jsonOutputFormat:
		if err := PrintJSONOutput(v.Writers.Writer, filteredOutputs); err != nil {
			return err
		}
	}

	return nil
}

// FilterOutputs narrows outputs to the value at index, an address such as `vpc.id`,
// `shard["web"].id`, or `shard[0].id`, still wrapped in the objects it was nested in.
// An address that names nothing yields [cty.NilVal], which prints as no output.
func FilterOutputs(outputs cty.Value, index string) (cty.Value, error) {
	if !outputs.IsKnown() || outputs.IsNull() || len(index) == 0 {
		return outputs, nil
	}

	segments, err := parseOutputAddress(index)
	if err != nil {
		return cty.NilVal, err
	}

	current := outputs

	for _, segment := range segments {
		if !current.Type().IsObjectType() && !current.Type().IsMapType() {
			return cty.NilVal, nil
		}

		next, exists := current.AsValueMap()[segment]
		if !exists {
			return cty.NilVal, nil
		}

		current = next
	}

	for _, segment := range slices.Backward(segments) {
		current = cty.ObjectVal(map[string]cty.Value{segment: current})
	}

	return current, nil
}

// parseOutputAddress splits an output address into the segments it names: names joined
// by dots, each optionally followed by bracketed keys, so that one element of an expanded
// unit can be addressed as `shard["web"]` and an iteration key containing a dot stays in
// one piece.
//
// Names are taken as written. A unit or stack name is a block label, so it need not be
// an HCL identifier: `01-network` is a valid name.
func parseOutputAddress(index string) ([]string, error) {
	var segments []string

	for pos := 0; pos < len(index); {
		segment, end, err := readSegment(index, pos)
		if err != nil {
			return nil, InvalidOutputAddressError{Address: index, Err: err}
		}

		segments = append(segments, segment)
		pos = end
	}

	return segments, nil
}

// readSegment returns the segment of address that starts at pos, and where it ends: the
// leading name, a dot and the name after it, or a bracketed key.
func readSegment(address string, pos int) (string, int, error) {
	if pos == 0 {
		return readName(address, pos)
	}

	switch address[pos] {
	case '.':
		return readName(address, pos+1)
	case '[':
		end := pos + indexKeyLen(address[pos:])
		key, err := parseIndexKey(address, pos, end)

		return key, end, err
	default:
		return "", 0, errNameAfterKey
	}
}

// readName returns the name of address that starts at pos, and where it ends.
func readName(address string, pos int) (string, int, error) {
	name := address[pos:]
	if end := strings.IndexAny(name, ".["); end >= 0 {
		name = name[:end]
	}

	if name == "" {
		return "", 0, errEmptyName
	}

	// Either character is the remains of a mistyped key, such as `shard]`.
	if strings.ContainsAny(name, `]"`) {
		return "", 0, fmt.Errorf("name %q contains a bracket or quote", name)
	}

	// cty holds attribute names in NFC.
	return norm.NFC.String(name), pos + len(name), nil
}

var (
	errEmptyName    = errors.New("empty name")
	errNameAfterKey = errors.New("expected a dot or a bracket after a key")
	errNotOneKey    = errors.New("brackets must hold a single key")
)

// indexKeyLen returns the length of the bracketed key that address starts with, skipping
// any closing bracket inside a quoted key. An unclosed key runs to the end of address.
func indexKeyLen(address string) int {
	quoted := false

	for i := 0; i < len(address); i++ {
		switch {
		case quoted && address[i] == '\\':
			i++
		case address[i] == '"':
			quoted = !quoted
		case !quoted && address[i] == ']':
			return i + 1
		}
	}

	return len(address)
}

// parseIndexKey returns the key in the brackets at address[start:end], read as an HCL
// index so that it takes the same quoted strings and numbers as `dependency.foo["web"]`.
func parseIndexKey(address string, start, end int) (string, error) {
	// The placeholder root is as long as what precedes the key, so that diagnostics
	// report columns of the whole address.
	root := strings.Repeat("x", utf8.RuneCountInString(address[:start]))

	traversal, diags := hclsyntax.ParseTraversalAbs([]byte(root+address[start:end]), "", hcl.InitialPos)
	if diags.HasErrors() {
		return "", diags
	}

	// HCL skips comments, so one that hides a quote or bracket from indexKeyLen can
	// leave more than a single key in the range.
	keys := traversal[1:]
	if len(keys) != 1 {
		return "", errNotOneKey
	}

	step, ok := keys[0].(hcl.TraverseIndex)
	if !ok {
		return "", errNotOneKey
	}

	key, err := convert.Convert(step.Key, cty.String)
	if err != nil {
		return "", err
	}

	return key.AsString(), nil
}

// InvalidOutputAddressError reports an output address that cannot be parsed. An address
// that parses but names no unit is not an error; it yields no output.
type InvalidOutputAddressError struct {
	Err     error
	Address string
}

func (err InvalidOutputAddressError) Error() string {
	return fmt.Sprintf("invalid output address %q: %v", err.Address, err.Err)
}

func (err InvalidOutputAddressError) Unwrap() error {
	return err.Err
}

// RunClean recursively removes all stack directories under the specified WorkingDir.
func RunClean(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	opts *options.TerragruntOptions,
) error {
	telemeter := telemetry.TelemeterFromContext(ctx)

	err := telemeter.Collect(ctx, l, "stack_clean", map[string]any{
		"stack_config_path": opts.TerragruntStackConfigPath,
		"working_dir":       opts.WorkingDir,
	}, func(ctx context.Context, l log.Logger) error {
		return clean.CleanStacks(l, v.FS, opts)
	})
	if err != nil {
		return fmt.Errorf("failed to clean stack directories under %q: %w", opts.WorkingDir, err)
	}

	return nil
}
