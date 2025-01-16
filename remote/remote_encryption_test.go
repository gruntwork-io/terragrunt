package remote_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/remote"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnmarshalConfig(t *testing.T) {
	t.Parallel()

	tc := []struct {
		name             string
		providerType     string
		encryptionConfig map[string]interface{}
		expectedError    bool
	}{
		{
			name:         "PBKDF2 full config",
			providerType: "pbkdf2",
			encryptionConfig: map[string]interface{}{
				"key_provider":  "pbkdf2",
				"passphrase":    "passphrase",
				"key_length":    32,
				"iterations":    10000,
				"salt_length":   16,
				"hash_function": "sha256",
			},
			expectedError: false,
		},
		{
			name:         "PBKDF2 invalid property",
			providerType: "pbkdf2",
			encryptionConfig: map[string]interface{}{
				"key_provider": "pbkdf2",
				"password":     "password123", // Invalid property
			},
			expectedError: true,
		},
		{
			name:         "PBKDF2 invalid config",
			providerType: "pbkdf2",
			encryptionConfig: map[string]interface{}{
				"key_provider": "pbkdf2",
				"passphrase":   123, // Invalid type
			},
			expectedError: true,
		},
		{
			name:         "AWSKMS full config",
			providerType: "aws_kms",
			encryptionConfig: map[string]interface{}{
				"key_provider": "aws_kms",
				"kms_key_id":   "123456789",
				"key_spec":     "AES_256",
			},
			expectedError: false,
		},
		{
			name:         "AWSKMS invalid property",
			providerType: "aws_kms",
			encryptionConfig: map[string]interface{}{
				"key_provider": "aws_kms",
				"password":     "password123", // Invalid property
			},
			expectedError: true,
		},
		{
			name:         "AWSKMS invalid config",
			providerType: "aws_kms",
			encryptionConfig: map[string]interface{}{
				"key_provider": "aws_kms",
				"kms_key_id":   123456789, // Invalid type
				"key_spec":     "AES_256",
			},
			expectedError: true,
		},
		{
			name:         "GCPKMS full config",
			providerType: "gcp_kms",
			encryptionConfig: map[string]interface{}{
				"key_provider":       "gcp_kms",
				"kms_encryption_key": "projects/123456789/locations/global/keyRings/my-key-ring/cryptoKeys/my-key",
				"key_length":         32,
			},
			expectedError: false,
		},
		{
			name:         "GCPKMS invalid property",
			providerType: "gcp_kms",
			encryptionConfig: map[string]interface{}{
				"key_provider": "gcp_kms",
				"password":     "password123", // Invalid property
			},
			expectedError: true,
		},
		{
			name:         "GCPKMS invalid config",
			providerType: "gcp_kms",
			encryptionConfig: map[string]interface{}{
				"key_provider":       "gcp_kms",
				"kms_encryption_key": 123456789, // Invalid type
				"key_length":         32,
			},
			expectedError: true,
		},
	}

	for _, tt := range tc {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			provider, err := remote.NewRemoteEncryptionKeyProvider(tt.providerType)
			if err != nil {
				t.Fatalf("failed to create provider: %v", err)
			}

			err = provider.UnmarshalConfig(tt.encryptionConfig)
			if tt.expectedError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
func TestToMap(t *testing.T) {
	t.Parallel()

	tc := []struct {
		name             string
		providerType     string
		encryptionConfig map[string]interface{}
		expectedMap      map[string]interface{}
		expectedError    bool
	}{
		{
			name:         "PBKDF2 full config",
			providerType: "pbkdf2",
			encryptionConfig: map[string]interface{}{
				"key_provider":  "pbkdf2",
				"passphrase":    "passphrase",
				"key_length":    32,
				"iterations":    10000,
				"salt_length":   16,
				"hash_function": "sha256",
			},
			expectedMap: map[string]interface{}{
				"key_provider":  "pbkdf2",
				"passphrase":    "passphrase",
				"key_length":    32,
				"iterations":    10000,
				"salt_length":   16,
				"hash_function": "sha256",
			},
			expectedError: false,
		},
		{
			name:         "PBKDF2 partial config",
			providerType: "pbkdf2",
			encryptionConfig: map[string]interface{}{
				"key_provider": "pbkdf2",
				"passphrase":   "passphrase",
			},
			expectedMap: map[string]interface{}{
				"key_provider":  "pbkdf2",
				"passphrase":    "passphrase",
				"key_length":    0,
				"iterations":    0,
				"salt_length":   0,
				"hash_function": "",
			},
			expectedError: false,
		},
		{
			name:         "AWSKMS full config",
			providerType: "aws_kms",
			encryptionConfig: map[string]interface{}{
				"key_provider": "aws_kms",
				"region":       "us-west-1",
				"kms_key_id":   "123456789",
				"key_spec":     "AES_256",
			},
			expectedMap: map[string]interface{}{
				"key_provider": "aws_kms",
				"region":       "us-west-1",
				"kms_key_id":   "123456789",
				"key_spec":     "AES_256",
			},
			expectedError: false,
		},
		{
			name:         "GCPKMS full config",
			providerType: "gcp_kms",
			encryptionConfig: map[string]interface{}{
				"key_provider":       "gcp_kms",
				"kms_encryption_key": "projects/123456789/locations/global/keyRings/my-key-ring/cryptoKeys/my-key",
				"key_length":         32,
			},
			expectedMap: map[string]interface{}{
				"key_provider":       "gcp_kms",
				"kms_encryption_key": "projects/123456789/locations/global/keyRings/my-key-ring/cryptoKeys/my-key",
				"key_length":         32,
			},
			expectedError: false,
		},
	}

	for _, tt := range tc {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			provider, err := remote.NewRemoteEncryptionKeyProvider(tt.providerType)
			if err != nil {
				t.Fatalf("failed to create provider: %v", err)
			}

			err = provider.UnmarshalConfig(tt.encryptionConfig)
			if err != nil {
				t.Fatalf("failed to unmarshal config: %v", err)
			}

			result, err := provider.ToMap()
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedMap, result)
			}
		})
	}
}
