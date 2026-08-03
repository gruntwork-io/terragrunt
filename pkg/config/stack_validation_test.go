package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	inthclparse "github.com/gruntwork-io/terragrunt/internal/hclparse"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
)

func TestValidateStackConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		config  *config.StackConfigFile
		wantErr string
	}{
		{
			name: "valid config",
			config: &config.StackConfigFile{
				Units: []*config.Unit{
					{
						Name:   "unit1",
						Source: "source1",
						Path:   "path1",
					},
					{
						Name:   "unit2",
						Source: "source2",
						Path:   "path2",
					},
				},
			},
			wantErr: "",
		},
		{
			name: "empty config",
			config: &config.StackConfigFile{
				Units: []*config.Unit{},
			},
			wantErr: "stack config must contain at least one unit",
		},
		{
			name: "empty unit name",
			config: &config.StackConfigFile{
				Units: []*config.Unit{
					{
						Name:   "",
						Source: "source1",
						Path:   "path1",
					},
				},
			},
			wantErr: "unit at index 0 has empty name",
		},
		{
			name: "whitespace unit name",
			config: &config.StackConfigFile{
				Units: []*config.Unit{
					{
						Name:   "  ",
						Source: "source1",
						Path:   "path1",
					},
				},
			},
			wantErr: "unit at index 0 has empty name",
		},
		{
			name: "empty unit source",
			config: &config.StackConfigFile{
				Units: []*config.Unit{
					{
						Name:   "unit1",
						Source: "",
						Path:   "path1",
					},
				},
			},
			wantErr: "unit 'unit1' has empty source",
		},
		{
			name: "whitespace unit source",
			config: &config.StackConfigFile{
				Units: []*config.Unit{
					{
						Name:   "unit1",
						Source: "   ",
						Path:   "path1",
					},
				},
			},
			wantErr: "unit 'unit1' has empty source",
		},
		{
			name: "empty unit path",
			config: &config.StackConfigFile{
				Units: []*config.Unit{
					{
						Name:   "unit1",
						Source: "source1",
						Path:   "",
					},
				},
			},
			wantErr: "unit 'unit1' has empty path",
		},
		{
			name: "whitespace unit path",
			config: &config.StackConfigFile{
				Units: []*config.Unit{
					{
						Name:   "unit1",
						Source: "source1",
						Path:   "  ",
					},
				},
			},
			wantErr: "unit 'unit1' has empty path",
		},
		{
			name: "duplicate unit names",
			config: &config.StackConfigFile{
				Units: []*config.Unit{
					{
						Name:   "unit1",
						Source: "source1",
						Path:   "path1",
					},
					{
						Name:   "unit1",
						Source: "source2",
						Path:   "path2",
					},
				},
			},
			wantErr: "duplicate unit name found: 'unit1'",
		},
		{
			name: "duplicate unit paths",
			config: &config.StackConfigFile{
				Units: []*config.Unit{
					{
						Name:   "unit1",
						Source: "source1",
						Path:   "path1",
					},
					{
						Name:   "unit2",
						Source: "source2",
						Path:   "path1",
					},
				},
			},
			wantErr: "duplicate unit path found: 'path1'",
		},

		{
			name: "valid config with stacks",
			config: &config.StackConfigFile{
				Stacks: []*config.Stack{
					{
						Name:   "stack1",
						Source: "source1",
						Path:   "path1",
					},
					{
						Name:   "stack2",
						Source: "source2",
						Path:   "path2",
					},
				},
			},
			wantErr: "",
		},
		{
			name: "empty stack name",
			config: &config.StackConfigFile{
				Stacks: []*config.Stack{
					{
						Name:   "",
						Source: "source1",
						Path:   "path1",
					},
				},
			},
			wantErr: "stack at index 0 has empty name",
		},
		{
			name: "whitespace stack name",
			config: &config.StackConfigFile{
				Stacks: []*config.Stack{
					{
						Name:   "  ",
						Source: "source1",
						Path:   "path1",
					},
				},
			},
			wantErr: "stack at index 0 has empty name",
		},
		{
			name: "empty stack source",
			config: &config.StackConfigFile{
				Stacks: []*config.Stack{
					{
						Name:   "stack1",
						Source: "",
						Path:   "path1",
					},
				},
			},
			wantErr: "stack 'stack1' has empty source",
		},
		{
			name: "whitespace stack source",
			config: &config.StackConfigFile{
				Stacks: []*config.Stack{
					{
						Name:   "stack1",
						Source: "   ",
						Path:   "path1",
					},
				},
			},
			wantErr: "stack 'stack1' has empty source",
		},
		{
			name: "empty stack path",
			config: &config.StackConfigFile{
				Stacks: []*config.Stack{
					{
						Name:   "stack1",
						Source: "source1",
						Path:   "",
					},
				},
			},
			wantErr: "stack 'stack1' has empty path",
		},
		{
			name: "whitespace stack path",
			config: &config.StackConfigFile{
				Stacks: []*config.Stack{
					{
						Name:   "stack1",
						Source: "source1",
						Path:   "  ",
					},
				},
			},
			wantErr: "stack 'stack1' has empty path",
		},
		{
			name: "duplicate stack names",
			config: &config.StackConfigFile{
				Stacks: []*config.Stack{
					{
						Name:   "stack1",
						Source: "source1",
						Path:   "path1",
					},
					{
						Name:   "stack1",
						Source: "source2",
						Path:   "path2",
					},
				},
			},
			wantErr: "duplicate stack name found: 'stack1'",
		},
		{
			name: "duplicate stack paths",
			config: &config.StackConfigFile{
				Stacks: []*config.Stack{
					{
						Name:   "stack1",
						Source: "source1",
						Path:   "path1",
					},
					{
						Name:   "stack2",
						Source: "source2",
						Path:   "path1",
					},
				},
			},
			wantErr: "duplicate stack path found: 'path1'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := config.ValidateStackConfig(tt.config, "/stack")
			if tt.wantErr != "" {
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestValidateStackConfigCrossKindPathCollision rejects a unit and a stack that resolve to the same generated path.
func TestValidateStackConfigCrossKindPathCollision(t *testing.T) {
	t.Parallel()

	cfg := &config.StackConfigFile{
		Units:  []*config.Unit{{Name: "a", Source: "src-a", Path: "collide"}},
		Stacks: []*config.Stack{{Name: "b", Source: "src-b", Path: "collide"}},
	}

	err := config.ValidateStackConfig(cfg, "/stack")
	require.Error(t, err, "a unit and a stack sharing a generated path must be rejected")
	require.Contains(t, err.Error(), "collide")
}

// TestValidateStackConfigCrossKindEqualRawPathNoStackNoCollision proves the cross-kind check
// compares the GENERATED path, not the raw path: a unit at .terragrunt-stack/x and a stack hoisted
// out via no_dot_terragrunt_stack to x do NOT collide even though the raw path string is equal.
func TestValidateStackConfigCrossKindEqualRawPathNoStackNoCollision(t *testing.T) {
	t.Parallel()

	noStack := true
	cfg := &config.StackConfigFile{
		Units:  []*config.Unit{{Name: "a", Source: "src-a", Path: "x"}},
		Stacks: []*config.Stack{{Name: "b", Source: "src-b", Path: "x", NoStack: &noStack}},
	}

	err := config.ValidateStackConfig(cfg, "/stack")
	require.NoError(
		t,
		err,
		"equal raw paths that generate to different dirs must not be flagged as a collision",
	)
}

// TestValidateStackConfigCrossKindDifferentRawSameGeneratedCollision proves the inverse: differing raw
// path strings that clean to the SAME generated dir must be flagged. Both hoist out via
// no_dot_terragrunt_stack; the unit's "./shared" and the stack's "shared" both clean to /stack/shared.
func TestValidateStackConfigCrossKindDifferentRawSameGeneratedCollision(t *testing.T) {
	t.Parallel()

	noStack := true
	cfg := &config.StackConfigFile{
		Units:  []*config.Unit{{Name: "a", Source: "src-a", Path: "./shared", NoStack: &noStack}},
		Stacks: []*config.Stack{{Name: "b", Source: "src-b", Path: "shared", NoStack: &noStack}},
	}

	err := config.ValidateStackConfig(cfg, "/stack")
	require.Error(
		t,
		err,
		"different raw paths cleaning to the same generated dir must be flagged as a collision",
	)
}

// TestValidateStackConfigNilElementDoesNotPanic proves a nil unit or stack element is skipped by the
// cross-kind check (reported as nil by the generic validator) instead of panicking before that error surfaces.
func TestValidateStackConfigNilElementDoesNotPanic(t *testing.T) {
	t.Parallel()

	cfg := &config.StackConfigFile{
		Units:  []*config.Unit{nil, {Name: "a", Source: "src-a", Path: "x"}},
		Stacks: []*config.Stack{nil, {Name: "b", Source: "src-b", Path: "y"}},
	}

	require.NotPanics(t, func() {
		err := config.ValidateStackConfig(cfg, "/stack")
		require.Error(t, err, "nil elements must surface as a validation error, not a panic")
		require.Contains(t, err.Error(), "is nil")
	})
}

// TestStackAutoIncludeBackstopPopulatesTypedErrorFields covers the pkg/config backstop in
// mergeStackAutoIncludeFile: a stale or hand-written terragrunt.autoinclude.stack.hcl whose injected
// unit values reference dependency outputs must surface the typed StackAutoIncludeDependencyValuesError
// with UnitName, StackName, and Subject populated. The fail-fast generation check (RFC #19) is
// unit-tested elsewhere; this asserts the second line of defence reads the same fields off the on-disk file.
func TestStackAutoIncludeBackstopPopulatesTypedErrorFields(t *testing.T) {
	t.Parallel()

	stackDir := t.TempDir()

	// A regular stack file so the parser does not short-circuit on the autoinclude filename.
	stackFilePath := filepath.Join(stackDir, config.DefaultStackFile)
	require.NoError(t, os.WriteFile(stackFilePath, []byte(`
unit "base" {
  source = "."
  path   = "base"
}
`), 0644))

	// The stale sibling autoinclude carries the unsupported cross-level pattern: a top-level
	// dependency block whose outputs feed an injected unit's values.
	autoIncludePath := filepath.Join(stackDir, config.DefaultAutoIncludeStackFile)
	require.NoError(t, os.WriteFile(autoIncludePath, []byte(`
dependency "producer" {
  config_path = "../producer"
}

unit "extra" {
  source = "."
  path   = "extra"

  values = {
    v = dependency.producer.outputs.val
  }
}
`), 0644))

	ctx, pctx := newTestParsingContext(t, stackFilePath)
	pctx.Experiments.EnableExperiment(experiment.StackDependencies)

	_, err := config.ReadStackConfigFile(ctx, logger.CreateLogger(), pctx, stackFilePath, nil)
	require.Error(
		t,
		err,
		"the backstop must reject a stale stack autoinclude carrying a dependency consumed by injected values",
	)

	var typed inthclparse.StackAutoIncludeDependencyValuesError
	require.ErrorAs(
		t,
		err,
		&typed,
		"the backstop must surface the typed StackAutoIncludeDependencyValuesError",
	)

	// StackName is derived from the stack directory base by the backstop.
	assert.Equal(
		t,
		filepath.Base(stackDir),
		typed.StackName,
		"StackName must be the stack directory base",
	)

	// The previously dropped fields must now be carried through the backstop path.
	assert.Equal(
		t,
		"extra",
		typed.UnitName,
		"UnitName must name the injected unit whose values consume the dependency",
	)
	require.NotNil(t, typed.Subject, "Subject must point at the offending values expression")
	assert.Equal(
		t,
		autoIncludePath,
		typed.Subject.Filename,
		"Subject must reference the stale autoinclude file on disk",
	)

	// The clear guidance must still ride along, never the cryptic low-level diagnostic.
	msg := err.Error()
	assert.Contains(t, msg, "supported cross-level pattern")
	assert.NotContains(t, msg, "no variable named dependency")
}

// TestStackAutoIncludeBackstopAllowsSupportedPattern is the negative guard: the backstop must
// not fire for the supported cross-level pattern where injected values reference only unit.X.path.
func TestStackAutoIncludeBackstopAllowsSupportedPattern(t *testing.T) {
	t.Parallel()

	stackDir := t.TempDir()

	stackFilePath := filepath.Join(stackDir, config.DefaultStackFile)
	require.NoError(t, os.WriteFile(stackFilePath, []byte(`
unit "base" {
  source = "."
  path   = "base"
}
`), 0644))

	autoIncludePath := filepath.Join(stackDir, config.DefaultAutoIncludeStackFile)
	require.NoError(t, os.WriteFile(autoIncludePath, []byte(`
unit "extra" {
  source = "."
  path   = "extra"

  values = {
    v = unit.base.path
  }
}
`), 0644))

	ctx, pctx := newTestParsingContext(t, stackFilePath)
	pctx.Experiments.EnableExperiment(experiment.StackDependencies)

	stackConfig, err := config.ReadStackConfigFile(
		ctx,
		logger.CreateLogger(),
		pctx,
		stackFilePath,
		nil,
	)
	require.NoError(t, err, "a supported unit.X.path values reference must not trip the backstop")
	require.NotNil(t, stackConfig)

	var typed inthclparse.StackAutoIncludeDependencyValuesError
	require.NotErrorAs(
		t,
		err,
		&typed,
		"the supported pattern must not produce the typed dependency-values error",
	)
}

// TestStackAutoIncludeOverridesSameNameUnit pins that a stack autoinclude whose injected unit name
// matches a base unit overrides the base block wholesale (source and path come from the autoinclude),
// while an injected unit with a new name is appended, matching unit autoinclude override semantics.
func TestStackAutoIncludeOverridesSameNameUnit(t *testing.T) {
	t.Parallel()

	stackDir := t.TempDir()

	stackFilePath := filepath.Join(stackDir, config.DefaultStackFile)
	require.NoError(t, os.WriteFile(stackFilePath, []byte(`
unit "vpc" {
  source = "base-source"
  path   = "base-path"
}

unit "untouched" {
  source = "untouched-source"
  path   = "untouched-path"
}
`), 0644))

	autoIncludePath := filepath.Join(stackDir, config.DefaultAutoIncludeStackFile)
	require.NoError(t, os.WriteFile(autoIncludePath, []byte(`
unit "vpc" {
  source = "injected-source"
  path   = "injected-path"
}

unit "added" {
  source = "added-source"
  path   = "added-path"
}
`), 0644))

	ctx, pctx := newTestParsingContext(t, stackFilePath)
	pctx.Experiments.EnableExperiment(experiment.StackDependencies)

	stackConfig, err := config.ReadStackConfigFile(
		ctx,
		logger.CreateLogger(),
		pctx,
		stackFilePath,
		nil,
	)
	require.NoError(
		t,
		err,
		"a same-name injected unit must override the base unit, not raise a duplicate-name error",
	)
	require.NotNil(t, stackConfig)

	byName := make(map[string]*config.Unit, len(stackConfig.Units))
	for _, u := range stackConfig.Units {
		byName[u.Name] = u
	}

	require.Len(
		t,
		stackConfig.Units,
		3,
		"override collapses the same-name unit while new names are appended",
	)

	require.Contains(t, byName, "vpc")
	assert.Equal(
		t,
		"injected-source",
		byName["vpc"].Source,
		"the injected unit overrides the base source",
	)
	assert.Equal(
		t,
		"injected-path",
		byName["vpc"].Path,
		"the injected unit overrides the base path",
	)

	require.Contains(t, byName, "untouched")
	assert.Equal(
		t,
		"untouched-source",
		byName["untouched"].Source,
		"an unmatched base unit is left intact",
	)

	require.Contains(t, byName, "added")
	assert.Equal(
		t,
		"added-source",
		byName["added"].Source,
		"an injected unit with a new name is appended",
	)
}

// TestStackAutoIncludeOverridesSameNameStack mirrors the unit override case for `stack` blocks: a same-name
// injected stack overrides the base stack wholesale, while a new-name injected stack is appended.
func TestStackAutoIncludeOverridesSameNameStack(t *testing.T) {
	t.Parallel()

	stackDir := t.TempDir()

	stackFilePath := filepath.Join(stackDir, config.DefaultStackFile)
	require.NoError(t, os.WriteFile(stackFilePath, []byte(`
stack "networking" {
  source = "base-source"
  path   = "net"
}

stack "untouched" {
  source = "untouched-source"
  path   = "untouched"
}
`), 0644))

	autoIncludePath := filepath.Join(stackDir, config.DefaultAutoIncludeStackFile)
	require.NoError(t, os.WriteFile(autoIncludePath, []byte(`
stack "networking" {
  source = "injected-source"
  path   = "net"
}

stack "added" {
  source = "added-source"
  path   = "added"
}
`), 0644))

	ctx, pctx := newTestParsingContext(t, stackFilePath)
	pctx.Experiments.EnableExperiment(experiment.StackDependencies)

	stackConfig, err := config.ReadStackConfigFile(
		ctx,
		logger.CreateLogger(),
		pctx,
		stackFilePath,
		nil,
	)
	require.NoError(
		t,
		err,
		"a same-name injected stack must override the base stack, not raise a duplicate-name error",
	)
	require.NotNil(t, stackConfig)

	byName := make(map[string]*config.Stack, len(stackConfig.Stacks))
	for _, s := range stackConfig.Stacks {
		byName[s.Name] = s
	}

	require.Len(
		t,
		stackConfig.Stacks,
		3,
		"override collapses the same-name stack while new names are appended",
	)

	require.Contains(t, byName, "networking")
	assert.Equal(
		t,
		"injected-source",
		byName["networking"].Source,
		"the injected stack overrides the base source",
	)

	require.Contains(t, byName, "untouched")
	assert.Equal(
		t,
		"untouched-source",
		byName["untouched"].Source,
		"an unmatched base stack is left intact",
	)

	require.Contains(t, byName, "added")
	assert.Equal(
		t,
		"added-source",
		byName["added"].Source,
		"an injected stack with a new name is appended",
	)
}

// TestStackAutoIncludeDuplicateNameWithinFileRejected pins that two same-name blocks within the autoinclude
// file itself are rejected with the same typed error as a base-file duplicate, not silently collapsed.
func TestStackAutoIncludeDuplicateNameWithinFileRejected(t *testing.T) {
	t.Parallel()

	stackDir := t.TempDir()

	stackFilePath := filepath.Join(stackDir, config.DefaultStackFile)
	require.NoError(t, os.WriteFile(stackFilePath, []byte(`
unit "base" {
  source = "."
  path   = "base"
}
`), 0644))

	autoIncludePath := filepath.Join(stackDir, config.DefaultAutoIncludeStackFile)
	require.NoError(t, os.WriteFile(autoIncludePath, []byte(`
unit "extra" {
  source = "."
  path   = "extra"
}

unit "extra" {
  source = "."
  path   = "extra-dup"
}
`), 0644))

	ctx, pctx := newTestParsingContext(t, stackFilePath)
	pctx.Experiments.EnableExperiment(experiment.StackDependencies)

	_, err := config.ReadStackConfigFile(ctx, logger.CreateLogger(), pctx, stackFilePath, nil)
	require.Error(
		t,
		err,
		"a duplicate name within the autoinclude file must be rejected, not silently collapsed",
	)

	var typed inthclparse.DuplicateUnitNameError
	require.ErrorAs(t, err, &typed)
	assert.Equal(t, "extra", typed.Name)
}

// TestStackRegularIncludeStillRejectsDuplicateName pins that a regular stack `include` block keeps the
// duplicate-name rejection (override is scoped to autoinclude only, not regular includes).
func TestStackRegularIncludeStillRejectsDuplicateName(t *testing.T) {
	t.Parallel()

	stackDir := t.TempDir()

	stackFilePath := filepath.Join(stackDir, config.DefaultStackFile)
	require.NoError(t, os.WriteFile(stackFilePath, []byte(`
unit "vpc" {
  source = "."
  path   = "vpc"
}

include "shared" {
  path = "./shared.hcl"
}
`), 0644))

	require.NoError(t, os.WriteFile(filepath.Join(stackDir, "shared.hcl"), []byte(`
unit "vpc" {
  source = "."
  path   = "vpc-from-include"
}
`), 0644))

	ctx, pctx := newTestParsingContext(t, stackFilePath)
	pctx.Experiments.EnableExperiment(experiment.StackDependencies)

	_, err := config.ReadStackConfigFile(ctx, logger.CreateLogger(), pctx, stackFilePath, nil)
	require.Error(
		t,
		err,
		"a regular include declaring a same-name unit must still be rejected as a duplicate",
	)

	var typed inthclparse.DuplicateUnitNameError
	require.ErrorAs(t, err, &typed)
	assert.Equal(t, "vpc", typed.Name)
}

// TestStackAutoIncludeOverrideUpdatesComponentPathRef pins that when the autoinclude override changes a
// component's path, a sibling value referencing unit.<name>.path resolves to the overridden path, not the
// stale base path the override replaced.
func TestStackAutoIncludeOverrideUpdatesComponentPathRef(t *testing.T) {
	t.Parallel()

	stackDir := t.TempDir()

	stackFilePath := filepath.Join(stackDir, config.DefaultStackFile)
	require.NoError(t, os.WriteFile(stackFilePath, []byte(`
unit "vpc" {
  source = "."
  path   = "alpha"
}

unit "consumer" {
  source = "."
  path   = "consumer"

  values = {
    p = unit.vpc.path
  }
}
`), 0644))

	autoIncludePath := filepath.Join(stackDir, config.DefaultAutoIncludeStackFile)
	require.NoError(t, os.WriteFile(autoIncludePath, []byte(`
unit "vpc" {
  source = "."
  path   = "omega"
}
`), 0644))

	ctx, pctx := newTestParsingContext(t, stackFilePath)
	pctx.Experiments.EnableExperiment(experiment.StackDependencies)

	stackConfig, err := config.ReadStackConfigFile(
		ctx,
		logger.CreateLogger(),
		pctx,
		stackFilePath,
		nil,
	)
	require.NoError(t, err)
	require.NotNil(t, stackConfig)

	var consumer *config.Unit

	for _, u := range stackConfig.Units {
		if u.Name == "consumer" {
			consumer = u
			break
		}
	}

	require.NotNil(t, consumer, "the consumer unit must survive the merge")
	require.NotNil(t, consumer.Values, "the consumer values must be evaluated")

	p := consumer.Values.GetAttr("p")
	require.Equal(t, cty.String, p.Type())
	assert.Contains(t, p.AsString(), "omega", "unit.vpc.path must resolve to the overridden path")
	assert.NotContains(
		t,
		p.AsString(),
		"alpha",
		"the stale base path must not leak into the sibling reference",
	)
}

// TestStackAutoIncludeOverridePathReferencesSiblingComponentRef pins that an injected block whose own path
// references unit.<name>.path of a base component parses successfully: the base refs are published before
// the autoinclude headers are decoded, so the cross-reference resolves instead of erroring the parse.
func TestStackAutoIncludeOverridePathReferencesSiblingComponentRef(t *testing.T) {
	t.Parallel()

	stackDir := t.TempDir()

	stackFilePath := filepath.Join(stackDir, config.DefaultStackFile)
	require.NoError(t, os.WriteFile(stackFilePath, []byte(`
unit "anchor" {
  source = "."
  path   = "anchor"
}

unit "vpc" {
  source = "."
  path   = "vpc-base"
}
`), 0644))

	autoIncludePath := filepath.Join(stackDir, config.DefaultAutoIncludeStackFile)
	require.NoError(t, os.WriteFile(autoIncludePath, []byte(`
unit "vpc" {
  source = "."
  path   = "${unit.anchor.path}-vpc"
}
`), 0644))

	ctx, pctx := newTestParsingContext(t, stackFilePath)
	pctx.Experiments.EnableExperiment(experiment.StackDependencies)

	stackConfig, err := config.ReadStackConfigFile(
		ctx,
		logger.CreateLogger(),
		pctx,
		stackFilePath,
		nil,
	)
	require.NoError(
		t,
		err,
		"an injected path referencing a base unit.<name>.path must resolve, not error the parse",
	)
	require.NotNil(t, stackConfig)

	var vpc *config.Unit

	for _, u := range stackConfig.Units {
		if u.Name == "vpc" {
			vpc = u
			break
		}
	}

	require.NotNil(t, vpc, "the overridden vpc unit must survive the merge")
	assert.Contains(
		t,
		vpc.Path,
		"anchor",
		"the injected path must resolve unit.anchor.path against the base component",
	)
}
