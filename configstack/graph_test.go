package configstack

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/stretchr/testify/assert"
)

func TestCheckForCycles(t *testing.T) {
	t.Parallel()

	////////////////////////////////////
	// These modules have no dependencies
	////////////////////////////////////
	a := &TerraformModule{Path: "a"}
	b := &TerraformModule{Path: "b"}
	c := &TerraformModule{Path: "c"}
	d := &TerraformModule{Path: "d"}

	////////////////////////////////////
	// These modules have dependencies, but no cycles
	////////////////////////////////////

	// e -> a
	e := &TerraformModule{Path: "e", Dependencies: []*TerraformModule{a}}

	// f -> a, b
	f := &TerraformModule{Path: "f", Dependencies: []*TerraformModule{a, b}}

	// g -> e -> a
	g := &TerraformModule{Path: "g", Dependencies: []*TerraformModule{e}}

	// h -> g -> e -> a
	// |            /
	//  --> f -> b
	// |
	//  --> c
	h := &TerraformModule{Path: "h", Dependencies: []*TerraformModule{g, f, c}}

	////////////////////////////////////
	// These modules have dependencies and cycles
	////////////////////////////////////

	// i -> i
	i := &TerraformModule{Path: "i", Dependencies: []*TerraformModule{}}
	i.Dependencies = append(i.Dependencies, i)

	// j -> k -> j
	j := &TerraformModule{Path: "j", Dependencies: []*TerraformModule{}}
	k := &TerraformModule{Path: "k", Dependencies: []*TerraformModule{j}}
	j.Dependencies = append(j.Dependencies, k)

	// l -> m -> n -> o -> l
	l := &TerraformModule{Path: "l", Dependencies: []*TerraformModule{}}
	o := &TerraformModule{Path: "o", Dependencies: []*TerraformModule{l}}
	n := &TerraformModule{Path: "n", Dependencies: []*TerraformModule{o}}
	m := &TerraformModule{Path: "m", Dependencies: []*TerraformModule{n}}
	l.Dependencies = append(l.Dependencies, m)

	testCases := []struct {
		modules  []*TerraformModule
		expected DependencyCycle
	}{
		{[]*TerraformModule{}, nil},
		{[]*TerraformModule{a}, nil},
		{[]*TerraformModule{a, b, c, d}, nil},
		{[]*TerraformModule{a, e}, nil},
		{[]*TerraformModule{a, b, f}, nil},
		{[]*TerraformModule{a, e, g}, nil},
		{[]*TerraformModule{a, b, c, e, f, g, h}, nil},
		{[]*TerraformModule{i}, DependencyCycle([]string{"i", "i"})},
		{[]*TerraformModule{j, k}, DependencyCycle([]string{"j", "k", "j"})},
		{[]*TerraformModule{l, o, n, m}, DependencyCycle([]string{"l", "m", "n", "o", "l"})},
		{[]*TerraformModule{a, l, b, o, n, f, m, h}, DependencyCycle([]string{"l", "m", "n", "o", "l"})},
	}

	for _, testCase := range testCases {
		actual := CheckForCycles(testCase.modules)
		if testCase.expected == nil {
			assert.Nil(t, actual)
		} else if assert.NotNil(t, actual, "For modules %v", testCase.modules) {
			actualErr := errors.Unwrap(actual).(DependencyCycle)
			assert.Equal(t, []string(testCase.expected), []string(actualErr), "For modules %v", testCase.modules)
		}
	}
}
