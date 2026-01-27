package externalcmd_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/runner/run/creds/providers/externalcmd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateResponse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		input       string
		expectError bool
		errorCount  int
	}{
		{
			name:        "empty object is valid",
			input:       `{}`,
			expectError: false,
		},
		{
			name: "valid awsCredentials",
			input: `{
				"awsCredentials": {
					"ACCESS_KEY_ID": "fake-access-key-id",
					"SECRET_ACCESS_KEY": "fake-secret-access-key"
				}
			}`,
			expectError: false,
		},
		{
			name: "valid awsCredentials with session token",
			input: `{
				"awsCredentials": {
					"ACCESS_KEY_ID": "fake-access-key-id",
					"SECRET_ACCESS_KEY": "fake-secret-access-key",
					"SESSION_TOKEN": "fake-session-token"
				}
			}`,
			expectError: false,
		},
		{
			name: "valid awsRole",
			input: `{
				"awsRole": {
					"roleARN": "arn:aws:iam::123456789012:role/MyRole"
				}
			}`,
			expectError: false,
		},
		{
			name: "valid awsRole with all fields",
			input: `{
				"awsRole": {
					"roleARN": "arn:aws:iam::123456789012:role/MyRole",
					"roleSessionName": "my-session",
					"duration": 3600,
					"webIdentityToken": "fake-web-identity-token"
				}
			}`,
			expectError: false,
		},
		{
			name: "valid envs",
			input: `{
				"envs": {
					"MY_VAR": "my-value",
					"ANOTHER_VAR": "another-value"
				}
			}`,
			expectError: false,
		},
		{
			name: "valid combined response",
			input: `{
				"awsCredentials": {
					"ACCESS_KEY_ID": "fake-access-key-id",
					"SECRET_ACCESS_KEY": "fake-secret-access-key"
				},
				"envs": {
					"CUSTOM_VAR": "custom-value"
				}
			}`,
			expectError: false,
		},
		{
			name: "invalid awsCredentials missing ACCESS_KEY_ID",
			input: `{
				"awsCredentials": {
					"SECRET_ACCESS_KEY": "fake-secret-access-key"
				}
			}`,
			expectError: true,
			errorCount:  1,
		},
		{
			name: "invalid awsCredentials missing SECRET_ACCESS_KEY",
			input: `{
				"awsCredentials": {
					"ACCESS_KEY_ID": "fake-access-key-id"
				}
			}`,
			expectError: true,
			errorCount:  1,
		},
		{
			name: "invalid awsRole missing roleARN",
			input: `{
				"awsRole": {
					"roleSessionName": "my-session"
				}
			}`,
			expectError: true,
			errorCount:  1,
		},
		{
			name: "invalid additional property at root",
			input: `{
				"unknownField": "value"
			}`,
			expectError: true,
			errorCount:  1,
		},
		{
			name: "invalid additional property in awsCredentials",
			input: `{
				"awsCredentials": {
					"ACCESS_KEY_ID": "fake-access-key-id",
					"SECRET_ACCESS_KEY": "fake-secret-access-key",
					"unknownField": "value"
				}
			}`,
			expectError: true,
			errorCount:  1,
		},
		{
			name: "invalid duration negative",
			input: `{
				"awsRole": {
					"roleARN": "arn:aws:iam::123456789012:role/MyRole",
					"duration": -1
				}
			}`,
			expectError: true,
			errorCount:  1,
		},
		{
			name:        "invalid json",
			input:       `{invalid`,
			expectError: true,
		},
		{
			name: "awsCredentials with envs",
			input: `{
				"awsCredentials": {
					"ACCESS_KEY_ID": "fake-access-key-id",
					"SECRET_ACCESS_KEY": "fake-secret-access-key",
					"SESSION_TOKEN": "session-token-value"
				},
				"envs": {
					"TF_VAR_foo": "bar"
				}
			}`,
			expectError: false,
		},
		{
			name: "awsRole with webIdentityToken",
			input: `{
				"awsRole": {
					"roleARN": "arn:aws:iam::123456789012:role/OIDCRole",
					"webIdentityToken": "fake-web-identity-token"
				}
			}`,
			expectError: false,
		},
		{
			name: "envs only",
			input: `{
				"envs": {
					"AWS_ACCESS_KEY_ID": "fake-access-key",
					"AWS_SECRET_ACCESS_KEY": "fake-secret-key",
					"AWS_SESSION_TOKEN": "fake-session-token"
				}
			}`,
			expectError: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := externalcmd.ValidateResponse([]byte(tc.input))

			if !tc.expectError {
				require.NoError(t, err)

				return
			}

			require.Error(t, err)

			if tc.errorCount > 0 {
				var schemaErr *externalcmd.SchemaValidationError

				require.ErrorAs(t, err, &schemaErr)

				assert.Len(t, schemaErr.Errors, tc.errorCount)
			}
		})
	}
}

func TestSchemaValidationError_Error(t *testing.T) {
	t.Parallel()

	err := &externalcmd.SchemaValidationError{
		Errors: []string{"error1", "error2"},
	}

	assert.Contains(t, err.Error(), "2 error(s)")
	assert.Contains(t, err.Error(), "error1")
	assert.Contains(t, err.Error(), "error2")
}

// TestValidateResponse_NoSensitiveDataInErrors verifies that validation error messages
// do not leak sensitive credential values to users.
func TestValidateResponse_NoSensitiveDataInErrors(t *testing.T) {
	t.Parallel()

	// These are fake sensitive values that should NEVER appear in error messages
	sensitiveValues := []string{
		"fake-access-key-id",
		"fake-secret-key",
		"fake-session-token",
		"fake-web-identity-token",
		"super-secret-env-value-12345",
	}

	tests := []struct {
		name  string
		input string
	}{
		{
			name: "wrong type for ACCESS_KEY_ID should not leak value",
			input: `{
				"awsCredentials": {
					"ACCESS_KEY_ID": {"nested": "fake-access-key-id"},
					"SECRET_ACCESS_KEY": "fake-secret-key"
				}
			}`,
		},
		{
			name: "wrong type for SECRET_ACCESS_KEY should not leak value",
			input: `{
				"awsCredentials": {
					"ACCESS_KEY_ID": "fake-access-key-id",
					"SECRET_ACCESS_KEY": ["fake-secret-key"]
				}
			}`,
		},
		{
			name: "malformed SECRET_ACCESS_KEY should not leak value",
			input: `{
				"awsCredentials": {
					"ACCESS_KEY_ID": "fake-access-key-id",
					"SECRET_ACCESS_KEY": ["fake-secret-key
				}
			}`,
		},
		{
			name: "wrong type for SESSION_TOKEN should not leak value",
			input: `{
				"awsCredentials": {
					"ACCESS_KEY_ID": "fake-access-key-id",
					"SECRET_ACCESS_KEY": "fake-secret-key",
					"SESSION_TOKEN": {"token": "fake-session-token"}
				}
			}`,
		},
		{
			name: "wrong type for webIdentityToken should not leak value",
			input: `{
				"awsRole": {
					"roleARN": "arn:aws:iam::123456789012:role/MyRole",
					"webIdentityToken": ["fake-web-identity-token"]
				}
			}`,
		},
		{
			name: "additional property with sensitive value should not leak",
			input: `{
				"awsCredentials": {
					"ACCESS_KEY_ID": "fake-access-key-id",
					"SECRET_ACCESS_KEY": "fake-secret-key",
					"SUPER_SECRET_FIELD": "super-secret-env-value-12345"
				}
			}`,
		},
		{
			name: "additional property at root with sensitive value should not leak",
			input: `{
				"secretField": "super-secret-env-value-12345"
			}`,
		},
		{
			name: "wrong type for envs value should not leak",
			input: `{
				"envs": {
					"SECRET_VAR": {"secret": "super-secret-env-value-12345"}
				}
			}`,
		},
		{
			name: "wrong type for duration with credentials present should not leak credentials",
			input: `{
				"awsRole": {
					"roleARN": "arn:aws:iam::123456789012:role/MyRole",
					"webIdentityToken": "fake-web-identity-token",
					"duration": "not-a-number"
				}
			}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := externalcmd.ValidateResponse([]byte(tc.input))
			require.Error(t, err, "expected validation error")

			errMsg := err.Error()

			for _, sensitive := range sensitiveValues {
				assert.NotContains(t, errMsg, sensitive,
					"error message should not contain sensitive value")
			}
		})
	}
}

// TestValidateResponse_ErrorMessagesAreDescriptive verifies that error messages
// provide useful information about what went wrong without exposing values.
func TestValidateResponse_ErrorMessagesAreDescriptive(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		input           string
		expectedPhrases []string
	}{
		{
			name: "missing required field mentions field path",
			input: `{
				"awsCredentials": {
					"SECRET_ACCESS_KEY": "secret"
				}
			}`,
			expectedPhrases: []string{"ACCESS_KEY_ID"},
		},
		{
			name: "type error mentions expected type",
			input: `{
				"awsRole": {
					"roleARN": "arn:aws:iam::123456789012:role/MyRole",
					"duration": "not-a-number"
				}
			}`,
			expectedPhrases: []string{"duration"},
		},
		{
			name: "additional property error mentions the property name but not value",
			input: `{
				"unknownField": "secret-value"
			}`,
			expectedPhrases: []string{"unknownField"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := externalcmd.ValidateResponse([]byte(tc.input))
			require.Error(t, err)

			errMsg := err.Error()

			for _, phrase := range tc.expectedPhrases {
				assert.Contains(
					t,
					errMsg,
					phrase,
					"error message should mention the field/property name for debugging",
				)
			}
		})
	}
}
