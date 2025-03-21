package codegen_test

import (
	"bytes"
	"fmt"
	"os"
	"testing"

	"github.com/gruntwork-io/terragrunt/codegen"
	"github.com/gruntwork-io/terragrunt/options"
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

	tc := []struct {
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
	}

	for _, tt := range tc {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			output, err := codegen.RemoteStateConfigToTerraformCode(tt.backend, tt.config, tt.encryption)
			// validates the first output.
			if tt.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.True(t, bytes.Contains(output, []byte(tt.backend)))
				// Comparing as string produces a nicer diff
				assert.Equal(t, string(tt.expected), string(output))
			}

			// runs the function a few of times again. All the outputs must be
			// equal to the first output.
			for range 20 {
				actual, _ := codegen.RemoteStateConfigToTerraformCode(tt.backend, tt.config, tt.encryption)
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

	tc := []struct {
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

	for _, tt := range tc {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			config := codegen.GenerateConfig{
				Path:             tt.path,
				IfExists:         tt.ifExists,
				CommentPrefix:    "",
				DisableSignature: true,
				Contents:         tt.contents,
				Disable:          tt.disabled,
				HclFmt:           tt.fmt,
			}

			opts, err := options.NewTerragruntOptionsForTest("mock-path-for-test.hcl")
			require.NoError(t, err)
			assert.NotNil(t, opts)

			err = codegen.WriteToFile(opts, "", config)
			require.NoError(t, err)

			assert.True(t, util.FileExists(tt.path))

			fileContent, err := os.ReadFile(tt.path)
			require.NoError(t, err)

			assert.Equal(t, tt.expected, string(fileContent))
		})
	}
}

func TestGenerateDisabling(t *testing.T) {
	t.Parallel()

	testDir := t.TempDir()

	tc := []struct {
		name     string
		path     string
		contents string
		ifExists codegen.GenerateConfigExists
		disabled bool
	}{
		{
			"generate-disabled-true",
			true,
			fmt.Sprintf("%s/%s", testDir, "disabled_true"),
			"this file should not be generated",
			codegen.ExistsError,
		},
		{
			"generate-disabled-false",
			false,
			fmt.Sprintf("%s/%s", testDir, "disabled_false"),
			"this file should be generated",
			codegen.ExistsError,
		},
	}

	for _, tt := range tc {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			config := codegen.GenerateConfig{
				Path:             tt.path,
				IfExists:         tt.ifExists,
				CommentPrefix:    "",
				DisableSignature: false,
				Contents:         tt.contents,
				Disable:          tt.disabled,
			}

			opts, err := options.NewTerragruntOptionsForTest("mock-path-for-test.hcl")
			require.NoError(t, err)
			assert.NotNil(t, opts)

			err = codegen.WriteToFile(opts, "", config)
			require.NoError(t, err)

			if tt.disabled {
				assert.True(t, util.FileNotExists(tt.path))
			} else {
				assert.True(t, util.FileExists(tt.path))
			}
		})
	}
}
