package hclparse_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
)

// testBlock mirrors the shape the real dependency/unit/stack structs take once
// OSS-3968 lands: an expansion block field plus ordinary attributes.
type testBlock struct {
	Expansion *testExpansion `hcl:"expansion,block"`
	Path      string         `hcl:"path,attr"`
}

type testExpansion struct {
	ForEach *cty.Value `hcl:"for_each,attr"`
	Count   *cty.Value `hcl:"count,attr"`
}

func TestExpandBlockUnexpanded(t *testing.T) {
	t.Parallel()

	instances, err := expand(t, `
dependency "a" {
  path = "../vpc"
}
`)
	require.NoError(t, err)
	require.Len(t, instances, 1)

	assert.Nil(t, instances[0].EachKey)
	assert.Nil(t, instances[0].CountIndex)
	assert.Empty(t, instances[0].Key())
	assert.Equal(t, "../vpc", instances[0].Value.(*testBlock).Path)
}

func TestExpandBlockForEachSet(t *testing.T) {
	t.Parallel()

	instances, err := expand(t, `
dependency "a" {
  expansion {
    for_each = local.services
  }

  path = "../${each.value}"
}
`)
	require.NoError(t, err)
	require.Len(t, instances, 2)

	keys := make([]string, 0, len(instances))
	paths := make([]string, 0, len(instances))

	for _, inst := range instances {
		require.NotNil(t, inst.EachKey)
		assert.Nil(t, inst.CountIndex)

		keys = append(keys, inst.Key())
		paths = append(paths, inst.Value.(*testBlock).Path)
	}

	assert.ElementsMatch(t, []string{"web", "api"}, keys)
	assert.ElementsMatch(t, []string{"../web", "../api"}, paths)
}

// TestExpandBlockForEachMap pins that a map exposes distinct each.key and each.value,
// unlike a set where the two coincide.
func TestExpandBlockForEachMap(t *testing.T) {
	t.Parallel()

	instances, err := expand(t, `
dependency "a" {
  expansion {
    for_each = local.service_map
  }

  path = "../${each.key}/${each.value}"
}
`)
	require.NoError(t, err)
	require.Len(t, instances, 2)

	paths := make([]string, 0, len(instances))
	for _, inst := range instances {
		paths = append(paths, inst.Value.(*testBlock).Path)
	}

	assert.ElementsMatch(t, []string{"../web/frontend", "../api/backend"}, paths)
}

func TestExpandBlockCount(t *testing.T) {
	t.Parallel()

	instances, err := expand(t, `
dependency "a" {
  expansion {
    count = 3
  }

  path = "../${count.index}"
}
`)
	require.NoError(t, err)
	require.Len(t, instances, 3)

	for index, inst := range instances {
		require.NotNil(t, inst.CountIndex)
		assert.Nil(t, inst.EachKey)
		assert.Equal(t, index, *inst.CountIndex)
	}

	assert.Equal(t, []string{"0", "1", "2"}, keysOf(instances))
	assert.Equal(t, "../2", instances[2].Value.(*testBlock).Path)
}

// TestExpandBlockEmptyExpansions pins that an empty collection and a zero count are
// legitimate no-ops rather than errors, so a config can expand to nothing.
func TestExpandBlockEmptyExpansions(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		cfg  string
	}{
		{
			name: "empty for_each",
			cfg: `
dependency "a" {
  expansion {
    for_each = local.no_services
  }

  path = "../x"
}
`,
		},
		{
			name: "zero count",
			cfg: `
dependency "a" {
  expansion {
    count = 0
  }

  path = "../x"
}
`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			instances, err := expand(t, tc.cfg)
			require.NoError(t, err)
			assert.Empty(t, instances)
		})
	}
}

func TestExpandBlockValidationErrors(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		target any
		name   string
		cfg    string
	}{
		{
			name: "both meta-args",
			cfg: `
dependency "a" {
  expansion {
    for_each = local.services
    count    = 2
  }

  path = "../x"
}
`,
			target: new(hclparse.ConflictingMetaArgsError),
		},
		{
			name: "neither meta-arg",
			cfg: `
dependency "a" {
  expansion {}

  path = "../x"
}
`,
			target: new(hclparse.MissingMetaArgError),
		},
		{
			name: "more than one expansion block",
			cfg: `
dependency "a" {
  expansion {
    count = 1
  }

  expansion {
    count = 2
  }

  path = "../x"
}
`,
			target: new(hclparse.DuplicateExpansionBlockError),
		},
		{
			name: "negative count",
			cfg: `
dependency "a" {
  expansion {
    count = -1
  }

  path = "../x"
}
`,
			target: new(hclparse.NegativeCountError),
		},
		{
			name: "unsupported for_each type",
			cfg: `
dependency "a" {
  expansion {
    for_each = local.not_a_collection
  }

  path = "../x"
}
`,
			target: new(hclparse.UnsupportedForEachTypeError),
		},
		{
			name: "for_each element key is not a string or number",
			cfg: `
dependency "a" {
  expansion {
    for_each = local.object_set
  }

  path = "../x"
}
`,
			target: new(hclparse.UnsupportedForEachKeyTypeError),
		},
		{
			name: "fractional count",
			cfg: `
dependency "a" {
  expansion {
    count = 1.5
  }

  path = "../x"
}
`,
			target: new(hclparse.InvalidCountError),
		},
		{
			name: "non-numeric count",
			cfg: `
dependency "a" {
  expansion {
    count = "two"
  }

  path = "../x"
}
`,
			target: new(hclparse.InvalidCountError),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := expand(t, tc.cfg)
			require.Error(t, err)
			require.ErrorAs(t, err, tc.target)
		})
	}
}

// TestExpandBlockRejectsNonConcreteValues pins that unknown and null meta-args are
// reported rather than crashing. cty's LengthInt and ElementIterator panic on both,
// and a for_each fed from a dependency output can be either.
func TestExpandBlockRejectsNonConcreteValues(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		target any
		name   string
		attr   string
	}{
		{
			name:   "unknown for_each",
			attr:   "for_each = local.unknown_services",
			target: new(hclparse.UnknownExpansionValueError),
		},
		{
			name:   "null for_each",
			attr:   "for_each = local.null_services",
			target: new(hclparse.NullExpansionValueError),
		},
		{
			name:   "unknown count",
			attr:   "count = local.unknown_count",
			target: new(hclparse.UnknownExpansionValueError),
		},
		{
			name:   "null count",
			attr:   "count = local.null_count",
			target: new(hclparse.NullExpansionValueError),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := expand(t, `
dependency "a" {
  expansion {
    `+tc.attr+`
  }

  path = "../x"
}
`)
			require.Error(t, err)
			require.ErrorAs(t, err, tc.target)
		})
	}
}

// TestExpandBlockInstanceLimit pins the expansion ceiling for both meta-args. The
// limit is injected so the bound is exercised without decoding DefaultMaxInstances
// instances, and the boundary itself is checked in both directions.
func TestExpandBlockInstanceLimit(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		attr    string
		limit   int
		want    int
		wantErr bool
	}{
		{name: "count at the limit", attr: "count = 3", limit: 3, want: 3},
		{name: "count above the limit", attr: "count = 4", limit: 3, wantErr: true},
		{name: "for_each at the limit", attr: "for_each = local.services", limit: 2, want: 2},
		{
			name:    "for_each above the limit",
			attr:    "for_each = local.services",
			limit:   1,
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			instances, err := expand(t, `
dependency "a" {
  expansion {
    `+tc.attr+`
  }

  path = "../x"
}
`, hclparse.WithMaxInstances(tc.limit))

			if !tc.wantErr {
				require.NoError(t, err)
				assert.Len(t, instances, tc.want)

				return
			}

			var typed hclparse.ExpansionLimitExceededError
			require.ErrorAs(t, err, &typed)
			assert.Equal(t, tc.limit, typed.Limit)
		})
	}
}

// TestExpandBlockDefaultInstanceLimit pins that the ceiling applies to callers that
// pass no options. The check runs before any allocation, so asking for more than a
// million instances costs nothing to reject.
func TestExpandBlockDefaultInstanceLimit(t *testing.T) {
	t.Parallel()

	_, err := expand(t, `
dependency "a" {
  expansion {
    count = 1000001
  }

  path = "../x"
}
`)

	var typed hclparse.ExpansionLimitExceededError
	require.ErrorAs(t, err, &typed)
	assert.Equal(t, hclparse.DefaultMaxInstances, typed.Limit)
}

// TestExpansionLimitExceededErrorGuidesTheUser pins that the message tells the user
// why the ceiling exists and how to ask for it to change, rather than just refusing.
func TestExpansionLimitExceededErrorGuidesTheUser(t *testing.T) {
	t.Parallel()

	msg := hclparse.ExpansionLimitExceededError{
		Attr:  "count",
		Size:  2_000_000,
		Limit: hclparse.DefaultMaxInstances,
	}.Error()

	assert.Contains(t, msg, "2000000")
	assert.Contains(t, msg, "1000000")
	assert.Contains(t, msg, "github.com/gruntwork-io/terragrunt/issues")
}

// TestExpandBlockNumericForEachKeys pins how a numeric each.key renders into an
// address, since OSS-3971 builds the dependency cty map from the same string.
func TestExpandBlockNumericForEachKeys(t *testing.T) {
	t.Parallel()

	instances, err := expand(t, `
dependency "a" {
  expansion {
    for_each = local.numeric_keys
  }

  path = "../${each.key}"
}
`)
	require.NoError(t, err)
	require.Len(t, instances, 2)

	assert.ElementsMatch(t, []string{"1", "2"}, keysOf(instances))
}

// TestExpandBlockRejectsEachUnderCount pins that the two iteration namespaces stay
// separate: count exposes count.index only, so a stray each reference is an error
// rather than a silently empty interpolation.
func TestExpandBlockRejectsEachUnderCount(t *testing.T) {
	t.Parallel()

	_, err := expand(t, `
dependency "a" {
  expansion {
    count = 2
  }

  path = "../${each.value}"
}
`)
	require.Error(t, err)
}

// TestExpandBlockRejectsLabeledExpansionBlock pins that expansion takes no label, so
// a stray one fails instead of being read as a differently-named block.
func TestExpandBlockRejectsLabeledExpansionBlock(t *testing.T) {
	t.Parallel()

	_, err := expand(t, `
dependency "a" {
  expansion "extra" {
    count = 2
  }

  path = "../x"
}
`)
	require.Error(t, err)
}

// TestExpandBlockRejectsUnknownExpansionAttribute pins that the expansion block has
// no catch-all, so a mistyped meta-arg fails loudly instead of expanding zero times.
// each_key is the case that matters most: it is engine-assigned metadata, and letting
// a config set it would let the address disagree with the iteration key.
func TestExpandBlockRejectsUnknownExpansionAttribute(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		attr string
	}{
		{name: "mistyped for_each", attr: `for_eech = local.services`},
		{name: "engine-assigned each_key", attr: `each_key = "web"`},
		{name: "engine-assigned count_index", attr: `count_index = 0`},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := expand(t, `
dependency "a" {
  expansion {
    `+tc.attr+`
  }

  path = "../x"
}
`)
			require.Error(t, err)
		})
	}
}

func expand(
	tb testing.TB,
	cfg string,
	opts ...hclparse.ExpandOption,
) ([]hclparse.Instance, error) {
	tb.Helper()

	file, diags := hclsyntax.ParseConfig([]byte(cfg), "terragrunt.hcl", hcl.InitialPos)
	require.False(tb, diags.HasErrors(), "fixture failed to parse: %v", diags)

	body, ok := file.Body.(*hclsyntax.Body)
	require.True(tb, ok)
	require.Len(tb, body.Blocks, 1)

	ctx := &hcl.EvalContext{
		Variables: map[string]cty.Value{
			"local": cty.ObjectVal(map[string]cty.Value{
				"services": cty.SetVal([]cty.Value{
					cty.StringVal("web"),
					cty.StringVal("api"),
				}),
				"service_map": cty.MapVal(map[string]cty.Value{
					"web": cty.StringVal("frontend"),
					"api": cty.StringVal("backend"),
				}),
				"no_services":      cty.SetValEmpty(cty.String),
				"not_a_collection": cty.StringVal("nope"),
				"numeric_keys": cty.SetVal([]cty.Value{
					cty.NumberIntVal(1),
					cty.NumberIntVal(2),
				}),
				"object_set": cty.SetVal([]cty.Value{
					cty.ObjectVal(map[string]cty.Value{"name": cty.StringVal("web")}),
				}),
				"unknown_services": cty.UnknownVal(cty.Set(cty.String)),
				"null_services":    cty.NullVal(cty.Set(cty.String)),
				"unknown_count":    cty.UnknownVal(cty.Number),
				"null_count":       cty.NullVal(cty.Number),
			}),
		},
	}

	return hclparse.ExpandBlock(body.Blocks[0].AsHCLBlock(), new(testBlock), ctx, opts...)
}

func keysOf(instances []hclparse.Instance) []string {
	keys := make([]string, 0, len(instances))
	for _, inst := range instances {
		keys = append(keys, inst.Key())
	}

	return keys
}
