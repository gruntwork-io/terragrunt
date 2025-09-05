package remotestate

import (
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/mitchellh/mapstructure"
)

type RemoteEncryptionConfig interface {
	UnmarshalConfig(encryptionConfig map[string]any) error
	ToMap() (map[string]any, error)
}

type RemoteEncryptionKeyProvider interface {
	RemoteEncryptionKeyProviderPBKDF2 | RemoteEncryptionKeyProviderGCPKMS | RemoteEncryptionKeyProviderAWSKMS
}

type RemoteEncryptionKeyProviderBase struct {
	KeyProvider string `mapstructure:"key_provider"`
}

type GenericRemoteEncryptionKeyProvider[T RemoteEncryptionKeyProvider] struct {
	Data T `mapstructure:",squash"`
}

func (b *GenericRemoteEncryptionKeyProvider[T]) UnmarshalConfig(encryptionConfig map[string]any) error {
	if encryptionConfig == nil {
		return errors.New("encryption config is empty")
	}

	// Decode the key provider type using the default decoder config
	if err := mapstructure.Decode(encryptionConfig, &b); err != nil {
		return errors.Errorf("failed to decode key provider: %w", err)
	}

	// Decode the key provider properties using, setting ErrorUnused to true to catch any unused properties
	decoderConfig := &mapstructure.DecoderConfig{
		Result:      &b.Data,
		ErrorUnused: true,
	}

	decoder, err := mapstructure.NewDecoder(decoderConfig)
	if err != nil {
		return errors.Errorf("failed to create decoder: %w", err)
	}

	if err := decoder.Decode(encryptionConfig); err != nil {
		return errors.Errorf("failed to decode key provider properties: %w", err)
	}

	return nil
}

func (b *GenericRemoteEncryptionKeyProvider[T]) ToMap() (map[string]any, error) {
	var result map[string]any

	err := mapstructure.Decode(b.Data, &result)
	if err != nil {
		return nil, errors.Errorf("failed to decode struct to map: %w", err)
	}

	return result, nil
}

func (*RemoteEncryptionKeyProviderPBKDF2) Name() string {
	return "pbkdf2"
}

func (*RemoteEncryptionKeyProviderGCPKMS) Name() string {
	return "gcp_kms"
}

func (*RemoteEncryptionKeyProviderAWSKMS) Name() string {
	return "aws_kms"
}

func NewRemoteEncryptionKeyProvider(providerType string) (RemoteEncryptionConfig, error) {
	switch providerType {
	case new(RemoteEncryptionKeyProviderPBKDF2).Name():
		return &GenericRemoteEncryptionKeyProvider[RemoteEncryptionKeyProviderPBKDF2]{}, nil
	case new(RemoteEncryptionKeyProviderGCPKMS).Name():
		return &GenericRemoteEncryptionKeyProvider[RemoteEncryptionKeyProviderGCPKMS]{}, nil
	case new(RemoteEncryptionKeyProviderAWSKMS).Name():
		return &GenericRemoteEncryptionKeyProvider[RemoteEncryptionKeyProviderAWSKMS]{}, nil
	default:
		return nil, errors.Errorf("unknown provider type: %s", providerType)
	}
}

type RemoteEncryptionKeyProviderPBKDF2 struct {
	RemoteEncryptionKeyProviderBase `mapstructure:",squash"`
	Passphrase                      string `mapstructure:"passphrase"`
	HashFunction                    string `mapstructure:"hash_function"`
	KeyLength                       int    `mapstructure:"key_length"`
	Iterations                      int    `mapstructure:"iterations"`
	SaltLength                      int    `mapstructure:"salt_length"`
}

type RemoteEncryptionKeyProviderAWSKMS struct {
	RemoteEncryptionKeyProviderBase `mapstructure:",squash"`
	KmsKeyID                        string `mapstructure:"kms_key_id"`
	KeySpec                         string `mapstructure:"key_spec"`
	Region                          string `mapstructure:"region"`
}

type RemoteEncryptionKeyProviderGCPKMS struct {
	RemoteEncryptionKeyProviderBase `mapstructure:",squash"`
	KmsEncryptionKey                string `mapstructure:"kms_encryption_key"`
	KeyLength                       int    `mapstructure:"key_length"`
}
