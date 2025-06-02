package codegen_test

import (
	"bytes"
	"fmt"
	"os"
	"testing"

	"github.com/gruntwork-io/terragrunt/codegen"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemoteStateConfigToTerraformCode(t *testing.T) {
	t.Parallel()

	expectedOrdered := []byte(`terraform {
  backend "ordered" {
    a = 1
    b = 2
    c = 3
  }
  encryption {
    key_provider "test" "default" {
      a = 1
      b = 2
      c = 3
    }
    method "aes_gcm" "default" {
      keys = key_provider.test.default
    }
    state {
      method = method.aes_gcm.default
    }
    plan {
      method = method.aes_gcm.default
    }
  }
}
`)
	expectedEmptyConfig := []byte(`terraform {
  backend "empty" {
  }
  encryption {
    key_provider "test" "default" {
    }
    method "aes_gcm" "default" {
      keys = key_provider.test.default
    }
    state {
      method = method.aes_gcm.default
    }
    plan {
      method = method.aes_gcm.default
    }
  }
}
`)
	expectedEmptyEncryption := []byte(`terraform {
  backend "empty" {
  }
}
`)
	expectedS3WithAssumeRole := []byte(`terraform {
  backend "s3" {
    assume_role = {
      duration        = "1h30m"
      external_id     = "123456789012"
      policy          = "{}"
      policy_arns     = ["arn:aws:iam::123456789012:policy/MyPolicy"]
      role_arn        = "arn:aws:iam::123456789012:role/MyRole"
      session_name    = "MySession"
      source_identity = "123456789012"
      tags = {
        key = "value"
      }
      transitive_tag_keys = ["key"]
    }
    bucket = "mybucket"
  }
}
`)
	expectedS3WithAssumeRoleWithWebIdentity := []byte(`terraform {
  backend "s3" {
    assume_role_with_web_identity = {
      duration                = "1h30m"
      policy                  = "{}"
      policy_arns             = ["arn:aws:iam::123456789012:policy/MyPolicy"]
      role_arn                = "arn:aws:iam::123456789012:role/MyRole"
      session_name            = "MySession"
      web_identity_token      = "123456789012"
      web_identity_token_file = "/path/to/web_identity_token_file"
    }
    bucket = "mybucket"
  }
}
`)

	testCases := []struct {
		name       string
		backend    string
		config     map[string]any
		encryption map[string]any
		expected   []byte
		expectErr  bool
	}{
		{
			"remote-state-config-unsorted-keys",
			"ordered",
			map[string]any{
				"b": 2,
				"a": 1,
				"c": 3,
			},
			map[string]any{
				"key_provider": "test",
				"b":            2,
				"a":            1,
				"c":            3,
			},
			expectedOrdered,
			false,
		},
		{
			"remote-state-config-empty",
			"empty",
			map[string]any{},
			map[string]any{
				"key_provider": "test",
			},
			expectedEmptyConfig,
			false,
		},
		{
			"remote-state-encryption-empty",
			"empty",
			map[string]any{},
			map[string]any{},
			expectedEmptyEncryption,
			false,
		},
		{
			"remote-state-encryption-missing-key-provider",
			"empty",
			map[string]any{},
			map[string]any{
				"a": 1,
			},
			[]byte(""),
			true,
		},
		{
			"s3-backend-with-assume-role",
			"s3",
			map[string]any{
				"bucket":      "mybucket",
				"assume_role": "{role_arn=\"arn:aws:iam::123456789012:role/MyRole\",tags={key=\"value\"}, duration=\"1h30m\", external_id=\"123456789012\", policy=\"{}\", policy_arns=[\"arn:aws:iam::123456789012:policy/MyPolicy\"], session_name=\"MySession\", source_identity=\"123456789012\", transitive_tag_keys=[\"key\"]}",
			},
			map[string]any{},
			expectedS3WithAssumeRole,
			false,
		},
		{
			"s3-backend-with-assume-role-with-web-identity",
			"s3",
			map[string]any{
				"bucket":                        "mybucket",
				"assume_role_with_web_identity": "{role_arn=\"arn:aws:iam::123456789012:role/MyRole\",duration=\"1h30m\", policy=\"{}\", policy_arns=[\"arn:aws:iam::123456789012:policy/MyPolicy\"], session_name=\"MySession\", web_identity_token=\"123456789012\", web_identity_token_file=\"/path/to/web_identity_token_file\"}",
			},
			map[string]any{},
			expectedS3WithAssumeRoleWithWebIdentity,
			false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			output, err := codegen.RemoteStateConfigToTerraformCode(tc.backend, tc.config, tc.encryption)
			// validates the first output.
			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.True(t, bytes.Contains(output, []byte(tc.backend)))
				// Comparing as string produces a nicer diff
				assert.Equal(t, string(tc.expected), string(output))
			}

			// runs the function a few of times again. All the outputs must be
			// equal to the first output.
			for range 20 {
				actual, _ := codegen.RemoteStateConfigToTerraformCode(tc.backend, tc.config, tc.encryption)
				assert.Equal(t, output, actual)
			}
		})
	}
}

func TestFmtGeneratedFile(t *testing.T) {
	t.Parallel()

	testDir := t.TempDir()

	bTrue := true
	bFalse := false

	testCases := []struct {
		fmt      *bool
		name     string
		path     string
		contents string
		expected string
		ifExists codegen.GenerateConfigExists
		disabled bool
	}{
		{
			name:     "fmt-simple-hcl-file",
			fmt:      &bTrue,
			path:     fmt.Sprintf("%s/%s", testDir, "fmt_simple.hcl"),
			contents: "variable \"msg\"{\ntype=string\n  default=\"hello\"\n}\n",
			expected: "variable \"msg\" {\n  type    = string\n  default = \"hello\"\n}\n",
			ifExists: codegen.ExistsError,
		},
		{
			name:     "fmt-hcl-file-by-default",
			path:     fmt.Sprintf("%s/%s", testDir, "fmt_hcl_file_by_default.hcl"),
			contents: "variable \"msg\"{\ntype=string\n  default=\"hello\"\n}\n",
			expected: "variable \"msg\" {\n  type    = string\n  default = \"hello\"\n}\n",
			ifExists: codegen.ExistsError,
		},
		{
			name:     "ignore-hcl-fmt",
			fmt:      &bFalse,
			path:     fmt.Sprintf("%s/%s", testDir, "ignore_hcl_fmt.hcl"),
			contents: "variable \"msg\"{\ntype=string\n  default=\"hello\"\n}\n",
			expected: "variable \"msg\"{\ntype=string\n  default=\"hello\"\n}\n",
			ifExists: codegen.ExistsError,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			config := codegen.GenerateConfig{
				Path:             tc.path,
				IfExists:         tc.ifExists,
				CommentPrefix:    "",
				DisableSignature: true,
				Contents:         tc.contents,
				Disable:          tc.disabled,
				HclFmt:           tc.fmt,
			}

			opts, err := options.NewTerragruntOptionsForTest("mock-path-for-test.hcl")
			require.NoError(t, err)
			assert.NotNil(t, opts)

			l := logger.CreateLogger()
			err = codegen.WriteToFile(l, opts, "", config)
			require.NoError(t, err)

			assert.True(t, util.FileExists(tc.path))

			fileContent, err := os.ReadFile(tc.path)
			require.NoError(t, err)

			assert.Equal(t, tc.expected, string(fileContent))
		})
	}
}

func TestGenerateDisabling(t *testing.T) {
	t.Parallel()

	testDir := t.TempDir()

	testCases := []struct {
		name     string
		path     string
		contents string
		ifExists codegen.GenerateConfigExists
		disabled bool
	}{
		{
			name:     "generate-disabled-true",
			path:     fmt.Sprintf("%s/%s", testDir, "disabled_true"),
			contents: "this file should not be generated",
			ifExists: codegen.ExistsError,
			disabled: true,
		},
		{
			name:     "generate-disabled-false",
			path:     fmt.Sprintf("%s/%s", testDir, "disabled_false"),
			contents: "this file should be generated",
			ifExists: codegen.ExistsError,
			disabled: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			config := codegen.GenerateConfig{
				Path:             tc.path,
				IfExists:         tc.ifExists,
				CommentPrefix:    "",
				DisableSignature: false,
				Contents:         tc.contents,
				Disable:          tc.disabled,
			}

			opts, err := options.NewTerragruntOptionsForTest("mock-path-for-test.hcl")
			require.NoError(t, err)
			assert.NotNil(t, opts)

			l := logger.CreateLogger()
			err = codegen.WriteToFile(l, opts, "", config)
			require.NoError(t, err)

			if tc.disabled {
				assert.True(t, util.FileNotExists(tc.path))
			} else {
				assert.True(t, util.FileExists(tc.path))
			}
		})
	}
}

func TestReplaceAllCommasOutsideQuotesWithNewLines(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:  "happy-path-basic-replacement",
			input: `key=value,another=value,third=value`,
			expected: `key=value
another=value
third=value`,
		},
		{
			name:  "comma-inside-quotes",
			input: `key="value,with,commas",another=value`,
			expected: `key="value,with,commas"
another=value`,
		},
		{
			name:  "mixed-quotes-and-commas",
			input: `key="value,with,commas",simple=value,quoted="hello,world"`,
			expected: `key="value,with,commas"
simple=value
quoted="hello,world"`,
		},
		{
			name:     "empty-string",
			input:    ``,
			expected: ``,
		},
		{
			name:     "no-commas",
			input:    `key=value`,
			expected: `key=value`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			actual := codegen.ReplaceAllCommasOutsideQuotesWithNewLines(tc.input)
			assert.Equal(t, tc.expected, actual)
		})
	}
}
