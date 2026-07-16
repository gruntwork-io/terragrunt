package config //nolint:testpackage // needs access to parseReadTerragruntConfigArgs

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
)

func TestParseReadTerragruntConfigArgs(t *testing.T) {
	t.Parallel()

	defaultVal := cty.ObjectVal(map[string]cty.Value{
		"data": cty.StringVal("default"),
	})

	//nolint:govet // fieldalignment is not important for test tables
	testCases := []struct {
		args        []cty.Value
		wantErrType any
		name        string
		wantPath    string
		wantCache   bool
		wantDefault bool
		wantErr     bool
	}{
		{
			name:      "path only",
			args:      []cty.Value{cty.StringVal("common.hcl")},
			wantCache: false,
			wantPath:  "common.hcl",
		},
		{
			name:        "path with default",
			args:        []cty.Value{cty.StringVal("common.hcl"), defaultVal},
			wantCache:   false,
			wantPath:    "common.hcl",
			wantDefault: true,
		},
		{
			name:      "with cache flag and path",
			args:      []cty.Value{cty.StringVal(terragruntWithCacheFlag), cty.StringVal("common.hcl")},
			wantCache: true,
			wantPath:  "common.hcl",
		},
		{
			name:        "with cache flag path and default",
			args:        []cty.Value{cty.StringVal(terragruntWithCacheFlag), cty.StringVal("common.hcl"), defaultVal},
			wantCache:   true,
			wantPath:    "common.hcl",
			wantDefault: true,
		},
		{
			name:        "no args",
			args:        []cty.Value{},
			wantErr:     true,
			wantErrType: WrongNumberOfParamsError{},
		},
		{
			name:        "cache flag only",
			args:        []cty.Value{cty.StringVal(terragruntWithCacheFlag)},
			wantErr:     true,
			wantErrType: WrongNumberOfParamsError{},
		},
		{
			name:        "too many args",
			args:        []cty.Value{cty.StringVal("a.hcl"), defaultVal, cty.StringVal("extra")},
			wantErr:     true,
			wantErrType: WrongNumberOfParamsError{},
		},
		{
			name:        "too many args with cache flag",
			args:        []cty.Value{cty.StringVal(terragruntWithCacheFlag), cty.StringVal("a.hcl"), defaultVal, cty.StringVal("extra")},
			wantErr:     true,
			wantErrType: WrongNumberOfParamsError{},
		},
		{
			name:        "unknown terragrunt flag",
			args:        []cty.Value{cty.StringVal("--terragrunt-unknown"), cty.StringVal("common.hcl")},
			wantErr:     true,
			wantErrType: UnknownReadTerragruntConfigOptionError{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			useCache, configPath, defaultValOut, err := parseReadTerragruntConfigArgs(tc.args)
			if tc.wantErr {
				require.Error(t, err)

				if tc.wantErrType != nil {
					require.ErrorAs(t, err, &tc.wantErrType)
				}

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.wantCache, useCache)
			assert.Equal(t, tc.wantPath, configPath)

			if tc.wantDefault {
				require.NotNil(t, defaultValOut)
				assert.True(t, defaultValOut.RawEquals(defaultVal))
			} else {
				assert.Nil(t, defaultValOut)
			}
		})
	}
}
