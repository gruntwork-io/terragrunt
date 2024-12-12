package codegen_test

import (
	"bytes"
	"fmt"
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

	tc := []struct {
		name       string
		backend    string
		config     map[string]interface{}
		encryption map[string]interface{}
		expected   []byte
		expectErr  bool
	}{
		{
			"remote-state-config-unsorted-keys",
			"ordered",
			map[string]interface{}{
				"b": 2,
				"a": 1,
				"c": 3,
			},
			map[string]interface{}{
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
			map[string]interface{}{},
			map[string]interface{}{
				"key_provider": "test",
			},
			expectedEmptyConfig,
			false,
		},
		{
			"remote-state-encryption-empty",
			"empty",
			map[string]interface{}{},
			map[string]interface{}{},
			expectedEmptyEncryption,
			false,
		},
		{
			"remote-state-encryption-missing-key-provider",
			"empty",
			map[string]interface{}{},
			map[string]interface{}{
				"a": 1,
			},
			[]byte(""),
			true,
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
				assert.Equal(t, tt.expected, output)
			}

			// runs the function a few of times again. All the outputs must be
			// equal to the first output.
			for i := 0; i < 20; i++ {
				actual, _ := codegen.RemoteStateConfigToTerraformCode(tt.backend, tt.config, tt.encryption)
				assert.Equal(t, output, actual)
			}
		})
	}
}

func TestGenerateDisabling(t *testing.T) {
	t.Parallel()

	testDir := t.TempDir()

	tc := []struct {
		name     string
		disabled bool
		path     string
		contents string
		ifExists codegen.GenerateConfigExists
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
