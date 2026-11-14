package stack

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	rewrite "github.com/gruntwork-io/terragrunt/internal/config/rewrite"
	"github.com/gruntwork-io/terragrunt/internal/configbridge"
	"github.com/gruntwork-io/terragrunt/internal/telemetry"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
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
		if current.IsNull() {
			return cty.NilVal, nil
		}

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
// A name need not be an HCL identifier, because a unit or stack name is a block label:
// `1-vpc` is a valid name.
func parseOutputAddress(index string) ([]string, error) {
	if len(index) > MaxOutputAddressLen {
		return nil, InvalidOutputAddressError{Address: index, Err: errAddressTooLong}
	}

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
		return readKey(address, pos)
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
		return "", 0, errKeySyntaxInName
	}

	// cty holds attribute names in NFC.
	return norm.NFC.String(name), pos + len(name), nil
}

var (
	errAddressTooLong  = fmt.Errorf("longer than %d bytes", MaxOutputAddressLen)
	errEmptyName       = errors.New("empty name")
	errNameAfterKey    = errors.New("expected a dot or a bracket after a key")
	errKeySyntaxInName = errors.New("a name cannot contain a closing bracket or a quote")
	errKeyNotAnIndex   = errors.New("a number key must be a whole number that fits in 64 bits")
)

// MaxOutputAddressLen is the longest output address, in bytes, that [FilterOutputs]
// reads.
//
// It bounds the time spent reading an address: each key is lexed along with everything
// after it, which is quadratic in the number of keys.
const MaxOutputAddressLen = 4096

// readKey returns the key in the brackets of address that open at pos, and where they
// close. It reads the key as an HCL index, so that it takes the same quoted strings and
// numbers as `dependency.foo["web"]`.
//
// Panics when HCL parses the brackets without error into something other than an index.
// The HCL parser reports an error for every bracketed step that is not a literal index.
func readKey(address string, pos int) (string, int, error) {
	end, err := keyEnd(address, pos)
	if err != nil {
		return "", 0, err
	}

	// A placeholder root as long as what precedes the key puts a diagnostic's column on
	// the key in a single-line address.
	root := strings.Repeat("x", utf8.RuneCountInString(address[:pos]))

	traversal, diags := hclsyntax.ParseTraversalAbs([]byte(root+address[pos:end]), "", hcl.InitialPos)
	if diags.HasErrors() {
		return "", 0, diags
	}

	key := traversal[1].(hcl.TraverseIndex).Key
	if key.Type() == cty.String {
		return key.AsString(), end, nil
	}

	// Formatting a number such as 1e99999999 in full takes minutes.
	number, accuracy := key.AsBigFloat().Int64()
	if accuracy != big.Exact {
		return "", 0, errKeyNotAnIndex
	}

	return strconv.FormatInt(number, 10), end, nil
}

// keyEnd returns where the brackets of address that open at pos close, as the HCL lexer
// sees it: a closing bracket inside a quoted key or a comment does not count. Unclosed
// brackets run to the end of address.
//
// Returns the lexer's diagnostic when the brackets hold something HCL cannot lex.
func keyEnd(address string, pos int) (int, error) {
	start := hcl.Pos{Line: 1, Column: utf8.RuneCountInString(address[:pos]) + 1, Byte: pos}
	tokens, diags := hclsyntax.LexExpression([]byte(address[pos:]), "", start)

	end := len(address)

	for _, token := range tokens {
		if token.Type == hclsyntax.TokenCBrack {
			end = token.Range.End.Byte

			break
		}
	}

	// The lexer also read what follows the key, where a name may hold characters that
	// HCL rejects.
	for _, diag := range diags {
		if diag.Subject.Start.Byte < end {
			return 0, diag
		}
	}

	return end, nil
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
