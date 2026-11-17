// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package funcs

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
)

type ErrorTemplateRecursionLimit struct {
	sources []string
}

func (err ErrorTemplateRecursionLimit) Error() string {
	trace := make([]string, 0)
	maxTrace := 16

	// Look for repetition in the first N sources
	for _, source := range err.sources[:min(maxTrace, len(err.sources))] {
		looped := slices.Contains(trace, source)

		trace = append(trace, source)

		if looped {
			break
		}
	}

	return fmt.Sprintf("maximum recursion depth %d reached in %s ... ", len(err.sources)-1, strings.Join(trace, ", "))
}

// BasenameFunc constructs a function that takes a string containing a filesystem path
// and removes all except the last portion from it.
var BasenameFunc = function.New(&function.Spec{
	Params: []function.Parameter{
		{
			Name: "path",
			Type: cty.String,
		},
	},
	Type:         function.StaticReturnType(cty.String),
	RefineResult: refineNotNull,
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		return cty.StringVal(filepath.Base(args[0].AsString())), nil
	},
})

// DirnameFunc constructs a function that takes a string containing a filesystem path
// and removes the last portion from it.
var DirnameFunc = function.New(&function.Spec{
	Params: []function.Parameter{
		{
			Name: "path",
			Type: cty.String,
		},
	},
	Type:         function.StaticReturnType(cty.String),
	RefineResult: refineNotNull,
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		return cty.StringVal(filepath.Dir(args[0].AsString())), nil
	},
})

// Basename takes a string containing a filesystem path and removes all except the last portion from it.
//
// The underlying function implementation works only with the path string and does not access the filesystem itself.
// It is therefore unable to take into account filesystem features such as symlinks.
//
// If the path is empty then the result is ".", representing the current working directory.
func Basename(path cty.Value) (cty.Value, error) {
	return BasenameFunc.Call([]cty.Value{path})
}

// Dirname takes a string containing a filesystem path and removes the last portion from it.
//
// The underlying function implementation works only with the path string and does not access the filesystem itself.
// It is therefore unable to take into account filesystem features such as symlinks.
//
// If the path is empty then the result is ".", representing the current working directory.
func Dirname(path cty.Value) (cty.Value, error) {
	return DirnameFunc.Call([]cty.Value{path})
}
