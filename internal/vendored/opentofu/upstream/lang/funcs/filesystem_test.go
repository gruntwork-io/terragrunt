// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package funcs

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/zclconf/go-cty/cty"
)

func TestBasename(t *testing.T) {
	tests := []struct {
		Path cty.Value
		Want cty.Value
		Err  bool
	}{
		{
			cty.StringVal("testdata/hello.txt"),
			cty.StringVal("hello.txt"),
			false,
		},
		{
			cty.StringVal("hello.txt"),
			cty.StringVal("hello.txt"),
			false,
		},
		{
			cty.StringVal(""),
			cty.StringVal("."),
			false,
		},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("Basename(%#v)", test.Path), func(t *testing.T) {
			got, err := Basename(test.Path)

			if test.Err {
				if err == nil {
					t.Fatal("succeeded; want error")
				}
				return
			} else if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}

			if !got.RawEquals(test.Want) {
				t.Errorf("wrong result\ngot:  %#v\nwant: %#v", got, test.Want)
			}
		})
	}
}

func TestDirname(t *testing.T) {
	tests := []struct {
		Path cty.Value
		Want cty.Value
		Err  bool
	}{
		{
			cty.StringVal("testdata/hello.txt"),
			cty.StringVal("testdata"),
			false,
		},
		{
			cty.StringVal("testdata/foo/hello.txt"),
			cty.StringVal(filepath.FromSlash("testdata/foo")),
			false,
		},
		{
			cty.StringVal("hello.txt"),
			cty.StringVal("."),
			false,
		},
		{
			cty.StringVal(""),
			cty.StringVal("."),
			false,
		},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("Dirname(%#v)", test.Path), func(t *testing.T) {
			got, err := Dirname(test.Path)

			if test.Err {
				if err == nil {
					t.Fatal("succeeded; want error")
				}
				return
			} else if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}

			if !got.RawEquals(test.Want) {
				t.Errorf("wrong result\ngot:  %#v\nwant: %#v", got, test.Want)
			}
		})
	}
}
