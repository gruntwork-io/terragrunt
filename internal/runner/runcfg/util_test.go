package runcfg_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/runner/runcfg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdjustSourceWithMap(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		sourceMap      map[string]string
		source         string
		modulePath     string
		expectedResult string
		expectedError  string
	}{
		{
			name:           "empty source map returns source unchanged",
			sourceMap:      nil,
			source:         "git::ssh://git@github.com/org/repo.git//path/to/module",
			modulePath:     "/path/to/config.hcl",
			expectedResult: "git::ssh://git@github.com/org/repo.git//path/to/module",
			expectedError:  "",
		},
		{
			name: "basic source map match with subdirectory",
			sourceMap: map[string]string{
				"git::ssh://git@github.com/org/repo.git": "/local/path",
			},
			source:         "git::ssh://git@github.com/org/repo.git//path/to/module",
			modulePath:     "/path/to/config.hcl",
			expectedResult: "/local/path//path/to/module",
			expectedError:  "",
		},
		{
			name: "source map match with query parameters",
			sourceMap: map[string]string{
				"git::ssh://git@github.com/org/repo.git": "/local/path",
			},
			source:         "git::ssh://git@github.com/org/repo.git//path/to/module?ref=master",
			modulePath:     "/path/to/config.hcl",
			expectedResult: "/local/path//path/to/module",
			expectedError:  "",
		},
		{
			name: "source map match without subdirectory - extracts module name",
			sourceMap: map[string]string{
				"git::ssh://git@github.com/org/module-name.git": "/local/path",
			},
			source:         "git::ssh://git@github.com/org/module-name.git?ref=v1.0.0",
			modulePath:     "/path/to/config.hcl",
			expectedResult: "/local/path//module-name",
			expectedError:  "",
		},
		{
			name: "no match in source map returns source unchanged",
			sourceMap: map[string]string{
				"git::ssh://git@github.com/org/other-repo.git": "/local/path",
			},
			source:         "git::ssh://git@github.com/org/repo.git//path/to/module",
			modulePath:     "/path/to/config.hcl",
			expectedResult: "git::ssh://git@github.com/org/repo.git//path/to/module",
			expectedError:  "",
		},
		{
			name: "empty URL and subdir returns error",
			sourceMap: map[string]string{
				"git::ssh://git@github.com/org/repo.git": "/local/path",
			},
			source:         "",
			modulePath:     "/path/to/config.hcl",
			expectedResult: "",
			expectedError:  "invalid",
		},
		{
			name: "empty URL but has subdir returns source unchanged",
			sourceMap: map[string]string{
				"git::ssh://git@github.com/org/repo.git": "/local/path",
			},
			source:         "//path/to/module",
			modulePath:     "/path/to/config.hcl",
			expectedResult: "//path/to/module",
			expectedError:  "",
		},
		{
			name: "multiple source map entries - matches correct one",
			sourceMap: map[string]string{
				"git::ssh://git@github.com/org/repo1.git": "/local/path1",
				"git::ssh://git@github.com/org/repo2.git": "/local/path2",
			},
			source:         "git::ssh://git@github.com/org/repo2.git//path/to/module",
			modulePath:     "/path/to/config.hcl",
			expectedResult: "/local/path2//path/to/module",
			expectedError:  "",
		},
		{
			name: "source map with trailing slash in mapped path",
			sourceMap: map[string]string{
				"git::ssh://git@github.com/org/repo.git": "/local/path/",
			},
			source:         "git::ssh://git@github.com/org/repo.git//module",
			modulePath:     "/path/to/config.hcl",
			expectedResult: "/local/path//module",
			expectedError:  "",
		},
		{
			name: "source map with leading slash in subdirectory",
			sourceMap: map[string]string{
				"git::ssh://git@github.com/org/repo.git": "/local/path",
			},
			source:         "git::ssh://git@github.com/org/repo.git///module",
			modulePath:     "/path/to/config.hcl",
			expectedResult: "/local/path//module",
			expectedError:  "",
		},
		{
			name: "complex URL with multiple query parameters",
			sourceMap: map[string]string{
				"git::ssh://git@github.com/org/repo.git": "/local/path",
			},
			source:         "git::ssh://git@github.com/org/repo.git//path/to/module?ref=master&depth=1",
			modulePath:     "/path/to/config.hcl",
			expectedResult: "/local/path//path/to/module",
			expectedError:  "",
		},
		{
			name: "module name extraction from URL with .git extension",
			sourceMap: map[string]string{
				"git::ssh://git@github.com/org/my-terraform-module.git": "/local/path",
			},
			source:         "git::ssh://git@github.com/org/my-terraform-module.git",
			modulePath:     "/path/to/config.hcl",
			expectedResult: "/local/path//my-terraform-module",
			expectedError:  "",
		},
		{
			name: "module name extraction from URL without .git extension",
			sourceMap: map[string]string{
				"git::ssh://git@github.com/org/my-module": "/local/path",
			},
			source:         "git::ssh://git@github.com/org/my-module",
			modulePath:     "/path/to/config.hcl",
			expectedResult: "/local/path//my-module",
			expectedError:  "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result, err := runcfg.AdjustSourceWithMap(tc.sourceMap, tc.source, tc.modulePath)

			if tc.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedError)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expectedResult, result)
			}
		})
	}
}

func TestGetModulePathFromSourceURL(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		sourceURL      string
		expectedResult string
		expectedError  string
	}{
		{
			name:           "extract module name from git URL with .git",
			sourceURL:      "git::ssh://git@github.com/org/module-name.git",
			expectedResult: "module-name",
			expectedError:  "",
		},
		{
			name:           "extract module name from git URL without .git",
			sourceURL:      "git::ssh://git@github.com/org/module-name",
			expectedResult: "module-name",
			expectedError:  "",
		},
		{
			name:           "extract module name with query parameters",
			sourceURL:      "git::ssh://git@github.com/org/my-module.git?ref=master",
			expectedResult: "my-module",
			expectedError:  "",
		},
		{
			name:           "extract module name with dashes",
			sourceURL:      "git::ssh://git@github.com/org/my-terraform-module.git",
			expectedResult: "my-terraform-module",
			expectedError:  "",
		},
		{
			name:           "extract module name with underscores",
			sourceURL:      "git::ssh://git@github.com/org/my_terraform_module.git",
			expectedResult: "my_terraform_module",
			expectedError:  "",
		},
		{
			name:           "invalid URL format returns error",
			sourceURL:      "invalid-url",
			expectedResult: "",
			expectedError:  "Unable to obtain the module path",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result, err := runcfg.GetModulePathFromSourceURL(tc.sourceURL)

			if tc.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedError)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expectedResult, result)
			}
		})
	}
}

func TestGetTerraformSourceURL(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name               string
		source             string
		sourceMap          map[string]string
		originalConfigPath string
		cfg                *runcfg.RunConfig
		expectedResult     string
		expectedError      string
	}{
		{
			name:      "source from options takes precedence",
			source:    "git::ssh://git@github.com/org/repo.git",
			sourceMap: map[string]string{},
			cfg: &runcfg.RunConfig{
				Terraform: runcfg.TerraformConfig{
					Source: "git::ssh://git@github.com/org/other-repo.git",
				},
			},
			expectedResult: "git::ssh://git@github.com/org/repo.git",
			expectedError:  "",
		},
		{
			name:   "source from config with source map",
			source: "",
			sourceMap: map[string]string{
				"git::ssh://git@github.com/org/repo.git": "/local/path",
			},
			originalConfigPath: "/path/to/config.hcl",
			cfg: &runcfg.RunConfig{
				Terraform: runcfg.TerraformConfig{
					Source: "git::ssh://git@github.com/org/repo.git//module?ref=master",
				},
			},
			expectedResult: "/local/path//module",
			expectedError:  "",
		},
		{
			name:           "no source returns current directory",
			source:         "",
			sourceMap:      map[string]string{},
			cfg:            &runcfg.RunConfig{},
			expectedResult: ".",
			expectedError:  "",
		},
		{
			name:           "nil terraform config returns current directory",
			source:         "",
			sourceMap:      map[string]string{},
			cfg:            &runcfg.RunConfig{},
			expectedResult: ".",
			expectedError:  "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result, err := runcfg.GetTerraformSourceURL(
				tc.source,
				tc.sourceMap,
				tc.originalConfigPath,
				tc.cfg,
			)

			if tc.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedError)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expectedResult, result)
			}
		})
	}
}

func TestShouldSkipCache(t *testing.T) {
	t.Parallel()

	const externalSource = "git::ssh://git@github.com/org/repo.git"

	testCases := []struct {
		cfg               *runcfg.RunConfig
		name              string
		source            string
		expectedErrSource string
		noCacheEnabled    bool
		expectedSkip      bool
	}{
		{
			name:           "experiment disabled keeps cache even when no_cache is set",
			noCacheEnabled: false,
			cfg:            &runcfg.RunConfig{Terraform: runcfg.TerraformConfig{NoCache: true}},
			expectedSkip:   false,
		},
		{
			name:           "no_cache unset keeps cache",
			noCacheEnabled: true,
			cfg:            &runcfg.RunConfig{},
			expectedSkip:   false,
		},
		{
			name:           "nil config keeps cache",
			noCacheEnabled: true,
			cfg:            nil,
			expectedSkip:   false,
		},
		{
			name:           "no_cache with no source skips cache",
			noCacheEnabled: true,
			cfg:            &runcfg.RunConfig{Terraform: runcfg.TerraformConfig{NoCache: true}},
			expectedSkip:   true,
		},
		{
			name:              "no_cache with config source errors",
			noCacheEnabled:    true,
			cfg:               &runcfg.RunConfig{Terraform: runcfg.TerraformConfig{NoCache: true, Source: externalSource}},
			expectedErrSource: externalSource,
		},
		{
			name:              "no_cache with CLI source errors",
			noCacheEnabled:    true,
			source:            externalSource,
			cfg:               &runcfg.RunConfig{Terraform: runcfg.TerraformConfig{NoCache: true}},
			expectedErrSource: externalSource,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			skip, err := runcfg.ShouldSkipCache(tc.noCacheEnabled, tc.source, tc.cfg)

			if tc.expectedErrSource != "" {
				var noCacheErr runcfg.NoCacheWithSourceError
				require.ErrorAs(t, err, &noCacheErr)
				assert.Equal(t, tc.expectedErrSource, noCacheErr.Source)
				assert.False(t, skip)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.expectedSkip, skip)
		})
	}
}

func TestNoCacheWithSourceError(t *testing.T) {
	t.Parallel()

	const rawSource = "git::https://user:ghp_SECRETTOKEN@github.com/org/repo.git"

	err := runcfg.NoCacheWithSourceError{Source: rawSource}

	var target runcfg.NoCacheWithSourceError
	require.ErrorAs(t, err, &target)
	assert.Equal(t, rawSource, target.Source, "raw source is preserved for callers that inspect it")

	errorMsg := err.Error()
	assert.NotContains(t, errorMsg, "ghp_SECRETTOKEN", "credentials must not leak into the error message")
	assert.NotContains(t, errorMsg, "user:")
	assert.Contains(t, errorMsg, "github.com/org/repo.git")
	assert.Contains(t, errorMsg, "no_cache")
	assert.Contains(t, errorMsg, "Remove the source")
}

func TestInvalidSourceURLWithMapError(t *testing.T) {
	t.Parallel()

	err := runcfg.InvalidSourceURLWithMapError{
		ModulePath:      "/path/to/config.hcl",
		ModuleSourceURL: "invalid-source",
	}

	errorMsg := err.Error()
	assert.Contains(t, errorMsg, "/path/to/config.hcl")
	assert.Contains(t, errorMsg, "invalid-source")
	assert.Contains(t, errorMsg, "invalid")
}

func TestParsingModulePathError(t *testing.T) {
	t.Parallel()

	err := runcfg.ParsingModulePathError{
		ModuleSourceURL: "git::invalid-url",
	}

	errorMsg := err.Error()
	assert.Contains(t, errorMsg, "git::invalid-url")
	assert.Contains(t, errorMsg, "Unable to obtain the module path")
}
