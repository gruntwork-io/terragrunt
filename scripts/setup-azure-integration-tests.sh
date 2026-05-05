#!/usr/bin/env bash
#
# Azure Integration Test Setup Script
#
# This script helps configure environment variables needed to run Azure
# integration tests for Terragrunt. It supports multiple authentication
# methods and test isolation configurations.
#
# Usage:
#   1. Run this script: ./scripts/setup-azure-integration-tests.sh
#   2. Source the generated config: source ~/.terragrunt-azure-test-env
#   3. Run tests: go test -v ./test/integration_azure_test.go -tags=azure
#
# Or use inline sourcing:
#   source <(./scripts/setup-azure-integration-tests.sh --print-env)
#

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Config file location
CONFIG_FILE="${HOME}/.terragrunt-azure-test-env"

print_header() {
    echo -e "${BLUE}╔════════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${BLUE}║     Terragrunt Azure Integration Test Setup                     ║${NC}"
    echo -e "${BLUE}╚════════════════════════════════════════════════════════════════╝${NC}"
    echo ""
}

print_section() {
    echo ""
    echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${GREEN}  $1${NC}"
    echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
}

print_info() {
    echo -e "${BLUE}ℹ${NC}  $1"
}

print_warning() {
    echo -e "${YELLOW}⚠${NC}  $1"
}

print_success() {
    echo -e "${GREEN}✓${NC}  $1"
}

print_error() {
    echo -e "${RED}✗${NC}  $1"
}

prompt_with_default() {
    local prompt="$1"
    local default="$2"
    local var_name="$3"
    local current_value="${!var_name:-$default}"
    
    if [[ -n "$current_value" && "$current_value" != "$default" ]]; then
        echo -e -n "${prompt} [${YELLOW}${current_value}${NC}]: "
    elif [[ -n "$default" ]]; then
        echo -e -n "${prompt} [${YELLOW}${default}${NC}]: "
    else
        echo -e -n "${prompt}: "
    fi
    
    read -r input
    if [[ -z "$input" ]]; then
        eval "$var_name=\"$current_value\""
    else
        eval "$var_name=\"$input\""
    fi
}

prompt_secret() {
    local prompt="$1"
    local var_name="$2"
    local current_value="${!var_name:-}"
    
    if [[ -n "$current_value" ]]; then
        echo -e -n "${prompt} [${YELLOW}****hidden****${NC}]: "
    else
        echo -e -n "${prompt}: "
    fi
    
    read -rs input
    echo ""
    if [[ -z "$input" && -n "$current_value" ]]; then
        eval "$var_name=\"$current_value\""
    else
        eval "$var_name=\"$input\""
    fi
}

prompt_yes_no() {
    local prompt="$1"
    local default="$2"
    local var_name="$3"
    
    local default_display
    if [[ "$default" == "true" ]]; then
        default_display="Y/n"
    else
        default_display="y/N"
    fi
    
    echo -e -n "${prompt} [${YELLOW}${default_display}${NC}]: "
    read -r input
    
    # Convert to lowercase for comparison (compatible with bash 3.x)
    local input_lower
    input_lower=$(echo "$input" | tr '[:upper:]' '[:lower:]')
    
    case "$input_lower" in
        y|yes) eval "$var_name=true" ;;
        n|no) eval "$var_name=false" ;;
        *) eval "$var_name=$default" ;;
    esac
}

detect_existing_config() {
    print_section "Detecting Existing Configuration"
    
    # Check for existing environment variables
    if [[ -n "${AZURE_SUBSCRIPTION_ID:-}" ]]; then
        print_success "Found AZURE_SUBSCRIPTION_ID"
        SUBSCRIPTION_ID="${AZURE_SUBSCRIPTION_ID}"
    elif [[ -n "${ARM_SUBSCRIPTION_ID:-}" ]]; then
        print_success "Found ARM_SUBSCRIPTION_ID"
        SUBSCRIPTION_ID="${ARM_SUBSCRIPTION_ID}"
    fi
    
    if [[ -n "${AZURE_TENANT_ID:-}" ]]; then
        print_success "Found AZURE_TENANT_ID"
        TENANT_ID="${AZURE_TENANT_ID}"
    elif [[ -n "${ARM_TENANT_ID:-}" ]]; then
        print_success "Found ARM_TENANT_ID"
        TENANT_ID="${ARM_TENANT_ID}"
    fi
    
    if [[ -n "${AZURE_CLIENT_ID:-}" ]]; then
        print_success "Found AZURE_CLIENT_ID"
        CLIENT_ID="${AZURE_CLIENT_ID}"
    elif [[ -n "${ARM_CLIENT_ID:-}" ]]; then
        print_success "Found ARM_CLIENT_ID"
        CLIENT_ID="${ARM_CLIENT_ID}"
    fi
    
    if [[ -n "${AZURE_CLIENT_SECRET:-}" ]]; then
        print_success "Found AZURE_CLIENT_SECRET"
        CLIENT_SECRET="${AZURE_CLIENT_SECRET}"
    elif [[ -n "${ARM_CLIENT_SECRET:-}" ]]; then
        print_success "Found ARM_CLIENT_SECRET"
        CLIENT_SECRET="${ARM_CLIENT_SECRET}"
    fi
    
    # Check for test-specific variables
    if [[ -n "${TERRAGRUNT_AZURE_TEST_STORAGE_ACCOUNT:-}" ]]; then
        print_success "Found TERRAGRUNT_AZURE_TEST_STORAGE_ACCOUNT"
        STORAGE_ACCOUNT="${TERRAGRUNT_AZURE_TEST_STORAGE_ACCOUNT}"
    fi
    
    if [[ -n "${TERRAGRUNT_AZURE_TEST_RESOURCE_GROUP:-}" ]]; then
        print_success "Found TERRAGRUNT_AZURE_TEST_RESOURCE_GROUP"
        RESOURCE_GROUP="${TERRAGRUNT_AZURE_TEST_RESOURCE_GROUP}"
    fi
    
    if [[ -n "${TERRAGRUNT_AZURE_TEST_LOCATION:-}" ]]; then
        print_success "Found TERRAGRUNT_AZURE_TEST_LOCATION"
        LOCATION="${TERRAGRUNT_AZURE_TEST_LOCATION}"
    fi
    
    # Try to get subscription from Azure CLI
    if [[ -z "${SUBSCRIPTION_ID:-}" ]] && command -v az &> /dev/null; then
        if az account show &> /dev/null; then
            print_info "Azure CLI is logged in, fetching subscription info..."
            SUBSCRIPTION_ID=$(az account show --query id -o tsv 2>/dev/null || echo "")
            TENANT_ID=$(az account show --query tenantId -o tsv 2>/dev/null || echo "")
            if [[ -n "$SUBSCRIPTION_ID" ]]; then
                print_success "Found subscription from Azure CLI: ${SUBSCRIPTION_ID:0:8}..."
            fi
        else
            print_warning "Azure CLI found but not logged in. Run 'az login' for easier setup."
        fi
    fi
    
    # Load existing config file if present
    if [[ -f "$CONFIG_FILE" ]]; then
        print_info "Found existing config at $CONFIG_FILE"
        # shellcheck source=/dev/null
        source "$CONFIG_FILE" 2>/dev/null || true
    fi
}

select_auth_method() {
    print_section "Authentication Method"
    
    echo "Select authentication method:"
    echo "  1) Azure CLI (az login) - Recommended for local development"
    echo "  2) Service Principal (Client ID/Secret)"
    echo "  3) Managed Identity (for Azure VMs/containers)"
    echo "  4) Access Key (storage account key)"
    echo ""
    
    local default_auth="1"
    if [[ -n "${CLIENT_ID:-}" && -n "${CLIENT_SECRET:-}" ]]; then
        default_auth="2"
    fi
    
    echo -e -n "Select option [${YELLOW}${default_auth}${NC}]: "
    read -r auth_choice
    auth_choice="${auth_choice:-$default_auth}"
    
    case "$auth_choice" in
        1) AUTH_METHOD="azure_cli" ;;
        2) AUTH_METHOD="service_principal" ;;
        3) AUTH_METHOD="managed_identity" ;;
        4) AUTH_METHOD="access_key" ;;
        *) AUTH_METHOD="azure_cli" ;;
    esac
    
    print_success "Selected: $AUTH_METHOD"
}

configure_auth() {
    print_section "Authentication Configuration"
    
    case "$AUTH_METHOD" in
        azure_cli)
            print_info "Using Azure CLI authentication"
            print_info "Make sure you're logged in with: az login"
            
            if command -v az &> /dev/null && az account show &> /dev/null; then
                print_success "Azure CLI is authenticated"
            else
                print_warning "Azure CLI not authenticated. Run 'az login' before running tests."
            fi
            
            USE_AZUREAD_AUTH="true"
            ;;
            
        service_principal)
            print_info "Configuring Service Principal authentication"
            
            prompt_with_default "Client ID (Application ID)" "" CLIENT_ID
            prompt_secret "Client Secret" CLIENT_SECRET
            prompt_with_default "Tenant ID" "${TENANT_ID:-}" TENANT_ID
            
            if [[ -z "$CLIENT_ID" || -z "$CLIENT_SECRET" || -z "$TENANT_ID" ]]; then
                print_error "Service Principal requires Client ID, Secret, and Tenant ID"
                exit 1
            fi
            
            USE_AZUREAD_AUTH="true"
            ;;
            
        managed_identity)
            print_info "Using Managed Identity authentication"
            print_info "Make sure this script is running on an Azure resource with MI enabled"
            
            prompt_with_default "Client ID (for user-assigned MI, leave empty for system-assigned)" "" CLIENT_ID
            
            USE_AZUREAD_AUTH="true"
            ;;
            
        access_key)
            print_info "Configuring Storage Account Access Key authentication"
            
            prompt_with_default "Storage Account Name" "${STORAGE_ACCOUNT:-}" STORAGE_ACCOUNT
            prompt_secret "Access Key" ACCESS_KEY
            
            if [[ -z "$STORAGE_ACCOUNT" || -z "$ACCESS_KEY" ]]; then
                print_error "Access Key auth requires Storage Account Name and Access Key"
                exit 1
            fi
            
            USE_AZUREAD_AUTH="false"
            ;;
    esac
}

configure_subscription() {
    print_section "Azure Subscription"
    
    prompt_with_default "Subscription ID" "${SUBSCRIPTION_ID:-}" SUBSCRIPTION_ID
    
    if [[ -z "$SUBSCRIPTION_ID" ]]; then
        print_error "Subscription ID is required"
        exit 1
    fi
    
    print_success "Using subscription: ${SUBSCRIPTION_ID:0:8}..."
}

configure_resources() {
    print_section "Azure Resources"
    
    prompt_with_default "Location (Azure region)" "${LOCATION:-swedencentral}" LOCATION
    prompt_with_default "Resource Group (optional, leave empty for isolation)" "${RESOURCE_GROUP:-}" RESOURCE_GROUP
    prompt_with_default "Storage Account (optional, leave empty for isolation)" "${STORAGE_ACCOUNT:-}" STORAGE_ACCOUNT
}

configure_isolation() {
    print_section "Test Isolation"
    
    echo "Test isolation ensures parallel tests don't conflict with each other."
    echo ""
    echo "Isolation modes:"
    echo "  full      - Create unique storage accounts and resource groups per test"
    echo "  container - Create unique containers in a shared storage account"
    echo "  none      - Use shared resources (not recommended for parallel tests)"
    echo ""
    
    local default_isolation="full"
    if [[ -n "${STORAGE_ACCOUNT:-}" && -n "${RESOURCE_GROUP:-}" ]]; then
        default_isolation="container"
    fi
    
    prompt_with_default "Isolation mode" "$default_isolation" ISOLATION_MODE
    
    case "$ISOLATION_MODE" in
        full)
            ISOLATE_STORAGE="true"
            ISOLATE_RESOURCE_GROUP="true"
            ;;
        container)
            ISOLATE_STORAGE="false"
            ISOLATE_RESOURCE_GROUP="false"
            ;;
        none)
            ISOLATE_STORAGE="false"
            ISOLATE_RESOURCE_GROUP="false"
            ;;
        *)
            ISOLATION_MODE="full"
            ISOLATE_STORAGE="true"
            ISOLATE_RESOURCE_GROUP="true"
            ;;
    esac
    
    prompt_yes_no "Enable resource cleanup after tests" "true" CLEANUP_ENABLED
}

generate_config() {
    print_section "Generating Configuration"
    
    cat > "$CONFIG_FILE" << EOF
# Terragrunt Azure Integration Test Configuration
# Generated on $(date)
# Source this file before running tests: source $CONFIG_FILE

#
# Azure Authentication
#

# Subscription ID (required)
export AZURE_SUBSCRIPTION_ID="${SUBSCRIPTION_ID}"
export ARM_SUBSCRIPTION_ID="${SUBSCRIPTION_ID}"
export TERRAGRUNT_AZURE_TEST_SUBSCRIPTION_ID="${SUBSCRIPTION_ID}"
EOF

    if [[ -n "${TENANT_ID:-}" ]]; then
        cat >> "$CONFIG_FILE" << EOF

# Tenant ID
export AZURE_TENANT_ID="${TENANT_ID}"
export ARM_TENANT_ID="${TENANT_ID}"
EOF
    fi

    if [[ "$AUTH_METHOD" == "service_principal" ]]; then
        cat >> "$CONFIG_FILE" << EOF

# Service Principal Credentials
export AZURE_CLIENT_ID="${CLIENT_ID}"
export ARM_CLIENT_ID="${CLIENT_ID}"
export AZURE_CLIENT_SECRET="${CLIENT_SECRET}"
export ARM_CLIENT_SECRET="${CLIENT_SECRET}"
EOF
    elif [[ "$AUTH_METHOD" == "managed_identity" && -n "${CLIENT_ID:-}" ]]; then
        cat >> "$CONFIG_FILE" << EOF

# Managed Identity (user-assigned)
export AZURE_CLIENT_ID="${CLIENT_ID}"
export ARM_CLIENT_ID="${CLIENT_ID}"
EOF
    elif [[ "$AUTH_METHOD" == "access_key" ]]; then
        cat >> "$CONFIG_FILE" << EOF

# Storage Account Access Key
export TERRAGRUNT_AZURE_TEST_ACCESS_KEY="${ACCESS_KEY}"
EOF
    fi

    cat >> "$CONFIG_FILE" << EOF

#
# Azure Resources
#

# Location for creating test resources
export TERRAGRUNT_AZURE_TEST_LOCATION="${LOCATION}"
export AZURE_LOCATION="${LOCATION}"
export ARM_LOCATION="${LOCATION}"
EOF

    if [[ -n "${RESOURCE_GROUP:-}" ]]; then
        cat >> "$CONFIG_FILE" << EOF

# Resource Group (shared across tests)
export TERRAGRUNT_AZURE_TEST_RESOURCE_GROUP="${RESOURCE_GROUP}"
EOF
    fi

    if [[ -n "${STORAGE_ACCOUNT:-}" ]]; then
        cat >> "$CONFIG_FILE" << EOF

# Storage Account (shared across tests)
export TERRAGRUNT_AZURE_TEST_STORAGE_ACCOUNT="${STORAGE_ACCOUNT}"
EOF
    fi

    cat >> "$CONFIG_FILE" << EOF

#
# Test Isolation Configuration
#

# Isolation mode: full, container, or none
export TERRAGRUNT_AZURE_TEST_ISOLATION="${ISOLATION_MODE}"

# Create isolated storage accounts per test
export TERRAGRUNT_AZURE_TEST_ISOLATE_STORAGE="${ISOLATE_STORAGE}"

# Create isolated resource groups per test
export TERRAGRUNT_AZURE_TEST_ISOLATE_RESOURCE_GROUP="${ISOLATE_RESOURCE_GROUP}"

# Clean up resources after tests
export TERRAGRUNT_AZURE_TEST_CLEANUP="${CLEANUP_ENABLED}"

#
# Azure AD Authentication
#

# Use Azure AD instead of access keys
export TERRAGRUNT_AZURE_USE_AZUREAD_AUTH="${USE_AZUREAD_AUTH}"
export USE_AZUREAD_AUTH="${USE_AZUREAD_AUTH}"

#
# Go Test Configuration
#

# Enable Azure tests
export GOFLAGS="-tags=azure"
EOF

    chmod 600 "$CONFIG_FILE"
    print_success "Configuration saved to $CONFIG_FILE"
}

print_summary() {
    print_section "Configuration Summary"
    
    echo ""
    echo "Authentication Method: $AUTH_METHOD"
    echo "Subscription ID:       ${SUBSCRIPTION_ID:0:8}..."
    echo "Location:              $LOCATION"
    echo "Isolation Mode:        $ISOLATION_MODE"
    echo "Cleanup Enabled:       $CLEANUP_ENABLED"
    
    if [[ -n "${RESOURCE_GROUP:-}" ]]; then
        echo "Resource Group:        $RESOURCE_GROUP"
    else
        echo "Resource Group:        (auto-created per test)"
    fi
    
    if [[ -n "${STORAGE_ACCOUNT:-}" ]]; then
        echo "Storage Account:       $STORAGE_ACCOUNT"
    else
        echo "Storage Account:       (auto-created per test)"
    fi
    
    print_section "Next Steps"
    
    echo ""
    echo -e "${GREEN}1.${NC} Source the configuration:"
    echo -e "   ${YELLOW}source $CONFIG_FILE${NC}"
    echo ""
    echo -e "${GREEN}2.${NC} Run all Azure integration tests:"
    echo -e "   ${YELLOW}go test -v ./test/integration_azure_test.go -tags=azure${NC}"
    echo ""
    echo -e "${GREEN}3.${NC} Run a specific test:"
    echo -e "   ${YELLOW}go test -v ./test/integration_azure_test.go -tags=azure -run TestAzure${NC}"
    echo ""
    echo -e "${GREEN}4.${NC} Run with race detector:"
    echo -e "   ${YELLOW}go test -v -race ./test/integration_azure_test.go -tags=azure${NC}"
    echo ""
    
    if [[ "$AUTH_METHOD" == "azure_cli" ]]; then
        print_info "Make sure you're logged in with: az login"
    fi
}

print_env_only() {
    # Load existing config if available
    if [[ -f "$CONFIG_FILE" ]]; then
        cat "$CONFIG_FILE"
    else
        echo "# No configuration file found at $CONFIG_FILE"
        echo "# Run this script without --print-env to generate one"
        exit 1
    fi
}

cleanup_existing_resources() {
    print_section "Cleaning Up Existing Test Resources"
    
    if ! command -v az &> /dev/null; then
        print_warning "Azure CLI not found. Skipping cleanup."
        return 0
    fi
    
    # Check if logged in
    if ! az account show &> /dev/null; then
        print_warning "Not logged into Azure CLI. Skipping cleanup."
        return 0
    fi
    
    print_info "Searching for existing terragrunt test resources..."
    
    # Find resource groups starting with terragrunt-test-
    local rg_list
    rg_list=$(az group list --query "[?starts_with(name, 'terragrunt-test-')].name" -o tsv 2>/dev/null || true)
    
    if [[ -n "$rg_list" ]]; then
        local rg_count
        rg_count=$(echo "$rg_list" | wc -l | tr -d ' ')
        print_warning "Found ${rg_count} resource group(s) starting with 'terragrunt-test-'"
        
        echo ""
        echo "Resource groups to delete:"
        echo "$rg_list" | head -20
        if [[ "$rg_count" -gt 20 ]]; then
            echo "  ... and $((rg_count - 20)) more"
        fi
        echo ""
        
        echo -e -n "Delete all ${rg_count} resource groups? [${YELLOW}Y/n${NC}]: "
        read -r confirm
        local confirm_lower
        confirm_lower=$(echo "$confirm" | tr '[:upper:]' '[:lower:]')
        
        if [[ -z "$confirm_lower" || "$confirm_lower" == "y" || "$confirm_lower" == "yes" ]]; then
            print_info "Deleting resource groups (async)..."
            local deleted=0
            local failed=0
            
            while IFS= read -r rg_name; do
                if [[ -n "$rg_name" ]]; then
                    if az group delete --name "$rg_name" --yes --no-wait 2>/dev/null; then
                        ((deleted++)) || true
                        echo -ne "\r  Queued ${deleted}/${rg_count} resource groups for deletion..."
                    else
                        ((failed++)) || true
                    fi
                fi
            done <<< "$rg_list"
            
            echo ""
            if [[ $deleted -gt 0 ]]; then
                print_success "Queued ${deleted} resource group(s) for deletion"
            fi
            if [[ $failed -gt 0 ]]; then
                print_warning "Failed to queue ${failed} resource group(s) for deletion"
            fi
            
            # Wait for deletions to complete if any were queued
            if [[ $deleted -gt 0 ]]; then
                print_info "Waiting for resource group deletions to complete..."
                print_info "(This may take a few minutes)"
                
                local max_wait=300  # 5 minutes max wait
                local waited=0
                local remaining=$deleted
                
                while [[ $waited -lt $max_wait && $remaining -gt 0 ]]; do
                    sleep 10
                    ((waited+=10)) || true
                    
                    # Check how many are still being deleted
                    local current_rgs
                    current_rgs=$(az group list --query "[?starts_with(name, 'terragrunt-test-')].name" -o tsv 2>/dev/null || true)
                    if [[ -z "$current_rgs" ]]; then
                        remaining=0
                    else
                        remaining=$(echo "$current_rgs" | wc -l | tr -d ' ')
                    fi
                    
                    echo -ne "\r  ${remaining} resource group(s) still deleting... (${waited}s elapsed)"
                done
                
                echo ""
                if [[ $remaining -eq 0 ]]; then
                    print_success "All resource groups deleted successfully"
                else
                    print_warning "${remaining} resource group(s) still deleting in background"
                fi
            fi
        else
            print_info "Skipping resource group cleanup"
        fi
    else
        print_success "No existing terragrunt-test-* resource groups found"
    fi
    
    # Also clean up any orphaned storage accounts (those not in resource groups)
    # This is rare but can happen if RG deletion failed midway
    local sa_list
    sa_list=$(az storage account list --query "[?starts_with(name, 'tg') && starts_with(name, 'tgtest')].name" -o tsv 2>/dev/null || true)
    
    if [[ -n "$sa_list" ]]; then
        local sa_count
        sa_count=$(echo "$sa_list" | wc -l | tr -d ' ')
        print_info "Found ${sa_count} potentially orphaned storage account(s)"
        echo "  (These will be deleted along with their resource groups)"
    fi
    
    echo ""
}

show_help() {
    echo "Azure Integration Test Setup Script"
    echo ""
    echo "Usage: $0 [options]"
    echo ""
    echo "Options:"
    echo "  --print-env    Print existing configuration for sourcing"
    echo "  --cleanup      Cleanup existing test resources only"
    echo "  --no-cleanup   Skip cleanup step during setup"
    echo "  --help         Show this help message"
    echo ""
    echo "Examples:"
    echo "  # Interactive setup (includes cleanup)"
    echo "  $0"
    echo ""
    echo "  # Cleanup existing test resources only"
    echo "  $0 --cleanup"
    echo ""
    echo "  # Source existing config"
    echo "  source <($0 --print-env)"
    echo ""
    echo "  # Or directly"
    echo "  source ~/.terragrunt-azure-test-env"
}

main() {
    local skip_cleanup=false
    local cleanup_only=false
    
    # Parse arguments
    case "${1:-}" in
        --print-env)
            print_env_only
            exit 0
            ;;
        --help|-h)
            show_help
            exit 0
            ;;
        --cleanup)
            cleanup_only=true
            ;;
        --no-cleanup)
            skip_cleanup=true
            ;;
    esac
    
    # Initialize variables
    SUBSCRIPTION_ID=""
    TENANT_ID=""
    CLIENT_ID=""
    CLIENT_SECRET=""
    ACCESS_KEY=""
    STORAGE_ACCOUNT=""
    RESOURCE_GROUP=""
    LOCATION=""
    AUTH_METHOD=""
    ISOLATION_MODE=""
    ISOLATE_STORAGE=""
    ISOLATE_RESOURCE_GROUP=""
    CLEANUP_ENABLED=""
    USE_AZUREAD_AUTH=""
    
    print_header
    
    # Run cleanup (always, unless --no-cleanup specified)
    if [[ "$skip_cleanup" == "false" ]]; then
        cleanup_existing_resources
    fi
    
    # Exit early if cleanup-only mode
    if [[ "$cleanup_only" == "true" ]]; then
        print_success "Cleanup complete!"
        exit 0
    fi
    
    detect_existing_config
    select_auth_method
    configure_auth
    configure_subscription
    configure_resources
    configure_isolation
    generate_config
    print_summary
}

main "$@"
