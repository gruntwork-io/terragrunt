package glob_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/glob"
)

// FuzzCompile pins that every pattern [glob.Compile] accepts can be matched
// against any string without panicking, including the pattern's own text.
func FuzzCompile(f *testing.F) {
	for _, seed := range []struct {
		pattern string
		s       string
	}{
		{pattern: "foo/**/bar.tf", s: "foo/bar.tf"},
		{pattern: "foo/{*.tf,**/*.tf}", s: "foo/a/b.tf"},
		{pattern: "main.tf{,.bak}", s: "main.tf"},
		{pattern: "[abc].tf", s: "b.tf"},
		{pattern: `a\*b.tf`, s: "a*b.tf"},
		{pattern: "a{", s: "a"},
		{pattern: "a{,}", s: "a"},
		{pattern: "a{*,}", s: "ab"},
		{pattern: "}{", s: "}"},
		{pattern: "{./a{", s: "a"},
	} {
		f.Add(seed.pattern, seed.s)
	}

	f.Fuzz(func(t *testing.T, pattern, s string) {
		for _, opts := range [][]glob.CompileOption{nil, {glob.WithoutSeparator()}} {
			m, err := glob.Compile(pattern, opts...)
			if err != nil {
				continue
			}

			m.Match(s)
			m.Match(pattern)
		}
	})
}
