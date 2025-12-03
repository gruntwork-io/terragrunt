// Package implementations provides production implementations of Azure service interfaces
package implementations

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage"

	"github.com/gruntwork-io/terragrunt/internal/azure/azurehelper"
	"github.com/gruntwork-io/terragrunt/internal/azure/interfaces"
	"github.com/gruntwork-io/terragrunt/internal/azure/types"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// StorageAccountServiceImpl is the production implementation of StorageAccountService
type StorageAccountServiceImpl struct {
	client *azurehelper.StorageAccountClient
}

const jwtTokenParts = 3

// NewStorageAccountService creates a new StorageAccountService implementation
func NewStorageAccountService(client *azurehelper.StorageAccountClient) interfaces.StorageAccountService {
	return &StorageAccountServiceImpl{
		client: client,
	}
}

// CreateStorageAccount creates a new storage account using the new types config
func (s *StorageAccountServiceImpl) CreateStorageAccount(ctx context.Context, cfg *types.StorageAccountConfig) error {
	// Convert the types.StorageAccountConfig to azurehelper.StorageAccountConfig
	helperConfig := azurehelper.StorageAccountConfig{
		StorageAccountName:    cfg.Name,
		ResourceGroupName:     cfg.ResourceGroupName,
		Location:              cfg.Location,
		EnableVersioning:      cfg.EnableVersioning,
		AllowBlobPublicAccess: cfg.AllowBlobPublicAccess,
		AccountKind:           string(cfg.AccountKind),
		AccountTier:           string(cfg.AccountTier),
		AccessTier:            string(cfg.AccessTier),
		ReplicationType:       string(cfg.ReplicationType),
		Tags:                  cfg.Tags,
	}

	return s.client.CreateStorageAccountIfNecessary(ctx, nil, helperConfig)
}

// DeleteStorageAccount deletes a storage account by resource group and account name
func (s *StorageAccountServiceImpl) DeleteStorageAccount(ctx context.Context, resourceGroupName, accountName string) error {
	return s.client.DeleteStorageAccount(ctx, log.Default())
}

// GetResourceID gets the resource ID of the storage account
func (s *StorageAccountServiceImpl) GetResourceID(ctx context.Context) string {
	// Use StorageAccountExists to get the account info which contains the ID
	_, account, err := s.client.StorageAccountExists(ctx)
	if err != nil || account == nil {
		return ""
	}

	if account.ID != nil {
		return *account.ID
	}

	return ""
}

// mapAzureAccountToInternalType converts an Azure SDK Account to our internal StorageAccount type
func (s *StorageAccountServiceImpl) mapAzureAccountToInternalType(account *armstorage.Account, resourceGroupName string) *types.StorageAccount {
	if account == nil {
		return nil
	}

	storageAccount := &types.StorageAccount{
		Name:              getStringValue(account.Name),
		ResourceGroupName: resourceGroupName,
		Location:          getStringValue(account.Location),
	}

	if account.Properties != nil {
		storageAccount.Properties = &types.StorageAccountProperties{
			SupportsHTTPSOnly: getBoolValue(account.Properties.EnableHTTPSTrafficOnly),
			IsHnsEnabled:      getBoolValue(account.Properties.IsHnsEnabled),
		}

		// Map provisioning state (it's an enum, need to convert)
		if account.Properties.ProvisioningState != nil {
			storageAccount.Properties.ProvisioningState = string(*account.Properties.ProvisioningState)
		}

		// Map access tier
		if account.Properties.AccessTier != nil {
			storageAccount.Properties.AccessTier = types.AccessTier(string(*account.Properties.AccessTier))
		}

		// Map primary status
		if account.Properties.StatusOfPrimary != nil {
			storageAccount.Properties.StatusOfPrimary = string(*account.Properties.StatusOfPrimary)
		}

		// Map secondary status
		if account.Properties.StatusOfSecondary != nil {
			storageAccount.Properties.StatusOfSecondary = string(*account.Properties.StatusOfSecondary)
		}

		// Map endpoints
		if account.Properties.PrimaryEndpoints != nil {
			storageAccount.Properties.PrimaryEndpoints = types.StorageEndpoints{
				Blob:  getStringValue(account.Properties.PrimaryEndpoints.Blob),
				Queue: getStringValue(account.Properties.PrimaryEndpoints.Queue),
				Table: getStringValue(account.Properties.PrimaryEndpoints.Table),
				File:  getStringValue(account.Properties.PrimaryEndpoints.File),
			}
		}

		if account.Properties.SecondaryEndpoints != nil {
			storageAccount.Properties.SecondaryEndpoints = types.StorageEndpoints{
				Blob:  getStringValue(account.Properties.SecondaryEndpoints.Blob),
				Queue: getStringValue(account.Properties.SecondaryEndpoints.Queue),
				Table: getStringValue(account.Properties.SecondaryEndpoints.Table),
				File:  getStringValue(account.Properties.SecondaryEndpoints.File),
			}
		}
	}

	// Map account kind from the top-level Kind field
	if account.Kind != nil {
		storageAccount.Properties.Kind = types.AccountKind(string(*account.Kind))
	}

	return storageAccount
}

// Helper functions for safe pointer dereferencing
func getStringValue(ptr *string) string {
	if ptr == nil {
		return ""
	}

	return *ptr
}

func getBoolValue(ptr *bool) bool {
	if ptr == nil {
		return false
	}

	return *ptr
}

// GetStorageAccount retrieves storage account information
func (s *StorageAccountServiceImpl) GetStorageAccount(ctx context.Context, resourceGroupName, accountName string) (*types.StorageAccount, error) {
	exists, account, err := s.client.StorageAccountExists(ctx)
	if err != nil {
		return nil, err
	}

	if !exists || account == nil {
		return nil, nil
	}

	return s.mapAzureAccountToInternalType(account, resourceGroupName), nil
}

// GetStorageAccountKeys retrieves storage account keys
func (s *StorageAccountServiceImpl) GetStorageAccountKeys(ctx context.Context, resourceGroupName, accountName string) ([]string, error) {
	return s.client.GetStorageAccountKeys(ctx)
}

// GetStorageAccountSAS generates a SAS token for the storage account
func (s *StorageAccountServiceImpl) GetStorageAccountSAS(ctx context.Context, resourceGroupName, accountName string) (string, error) {
	return s.client.GetStorageAccountSAS(ctx, "", nil)
}

// GetStorageAccountProperties retrieves properties of a storage account
func (s *StorageAccountServiceImpl) GetStorageAccountProperties(ctx context.Context, resourceGroupName, accountName string) (*types.StorageAccountProperties, error) {
	// Get the properties from the Azure client
	azureProps, err := s.client.GetStorageAccountProperties(ctx)
	if err != nil {
		return nil, err
	}

	if azureProps == nil {
		return nil, nil
	}

	// Convert Azure properties to our internal type
	props := &types.StorageAccountProperties{
		SupportsHTTPSOnly: getBoolValue(azureProps.EnableHTTPSTrafficOnly),
		IsHnsEnabled:      getBoolValue(azureProps.IsHnsEnabled),
	}

	// Map provisioning state
	if azureProps.ProvisioningState != nil {
		props.ProvisioningState = string(*azureProps.ProvisioningState)
	}

	// Map access tier
	if azureProps.AccessTier != nil {
		props.AccessTier = types.AccessTier(string(*azureProps.AccessTier))
	}

	// Map primary status
	if azureProps.StatusOfPrimary != nil {
		props.StatusOfPrimary = string(*azureProps.StatusOfPrimary)
	}

	// Map secondary status
	if azureProps.StatusOfSecondary != nil {
		props.StatusOfSecondary = string(*azureProps.StatusOfSecondary)
	}

	// Map endpoints
	if azureProps.PrimaryEndpoints != nil {
		props.PrimaryEndpoints = types.StorageEndpoints{
			Blob:  getStringValue(azureProps.PrimaryEndpoints.Blob),
			Queue: getStringValue(azureProps.PrimaryEndpoints.Queue),
			Table: getStringValue(azureProps.PrimaryEndpoints.Table),
			File:  getStringValue(azureProps.PrimaryEndpoints.File),
		}
	}

	if azureProps.SecondaryEndpoints != nil {
		props.SecondaryEndpoints = types.StorageEndpoints{
			Blob:  getStringValue(azureProps.SecondaryEndpoints.Blob),
			Queue: getStringValue(azureProps.SecondaryEndpoints.Queue),
			Table: getStringValue(azureProps.SecondaryEndpoints.Table),
			File:  getStringValue(azureProps.SecondaryEndpoints.File),
		}
	}

	return props, nil
}

// IsVersioningEnabled checks if blob versioning is enabled for the storage account
func (s *StorageAccountServiceImpl) IsVersioningEnabled(ctx context.Context) (bool, error) {
	return s.client.GetStorageAccountVersioning(ctx)
}

// ResourceGroupServiceImpl is the production implementation of ResourceGroupService
type ResourceGroupServiceImpl struct {
	client *azurehelper.ResourceGroupClient
}

// NewResourceGroupService creates a new ResourceGroupService implementation
func NewResourceGroupService(client *azurehelper.ResourceGroupClient) interfaces.ResourceGroupService {
	return &ResourceGroupServiceImpl{
		client: client,
	}
}

// EnsureResourceGroup ensures a resource group exists
func (r *ResourceGroupServiceImpl) EnsureResourceGroup(ctx context.Context, l log.Logger, resourceGroupName, location string, tags map[string]string) error {
	return r.client.EnsureResourceGroup(ctx, l, resourceGroupName, location, tags)
}

// ResourceGroupExists checks if a resource group exists
func (r *ResourceGroupServiceImpl) ResourceGroupExists(ctx context.Context, resourceGroupName string) (bool, error) {
	return r.client.ResourceGroupExists(ctx, resourceGroupName)
}

// DeleteResourceGroup deletes a resource group
func (r *ResourceGroupServiceImpl) DeleteResourceGroup(ctx context.Context, l log.Logger, resourceGroupName string) error {
	return r.client.DeleteResourceGroup(ctx, l, resourceGroupName)
}

// GetResourceGroup gets resource group information
func (r *ResourceGroupServiceImpl) GetResourceGroup(ctx context.Context, resourceGroupName string) (*armresources.ResourceGroup, error) {
	return r.client.GetResourceGroup(ctx, resourceGroupName)
}

// RBACServiceImpl is the production implementation of RBACService
type RBACServiceImpl struct {
	credential     azcore.TokenCredential
	subscriptionID string // Adding subscriptionID separately since it's not in the interface.RBACConfig
	config         interfaces.RBACConfig
}

// NewRBACService creates a new RBACService implementation
func NewRBACService(credential azcore.TokenCredential, config interfaces.RBACConfig, subscriptionID string) interfaces.RBACService {
	return &RBACServiceImpl{
		credential:     credential,
		config:         config,
		subscriptionID: subscriptionID,
	}
}

// AssignRole assigns a role to a principal at the specified scope
func (r *RBACServiceImpl) AssignRole(ctx context.Context, l log.Logger, roleName, principalID, scope string) error {
	// Get role definition ID from role name
	roleDefID, err := r.getRoleDefinitionID(ctx, roleName)
	if err != nil {
		return fmt.Errorf("failed to get role definition for %s: %w", roleName, err)
	}

	client, err := armauthorization.NewRoleAssignmentsClient(r.subscriptionID, r.credential, nil)
	if err != nil {
		return err
	}

	// Generate a unique role assignment name
	roleAssignmentName := azurehelper.GenerateUUID()

	assignment := armauthorization.RoleAssignmentCreateParameters{
		Properties: &armauthorization.RoleAssignmentProperties{
			PrincipalID:      &principalID,
			RoleDefinitionID: &roleDefID,
		},
	}

	_, err = client.Create(ctx, scope, roleAssignmentName, assignment, nil)
	if err != nil {
		l.Debugf("Failed to assign role %s to principal %s at scope %s: %v", roleName, principalID, scope, err)
		return err
	}

	l.Debugf("Successfully assigned role %s to principal %s at scope %s", roleName, principalID, scope)

	return nil
}

// RemoveRole removes a role assignment from a principal at the specified scope
func (r *RBACServiceImpl) RemoveRole(ctx context.Context, l log.Logger, roleName, principalID, scope string) error {
	client, err := armauthorization.NewRoleAssignmentsClient(r.subscriptionID, r.credential, nil)
	if err != nil {
		return err
	}

	// List existing role assignments to find the one to remove
	assignments, err := r.listRoleAssignments(ctx, scope)
	if err != nil {
		return err
	}

	for _, assignment := range assignments {
		props := assignment.Properties
		if props == nil || props.PrincipalID == nil || props.RoleDefinitionID == nil {
			continue
		}

		if *props.PrincipalID != principalID {
			continue
		}

		// Role name should match if provided
		if roleName != "" {
			roleDefID := *props.RoleDefinitionID
			if !strings.Contains(strings.ToLower(roleDefID), strings.ToLower(roleName)) {
				continue
			}
		}

		if assignment.Name == nil {
			continue
		}

		_, err = client.Delete(ctx, scope, *assignment.Name, nil)
		if err != nil {
			l.Debugf("Failed to remove role assignment %s: %v", *assignment.Name, err)
			return err
		}

		l.Debugf("Successfully removed role assignment %s", *assignment.Name)

		return nil
	}

	l.Debugf("No role assignment found for principal %s at scope %s", principalID, scope)

	return nil
}

// HasRoleAssignment checks if a principal has a specific role assignment at the given scope
func (r *RBACServiceImpl) HasRoleAssignment(ctx context.Context, principalID, roleDefinitionID, scope string) (bool, error) {
	assignments, err := r.listRoleAssignments(ctx, scope)
	if err != nil {
		return false, err
	}

	for _, assignment := range assignments {
		if assignment.Properties != nil &&
			assignment.Properties.PrincipalID != nil &&
			assignment.Properties.RoleDefinitionID != nil &&
			*assignment.Properties.PrincipalID == principalID &&
			*assignment.Properties.RoleDefinitionID == roleDefinitionID {
			return true, nil
		}
	}

	return false, nil
}

// ListRoleAssignments lists all role assignments at the specified scope
func (r *RBACServiceImpl) ListRoleAssignments(ctx context.Context, scope string) ([]interfaces.RoleAssignment, error) {
	assignments, err := r.listRoleAssignments(ctx, scope)
	if err != nil {
		return nil, err
	}

	result := make([]interfaces.RoleAssignment, 0, len(assignments))

	for _, assignment := range assignments {
		props := assignment.Properties
		if props == nil || props.PrincipalID == nil || props.RoleDefinitionID == nil || props.Scope == nil {
			continue
		}

		roleName := extractRoleNameFromDefinitionID(*props.RoleDefinitionID)

		roleAssignment := interfaces.RoleAssignment{
			RoleName:    roleName,
			PrincipalID: *props.PrincipalID,
			Scope:       *props.Scope,
			Description: "", // No description available from the SDK
		}

		result = append(result, roleAssignment)
	}

	return result, nil
}

// AssignStorageBlobDataOwnerRole assigns the Storage Blob Data Owner role to the current principal
func (r *RBACServiceImpl) AssignStorageBlobDataOwnerRole(ctx context.Context, l log.Logger, storageAccountScope string) error {
	// Storage Blob Data Owner role definition ID
	roleDefinitionID := "/subscriptions/" + r.subscriptionID + "/providers/Microsoft.Authorization/roleDefinitions/b7e6dc6d-f1e8-4753-8033-0f276bb0955b"

	principalID, err := r.GetPrincipalID(ctx)
	if err != nil {
		return err
	}

	return r.AssignRole(ctx, l, roleDefinitionID, principalID, storageAccountScope)
}

// GetCurrentPrincipal gets the current principal's information
func (r *RBACServiceImpl) GetCurrentPrincipal(ctx context.Context) (*interfaces.Principal, error) {
	token, err := r.credential.GetToken(ctx, policy.TokenRequestOptions{
		Scopes: []string{"https://management.azure.com/.default"},
	})
	if err != nil {
		return nil, err
	}

	// Parse the token to extract the principal ID (OID claim)
	principalID, principalType, err := r.extractPrincipalInfoFromToken(token.Token)
	if err != nil {
		return nil, err
	}

	return &interfaces.Principal{
		ID:   principalID,
		Type: principalType,
	}, nil
}

// GetPrincipal gets information about a specific principal
func (r *RBACServiceImpl) GetPrincipal(ctx context.Context, principalID string) (*interfaces.Principal, error) {
	// Currently we only support getting the current principal
	// The full implementation would require Azure AD Graph API access
	currentPrincipal, err := r.GetCurrentPrincipal(ctx)
	if err != nil {
		return nil, err
	}

	if currentPrincipal.ID == principalID {
		return currentPrincipal, nil
	}

	return nil, fmt.Errorf("principal %s not found or not accessible", principalID)
}

// GetPrincipalID gets the ID of the current principal
func (r *RBACServiceImpl) GetPrincipalID(ctx context.Context) (string, error) {
	principal, err := r.GetCurrentPrincipal(ctx)
	if err != nil {
		return "", err
	}

	return principal.ID, nil
}

// getRoleDefinitionID gets the full ID for a role by name
func (r *RBACServiceImpl) getRoleDefinitionID(ctx context.Context, roleName string) (string, error) {
	// Create client with updated SDK signature
	client, err := armauthorization.NewRoleDefinitionsClient(r.credential, nil)
	if err != nil {
		return "", err
	}

	scope := "/subscriptions/" + r.subscriptionID
	filter := fmt.Sprintf("roleName eq '%s'", roleName)

	// Use the updated SDK method signature
	pager := client.NewListPager(scope, &armauthorization.RoleDefinitionsClientListOptions{
		Filter: &filter,
	})

	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return "", err
		}

		for _, def := range page.Value {
			if def.Properties != nil && def.Properties.RoleName != nil && strings.EqualFold(*def.Properties.RoleName, roleName) {
				return *def.ID, nil
			}
		}
	}

	return "", fmt.Errorf("role definition '%s' not found", roleName)
}

// listRoleAssignments gets all role assignments at a scope
func (r *RBACServiceImpl) listRoleAssignments(ctx context.Context, scope string) ([]*armauthorization.RoleAssignment, error) {
	// Create client with updated SDK signature
	client, err := armauthorization.NewRoleAssignmentsClient(r.subscriptionID, r.credential, nil)
	if err != nil {
		return nil, err
	}

	var assignments []*armauthorization.RoleAssignment

	// Use the updated SDK method signature
	pager := client.NewListForScopePager(scope, nil)

	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}

		assignments = append(assignments, page.Value...)
	}

	return assignments, nil
}

// IsPermissionError checks if an error is a permission error
func (r *RBACServiceImpl) IsPermissionError(err error) bool {
	if err == nil {
		return false
	}

	errMsg := strings.ToLower(err.Error())

	unauthorized := strings.Contains(errMsg, "unauthorized")
	forbidden := strings.Contains(errMsg, "forbidden")
	noPermission := strings.Contains(errMsg, "no permission")
	accessDenied := strings.Contains(errMsg, "access denied")

	return unauthorized || forbidden || noPermission || accessDenied
}

// AuthenticationServiceImpl is the production implementation of AuthenticationService
type AuthenticationServiceImpl struct {
	credential azcore.TokenCredential
	config     interfaces.AuthenticationConfig
}

// NewAuthenticationService creates a new AuthenticationService implementation
func NewAuthenticationService(credential azcore.TokenCredential, config interfaces.AuthenticationConfig) interfaces.AuthenticationService {
	return &AuthenticationServiceImpl{
		credential: credential,
		config:     config,
	}
}

// GetCredential returns the current credential
func (a *AuthenticationServiceImpl) GetCredential(ctx context.Context, config map[string]interface{}) (azcore.TokenCredential, error) {
	// For production, we could implement different credential types based on config
	// For now, return the existing credential
	return a.credential, nil
}

// ValidateCredentials validates that the current credentials are valid
func (a *AuthenticationServiceImpl) ValidateCredentials(ctx context.Context) error {
	// Try to get a token to validate credentials
	token, err := a.credential.GetToken(ctx, policy.TokenRequestOptions{
		Scopes: []string{"https://management.azure.com/.default"},
	})
	if err != nil {
		return err
	}

	if token.Token == "" {
		return interfaces.ErrInvalidCredentials
	}

	return nil
}

// RefreshCredentials refreshes the current credentials if they support refresh
func (a *AuthenticationServiceImpl) RefreshCredentials(ctx context.Context) error {
	// Most Azure credentials auto-refresh, so this is typically a no-op
	return nil
}

// RefreshToken refreshes the current access token
func (a *AuthenticationServiceImpl) RefreshToken(ctx context.Context) error {
	// Most Azure credentials auto-refresh
	return nil
}

// IsServicePrincipal checks if using service principal auth
func (a *AuthenticationServiceImpl) IsServicePrincipal(ctx context.Context) (bool, error) {
	return a.config.ClientID != "" && a.config.ClientSecret != "", nil
}

// IsManagedIdentity checks if using managed identity auth
func (a *AuthenticationServiceImpl) IsManagedIdentity(ctx context.Context) (bool, error) {
	return a.config.UseManagedIdentity, nil
}

// GetCurrentPrincipal retrieves information about the currently authenticated principal
func (a *AuthenticationServiceImpl) GetCurrentPrincipal(ctx context.Context) (interface{}, error) {
	token, err := a.credential.GetToken(ctx, policy.TokenRequestOptions{
		Scopes: []string{"https://management.azure.com/.default"},
	})
	if err != nil {
		return nil, err
	}

	claims, err := parseJWTToken(token.Token)
	if err != nil {
		return nil, err
	}

	// Extract the principal information
	principalInfo := make(map[string]interface{})

	// Extract Object ID (principal ID)
	if oid, ok := claims["oid"].(string); ok && oid != "" {
		principalInfo["id"] = oid
	} else if sub, ok := claims["sub"].(string); ok && sub != "" {
		principalInfo["id"] = sub
	} else {
		return nil, errors.New("could not extract principal ID from token claims")
	}

	// Set the principal type
	if _, ok := claims["idp"].(string); ok {
		principalInfo["type"] = "User"
	} else {
		principalInfo["type"] = "ServicePrincipal"
	}

	return principalInfo, nil
}

// GetSubscriptionID returns the current subscription ID
func (a *AuthenticationServiceImpl) GetSubscriptionID(ctx context.Context) (string, error) {
	return a.config.SubscriptionID, nil
}

// GetTenantID returns the current tenant ID
func (a *AuthenticationServiceImpl) GetTenantID(ctx context.Context) (string, error) {
	return a.config.TenantID, nil
}

// GetAccessToken gets an access token for the specified scopes
func (a *AuthenticationServiceImpl) GetAccessToken(ctx context.Context, scopes []string) (string, error) {
	token, err := a.credential.GetToken(ctx, policy.TokenRequestOptions{
		Scopes: scopes,
	})
	if err != nil {
		return "", err
	}

	return token.Token, nil
}

// GetTokenClaims extracts claims from the current token
func (a *AuthenticationServiceImpl) GetTokenClaims(ctx context.Context) (map[string]interface{}, error) {
	token, err := a.credential.GetToken(ctx, policy.TokenRequestOptions{
		Scopes: []string{"https://management.azure.com/.default"},
	})
	if err != nil {
		return nil, err
	}

	return parseJWTToken(token.Token)
}

// GetAuthenticationMethod returns the current authentication method
func (a *AuthenticationServiceImpl) GetAuthenticationMethod(ctx context.Context) (string, error) {
	// This is a simple implementation that determines the auth method based on the credential type
	// In a real implementation, this would inspect the credential type more thoroughly
	// In a real implementation, we would determine this from the credential type
	if a.config.UseManagedIdentity {
		return "managed-identity", nil
	}

	if a.config.ClientID != "" {
		return "service-principal", nil
	}

	return "unknown", nil
}

// GetClientID returns the current client ID
func (a *AuthenticationServiceImpl) GetClientID(ctx context.Context) (string, error) {
	return a.config.ClientID, nil
}

// GetCloudEnvironment returns the current Azure cloud environment
func (a *AuthenticationServiceImpl) GetCloudEnvironment(ctx context.Context) (string, error) {
	// This could be configurable in the future, for now default to public cloud
	return "AzurePublicCloud", nil
}

// GetConfiguration returns the current authentication configuration
func (a *AuthenticationServiceImpl) GetConfiguration(ctx context.Context) (map[string]interface{}, error) {
	// Convert the config struct to a map
	return map[string]interface{}{
		"subscription_id":      a.config.SubscriptionID,
		"tenant_id":            a.config.TenantID,
		"client_id":            a.config.ClientID,
		"client_secret":        a.config.ClientSecret,
		"use_managed_identity": a.config.UseManagedIdentity,
	}, nil
}

// UpdateConfiguration updates the current authentication configuration with new values
func (a *AuthenticationServiceImpl) UpdateConfiguration(ctx context.Context, updates map[string]interface{}) error {
	if updates == nil {
		return nil
	}

	// Update subscription ID if provided
	if subscriptionID, ok := updates["subscriptionId"].(string); ok {
		a.config.SubscriptionID = subscriptionID
	}

	// Update tenant ID if provided
	if tenantID, ok := updates["tenantId"].(string); ok {
		a.config.TenantID = tenantID
	}

	// Update client ID if provided
	if clientID, ok := updates["clientId"].(string); ok {
		a.config.ClientID = clientID
	}

	// Update client secret if provided
	if clientSecret, ok := updates["clientSecret"].(string); ok {
		a.config.ClientSecret = clientSecret
	}

	// Update managed identity flag if provided
	if useManagedIdentity, ok := updates["useManagedIdentity"].(bool); ok {
		a.config.UseManagedIdentity = useManagedIdentity
	}

	// Validate the updated configuration
	return a.ValidateCredentials(ctx)
}

// IsAuthenticationError checks if an error is related to authentication
func (a *AuthenticationServiceImpl) IsAuthenticationError(err error) bool {
	if err == nil {
		return false
	}

	errMsg := strings.ToLower(err.Error())

	unauthorized := strings.Contains(errMsg, "unauthorized")
	authentication := strings.Contains(errMsg, "authentication")
	unauthenticated := strings.Contains(errMsg, "unauthenticated")
	invalidClient := strings.Contains(errMsg, "invalid_client")
	invalidToken := strings.Contains(errMsg, "invalid_token")
	tokenExpired := strings.Contains(errMsg, "token expired")
	aadError := strings.Contains(errMsg, "aadsts")

	return unauthorized || authentication || unauthenticated || invalidClient || invalidToken || tokenExpired || aadError
}

// IsAzureAD checks if using Azure AD authentication
func (a *AuthenticationServiceImpl) IsAzureAD(ctx context.Context) (bool, error) {
	// This would require inspecting the credential type or configuration
	// For now, we'll assume it's Azure AD if it's not service principal or managed identity
	isSP, err := a.IsServicePrincipal(ctx)
	if err != nil {
		return false, err
	}

	if isSP {
		return false, nil
	}

	isMSI, err := a.IsManagedIdentity(ctx)
	if err != nil {
		return false, err
	}

	if isMSI {
		return false, nil
	}

	// If not service principal or managed identity, assume Azure AD
	return true, nil
}

// ProductionServiceContainer implements the real Azure service container
type ProductionServiceContainer struct {
	config map[string]interface{}
	cache  map[string]interface{}
}

// NewProductionServiceContainer creates a new production service container
func NewProductionServiceContainer(config map[string]interface{}) interfaces.AzureServiceContainer {
	if config == nil {
		config = make(map[string]interface{})
	}

	return &ProductionServiceContainer{
		config: config,
		cache:  make(map[string]interface{}),
	}
}

// GetStorageAccountService returns a production storage account service
func (c *ProductionServiceContainer) GetStorageAccountService(ctx context.Context, l log.Logger, config map[string]interface{}) (interfaces.StorageAccountService, error) {
	// Merge container config with service config
	mergedConfig := mergeConfig(c.config, config)

	// Create Azure storage client
	client, err := createStorageClient(ctx, mergedConfig)
	if err != nil {
		return nil, err
	}

	return NewStorageAccountService(client), nil
}

// GetBlobService returns a production blob service
func (c *ProductionServiceContainer) GetBlobService(ctx context.Context, l log.Logger, config map[string]interface{}) (interfaces.BlobService, error) {
	// Merge container config with service config
	mergedConfig := mergeConfig(c.config, config)

	// Create Azure blob client
	client, err := createBlobClient(ctx, mergedConfig)
	if err != nil {
		return nil, err
	}

	return NewBlobService(client), nil
}

// GetRBACService returns a production RBAC service
func (c *ProductionServiceContainer) GetRBACService(ctx context.Context, l log.Logger, config map[string]interface{}) (interfaces.RBACService, error) {
	// Merge container config with service config
	mergedConfig := mergeConfig(c.config, config)

	// Create Azure RBAC client
	client, err := createRBACClient(mergedConfig)
	if err != nil {
		return nil, err
	}

	// Extract configuration
	subscriptionID, _ := config["subscriptionId"].(string)
	rbacConfig := interfaces.DefaultRBACConfig()

	return NewRBACService(client, rbacConfig, subscriptionID), nil
}

// GetAuthenticationService returns a production authentication service
func (c *ProductionServiceContainer) GetAuthenticationService(ctx context.Context, l log.Logger, config map[string]interface{}) (interfaces.AuthenticationService, error) {
	// Merge container config with service config
	mergedConfig := mergeConfig(c.config, config)

	// Extract configuration (empty values are valid defaults for optional fields)
	subscriptionID, _ := mergedConfig["subscriptionId"].(string)       //nolint:errcheck // type assertion returns zero value if missing
	tenantID, _ := mergedConfig["tenantId"].(string)                   //nolint:errcheck // type assertion returns zero value if missing
	clientID, _ := mergedConfig["clientId"].(string)                   //nolint:errcheck // type assertion returns zero value if missing
	clientSecret, _ := mergedConfig["clientSecret"].(string)           //nolint:errcheck // type assertion returns zero value if missing
	useManagedIdentity, _ := mergedConfig["useManagedIdentity"].(bool) //nolint:errcheck // type assertion returns zero value if missing

	authConfig := interfaces.AuthenticationConfig{
		SubscriptionID:     subscriptionID,
		TenantID:           tenantID,
		ClientID:           clientID,
		ClientSecret:       clientSecret,
		UseManagedIdentity: useManagedIdentity,
	}

	// Get credential from config
	cred, err := createAuthenticationCredential(authConfig)
	if err != nil {
		return nil, err
	}

	service := NewAuthenticationService(cred, authConfig)

	return service, nil
}

// GetResourceGroupService returns a production resource group service
func (c *ProductionServiceContainer) GetResourceGroupService(ctx context.Context, l log.Logger, config map[string]interface{}) (interfaces.ResourceGroupService, error) {
	// Merge container config with service config
	mergedConfig := mergeConfig(c.config, config)

	// Create Azure resource group client
	// Extract required fields from config
	subscriptionID, _ := mergedConfig["subscriptionId"].(string)
	if subscriptionID == "" {
		return nil, errors.New("subscription ID is required")
	}

	client, err := azurehelper.CreateResourceGroupClient(ctx, l, subscriptionID)
	if err != nil {
		return nil, err
	}

	return NewResourceGroupService(client), nil
}

// Cleanup performs any necessary cleanup operations for the service container
func (c *ProductionServiceContainer) Cleanup(ctx context.Context, l log.Logger) error {
	// No cleanup needed for production container
	// This method exists to satisfy the interface
	return nil
}

// GetRegisteredServices returns a list of currently registered service names
func (c *ProductionServiceContainer) GetRegisteredServices() []string {
	services := make([]string, 0, len(c.cache))
	for serviceName := range c.cache {
		services = append(services, serviceName)
	}

	return services
}

// GetServiceInfo returns information about a registered service
func (c *ProductionServiceContainer) GetServiceInfo(serviceName string) (map[string]interface{}, error) {
	if info, exists := c.cache[serviceName]; exists {
		return info.(map[string]interface{}), nil
	}

	return nil, fmt.Errorf("service %s not registered", serviceName)
}

// HasService checks if a specific service type is registered
func (c *ProductionServiceContainer) HasService(serviceType string) bool {
	_, exists := c.cache[serviceType]
	return exists
}

// Health checks the health of all services in the container
func (c *ProductionServiceContainer) Health(ctx context.Context, l log.Logger) error {
	// Check health of each registered service
	for serviceName, service := range c.cache {
		// Check if service implements a health check interface
		// For now, we'll just log that we're checking the service
		l.Debugf("Checking health of service: %s", serviceName)

		// TODO: Implement actual health checks for each service type
		// This could involve:
		// - Checking if credentials are valid
		// - Testing connectivity to Azure
		// - Verifying required permissions
		// For now, we'll just verify the service exists
		if service == nil {
			return fmt.Errorf("service %s is not properly initialized", serviceName)
		}
	}

	return nil
}

// Initialize initializes the service container with the provided configuration
func (c *ProductionServiceContainer) Initialize(ctx context.Context, l log.Logger, config map[string]interface{}) error {
	// Store the configuration
	c.config = mergeConfig(c.config, config)

	// Log initialization
	l.Debugf("Initializing Azure service container with configuration")

	// Validate required configuration
	if subscriptionID, ok := c.config["subscriptionId"].(string); !ok || subscriptionID == "" {
		return errors.New("subscription ID is required")
	}

	// Initialize the cache if it doesn't exist
	if c.cache == nil {
		c.cache = make(map[string]interface{})
	}

	// Optional: Pre-initialize commonly used services. This is optional and services can be created on-demand instead.
	if _, err := c.GetAuthenticationService(ctx, l, config); err != nil {
		l.Debugf("Warning: Failed to pre-initialize authentication service: %v", err)
	}

	return nil
}

// Reset clears all registered services and configuration from the container
func (c *ProductionServiceContainer) Reset(ctx context.Context, l log.Logger) error {
	c.config = make(map[string]interface{})
	c.cache = make(map[string]interface{})

	// Log the reset operation
	l.Debugf("Reset Azure service container - cleared all services and configuration")

	return nil
}

// RegisterAuthenticationService registers a custom AuthenticationService implementation
func (c *ProductionServiceContainer) RegisterAuthenticationService(service interfaces.AuthenticationService) {
	if service != nil {
		c.cache["authentication"] = service
	}
}

// RegisterBlobService registers a custom BlobService implementation
func (c *ProductionServiceContainer) RegisterBlobService(service interfaces.BlobService) {
	if service != nil {
		c.cache["blob"] = service
	}
}

// RegisterRBACService registers a custom RBACService implementation
func (c *ProductionServiceContainer) RegisterRBACService(service interfaces.RBACService) {
	if service != nil {
		c.cache["rbac"] = service
	}
}

// RegisterStorageAccountService registers a custom StorageAccountService implementation
func (c *ProductionServiceContainer) RegisterStorageAccountService(service interfaces.StorageAccountService) {
	if service != nil {
		c.cache["storageaccount"] = service
	}
}

// RegisterResourceGroupService registers a custom ResourceGroupService implementation
func (c *ProductionServiceContainer) RegisterResourceGroupService(service interfaces.ResourceGroupService) {
	if service != nil {
		c.cache["resourcegroup"] = service
	}
}

// Helper functions

// Helper function to extract role name from role definition ID
func extractRoleNameFromDefinitionID(roleDefinitionID string) string {
	// Role definition ID format is usually:
	// /subscriptions/{subscriptionId}/providers/Microsoft.Authorization/roleDefinitions/{definitionID}
	// For simplicity, just return the last part
	parts := strings.Split(roleDefinitionID, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}

	return roleDefinitionID
}

// Helper functions for client creation

func createStorageClient(ctx context.Context, config map[string]interface{}) (*azurehelper.StorageAccountClient, error) {
	// This is a stub that will return descriptive error
	// Using the existing function requires a logger and configuration
	// that may not be available in all contexts
	return nil, errors.New("storage account client creation not initialized: use proper initialization through a service container instead")
}

func createBlobClient(ctx context.Context, config map[string]interface{}) (*azurehelper.BlobServiceClient, error) {
	// This is a stub that will return descriptive error
	// Using the existing function requires a logger and configuration
	// that may not be available in all contexts
	return nil, errors.New("blob service client creation not initialized: use proper initialization through a service container instead")
}

func createRBACClient(config map[string]interface{}) (azcore.TokenCredential, error) {
	// Extract subscription ID from config
	subscriptionID, _ := config["subscriptionId"].(string)
	if subscriptionID == "" {
		return nil, errors.New("subscription ID is required for RBAC operations")
	}

	// Create credentials based on config
	tenantID, _ := config["tenantId"].(string)
	clientID, _ := config["clientId"].(string)
	clientSecret, _ := config["clientSecret"].(string)
	useManagedIdentity, _ := config["useManagedIdentity"].(bool)

	// Create the credential based on available authentication methods
	switch {
	case useManagedIdentity:
		// Use managed identity if specified
		cred, err := azidentity.NewDefaultAzureCredential(nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create default azure credential: %w", err)
		}

		return cred, nil
	case clientID != "" && clientSecret != "" && tenantID != "":
		// Use service principal if credentials are provided
		cred, err := azidentity.NewClientSecretCredential(tenantID, clientID, clientSecret, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create client secret credential: %w", err)
		}

		return cred, nil
	default:
		// Fall back to default credential
		cred, err := azidentity.NewDefaultAzureCredential(nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create default azure credential: %w", err)
		}

		return cred, nil
	}
}

// Helper function to create an authentication credential
func createAuthenticationCredential(config interfaces.AuthenticationConfig) (azcore.TokenCredential, error) {
	// Check which authentication method to use
	switch {
	case config.UseManagedIdentity:
		// Use managed identity if specified
		cred, err := azidentity.NewDefaultAzureCredential(nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create default azure credential: %w", err)
		}

		return cred, nil
	case config.ClientID != "" && config.ClientSecret != "" && config.TenantID != "":
		// Use service principal if credentials are provided
		cred, err := azidentity.NewClientSecretCredential(config.TenantID, config.ClientID, config.ClientSecret, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create client secret credential: %w", err)
		}

		return cred, nil
	case config.TenantID != "":
		// If tenant ID is provided, try to create a default credential
		options := &azidentity.DefaultAzureCredentialOptions{
			TenantID: config.TenantID,
		}

		cred, err := azidentity.NewDefaultAzureCredential(options)
		if err != nil {
			return nil, fmt.Errorf("failed to create default azure credential with tenant ID: %w", err)
		}

		return cred, nil
	default:
		// Fall back to default credential
		cred, err := azidentity.NewDefaultAzureCredential(nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create default azure credential: %w", err)
		}

		return cred, nil
	}
}

// Helper method to merge config maps
func mergeConfig(base, override map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	// Copy base config
	for k, v := range base {
		result[k] = v
	}

	// Override with specific config
	for k, v := range override {
		result[k] = v
	}

	return result
}

// IsPermissionError checks if an error is related to insufficient permissions
func (a *AuthenticationServiceImpl) IsPermissionError(err error) bool {
	if err == nil {
		return false
	}

	// Check common Azure authentication/permission error patterns
	errMsg := strings.ToLower(err.Error())

	forbidden := strings.Contains(errMsg, "forbidden")
	unauthorized := strings.Contains(errMsg, "unauthorized")
	insufficient := strings.Contains(errMsg, "insufficient privileges")
	accessDenied := strings.Contains(errMsg, "access denied")
	permission := strings.Contains(errMsg, "permission")
	roleAssignment := strings.Contains(errMsg, "role assignment")
	aadsts50105 := strings.Contains(errMsg, "aadsts50105")
	aadsts65001 := strings.Contains(errMsg, "aadsts65001")
	aadsts50001 := strings.Contains(errMsg, "aadsts50001")

	return forbidden || unauthorized || insufficient || accessDenied || permission || roleAssignment || aadsts50105 || aadsts65001 || aadsts50001
}

// IsTokenExpiredError checks if an error is related to an expired token
func (a *AuthenticationServiceImpl) IsTokenExpiredError(err error) bool {
	if err == nil {
		return false
	}

	errMsg := strings.ToLower(err.Error())

	tokenExpired := strings.Contains(errMsg, "token expired")
	tokenHasExpired := strings.Contains(errMsg, "token has expired")
	aadsts50013 := strings.Contains(errMsg, "aadsts50013")
	aadsts70043 := strings.Contains(errMsg, "aadsts70043")
	jwtExpired := strings.Contains(errMsg, "jwt token expired")
	tokenIsExpired := strings.Contains(errMsg, "token is expired")

	return tokenExpired || tokenHasExpired || aadsts50013 || aadsts70043 || jwtExpired || tokenIsExpired
}

// SetCloudEnvironment sets the current Azure cloud environment
func (a *AuthenticationServiceImpl) SetCloudEnvironment(ctx context.Context, environment string) error {
	// In a real implementation, this would validate and set the cloud environment
	// For now, we only support AzurePublicCloud
	if environment != "" && environment != "AzurePublicCloud" {
		return fmt.Errorf("unsupported cloud environment: %s, only AzurePublicCloud is currently supported", environment)
	}

	return nil
}

// extractPrincipalInfoFromToken parses a JWT token and extracts the principal ID and type
// For Azure AD tokens, the principal ID is typically in the "oid" claim (Object ID)
func (r *RBACServiceImpl) extractPrincipalInfoFromToken(tokenString string) (string, string, error) {
	claims, err := parseJWTToken(tokenString)
	if err != nil {
		return "", "", err
	}

	// Extract the Object ID (oid) claim which is the principal ID in Azure AD
	var principalID string
	if oid, ok := claims["oid"].(string); ok && oid != "" {
		principalID = oid
	} else if sub, ok := claims["sub"].(string); ok && sub != "" {
		// Fall back to subject claim if oid is not available
		principalID = sub
	} else {
		return "", "", errors.New("could not extract principal ID from token claims")
	}

	// Determine principal type
	principalType := "ServicePrincipal" // Default to service principal
	if idpVal, ok := claims["idp"].(string); ok && idpVal != "" {
		principalType = "User" // If idp claim is present, it's likely a user
	}

	return principalID, principalType, nil
}

// parseJWTToken is a helper function that parses a JWT token and returns its claims
func parseJWTToken(tokenString string) (map[string]interface{}, error) {
	// Split the JWT token into parts
	parts := strings.Split(tokenString, ".")
	if len(parts) != jwtTokenParts {
		return nil, errors.New("invalid token format")
	}

	// Decode the payload (second part)
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("error decoding token payload: %w", err)
	}

	// Parse the JSON payload
	var claims map[string]interface{}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("error parsing token claims: %w", err)
	}

	return claims, nil
}
