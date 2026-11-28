package runner_test

import (
	"fmt"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/runner"
	"github.com/gruntwork-io/terragrunt/internal/semver"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	thlogger "github.com/gruntwork-io/terragrunt/test/helpers/logger"
)

// fuzzMaxAttrs bounds the attributes each fuzzed unit sets.
const fuzzMaxAttrs = 8

// FuzzFindDependentUnits sets top-level attributes on a unit and on a unit that depends on it, then checks that
// [runner.FindDependentUnits] reports the dependents a stack built from the caller's options lists.
//
// Attribute names come from the tags of [config.UnitConfig], so an attribute added there is fuzzed without this
// test naming it. A mismatch means the options the search builds for itself lost something the caller's options
// carry. Inputs whose stack fails to build from the caller's options are skipped.
func FuzzFindDependentUnits(f *testing.F) {
	attrs := unitConfigAttrs()

	f.Add([]byte{}, []byte{})

	f.Fuzz(func(t *testing.T, vpcPicks, appPicks []byte) {
		v := memVenv(tfVersionOutput)
		vpcDir := writeUnit(t, v, memRoot, "vpc", fuzzAttrBody(attrs, vpcPicks))
		writeUnit(t, v, memRoot, "app", dependencyBlock("../vpc")+"\n"+fuzzAttrBody(attrs, appPicks))
		require.NoError(t, v.FS.MkdirAll(filepath.Join(memRoot, ".git"), 0o755))

		ctx := config.WithCaches(t.Context())
		l := thlogger.CreateLogger()

		rootOpts := newStackOpts(t, memRoot, tf.CommandNameDestroy)
		rootOpts.TerragruntVersion = semver.MustParse("1.2.3")

		rnr, err := runner.New(ctx, l, v, new(config.Store), rootOpts)
		if err != nil {
			return
		}

		want := slices.Sorted(slices.Values(rnr.ListStackDependentUnits()[vpcDir]))

		opts := newStackOpts(t, vpcDir, tf.CommandNameDestroy)
		opts.TerragruntVersion = rootOpts.TerragruntVersion

		units := runner.FindDependentUnits(ctx, l, v, new(config.Store), opts, &config.TerragruntConfig{})

		got := make([]string, 0, len(units))
		for _, unit := range units {
			got = append(got, unit.Path())
		}

		assert.Equal(t, want, got)
	})
}

// unitAttr is a top-level attribute of a unit config, with the literals the fuzzer may assign it.
type unitAttr struct {
	name   string
	values []string
}

// unitConfigAttrs returns the string, bool, and integer attributes [config.UnitConfig] decodes.
func unitConfigAttrs() []unitAttr {
	strValues := []string{
		`""`, `"tofu"`, `"terraform"`, `">= 0.0.0"`, `">= 99.0.0"`, `"~> 1.0"`, `"not a constraint"`,
		`"arn:aws:iam::123456789012:role/fuzz"`, `"fuzz"`, `"1h"`,
	}
	boolValues := []string{"true", "false"}
	intValues := []string{"0", "1", "-1", "3600"}

	typ := reflect.TypeFor[config.UnitConfig]()

	var attrs []unitAttr

	for field := range typ.Fields() {
		name, kind, ok := strings.Cut(field.Tag.Get("hcl"), ",")
		if !ok || kind != "attr" {
			continue
		}

		switch field.Type {
		case reflect.TypeFor[*string]():
			attrs = append(attrs, unitAttr{name: name, values: strValues})
		case reflect.TypeFor[*bool]():
			attrs = append(attrs, unitAttr{name: name, values: boolValues})
		case reflect.TypeFor[*int64]():
			attrs = append(attrs, unitAttr{name: name, values: intValues})
		}
	}

	return attrs
}

// fuzzAttrBody renders up to [fuzzMaxAttrs] attributes, each picked by a pair of bytes: the first selects the
// attribute and the second its value. An attribute picked twice keeps its first value.
func fuzzAttrBody(attrs []unitAttr, picks []byte) string {
	seen := map[string]struct{}{}

	var b strings.Builder

	for i := 0; i+1 < len(picks) && len(seen) < fuzzMaxAttrs; i += 2 {
		attr := attrs[int(picks[i])%len(attrs)]
		if _, dup := seen[attr.name]; dup {
			continue
		}

		seen[attr.name] = struct{}{}

		fmt.Fprintf(&b, "%s = %s\n", attr.name, attr.values[int(picks[i+1])%len(attr.values)])
	}

	return b.String()
}
