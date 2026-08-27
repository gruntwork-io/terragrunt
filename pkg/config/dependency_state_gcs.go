package config

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	gcsbackend "github.com/gruntwork-io/terragrunt/internal/remotestate/backend/gcs"

	"cloud.google.com/go/storage"
	"github.com/gruntwork-io/terragrunt/internal/remotestate"
	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// gcsStringConfigKeys names the backend settings a direct read consumes as strings.
var gcsStringConfigKeys = []string{
	"access_token",
	"bucket",
	"credentials",
	"encryption_key",
	"kms_encryption_key",
	"prefix",
}

// gcsUnsupportedConfigKeys names settings whose endpoint host or impersonation OAuth scope gcphelper resolves differently.
var gcsUnsupportedConfigKeys = []string{
	"impersonate_service_account",
	"impersonate_service_account_delegates",
	"storage_custom_endpoint",
	"universe_domain",
}

// gcsUnsupportedEnvKeys names venv variables the native backend honors and gcphelper does not.
var gcsUnsupportedEnvKeys = []string{
	"GOOGLE_BACKEND_CREDENTIALS",
	"GOOGLE_BACKEND_IMPERSONATE_SERVICE_ACCOUNT",
	"GOOGLE_BACKEND_IMPERSONATE_SERVICE_ACCOUNT_DELEGATES",
	"GOOGLE_BACKEND_STORAGE_CUSTOM_ENDPOINT",
	"GOOGLE_BACKEND_UNIVERSE_DOMAIN",
	"GOOGLE_IMPERSONATE_SERVICE_ACCOUNT",
	"GOOGLE_IMPERSONATE_SERVICE_ACCOUNT_DELEGATES",
}

// gcsUnsupportedProcessEnvKeys names variables that redirect the in-process client whether the venv or the process sets them.
var gcsUnsupportedProcessEnvKeys = []string{
	"GOOGLE_CLOUD_UNIVERSE_DOMAIN",
	"GOOGLE_EXTERNAL_ACCOUNT_ALLOW_EXECUTABLES",
	"GOOGLE_STORAGE_CUSTOM_ENDPOINT",
}

// gcsProcessSharedEnvKeys names variables the in-process client reads from the process, so a differing venv value diverges.
var gcsProcessSharedEnvKeys = []string{
	"ALL_PROXY",
	"APPDATA",
	"AWS_ACCESS_KEY_ID",
	"AWS_DEFAULT_REGION",
	"AWS_REGION",
	"AWS_SECRET_ACCESS_KEY",
	"AWS_SESSION_TOKEN",
	"GCE_METADATA_HOST",
	"GOOGLE_API_USE_CLIENT_CERTIFICATE",
	"GOOGLE_API_USE_MTLS_ENDPOINT",
	"GOOGLE_CLOUD_QUOTA_PROJECT",
	"HOME",
	"HTTPS_PROXY",
	"HTTP_PROXY",
	"NO_PROXY",
	"SSL_CERT_DIR",
	"SSL_CERT_FILE",
	"STORAGE_EMULATOR_HOST",
	"STORAGE_EMULATOR_HOST_GRPC",
	"USERPROFILE",
	"all_proxy",
	"http_proxy",
	"https_proxy",
	"no_proxy",
}

// gcsDirectStateReadSettings carries the backend values the GCS direct-read predicates share.
type gcsDirectStateReadSettings struct {
	accessToken             string
	credentials             string
	encryptionKey           string
	kmsEncryptionKey        string
	accessTokenConfigured   bool
	encryptionKeyConfigured bool
}

// gcsDirectStateReadSupported keeps configurations whose native GCS credential or
// endpoint semantics are not yet mirrored by gcphelper on the native backend path.
// This makes the experiment an optimization without changing which identity or host
// OpenTofu/Terraform would use.
func gcsDirectStateReadSupported(pctx *ParsingContext, remoteState *remotestate.RemoteState) bool {
	config := remoteState.BackendConfig
	if !gcsBackendConfigKeysSupported(config) {
		return false
	}

	settings, valid := gcsBackendConfigSettings(config)
	if !valid {
		return false
	}

	if !gcsConfiguredCredentialsSupported(settings) {
		return false
	}

	if pctx.Venv == nil {
		return false
	}

	if !gcsDirectStateReadEnvSupported(pctx.Venv, settings) {
		return false
	}

	return gcsCredentialSourcesSupported(pctx, settings)
}

// gcsEncryptionKeyDirectStateReadSupported limits direct reads to encryption-key
// forms whose path-or-contents behavior is independent of the native backend's
// working directory and home-directory expansion.
func gcsEncryptionKeyDirectStateReadSupported(value string) bool {
	if value == "" {
		return true
	}

	decoded, err := base64.StdEncoding.DecodeString(value)
	if err == nil && len(decoded) == gcsEncryptionKeyBytes {
		return true
	}

	return filepath.IsAbs(value) && !strings.HasPrefix(value, "~")
}

// gcsCredentialFileDirectStateReadSupported retains credential files whose
// authentication does not launch or configure an external subject-token
// provider from the Terragrunt process environment.
func gcsCredentialFileDirectStateReadSupported(pctx *ParsingContext, filename string) bool {
	if filename == "" {
		return true
	}

	file, err := pctx.Venv.FS.Open(filename)
	if err != nil {
		return false
	}

	var metadata struct {
		Type string `json:"type"`
	}

	decoder := json.NewDecoder(file)
	validJSON := decoder.Decode(&metadata) == nil

	if validJSON {
		_, trailingErr := decoder.Token()
		validJSON = errors.Is(trailingErr, io.EOF)
	}

	closeErr := file.Close()

	if !validJSON || closeErr != nil {
		return false
	}

	return metadata.Type == "service_account" || metadata.Type == "authorized_user"
}

func gcsBackendConfigKeyKnown(key string) bool {
	switch key {
	case "access_token",
		"bucket",
		"credentials",
		"encryption_key",
		"impersonate_service_account",
		"impersonate_service_account_delegates",
		"kms_encryption_key",
		"path",
		"prefix",
		"storage_custom_endpoint",
		"universe_domain",
		// Terragrunt-only bootstrap settings are stripped before native init and
		// do not change which state object or identity is used for a read.
		"enable_bucket_policy_only",
		"gcs_bucket_labels",
		"location",
		"project",
		"skip_bucket_creation",
		"skip_bucket_versioning":
		return true
	default:
		return false
	}
}

// getTerragruntOutputJSONFromRemoteStateGCS pulls the output directly from a GCS bucket without calling OpenTofu/Terraform.
func getTerragruntOutputJSONFromRemoteStateGCS(
	ctx context.Context,
	l log.Logger,
	pctx *ParsingContext,
	remoteState *remotestate.RemoteState,
	workspace string,
) ([]byte, error) {
	extendedConfig, err := gcsbackend.Config(remoteState.BackendConfig).ExtendedGCSConfig()
	if err != nil {
		return nil, err
	}

	stateConfig := &extendedConfig.RemoteStateConfigGCS
	bucket := stateConfig.Bucket
	key := gcsStateObjectKey(stateConfig, workspace)
	location := fmt.Sprintf("gs://%s/%s", bucket, key)

	open := func(ctx context.Context, l log.Logger) (io.ReadCloser, error) {
		client, err := gcsbackend.NewClient(ctx, pctx.Venv, extendedConfig, &backend.Options{})
		if err != nil {
			return nil, fmt.Errorf("building GCS client for %s: %w", location, err)
		}

		object, err := gcsObjectWithEncryptionKey(pctx, client.Bucket(bucket).Object(key), stateConfig.EncryptionKey)
		if err != nil {
			return nil, errors.Join(fmt.Errorf("configuring GCS state object %s: %w", location, err), client.Close())
		}

		reader, err := object.NewReader(ctx)
		if err != nil {
			return nil, errors.Join(fmt.Errorf("opening dependency state at %s: %w", location, err), client.Close())
		}

		return stateStream{Reader: reader, closers: []io.Closer{reader, client}}, nil
	}

	return readDependencyStateOutputs(ctx, l, "dependency_output_state_gcs", map[string]any{
		"bucket": bucket,
		"key":    key,
	}, location, open)
}

// gcsStateObjectKey mirrors the GCS backend's workspace object layout.
func gcsStateObjectKey(config *gcsbackend.RemoteStateConfigGCS, workspace string) string {
	return path.Join(strings.TrimLeft(config.Prefix, "/"), workspace+".tfstate")
}

// gcsObjectWithEncryptionKey applies the GCS backend's optional customer-supplied encryption key.
func gcsObjectWithEncryptionKey(
	pctx *ParsingContext,
	object *storage.ObjectHandle,
	configuredKey string,
) (*storage.ObjectHandle, error) {
	encodedKey := configuredKey
	if encodedKey == "" {
		encodedKey = pctx.Venv.Env["GOOGLE_ENCRYPTION_KEY"]
	}

	if encodedKey == "" {
		return object, nil
	}

	keyContents, err := gcsEncryptionKeyContents(pctx, encodedKey)
	if err != nil {
		return nil, err
	}

	decodedKey, err := base64.StdEncoding.DecodeString(keyContents)
	if err != nil {
		return nil, fmt.Errorf("decoding encryption_key as base64: %w", err)
	}

	if len(decodedKey) != gcsEncryptionKeyBytes {
		return nil, fmt.Errorf(
			"decoding encryption_key: expected %d bytes, got %d",
			gcsEncryptionKeyBytes,
			len(decodedKey),
		)
	}

	return object.Key(decodedKey), nil
}

// gcsEncryptionKeyContents resolves an encryption key as a dependency-relative file or literal data.
func gcsEncryptionKeyContents(pctx *ParsingContext, value string) (string, error) {
	filename := value
	if !filepath.IsAbs(filename) {
		filename = filepath.Join(filepath.Dir(pctx.TerragruntConfigPath), filename)
	}

	exists, err := vfs.FileExists(pctx.Venv.FS, filename)
	if err != nil {
		return "", fmt.Errorf("checking encryption_key file %s: %w", filename, err)
	}

	if !exists {
		return value, nil
	}

	if vfs.IsDir(pctx.Venv.FS, filename) {
		return "", fmt.Errorf("encryption_key path %s is a directory", filename)
	}

	contents, err := vfs.ReadFile(pctx.Venv.FS, filename)
	if err != nil {
		return "", fmt.Errorf("reading encryption_key file %s: %w", filename, err)
	}

	// Returned verbatim: the native backend decodes the raw file, and the base64
	// decoder already ignores CR and LF.
	return string(contents), nil
}

// gcsBackendConfigKeysSupported rejects backend settings a direct read cannot reproduce, whatever their value.
func gcsBackendConfigKeysSupported(config backend.Config) bool {
	for key := range config {
		if !gcsBackendConfigKeyKnown(key) {
			return false
		}
	}

	// path was removed from the native GCS backend and is rejected even when empty or null, so never bypass that.
	if _, configured := config["path"]; configured {
		return false
	}

	for _, key := range gcsUnsupportedConfigKeys {
		if backendConfigValueSet(config, key) {
			return false
		}
	}

	return true
}

// gcsBackendConfigSettings reads the shared settings, reporting false when the native backend would reject a value's type.
func gcsBackendConfigSettings(config backend.Config) (gcsDirectStateReadSettings, bool) {
	values := make(map[string]string)
	configuredValues := make(map[string]bool)

	for _, key := range gcsStringConfigKeys {
		value, configured, valid := backendConfigString(config, key)
		if !valid {
			return gcsDirectStateReadSettings{}, false
		}

		values[key] = value
		configuredValues[key] = configured
	}

	return gcsDirectStateReadSettings{
		accessToken:             values["access_token"],
		credentials:             values["credentials"],
		encryptionKey:           values["encryption_key"],
		kmsEncryptionKey:        values["kms_encryption_key"],
		accessTokenConfigured:   configuredValues["access_token"],
		encryptionKeyConfigured: configuredValues["encryption_key"],
	}, true
}

// gcsConfiguredCredentialsSupported leaves the native backend responsible for the values it validates or resolves itself.
func gcsConfiguredCredentialsSupported(settings gcsDirectStateReadSettings) bool {
	// The native backend rejects simultaneous CSEK and CMEK configuration.
	if settings.encryptionKey != "" && settings.kmsEncryptionKey != "" {
		return false
	}

	if settings.credentials == "" {
		return true
	}

	// gcphelper accepts only an absolute path for credentials, so inline JSON stays on the native path.
	return !strings.HasPrefix(strings.TrimSpace(settings.credentials), "{") && filepath.IsAbs(settings.credentials)
}

// gcsDirectStateReadEnvSupported rejects environments whose credential or encryption inputs the in-process client resolves differently.
func gcsDirectStateReadEnvSupported(v *venv.Venv, settings gcsDirectStateReadSettings) bool {
	env := v.Env

	// The same CSEK and CMEK conflict is the native backend's to reject once its environment fallbacks are folded in.
	if (settings.encryptionKey != "" || env["GOOGLE_ENCRYPTION_KEY"] != "") &&
		(settings.kmsEncryptionKey != "" || env["GOOGLE_KMS_ENCRYPTION_KEY"] != "") {
		return false
	}

	if !gcsUnsupportedEnvUnset(v) {
		return false
	}

	if !gcsProcessEnvDivergenceSupported(v) {
		return false
	}

	if !gcsEnvFallbackSuppressionSupported(env, settings) {
		return false
	}

	return gcsEncryptionKeyDirectStateReadSupported(gcsEffectiveEncryptionKey(env, settings))
}

// gcsUnsupportedEnvUnset rejects variables selecting an identity or host that gcphelper ignores or reads past the venv.
func gcsUnsupportedEnvUnset(v *venv.Venv) bool {
	env := v.Env

	for _, key := range gcsUnsupportedEnvKeys {
		if env[key] != "" {
			return false
		}
	}

	for _, key := range gcsUnsupportedProcessEnvKeys {
		processValue := os.Getenv(key) //nolint:forbidigo // Read under EnvLock.
		if env[key] != "" || processValue != "" {
			return false
		}
	}

	return true
}

// gcsProcessEnvDivergenceSupported rejects venv values the in-process client would not see, since it reads the real process.
func gcsProcessEnvDivergenceSupported(v *venv.Venv) bool {
	env := v.Env

	value, configured := env["GOOGLE_APPLICATION_CREDENTIALS"]

	processCredentials := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS") //nolint:forbidigo // Read under EnvLock.
	if configured && value == "" && processCredentials != "" {
		return false
	}

	for _, key := range gcsProcessSharedEnvKeys {
		processValue := os.Getenv(key) //nolint:forbidigo // Read under EnvLock.
		if value, configured := env[key]; configured && value != processValue {
			return false
		}
	}

	return true
}

// gcsEnvFallbackSuppressionSupported rejects empty configured values whose suppression of an environment fallback gcphelper misses.
func gcsEnvFallbackSuppressionSupported(env map[string]string, settings gcsDirectStateReadSettings) bool {
	if settings.accessTokenConfigured && settings.accessToken == "" && env["GOOGLE_OAUTH_ACCESS_TOKEN"] != "" {
		return false
	}

	if settings.encryptionKeyConfigured && settings.encryptionKey == "" && env["GOOGLE_ENCRYPTION_KEY"] != "" {
		return false
	}

	return true
}

// gcsEffectiveEncryptionKey mirrors the native backend's precedence of a configured key over the environment.
func gcsEffectiveEncryptionKey(env map[string]string, settings gcsDirectStateReadSettings) string {
	if settings.encryptionKeyConfigured {
		return settings.encryptionKey
	}

	return env["GOOGLE_ENCRYPTION_KEY"]
}

// gcsCredentialSourcesSupported rejects credential files and locations whose authentication the native backend performs differently.
func gcsCredentialSourcesSupported(pctx *ParsingContext, settings gcsDirectStateReadSettings) bool {
	env := pctx.Venv.Env
	applicationCredentials := env["GOOGLE_APPLICATION_CREDENTIALS"]

	if !gcsCredentialFileDirectStateReadSupported(pctx, settings.credentials) ||
		!gcsCredentialFileDirectStateReadSupported(pctx, applicationCredentials) {
		return false
	}

	if env["GOOGLE_CREDENTIALS"] != "" ||
		(applicationCredentials != "" && !filepath.IsAbs(applicationCredentials)) {
		return false
	}

	return gcsCredentialPrecedenceSupported(env, settings)
}

// gcsCredentialPrecedenceSupported rejects simultaneous sources, since OpenTofu prefers tokens then credentials then ADC while gcphelper inverts that.
func gcsCredentialPrecedenceSupported(env map[string]string, settings gcsDirectStateReadSettings) bool {
	applicationCredentials := env["GOOGLE_APPLICATION_CREDENTIALS"]
	explicitCredentials := settings.credentials != "" || applicationCredentials != ""

	if settings.accessToken != "" && explicitCredentials {
		return false
	}

	if env["GOOGLE_OAUTH_ACCESS_TOKEN"] != "" && explicitCredentials {
		return false
	}

	return settings.credentials == "" || applicationCredentials == ""
}
