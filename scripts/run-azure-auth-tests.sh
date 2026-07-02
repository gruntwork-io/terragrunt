#!/usr/bin/env bash
# run-azure-auth-tests.sh
#
# Checks out pr3-azurerm-backend, then runs the full Azure test suite
# (unit + integration) for each supported auth method.
#
# Designed to run on the tg-azurerm-test VM (swedencentral, system-assigned MSI).
#
# ── Required env vars ────────────────────────────────────────────────────────
#   ARM_SUBSCRIPTION_ID          Azure subscription ID (always required)
#
# ── Optional env vars (passed in from outside) ───────────────────────────────
#   SP_CLIENT_ID                 Enable service-principal auth tests
#   SP_CLIENT_SECRET
#   SP_TENANT_ID                 Defaults to TENANT_ID constant below
#   OIDC_CLIENT_ID               Enable OIDC auth tests (app with fed-credential)
#   OIDC_TENANT_ID               Defaults to SP_TENANT_ID / TENANT_ID
#   ARM_SAS_TOKEN                Enable SAS-token auth tests (data-plane only)
#   ARM_ACCESS_KEY_SA            Storage account name (for access-key fetching)
#   ARM_ACCESS_KEY_RG            Resource group of that storage account
#   ARM_ACCESS_KEY               Enable access-key auth tests (data-plane only)
#   RBAC_PRINCIPAL_ID            Overrides auto-detected MSI principal for RBAC test
#   INTEGRATION_TIMEOUT          Timeout per auth method (default: 40m)
#   SKIP_SETUP                   Set to 1 to skip repo clone/update step
#   RUN_METHODS                  Comma-separated list of methods to run
#                                (default: msi,sp,oidc,sas,accesskey)
# ─────────────────────────────────────────────────────────────────────────────

set -euo pipefail

# ── Constants ──────────────────────────────────────────────────────────────
readonly SUBSCRIPTION_ID="${ARM_SUBSCRIPTION_ID:?ARM_SUBSCRIPTION_ID is required}"
readonly TENANT_ID="4af954c0-b708-40fe-a81c-107c117544f1"
readonly MSI_PRINCIPAL_ID="4fd6e020-b335-4caf-be78-44027316a683"
readonly REPO_URL="https://github.com/omattsson/terragrunt.git"
readonly BRANCH="pr3-azurerm-backend"
readonly REPO_DIR="${HOME}/terragrunt"
readonly TEST_LOCATION="swedencentral"
readonly INTEGRATION_TIMEOUT="${INTEGRATION_TIMEOUT:-40m}"
readonly RUN_METHODS="${RUN_METHODS:-msi,sp,oidc,sas,accesskey}"
readonly LOG_DIR="${HOME}/azure-test-logs/$(date +%Y%m%d-%H%M%S)"

export PATH="/usr/local/go/bin:${PATH}"

# ── Colour helpers ──────────────────────────────────────────────────────────
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
CYAN='\033[0;36m'; BOLD='\033[1m'; RESET='\033[0m'

info()    { echo -e "${CYAN}[INFO]${RESET}  $*"; }
ok()      { echo -e "${GREEN}[OK]${RESET}    $*"; }
warn()    { echo -e "${YELLOW}[WARN]${RESET}  $*"; }
err()     { echo -e "${RED}[ERR]${RESET}   $*" >&2; }
section() { echo -e "\n${BOLD}══════════════════════════════════════════════════${RESET}"; \
            echo -e "${BOLD}  $*${RESET}"; \
            echo -e "${BOLD}══════════════════════════════════════════════════${RESET}\n"; }

# ── Result tracking ─────────────────────────────────────────────────────────
declare -A UNIT_RESULTS=()
declare -A INTEG_RESULTS=()

record() { # record <method> unit|integ PASS|FAIL
    local method="$1" kind="$2" result="$3"
    case "$kind" in
        unit)  UNIT_RESULTS["$method"]="$result" ;;
        integ) INTEG_RESULTS["$method"]="$result" ;;
    esac
}

# ── Method selection ─────────────────────────────────────────────────────────
method_enabled() { echo ",$RUN_METHODS," | grep -q ",$1,"; }

# ── Clear all credential env vars between runs ──────────────────────────────
clear_creds() {
    unset AZURE_TENANT_ID AZURE_CLIENT_ID AZURE_CLIENT_SECRET \
          AZURE_FEDERATED_TOKEN_FILE ARM_USE_OIDC ARM_USE_MSI \
          ARM_SAS_TOKEN ARM_ACCESS_KEY ARM_CLIENT_ID ARM_CLIENT_SECRET \
          ARM_TENANT_ID 2>/dev/null || true
}

# ── Run a test pass and record result ───────────────────────────────────────
run_unit() {
    local method="$1"
    local log="${LOG_DIR}/${method}-unit.log"
    info "Running unit tests for auth method: ${method}"
    if go test -tags=azure -count=1 -v -timeout 5m ./internal/azurehelper/... \
           >"$log" 2>&1; then
        ok "Unit tests PASSED (${method})"
        record "$method" unit PASS
    else
        err "Unit tests FAILED (${method}) — see ${log}"
        record "$method" unit FAIL
    fi
}

run_integ() {
    local method="$1"
    shift
    local extra_run="${1:-TestAzure}"   # regex passed to -run
    local log="${LOG_DIR}/${method}-integ.log"
    info "Running integration tests for auth method: ${method} (run=${extra_run})"

    local integ_env=(
        "ARM_SUBSCRIPTION_ID=${SUBSCRIPTION_ID}"
        "TERRAGRUNT_AZURE_TEST_LOCATION=${TEST_LOCATION}"
    )
    # Propagate RBAC principal if available
    local principal="${RBAC_PRINCIPAL_ID:-${MSI_PRINCIPAL_ID}}"
    integ_env+=("TERRAGRUNT_AZURE_TEST_PRINCIPAL_ID=${principal}")

    # Propagate current credential env vars into the test subprocess
    for v in AZURE_TENANT_ID AZURE_CLIENT_ID AZURE_CLIENT_SECRET \
              AZURE_FEDERATED_TOKEN_FILE ARM_USE_OIDC ARM_USE_MSI \
              ARM_SAS_TOKEN ARM_ACCESS_KEY ARM_CLIENT_ID ARM_CLIENT_SECRET \
              ARM_TENANT_ID; do
        [[ -n "${!v:-}" ]] && integ_env+=("${v}=${!v}")
    done

    if env "${integ_env[@]}" \
       go test -tags=azure -count=1 -v \
           -timeout "${INTEGRATION_TIMEOUT}" \
           ./test/ -run "${extra_run}" \
           >"$log" 2>&1; then
        ok "Integration tests PASSED (${method})"
        record "$method" integ PASS
    else
        err "Integration tests FAILED (${method}) — see ${log}"
        record "$method" integ FAIL
    fi
}

# ─────────────────────────────────────────────────────────────────────────────
# STEP 1 — Repo setup
# ─────────────────────────────────────────────────────────────────────────────
section "Step 1: Repo setup"

if [[ "${SKIP_SETUP:-0}" != "1" ]]; then
    if [[ -d "${REPO_DIR}/.git" ]]; then
        info "Updating existing clone at ${REPO_DIR}"
        git -C "${REPO_DIR}" fetch origin
        git -C "${REPO_DIR}" checkout "${BRANCH}"
        git -C "${REPO_DIR}" reset --hard "origin/${BRANCH}"
    else
        info "Cloning ${REPO_URL} → ${REPO_DIR}"
        git clone --depth=50 --branch "${BRANCH}" "${REPO_URL}" "${REPO_DIR}"
    fi
else
    warn "SKIP_SETUP=1: skipping repo clone/update"
fi

cd "${REPO_DIR}"
info "Repo HEAD: $(git log --oneline -1)"

info "Building module to verify compilation…"
go build ./... && ok "Build clean"

mkdir -p "${LOG_DIR}"
info "Test logs: ${LOG_DIR}"

# ─────────────────────────────────────────────────────────────────────────────
# STEP 2 — MSI (Managed Identity)
# The VM's system-assigned identity is used automatically when no
# EnvironmentCredential vars are set. DefaultAzureCredential falls through to
# ManagedIdentityCredential via IMDS.
# ─────────────────────────────────────────────────────────────────────────────
if method_enabled msi; then
    section "Auth method: MSI (Managed Identity)"
    clear_creds
    # Explicitly hint the builder towards MSI path for unit tests
    export ARM_USE_MSI=true
    run_unit msi
    # Integration tests always use DefaultAzureCredential (UseAzureADAuth=true);
    # on this VM with no EnvironmentCredential vars set, DefaultAzureCredential
    # will reach ManagedIdentityCredential — i.e. same as MSI.
    unset ARM_USE_MSI
    run_integ msi "TestAzure"
    clear_creds
fi

# ─────────────────────────────────────────────────────────────────────────────
# STEP 3 — Service Principal (client-secret)
# Requires SP_CLIENT_ID and SP_CLIENT_SECRET.
# DefaultAzureCredential → EnvironmentCredential path.
# ─────────────────────────────────────────────────────────────────────────────
if method_enabled sp; then
    if [[ -z "${SP_CLIENT_ID:-}" || -z "${SP_CLIENT_SECRET:-}" ]]; then
        warn "Skipping service-principal tests: SP_CLIENT_ID / SP_CLIENT_SECRET not set"
        record sp unit SKIP
        record sp integ SKIP
    else
        section "Auth method: Service Principal (client-secret)"
        clear_creds
        export AZURE_TENANT_ID="${SP_TENANT_ID:-${TENANT_ID}}"
        export AZURE_CLIENT_ID="${SP_CLIENT_ID}"
        export AZURE_CLIENT_SECRET="${SP_CLIENT_SECRET}"
        # Also set ARM_ variants used by config builder env fallback
        export ARM_TENANT_ID="${AZURE_TENANT_ID}"
        export ARM_CLIENT_ID="${AZURE_CLIENT_ID}"
        export ARM_CLIENT_SECRET="${AZURE_CLIENT_SECRET}"
        run_unit sp
        run_integ sp "TestAzure"
        clear_creds
    fi
fi

# ─────────────────────────────────────────────────────────────────────────────
# STEP 4 — OIDC (Workload Identity)
# Requires a registered app with a federated credential that trusts the MSI
# token issued by this VM, or an externally supplied token file.
#
# Approach: use the VM's MSI to get a token for the target audience, write it
# to a file, and set AZURE_FEDERATED_TOKEN_FILE. This only works if OIDC_CLIENT_ID
# has a federated credential with issuer=https://login.microsoftonline.com/<tenant>/
# and subject=system:serviceaccount (or equivalent).
#
# For practical testing without a pre-configured federated credential, provide
# a pre-obtained JWT in OIDC_TOKEN_FILE_PATH and set OIDC_CLIENT_ID.
# ─────────────────────────────────────────────────────────────────────────────
if method_enabled oidc; then
    if [[ -z "${OIDC_CLIENT_ID:-}" ]]; then
        warn "Skipping OIDC tests: OIDC_CLIENT_ID not set"
        record oidc unit SKIP
        record oidc integ SKIP
    else
        section "Auth method: OIDC (Workload Identity)"
        clear_creds

        local_tenant="${OIDC_TENANT_ID:-${SP_TENANT_ID:-${TENANT_ID}}}"
        token_file="${OIDC_TOKEN_FILE:-/tmp/tg-test-oidc-token}"

        if [[ -n "${OIDC_TOKEN:-}" ]]; then
            # Caller provided a raw token string
            echo -n "${OIDC_TOKEN}" > "${token_file}"
        elif [[ -f "${token_file}" ]]; then
            info "Using existing token file: ${token_file}"
        else
            # Try to get a token from IMDS for the AAD audience
            info "Fetching MSI token for OIDC audience…"
            msi_response=$(curl -sf \
                -H "Metadata: true" \
                "http://169.254.169.254/metadata/identity/oauth2/token?api-version=2018-02-01&resource=api://${OIDC_CLIENT_ID}" \
            ) || { warn "IMDS token fetch failed — skipping OIDC"; record oidc unit SKIP; record oidc integ SKIP; clear_creds; }
            if [[ -n "${msi_response:-}" ]]; then
                echo "${msi_response}" | python3 -c "import json,sys; print(json.load(sys.stdin)['access_token'])" > "${token_file}"
                info "Token written to ${token_file}"
            fi
        fi

        if [[ -f "${token_file}" ]]; then
            export ARM_USE_OIDC=true
            export ARM_TENANT_ID="${local_tenant}"
            export ARM_CLIENT_ID="${OIDC_CLIENT_ID}"
            export ARM_OIDC_TOKEN_FILE_PATH="${token_file}"
            export AZURE_FEDERATED_TOKEN_FILE="${token_file}"
            export AZURE_CLIENT_ID="${OIDC_CLIENT_ID}"
            export AZURE_TENANT_ID="${local_tenant}"
            run_unit oidc
            run_integ oidc "TestAzure"
        fi
        clear_creds
    fi
fi

# ─────────────────────────────────────────────────────────────────────────────
# STEP 5 — SAS Token (data-plane only)
# Control-plane (ARM) operations are not available with a SAS token.
# Only TestAzureSASTokenAuthIsDataPlaneOnly is expected to pass fully;
# bootstrap tests verify they fail with a clear error.
# ─────────────────────────────────────────────────────────────────────────────
if method_enabled sas; then
    if [[ -z "${ARM_SAS_TOKEN:-}" ]]; then
        warn "Skipping SAS-token tests: ARM_SAS_TOKEN not set"
        record sas unit SKIP
        record sas integ SKIP
    else
        section "Auth method: SAS Token (data-plane only)"
        sas_token_val="${ARM_SAS_TOKEN}"
        clear_creds
        export ARM_SAS_TOKEN="${sas_token_val}"
        run_unit sas
        # Only run the SAS-specific integration test
        run_integ sas "TestAzureSASTokenAuthIsDataPlaneOnly"
        clear_creds
    fi
fi

# ─────────────────────────────────────────────────────────────────────────────
# STEP 6 — Access Key (data-plane only)
# Like SAS, access keys bypass the ARM control plane.
# ─────────────────────────────────────────────────────────────────────────────
if method_enabled accesskey; then
    if [[ -z "${ARM_ACCESS_KEY:-}" ]]; then
        warn "Skipping access-key tests: ARM_ACCESS_KEY not set"
        record accesskey unit SKIP
        record accesskey integ SKIP
    else
        section "Auth method: Access Key (data-plane only)"
        access_key_val="${ARM_ACCESS_KEY}"
        clear_creds
        export ARM_ACCESS_KEY="${access_key_val}"
        # ARM_ACCESS_KEY_SA / ARM_ACCESS_KEY_RG point at the pre-existing
        # storage account TestAzureBlobOperations should reuse (since the
        # access-key auth method cannot bootstrap one via the ARM control
        # plane). clear_creds intentionally leaves these untouched.
        [[ -n "${ARM_ACCESS_KEY_SA:-}" ]] && export ARM_ACCESS_KEY_SA
        [[ -n "${ARM_ACCESS_KEY_RG:-}" ]] && export ARM_ACCESS_KEY_RG
        run_unit accesskey
        # TestAzureBlobOperations is the primary data-plane blob test
        run_integ accesskey "TestAzureBlobOperations"
        clear_creds
    fi
fi

# ─────────────────────────────────────────────────────────────────────────────
# STEP 7 — Summary
# ─────────────────────────────────────────────────────────────────────────────
section "Results Summary"

all_methods=(msi sp oidc sas accesskey)

printf "%-14s  %-10s  %-12s\n" "AUTH METHOD" "UNIT" "INTEGRATION"
printf "%-14s  %-10s  %-12s\n" "───────────" "────" "───────────"

overall_pass=true
for m in "${all_methods[@]}"; do
    if ! method_enabled "$m"; then
        continue
    fi
    unit="${UNIT_RESULTS[$m]:-N/A}"
    integ="${INTEG_RESULTS[$m]:-N/A}"

    u_col="${RESET}"
    i_col="${RESET}"
    if [[ "$unit"  == "PASS" ]]; then u_col="${GREEN}"; fi
    if [[ "$unit"  == "FAIL" ]]; then u_col="${RED}"; overall_pass=false; fi
    if [[ "$integ" == "PASS" ]]; then i_col="${GREEN}"; fi
    if [[ "$integ" == "FAIL" ]]; then i_col="${RED}"; overall_pass=false; fi

    printf "%-14s  ${u_col}%-10s${RESET}  ${i_col}%-12s${RESET}\n" \
        "$m" "$unit" "$integ"
done

echo ""
echo "Logs: ${LOG_DIR}"

if $overall_pass; then
    ok "All enabled auth methods PASSED."
    exit 0
else
    err "One or more auth methods FAILED. See logs above."
    exit 1
fi
