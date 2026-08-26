package config

import (
	"context"
	"fmt"
	"io"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	azurermbackend "github.com/gruntwork-io/terragrunt/internal/remotestate/backend/azurerm"
	"github.com/gruntwork-io/terragrunt/internal/vfs"

	"github.com/gruntwork-io/terragrunt/internal/remotestate"
	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// azureTrimmedStringConfigKeys reach the native backend verbatim, so surrounding whitespace selects a different identity or blob.
var azureTrimmedStringConfigKeys = []string{
	"access_key",
	"client_id",
	"client_secret",
	"container_name",
	"environment",
	"key",
	"oidc_token_file_path",
	"resource_group_name",
	"sas_token",
	"storage_account_name",
	"subscription_id",
	"tenant_id",
}

// azureUnsupportedConfigKeys are backend config keys whose mere presence requires the native backend path.
var azureUnsupportedConfigKeys = []string{
	"ado_service_connection_id",
	"client_certificate",
	"client_certificate_password",
	"client_certificate_path",
	"client_id_file_path",
	"client_secret_file_path",
	"customer_provided_key",
	"encryption_scope",
	"endpoint",
	"metadata_host",
	"msi_endpoint",
	"msi_resource_id",
	"oidc_token",
	// azurehelper never reads these, and the native backend treats an explicit empty value as suppressing its env fallback.
	"lookup_blob_endpoint",
	"oidc_request_token",
	"oidc_request_url",
	// azurehelper does not apply the native backend's request timeout.
	"timeout_seconds",
}

// azureProcessEnvPassthroughKeys reach the in-process SDK from the Terragrunt process, so a diverging dependency value changes transport behavior.
var azureProcessEnvPassthroughKeys = []string{
	"ALL_PROXY",
	"HTTPS_PROXY",
	"HTTP_PROXY",
	"NO_PROXY",
	"SSL_CERT_DIR",
	"SSL_CERT_FILE",
	"all_proxy",
	"http_proxy",
	"https_proxy",
	"no_proxy",
}

// azureTrimmedEnvKeys reach the native backend verbatim, so surrounding whitespace selects a different identity.
var azureTrimmedEnvKeys = []string{
	"ARM_ACCESS_KEY",
	"ARM_CLIENT_ID",
	"ARM_CLIENT_SECRET",
	"ARM_ENVIRONMENT",
	"ARM_OIDC_TOKEN_FILE_PATH",
	"ARM_SAS_TOKEN",
	"ARM_SUBSCRIPTION_ID",
	"ARM_TENANT_ID",
	"ARM_USE_AZUREAD",
	"ARM_USE_CLI",
	"ARM_USE_MSI",
	"ARM_USE_OIDC",
	"AZURE_FEDERATED_TOKEN_FILE",
}

// azureUnsupportedEnvKeys select native authentication, endpoint, timeout, or encryption behavior azurehelper does not mirror.
var azureUnsupportedEnvKeys = []string{
	"ARM_ADO_PIPELINE_SERVICE_CONNECTION_ID",
	"ARM_CLIENT_CERTIFICATE",
	"ARM_CLIENT_CERTIFICATE_PASSWORD",
	"ARM_CLIENT_CERTIFICATE_PATH",
	"ARM_CLIENT_ID_FILE_PATH",
	"ARM_CLIENT_SECRET_FILE_PATH",
	"ARM_CUSTOMER_PROVIDED_KEY",
	azureEncryptionScopeEnv,
	"ARM_METADATA_HOST",
	"ARM_METADATA_HOSTNAME",
	"ARM_MSI_ENDPOINT",
	"ARM_MSI_RESOURCE_ID",
	"ARM_OIDC_AZURE_SERVICE_CONNECTION_ID",
	"ARM_OIDC_REQUEST_TOKEN",
	"ARM_OIDC_REQUEST_URL",
	"ARM_OIDC_TOKEN",
	"ARM_RESOURCE_GROUP_NAME",
	"ARM_STORAGE_ACCOUNT_NAME",
	"ARM_TIMEOUT_SECONDS",
	"ARM_USE_AKS_WORKLOAD_IDENTITY",
	"ARM_USE_AZUREAD_AUTH",
	"ARM_USE_DNS_ZONE_ENDPOINT",
	"AZURESUBSCRIPTION_SERVICE_CONNECTION_ID",
	"AZURE_CLIENT_ID",
	"AZURE_CLIENT_SECRET",
	"AZURE_ENVIRONMENT",
	"AZURE_FEDERATED_TOKEN_FILE",
	"AZURE_MSI_RESOURCE_ID",
	"AZURE_RESOURCE_GROUP_NAME",
	"AZURE_STORAGE_ACCOUNT",
	"AZURE_STORAGE_KEY",
	"AZURE_STORAGE_SAS_TOKEN",
	"AZURE_SUBSCRIPTION_ID",
	"AZURE_TENANT_ID",
	"ACTIONS_ID_TOKEN_REQUEST_TOKEN",
	"ACTIONS_ID_TOKEN_REQUEST_URL",
	"SYSTEM_ACCESSTOKEN",
	"SYSTEM_OIDCREQUESTURI",
}

// azureManagedIdentityEnvKeys steer managed-identity discovery, which the in-process SDK reads from the Terragrunt process, not the venv.
var azureManagedIdentityEnvKeys = []string{
	"DEFAULT_IDENTITY_CLIENT_ID",
	"IDENTITY_ENDPOINT",
	"IDENTITY_HEADER",
	"IDENTITY_SERVER_THUMBPRINT",
	"IMDS_ENDPOINT",
	"MSI_ENDPOINT",
	"MSI_SECRET",
}

// azureWorkloadIdentityEnvKeys steer the AKS token proxy, which the in-process SDK reads from the Terragrunt process, not the venv.
var azureWorkloadIdentityEnvKeys = []string{
	"AZURE_KUBERNETES_CA_DATA",
	"AZURE_KUBERNETES_CA_FILE",
	"AZURE_KUBERNETES_SNI_NAME",
	"AZURE_KUBERNETES_TOKEN_PROXY",
}

// azureConfigEnvFallbacks maps a backend config key to the environment variables the native backend falls back to when it is absent.
var azureConfigEnvFallbacks = map[string][]string{
	"access_key":           {"ARM_ACCESS_KEY"},
	"client_id":            {"ARM_CLIENT_ID"},
	"client_secret":        {"ARM_CLIENT_SECRET"},
	"environment":          {"ARM_ENVIRONMENT"},
	"oidc_token_file_path": {"ARM_OIDC_TOKEN_FILE_PATH", "AZURE_FEDERATED_TOKEN_FILE"},
	"sas_token":            {"ARM_SAS_TOKEN"},
	"subscription_id":      {"ARM_SUBSCRIPTION_ID"},
	"tenant_id":            {"ARM_TENANT_ID"},
}

// azureSupportedCloudEnvironments are the cloud aliases azurehelper resolves to the same endpoints as the native backend.
var azureSupportedCloudEnvironments = []string{"", "public", "usgovernment", "china"}

// azureAuthToggles carries the authentication switches resolved from backend config and environment.
type azureAuthToggles struct {
	useAzureAD bool
	useMSI     bool
	useOIDC    bool
}

// azureCredentials carries the credential coordinates resolved from backend config and environment.
type azureCredentials struct {
	accessKey      string
	sasToken       string
	clientID       string
	clientSecret   string
	tenantID       string
	subscriptionID string
}

// azureDirectStateReadSupported reports whether azurehelper reads this state with the identity and validation the native azurerm backend would.
func azureDirectStateReadSupported(pctx *ParsingContext, remoteState *remotestate.RemoteState) bool {
	config := remoteState.BackendConfig
	if !azureBackendConfigSupported(config) {
		return false
	}

	if pctx.Venv == nil {
		return false
	}

	env := pctx.Venv.Env
	if !azureEnvSupported(pctx.Venv) {
		return false
	}

	toggles, ok := azureResolveAuthToggles(config, pctx.Venv)
	if !ok {
		return false
	}

	if azureConfigEmptyOverridesEnv(config, env) {
		return false
	}

	creds, ok := azureResolveCredentials(config, env)
	if !ok {
		return false
	}

	resourceGroup, ok := azureResolveStorageCoordinates(config)
	if !ok {
		return false
	}

	if !azureCloudEnvironmentSupported(config, env) {
		return false
	}

	tokenFile, ok := azureResolveOIDCTokenFile(pctx, config, env, toggles.useOIDC)
	if !ok {
		return false
	}

	return azureCredentialMethodSupported(toggles, creds, resourceGroup, tokenFile)
}

// azureBackendConfigSupported rejects config keys that are unknown, whitespace-padded, or handled only by the native backend.
func azureBackendConfigSupported(config backend.Config) bool {
	for key := range config {
		if !azureBackendConfigKeyKnown(key) {
			return false
		}
	}

	for _, key := range azureTrimmedStringConfigKeys {
		value, configured, valid := backendConfigString(config, key)
		if !valid || (configured && value != strings.TrimSpace(value)) {
			return false
		}
	}

	for _, key := range azureUnsupportedConfigKeys {
		if _, configured := config[key]; configured {
			return false
		}
	}

	return true
}

// azureEnvSupported rejects dependency environments that diverge from the Terragrunt process or that azurehelper cannot mirror.
func azureEnvSupported(v *venv.Venv) bool {
	env := v.Env

	for _, key := range azureProcessEnvPassthroughKeys {
		if value, configured := env[key]; configured && value != v.ProcessEnv(key) {
			return false
		}
	}

	for _, key := range azureTrimmedEnvKeys {
		if value := env[key]; value != strings.TrimSpace(value) {
			return false
		}
	}

	for _, key := range azureUnsupportedEnvKeys {
		if env[key] != "" {
			return false
		}
	}

	_, configured := env["AZURE_REGIONAL_AUTHORITY_NAME"]

	return !configured
}

// azureResolveAuthToggles rejects the auth switches azurehelper cannot honor and returns the ones later checks still need.
func azureResolveAuthToggles(config backend.Config, v *venv.Venv) (azureAuthToggles, bool) {
	env := v.Env

	if !azureCLIIdentitySupported(config, env) {
		return azureAuthToggles{}, false
	}

	useAzureAD := backendConfigBoolWithEnv(config, "use_azuread_auth", env["ARM_USE_AZUREAD"])
	if !useAzureAD.valid {
		return azureAuthToggles{}, false
	}

	useMSI := backendConfigBoolWithEnv(config, "use_msi", env["ARM_USE_MSI"])
	if !useMSI.valid {
		return azureAuthToggles{}, false
	}

	if useMSI.value && !azureEnvKeysUnset(env, azureManagedIdentityEnvKeys) {
		return azureAuthToggles{}, false
	}

	useOIDC := backendConfigBoolWithEnv(config, "use_oidc", env["ARM_USE_OIDC"])
	if !useOIDC.valid || (useMSI.value && useOIDC.value) {
		return azureAuthToggles{}, false
	}

	if useOIDC.value && !azureEnvKeysUnsetInVenvAndProcess(v, azureWorkloadIdentityEnvKeys) {
		return azureAuthToggles{}, false
	}

	if snapshot := backendConfigBoolWithEnv(config, "snapshot", env["ARM_SNAPSHOT"]); !snapshot.valid {
		return azureAuthToggles{}, false
	}

	return azureAuthToggles{
		useAzureAD: useAzureAD.value,
		useMSI:     useMSI.value,
		useOIDC:    useOIDC.value,
	}, true
}

// azureCLIIdentitySupported rejects AKS workload identity and any configuration that disables the Azure CLI credential.
func azureCLIIdentitySupported(config backend.Config, env map[string]string) bool {
	useAKS := backendConfigStrictBoolWithEnv(
		config,
		"use_aks_workload_identity",
		env["ARM_USE_AKS_WORKLOAD_IDENTITY"],
		false,
	)
	if !useAKS.valid || useAKS.value {
		return false
	}

	useCLI := backendConfigStrictBoolWithEnv(config, "use_cli", env["ARM_USE_CLI"], true)

	return useCLI.valid && useCLI.value
}

// azureEnvKeysUnset reports whether every key is unset, since even an empty value suppresses a native fallback.
func azureEnvKeysUnset(env map[string]string, keys []string) bool {
	for _, key := range keys {
		if _, configured := env[key]; configured {
			return false
		}
	}

	return true
}

// azureEnvKeysUnsetInVenvAndProcess also rejects keys the in-process SDK would read from the Terragrunt process.
func azureEnvKeysUnsetInVenvAndProcess(v *venv.Venv, keys []string) bool {
	for _, key := range keys {
		if _, configured := v.Env[key]; configured || v.ProcessEnv(key) != "" {
			return false
		}
	}

	return true
}

// azureConfigEmptyOverridesEnv reports whether an explicit empty config value suppresses an env fallback azurehelper still applies.
func azureConfigEmptyOverridesEnv(config backend.Config, env map[string]string) bool {
	for configKey, envKeys := range azureConfigEnvFallbacks {
		value, configured, _ := backendConfigString(config, configKey)
		if configured && value == "" && firstNonEmptyFromMap(env, envKeys...) != "" {
			return true
		}
	}

	return false
}

// azureResolveCredentials resolves each credential from config or its env fallback, rejecting conflicting shared-key sources.
func azureResolveCredentials(config backend.Config, env map[string]string) (*azureCredentials, bool) {
	accessKey, valid := backendConfigStringWithEnv(config, "access_key", env, "ARM_ACCESS_KEY")
	if !valid {
		return nil, false
	}

	sasToken, valid := backendConfigStringWithEnv(config, "sas_token", env, "ARM_SAS_TOKEN")
	if !valid || (accessKey != "" && sasToken != "") {
		return nil, false
	}

	clientID, valid := backendConfigStringWithEnv(config, "client_id", env, "ARM_CLIENT_ID")
	if !valid {
		return nil, false
	}

	clientSecret, valid := backendConfigStringWithEnv(config, "client_secret", env, "ARM_CLIENT_SECRET")
	if !valid {
		return nil, false
	}

	tenantID, valid := backendConfigStringWithEnv(config, "tenant_id", env, "ARM_TENANT_ID")
	if !valid {
		return nil, false
	}

	subscriptionID, valid := backendConfigStringWithEnv(config, "subscription_id", env, "ARM_SUBSCRIPTION_ID")
	if !valid {
		return nil, false
	}

	return &azureCredentials{
		accessKey:      accessKey,
		sasToken:       sasToken,
		clientID:       clientID,
		clientSecret:   clientSecret,
		tenantID:       tenantID,
		subscriptionID: subscriptionID,
	}, true
}

// azureResolveStorageCoordinates validates the blob coordinates and returns the resource group the storage-key lookup needs.
func azureResolveStorageCoordinates(config backend.Config) (string, bool) {
	resourceGroup, _, valid := backendConfigString(config, "resource_group_name")
	if !valid {
		return "", false
	}

	storageAccount, _, valid := backendConfigString(config, "storage_account_name")
	if !valid || !azureStorageAccountNameValid(storageAccount) {
		return "", false
	}

	container, _, valid := backendConfigString(config, "container_name")
	if !valid || !azureContainerNameValid(container) {
		return "", false
	}

	return resourceGroup, true
}

// azureCloudEnvironmentSupported keeps cloud aliases azurehelper does not resolve to the same endpoints on the native path.
func azureCloudEnvironmentSupported(config backend.Config, env map[string]string) bool {
	environment, valid := backendConfigStringWithEnv(config, "environment", env, "ARM_ENVIRONMENT")

	return valid && slices.Contains(azureSupportedCloudEnvironments, environment)
}

// azureResolveOIDCTokenFile rejects token files that are not absolute, not regular, or unreadable from this process.
func azureResolveOIDCTokenFile(
	pctx *ParsingContext,
	config backend.Config,
	env map[string]string,
	useOIDC bool,
) (string, bool) {
	tokenFile, valid := backendConfigStringWithEnv(
		config,
		"oidc_token_file_path",
		env,
		"ARM_OIDC_TOKEN_FILE_PATH",
	)
	if !valid || (tokenFile != "" && (!useOIDC || !filepath.IsAbs(tokenFile))) {
		return "", false
	}

	if tokenFile == "" {
		return "", true
	}

	if vfs.IsDir(pctx.Venv.FS, tokenFile) {
		return "", false
	}

	if _, err := vfs.ReadFile(pctx.Venv.FS, tokenFile); err != nil {
		return "", false
	}

	return tokenFile, true
}

// azureCredentialMethodSupported keeps credential methods and storage-key lookups azurehelper resolves differently.
func azureCredentialMethodSupported(
	toggles azureAuthToggles,
	creds *azureCredentials,
	resourceGroup string,
	tokenFile string,
) bool {
	hasSharedKey := creds.accessKey != "" || creds.sasToken != ""
	hasServicePrincipal := creds.clientID != "" && creds.clientSecret != "" && creds.tenantID != ""

	if !hasSharedKey && !hasServicePrincipal && !azureTokenCredentialComplete(toggles, creds, tokenFile) {
		return false
	}

	// Native token auth looks up a storage account key unless use_azuread_auth is set, and needs both ARM fields to do so.
	if !hasSharedKey && !toggles.useAzureAD && (resourceGroup == "" || creds.subscriptionID == "") {
		return false
	}

	return true
}

// azureTokenCredentialComplete rejects incomplete methods the native backend can skip past but azurehelper selects eagerly.
func azureTokenCredentialComplete(toggles azureAuthToggles, creds *azureCredentials, tokenFile string) bool {
	if toggles.useOIDC {
		return tokenFile != "" && creds.clientID != "" && creds.tenantID != ""
	}

	// The native backend's remaining default is Azure CLI only, while azurehelper falls back to a broader credential chain.
	return toggles.useMSI
}

func azureBackendConfigKeyKnown(key string) bool {
	switch key {
	case "access_key",
		"ado_service_connection_id",
		"client_certificate",
		"client_certificate_password",
		"client_certificate_path",
		"client_id",
		"client_id_file_path",
		"client_secret",
		"client_secret_file_path",
		"container_name",
		"customer_provided_key",
		"encryption_scope",
		"endpoint",
		"environment",
		"key",
		"lookup_blob_endpoint",
		"metadata_host",
		"msi_endpoint",
		"oidc_request_token",
		"oidc_request_url",
		"oidc_token",
		"oidc_token_file_path",
		"resource_group_name",
		"sas_token",
		"snapshot",
		"storage_account_name",
		"subscription_id",
		"tenant_id",
		"timeout_seconds",
		"use_aks_workload_identity",
		"use_azuread_auth",
		"use_cli",
		"use_msi",
		"use_oidc",
		// Terragrunt-only lifecycle settings are stripped before native init.
		"access_tier",
		"account_kind",
		"account_replication_type",
		"account_tier",
		"allow_blob_public_access",
		"enable_soft_delete",
		"location",
		"msi_resource_id",
		"skip_container_creation",
		"skip_resource_group_creation",
		"skip_storage_account_creation",
		"skip_versioning",
		"soft_delete_retention_days",
		"tags":
		return true
	default:
		return false
	}
}

func azureStorageAccountNameValid(name string) bool {
	if len(name) < 3 || len(name) > 24 {
		return false
	}

	for _, char := range name {
		if (char < 'a' || char > 'z') && (char < '0' || char > '9') {
			return false
		}
	}

	return true
}

func azureContainerNameValid(name string) bool {
	if len(name) < 3 || len(name) > 63 || name[0] == '-' || name[len(name)-1] == '-' {
		return false
	}

	previousHyphen := false

	for _, char := range name {
		if char == '-' {
			if previousHyphen {
				return false
			}

			previousHyphen = true

			continue
		}

		if (char < 'a' || char > 'z') && (char < '0' || char > '9') {
			return false
		}

		previousHyphen = false
	}

	return true
}

// getTerragruntOutputJSONFromRemoteStateAzurerm pulls the output directly from Azure Blob Storage without calling OpenTofu/Terraform.
func getTerragruntOutputJSONFromRemoteStateAzurerm(
	ctx context.Context,
	l log.Logger,
	pctx *ParsingContext,
	remoteState *remotestate.RemoteState,
	workspace string,
) ([]byte, error) {
	extendedConfig, err := azurermbackend.Config(remoteState.BackendConfig).ExtendedAzurermConfig()
	if err != nil {
		return nil, err
	}

	stateConfig := &extendedConfig.RemoteStateConfigAzurerm
	account := stateConfig.StorageAccountName
	container := stateConfig.ContainerName
	key := azurermStateBlobKey(stateConfig.Key, workspace)
	location := fmt.Sprintf("azurerm://%s/%s/%s", account, container, key)

	open := func(ctx context.Context, l log.Logger) (io.ReadCloser, error) {
		readCtx, cancel := context.WithTimeout(ctx, defaultAzureReadTimeout)

		backendConfig := maps.Clone(remoteState.BackendConfig)
		backendConfig["key"] = key

		reader, err := azurermbackend.OpenStateBlob(readCtx, l, pctx.Venv, backendConfig)
		if err != nil {
			cancel()

			return nil, fmt.Errorf("opening dependency state at %s: %w", location, err)
		}

		// The read runs under readCtx, so the timeout is released with the stream.
		return stateStream{
			Reader:  reader,
			closers: []io.Closer{reader, closerFunc(func() error { cancel(); return nil })},
		}, nil
	}

	return readDependencyStateOutputs(ctx, l, "dependency_output_state_azurerm", map[string]any{
		"account":   account,
		"container": container,
		"key":       key,
	}, location, open)
}

// azurermStateBlobKey mirrors the azurerm backend's workspace blob layout.
func azurermStateBlobKey(key, workspace string) string {
	if workspace == defaultStateWorkspace {
		return key
	}

	return key + "env:" + workspace
}
