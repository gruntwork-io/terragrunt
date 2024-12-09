package remote

import (
	"fmt"

	"github.com/mitchellh/mapstructure"
)

type RemoteEncryptionConfig interface {
	UnmarshalConfig(encryptionConfig map[string]interface{}) error
	ToMap() (map[string]interface{}, error)
}

type RemoteEncryptionKeyProvider interface {
	RemoteEncryptionKeyProviderPBKDF2 | RemoteEncryptionKeyProviderGCPKMS | RemoteEncryptionKeyProviderAWSKMS
}

type RemoteEncryptionKeyProviderBase struct {
	KeyProvider string `mapstructure:"key_provider"`
}

type GenericRemoteEncryptionKeyProvider[T RemoteEncryptionKeyProvider] struct {
	T T
}

func (b *GenericRemoteEncryptionKeyProvider[T]) UnmarshalConfig(encryptionConfig map[string]interface{}) error {
	// Decode the key provider type using the default decoder config
	if err := mapstructure.Decode(encryptionConfig, &b); err != nil {
		return fmt.Errorf("failed to decode key provider: %w", err)
	}

	// Decode the key provider properties using, setting ErrorUnused to true to catch any unused properties
	decoderConfig := &mapstructure.DecoderConfig{
		Result:      &b.T,
		ErrorUnused: true,
	}
	decoder, err := mapstructure.NewDecoder(decoderConfig)
	if err != nil {
		return fmt.Errorf("failed to create decoder: %w", err)
	}
	if err := decoder.Decode(encryptionConfig); err != nil {
		return fmt.Errorf("failed to decode key provider properties: %w", err)
	}

	return nil
}

func (b *GenericRemoteEncryptionKeyProvider[T]) ToMap() (map[string]interface{}, error) {
	var result map[string]interface{}
	err := mapstructure.Decode(b.T, &result)
	if err != nil {
		return nil, fmt.Errorf("failed to decode struct to map: %w", err)
	}
	return result, nil
}

func NewRemoteEncryptionKeyProvider(providerType string) (RemoteEncryptionConfig, error) {
	switch providerType {
	case "pbkdf2":
		return &GenericRemoteEncryptionKeyProvider[RemoteEncryptionKeyProviderPBKDF2]{}, nil
	case "gcp_kms":
		return &GenericRemoteEncryptionKeyProvider[RemoteEncryptionKeyProviderGCPKMS]{}, nil
	case "aws_kms":
		return &GenericRemoteEncryptionKeyProvider[RemoteEncryptionKeyProviderAWSKMS]{}, nil
	default:
		return nil, fmt.Errorf("unknown provider type: %s", providerType)
	}
}

type RemoteEncryptionKeyProviderPBKDF2 struct {
	RemoteEncryptionKeyProviderBase `mapstructure:",squash"`
	Passphrase                      string `mapstructure:"passphrase"`
	KeyLength                       int    `mapstructure:"key_length"`
	Iterations                      int    `mapstructure:"iterations"`
	SaltLength                      int    `mapstructure:"salt_length"`
	HashFunction                    string `mapstructure:"hash_function"`
}

type RemoteEncryptionKeyProviderAWSKMS struct {
	RemoteEncryptionKeyProviderBase `mapstructure:",squash"`
	KmsKeyId                        int    `mapstructure:"kms_key_id"`
	KeySpec                         string `mapstructure:"key_spec"`
}

type RemoteEncryptionKeyProviderGCPKMS struct {
	RemoteEncryptionKeyProviderBase `mapstructure:",squash"`
	KmsEncryptionKey                string `mapstructure:"kms_encryption_key"`
	KeyLength                       int    `mapstructure:"key_length"`
}
