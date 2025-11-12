# Release Scripts

This directory contains scripts used by the GitHub Actions release workflow to build, sign, and publish Terragrunt releases.

## Overview

All inline bash and PowerShell code from GitHub Actions workflows has been extracted into these standalone scripts for better:
- **Syntax highlighting** - Proper IDE support
- **Linting** - Can run shellcheck/PSScriptAnalyzer
- **Testing** - Scripts can be tested independently
- **Maintainability** - Easier to update and debug
- **Reusability** - Can be called from multiple workflows

## Script Overview

### Core Library
- **`lib-release-config.sh`** - Helper library to read centralized release configuration from JSON

### General Release Scripts
- **`get-version.sh`** - Extracts and validates version from git tag or workflow input
- **`check-release-exists.sh`** - Checks if GitHub release exists for a version
- **`verify-binaries-downloaded.sh`** - Verifies expected binaries were downloaded from artifacts
- **`set-permissions.sh`** - Sets executable permissions (+x) on all binaries
- **`create-archives.sh`** - Creates ZIP and TAR.GZ archives for each binary
- **`generate-checksums.sh`** - Generates SHA256SUMS file for all release files
- **`verify-files.sh`** - Verifies all required files are present before upload
- **`upload-assets.sh`** - Uploads all assets to GitHub release (with optional --clobber)
- **`verify-assets-uploaded.sh`** - Verifies uploads and retries missing files
- **`generate-upload-summary.sh`** - Generates GitHub Actions step summary with release details

### macOS Signing Scripts
- **`prepare-macos-artifacts.sh`** - Prepares macOS artifacts for signing (filters darwin_* binaries)
- **`install-gon.sh`** - Downloads and installs gon tool for macOS code signing
- **`sign-macos-binaries.sh`** - Signs macOS binaries using gon and Apple notarization

### Windows Signing Scripts
- **`prepare-windows-artifacts.ps1`** - Prepares Windows artifacts for signing
- **`install-go-winres.ps1`** - Installs go-winres tool for patching Windows resources
- **`verify-smctl.ps1`** - Verifies DigiCert smctl tool is installed and accessible
- **`restore-p12-certificate.ps1`** - Restores P12 client certificate from base64 encoding
- **`sign-windows.ps1`** - Config-driven: patches all binaries, signs only those marked `signed: true`

## Centralized Configuration

Release asset configuration is maintained in a single source of truth:

### `../assets/release-assets-config.json`
JSON file defining all platforms, binaries, archive formats, and additional files.

**Schema:**
```json
{
  "platforms": [
    {
      "os": "darwin|linux|windows",
      "arch": "386|amd64|arm64",
      "signed": true|false,
      "binary": "terragrunt_<os>_<arch>[.exe]"
    }
  ],
  "archive_formats": [
    {
      "extension": "zip|tar.gz",
      "description": "Format description"
    }
  ],
  "additional_files": [
    {
      "name": "SHA256SUMS",
      "description": "File description"
    }
  ]
}
```

### `lib-release-config.sh`
Helper library providing functions to read the centralized configuration.

**Usage:**
```bash
#!/bin/bash
source "$(dirname "$0")/lib-release-config.sh"

# Verify config file exists
verify_config_file

# Get all binary filenames
get_all_binaries

# Get binary count
get_binary_count

# Get total expected file count
get_total_file_count

# Get archive extensions
get_archive_extensions

# Get additional files
get_additional_files

# Get all expected files (binaries + archives + additional)
get_all_expected_files

# Get platform info for specific binary
get_platform_info "terragrunt_darwin_amd64"

# Generate markdown table rows for summary
generate_platform_table_rows
```

**Scripts Using Configuration:**
- `verify-binaries-downloaded.sh` - Uses `get_binary_count()`
- `set-permissions.sh` - Uses `get_all_binaries()`
- `verify-assets-uploaded.sh` - Uses `get_all_expected_files()`, `get_total_file_count()`, `get_binary_count()`
- `generate-upload-summary.sh` - Uses `get_binary_count()`, `get_total_file_count()`, `generate_platform_table_rows()`

## General Scripts

### `get-version.sh`
Extracts version from either workflow dispatch input or git tag.

**Environment Variables:**
- `INPUT_TAG`: Tag provided via workflow_dispatch
- `EVENT_NAME`: GitHub event name (workflow_dispatch or push)
- `GITHUB_REF`: Git reference (e.g., refs/tags/v0.93.4)
- `GITHUB_OUTPUT`: Path to GitHub output file

**Usage:**
```bash
.github/scripts/release/get-version.sh
```

### `check-release-exists.sh`
Checks if a GitHub release exists for a given tag using the GitHub CLI.

**Environment Variables:**
- `VERSION`: The version/tag to check for
- `GH_TOKEN`: GitHub token for authentication
- `GITHUB_OUTPUT`: Path to GitHub output file

**Usage:**
```bash
export VERSION=v0.93.4
export GH_TOKEN=<token>
.github/scripts/release/check-release-exists.sh
```

### `verify-binaries-downloaded.sh`
Verifies all expected binaries were downloaded from build artifacts.

**Parameters:**
- `bin-directory`: Directory containing binaries (default: `bin`)
- `expected-count`: Minimum number of binaries expected (default: `7`)

**Usage:**
```bash
.github/scripts/release/verify-binaries-downloaded.sh bin 7
```

**Features:**
- Lists all downloaded binaries with details
- Counts total files using `find`
- Validates minimum expected count
- Exits with error if count is below threshold

### `set-permissions.sh`
Sets executable permissions (+x) on all Terragrunt binaries.

**Usage:**
```bash
.github/scripts/release/set-permissions.sh bin
```

### `create-archives.sh`
Creates both ZIP and TAR.GZ archives for each binary, preserving execute permissions.

**Usage:**
```bash
.github/scripts/release/create-archives.sh bin
```

**Output:**
- Creates `.zip` and `.tar.gz` for each binary
- ZIP files preserve Unix permissions
- TAR.GZ files natively preserve all file attributes

### `generate-checksums.sh`
Generates SHA256 checksums for all release files (binaries and archives).

**Usage:**
```bash
.github/scripts/release/generate-checksums.sh bin
```

**Output:**
- Creates `SHA256SUMS` file with checksums for all files

### `verify-files.sh`
Verifies all required files are present before upload.

**Usage:**
```bash
.github/scripts/release/verify-files.sh bin
```

**Checks:**
- All platform binaries (macOS, Linux, Windows)
- SHA256SUMS file

### `upload-assets.sh`
Uploads all release assets to an existing GitHub release.

**Environment Variables:**
- `VERSION`: The version/tag to upload to
- `GH_TOKEN`: GitHub token for authentication

**Usage:**
```bash
export VERSION=v0.93.4
export GH_TOKEN=<token>
.github/scripts/release/upload-assets.sh bin
```

### `verify-assets-uploaded.sh`
Verifies all assets were successfully uploaded and retries any missing files.

**Environment Variables:**
- `VERSION`: The version/tag to verify
- `GH_TOKEN`: GitHub token for authentication
- `CLOBBER`: Set to 'true' to overwrite existing assets during retry (default: false)

**Usage:**
```bash
export VERSION=v0.93.4
export GH_TOKEN=<token>
export CLOBBER=false
.github/scripts/release/verify-assets-uploaded.sh bin
```

**Features:**
- Checks for 22 expected files (7 binaries + 7 ZIPs + 7 TAR.GZ + SHA256SUMS)
- Automatically retries failed uploads (max 10 attempts)
- Verifies asset downloadability

### `generate-upload-summary.sh`
Generates a GitHub Actions step summary with release upload details.

**Environment Variables:**
- `VERSION`: Release version/tag
- `RELEASE_ID`: GitHub release ID
- `IS_DRAFT`: Whether release was a draft
- `GITHUB_STEP_SUMMARY`: Path to GitHub step summary file

**Usage:**
```bash
export VERSION=v0.93.4
export RELEASE_ID=123456
export IS_DRAFT=false
export GITHUB_STEP_SUMMARY=$GITHUB_STEP_SUMMARY
.github/scripts/release/generate-upload-summary.sh
```

**Features:**
- Creates formatted markdown summary
- Shows release details (version, ID, draft status)
- Displays platform/architecture table
- Lists archive files and totals
- Always runs (even on failure) via `if: always()` in workflow

## macOS Scripts

### `prepare-macos-artifacts.sh`
Prepares macOS artifacts for signing by copying them from the artifacts directory to the bin directory.

**Usage:**
```bash
.github/scripts/release/prepare-macos-artifacts.sh artifacts bin
```

### `install-gon.sh`
Downloads and installs the gon binary for macOS code signing and notarization.

**Environment Variables:**
- `GON_VERSION`: Version of gon to install (default: v0.0.37)

**Usage:**
```bash
export GON_VERSION=v0.0.37
.github/scripts/release/install-gon.sh
# or pass version as argument
.github/scripts/release/install-gon.sh v0.0.37
```

**Features:**
- Downloads gon from GitHub releases
- Installs to `/usr/local/bin`
- Verifies installation
- Cleans up temporary files

### `sign-macos-binaries.sh`
Signs macOS binaries using gon and Apple notarization service.

**Environment Variables:**
- `AC_PASSWORD`: Apple Connect password
- `AC_PROVIDER`: Apple Connect provider
- `AC_USERNAME`: Apple Connect username
- `MACOS_CERTIFICATE`: macOS certificate in P12 format (base64 encoded)
- `MACOS_CERTIFICATE_PASSWORD`: Certificate password

**Usage:**
```bash
export AC_PASSWORD=<password>
export AC_PROVIDER=<provider>
export AC_USERNAME=<username>
export MACOS_CERTIFICATE=<base64-cert>
export MACOS_CERTIFICATE_PASSWORD=<password>
.github/scripts/release/sign-macos-binaries.sh bin
```

**Process:**
1. Signs amd64 binary using `.gon_amd64.hcl`
2. Signs arm64 binary using `.gon_arm64.hcl`
3. Removes unsigned binaries from bin directory
4. Extracts signed binaries from ZIP archives
5. Moves signed binaries to bin directory (replacing unsigned versions)
6. Verifies signatures using `codesign -dv --verbose=4`

## Windows Scripts

### `prepare-windows-artifacts.ps1`
Prepares Windows artifacts for signing by copying them from the artifacts directory to the bin directory.

**Parameters:**
- `-ArtifactsDirectory`: Source directory (default: `artifacts`)
- `-BinDirectory`: Destination directory (default: `bin`)

**Usage:**
```powershell
.github/scripts/release/prepare-windows-artifacts.ps1 -ArtifactsDirectory artifacts -BinDirectory bin
```

### `install-go-winres.ps1`
Installs go-winres tool and adds it to PATH.

**Usage:**
```powershell
.github/scripts/release/install-go-winres.ps1
```

**Features:**
- Installs go-winres from GitHub
- Adds Go bin directory to PATH
- Exports PATH to GitHub environment
- Verifies installation with `go-winres help`

### `verify-smctl.ps1`
Verifies that DigiCert smctl tool is installed and accessible.

**Usage:**
```powershell
.github/scripts/release/verify-smctl.ps1
```

**Checks:**
- smctl.exe is in PATH
- Displays smctl version
- Confirms tool is ready for use

### `restore-p12-certificate.ps1`
Restores P12 client certificate from base64 encoding.

**Environment Variables:**
- `WINDOWS_SIGNING_P12_BASE64`: Base64 encoded P12 certificate (required)
- `RUNNER_TEMP`: Temporary directory for certificate file
- `GITHUB_ENV`: Path to GitHub environment file

**Usage:**
```powershell
$env:WINDOWS_SIGNING_P12_BASE64 = "<base64-string>"
.github/scripts/release/restore-p12-certificate.ps1
```

**Output:**
- Creates certificate file in `$RUNNER_TEMP/sm_client_auth.p12`
- Exports `SM_CLIENT_CERT_FILE` environment variable

### `sign-windows.ps1`
Comprehensive Windows binary signing script using DigiCert KeyLocker. **Fully driven by configuration**.

**Environment Variables:**
- `GITHUB_REF_NAME`: Git ref name (e.g., v0.93.4 or beta-2025111001)
- `SM_HOST`: DigiCert host URL
- `SM_API_KEY`: DigiCert API key
- `SM_CLIENT_CERT_FILE`: Path to P12 certificate file
- `SM_CLIENT_CERT_PASSWORD`: Certificate password
- `SM_KEYPAIR_ALIAS`: DigiCert keypair alias

**Parameters:**
- `-BinDirectory`: Directory containing binaries (default: `bin`)

**Usage:**
```powershell
.github/scripts/release/sign-windows.ps1 -BinDirectory bin
```

**Process:**
1. **Configuration Loading**: Reads `release-assets-config.json` to discover all Windows platforms
2. **Version Detection**: Parses git ref to extract version
   - Standard: `v0.93.4` → `0.93.4`
   - Pre-release: `beta-2025111001` → `2025.1110.01.0`
   - Generic: `<prefix>-YYYYMMDDNN` → `YYYY.MMDD.NN.0`
3. **Resource Generation**: Dynamically generates `winres.json` with version info, icon, manifest, and metadata
4. **Binary Patching**: Uses go-winres to patch **all** Windows binaries with icon and metadata (regardless of signing status)
5. **Credential Setup**: Saves DigiCert credentials
6. **Healthcheck**: Runs smctl healthcheck
7. **Certificate Sync**: Syncs certificates from DigiCert KeyLocker
8. **Selective Signing**: For each Windows platform in config:
   - If `"signed": true` → Signs with DigiCert and verifies signature
   - If `"signed": false` → Skips signing (patching only)
9. **Summary**: Reports how many binaries were signed vs. patched-only

**Configuration-Driven Behavior:**
The script reads `.github/assets/release-assets-config.json` to determine:
- Which Windows binaries exist (`binary` field)
- Which binaries should be signed (`signed: true/false`)
- All patching decisions are automated based on JSON

**Example Config:**
```json
{
  "platforms": [
    {
      "os": "windows",
      "arch": "386",
      "signed": false,
      "binary": "terragrunt_windows_386.exe"
    },
    {
      "os": "windows",
      "arch": "amd64",
      "signed": true,
      "binary": "terragrunt_windows_amd64.exe"
    }
  ]
}
```

With this config, the script will:
- Patch both binaries with icon/manifest
- Sign only the amd64 binary (conserving signature quota)
- Leave 386 unsigned

## Testing

### Bash Scripts

```bash
# Install shellcheck
sudo apt-get install shellcheck  # Ubuntu/Debian
# or
brew install shellcheck  # macOS

# Check all bash scripts
shellcheck .github/scripts/release/*.sh

# Test individual script
export VERSION=v0.93.4
export GH_TOKEN=<token>
.github/scripts/release/get-version.sh
```

### PowerShell Scripts

```powershell
# Install PSScriptAnalyzer
Install-Module -Name PSScriptAnalyzer -Force -Scope CurrentUser

# Analyze all PowerShell scripts
Get-ChildItem .github/scripts/release/*.ps1 | ForEach-Object {
    Invoke-ScriptAnalyzer -Path $_.FullName
}

# Test individual script
.github/scripts/release/verify-smctl.ps1
```

## Workflow Integration

These scripts are used by:

- **`.github/workflows/release.yml`** - Main release workflow
  - Uses: `get-version.sh`, `check-release-exists.sh`, `set-permissions.sh`, `create-archives.sh`, `generate-checksums.sh`, `verify-files.sh`, `upload-assets.sh`, `verify-assets-uploaded.sh`

- **`.github/workflows/sign-macos.yml`** - macOS signing workflow
  - Uses: `prepare-macos-artifacts.sh`, `install-gon.sh`, `sign-macos-binaries.sh`

- **`.github/workflows/sign-windows.yml`** - Windows signing workflow
  - Uses: `prepare-windows-artifacts.ps1`, `install-go-winres.ps1`, `verify-smctl.ps1`, `restore-p12-certificate.ps1`, `sign-windows.ps1`

## Version Format Support

The scripts support multiple version tag formats:

| Format   | Example            | Windows FileVersion | Description                     |
|----------|--------------------|---------------------|---------------------------------|
| Standard | `v0.93.4`          | `0.93.4.0`          | Semantic version with v prefix  |
| Beta     | `beta-2025111001`  | `2025.1110.01.0`    | Pre-release with date timestamp |
| Alpha    | `alpha-2025110301` | `2025.1103.01.0`    | Alpha with date timestamp       |

**Windows Version Constraints:**
- Each component must be ≤ 65535
- Format: YYYY.MMDD.NN.0 keeps all components within limits

## Security Notes

- All scripts use proper quoting to prevent command injection
- Environment variables are validated before use
- Sensitive data (tokens, passwords) passed via environment variables only
- Scripts fail fast on errors:
  - Bash: `set -e`
  - PowerShell: `$ErrorActionPreference = 'Stop'`
- No secrets are logged or printed to stdout
- Certificate files are stored in temporary directories

## Script Conventions

### Bash Scripts
- Use `#!/bin/bash` shebang
- Enable fail-fast: `set -e`
- Use functions for organization
- Validate environment variables with `assert_env_var_not_empty`
- Accept directory paths as arguments (default: `bin`)
- Use `printf` instead of `echo` for variable output
- Proper quoting: `"$variable"` not `$variable`

### PowerShell Scripts
- Use strict error handling: `$ErrorActionPreference = 'Stop'`
- Use parameters with defaults: `param([string]$BinDirectory = "bin")`
- Check exit codes: `if ($LASTEXITCODE -ne 0) { exit 1 }`
- Organize code into functions
- Use `Write-Host` for informational output
- Use `Write-Error` for errors
