# Script to sign Windows binaries using DigiCert and patch with go-winres
# Usage: sign-windows.ps1 -BinDirectory <path>
# Environment variables:
#   GITHUB_REF_NAME: Git ref name (e.g., v0.93.4 or beta-2025111001)
#   SM_HOST: DigiCert host
#   SM_API_KEY: DigiCert API key
#   SM_CLIENT_CERT_FILE: Path to P12 certificate file
#   SM_CLIENT_CERT_PASSWORD: Certificate password
#   SM_KEYPAIR_ALIAS: DigiCert keypair alias

param(
    [Parameter(Mandatory=$false)]
    [string]$BinDirectory = "bin"
)

$ErrorActionPreference = 'Stop'

function Assert-EnvVar {
    param([string]$Name)

    $value = [Environment]::GetEnvironmentVariable($Name)
    if ([string]::IsNullOrEmpty($value)) {
        Write-Error "ERROR: Required environment variable $Name not set."
        exit 1
    }
}

function Update-WinresVersion {
    # Get version from git ref
    $rawVersion = $env:GITHUB_REF_NAME
    if ([string]::IsNullOrEmpty($rawVersion) -or $rawVersion -eq "refs/heads/main") {
        $rawVersion = "0.0.0-dev"
    }

    Write-Host "Raw version from git ref: $rawVersion"

    # Parse version based on tag format
    $version = ""
    $major = "0"
    $minor = "0"
    $patch = "0"
    $build = "0"

    if ($rawVersion -match '^v(\d+)\.(\d+)\.(\d+)') {
        # Standard version tag: v0.93.4
        $major = $matches[1]
        $minor = $matches[2]
        $patch = $matches[3]
        $version = "$major.$minor.$patch"
        Write-Host "Detected standard version tag: v$version"
    }
    elseif ($rawVersion -match '^([a-zA-Z0-9_-]+)-(\d{4})(\d{2})(\d{2})(\d{2})') {
        # Generic pre-release tag: <prefix>-YYYYMMDDNN
        # Examples: beta-2025111001, alpha-2025110301, rc-2025120101, dev-2025110501
        # Extract prefix and date components: prefix-YYYY-MM-DD-build
        $prefix = $matches[1]
        $year = $matches[2]
        $month = $matches[3]
        $day = $matches[4]
        $buildNum = $matches[5]

        # Windows FileVersion components are limited to 65535
        # Use format: YYYY.MMDD.NN.0 (all components within limits)
        $major = $year
        $minor = "$month$day"
        $patch = $buildNum
        $version = "$prefix-$year$month$day$buildNum"
        Write-Host "Detected pre-release tag: $version (Prefix: $prefix, FileVersion will be $year.$month$day.$buildNum.0)"
    }
    elseif ($rawVersion -match '^\d+\.\d+') {
        # Version without 'v' prefix
        $version = $rawVersion
        $versionParts = $version.Split('.')
        $major = if ($versionParts.Length -gt 0) { $versionParts[0] } else { "0" }
        $minor = if ($versionParts.Length -gt 1) { $versionParts[1] } else { "0" }
        $patch = if ($versionParts.Length -gt 2) { $versionParts[2].Split('-')[0] } else { "0" }
        Write-Host "Detected version without prefix: $version"
    }
    else {
        # Branch name or dev version
        $major = "0"
        $minor = "0"
        $patch = "0"
        $version = "0.0.0-dev"
        Write-Host "Using dev version: $version"
    }

    $fileVersion = "$major.$minor.$patch.0"
    $copyrightYear = (Get-Date).Year

    Write-Host "Final version: $version"
    Write-Host "File version (for Windows): $fileVersion"
    Write-Host ""

    # Copy winres.json from .github/assets to current directory
    Copy-Item ".github/assets/winres.json" "winres.json"
    Write-Host "Copied winres.json from .github/assets/"

    # Read winres.json
    $winresConfig = Get-Content "winres.json" -Raw | ConvertFrom-Json

    # Update version fields
    $winresConfig.RT_VERSION.'#1'.'0409'.fixed.file_version = $fileVersion
    $winresConfig.RT_VERSION.'#1'.'0409'.fixed.product_version = $fileVersion
    $winresConfig.RT_VERSION.'#1'.'0409'.info.'0409'.FileVersion = $version
    $winresConfig.RT_VERSION.'#1'.'0409'.info.'0409'.ProductVersion = $version
    $winresConfig.RT_VERSION.'#1'.'0409'.info.'0409'.LegalCopyright = "Copyright (C) $copyrightYear Gruntwork, Inc."
    $winresConfig.RT_MANIFEST.'#1'.'0409'.assembly.identity.version = $fileVersion

    # Write updated winres.json to current directory
    $jsonOutput = $winresConfig | ConvertTo-Json -Depth 10 -Compress:$false
    [System.IO.File]::WriteAllText("winres.json", $jsonOutput)

    Write-Host "Updated winres.json:"
    Get-Content winres.json

    # Verify icon file exists
    Write-Host ""
    Write-Host "Verifying icon file..."
    if (Test-Path ".github/assets/terragrunt.png") {
        Write-Host "Icon file exists: .github/assets/terragrunt.png"
        $iconInfo = Get-Item ".github/assets/terragrunt.png"
        Write-Host "Icon size: $($iconInfo.Length) bytes"
    } else {
        Write-Error "Icon file not found: .github/assets/terragrunt.png"
        exit 1
    }
}

function Patch-Binaries {
    # Add Go bin to PATH
    $goPath = & go env GOPATH
    $goBinPath = Join-Path $goPath "bin"
    $env:PATH = "$goBinPath;$env:PATH"

    Write-Host "Patching Windows binaries with icon and version info..."

    # Patch both Windows binaries
    $binaries = @("$BinDirectory\terragrunt_windows_386.exe", "$BinDirectory\terragrunt_windows_amd64.exe")

    foreach ($binary in $binaries) {
        if (Test-Path $binary) {
            Write-Host "Patching $binary..."
            & go-winres patch --in winres.json $binary

            if ($LASTEXITCODE -ne 0) {
                Write-Error "Failed to patch $binary"
                exit 1
            }

            Write-Host "Successfully patched $binary"
        } else {
            Write-Error "Binary not found: $binary"
            exit 1
        }
    }

    Write-Host "All Windows binaries patched with resources"
}

function Save-Credentials {
    Write-Host "Saving credentials to Windows Credential Manager..."

    & smctl.exe credentials save $env:SM_API_KEY $env:SM_CLIENT_CERT_PASSWORD

    if ($LASTEXITCODE -ne 0) {
        Write-Error "Failed to save credentials"
        exit 1
    }

    Write-Host "Credentials saved to Windows Credential Manager"
}

function Invoke-Healthcheck {
    Write-Host "Running smctl healthcheck..."

    & smctl.exe healthcheck

    if ($LASTEXITCODE -ne 0) {
        Write-Warning "Healthcheck failed (exit code: $LASTEXITCODE)"
        Write-Warning "Continuing anyway - signing step will be the real test"
    } else {
        Write-Host "Healthcheck passed"
    }
}

function Sync-Certificates {
    Write-Host "Syncing certificates from DigiCert KeyLocker..."

    & smctl.exe windows certsync --keypair-alias "$env:SM_KEYPAIR_ALIAS"

    if ($LASTEXITCODE -ne 0) {
        Write-Error "Certificate sync failed"
        exit 1
    }

    Write-Host "Certificates synced to Windows store"
}

function Sign-Binary {
    param([string]$BinaryPath)

    Write-Host "Signing: $BinaryPath"

    & smctl.exe sign `
        --keypair-alias "$env:SM_KEYPAIR_ALIAS" `
        --input "$BinaryPath" `
        --simple `
        --verbose

    if ($LASTEXITCODE -ne 0) {
        Write-Error "Signing failed for $BinaryPath"
        exit 1
    }

    Write-Host "Successfully signed $BinaryPath"
}

function Verify-Signature {
    param([string]$BinaryPath)

    Write-Host "Verifying signature on: $BinaryPath"

    & smctl.exe sign verify --input "$BinaryPath"

    if ($LASTEXITCODE -ne 0) {
        Write-Warning "Signature verification returned non-zero exit code (may be expected)"
    } else {
        Write-Host "Signature verified successfully"
    }
}

function Main {
    # Verify environment variables
    Assert-EnvVar "SM_HOST"
    Assert-EnvVar "SM_API_KEY"
    Assert-EnvVar "SM_CLIENT_CERT_FILE"
    Assert-EnvVar "SM_CLIENT_CERT_PASSWORD"
    Assert-EnvVar "SM_KEYPAIR_ALIAS"
    Assert-EnvVar "GITHUB_REF_NAME"

    if (-not (Test-Path $BinDirectory)) {
        Write-Error "Directory $BinDirectory does not exist"
        exit 1
    }

    # Update winres.json with version info
    Update-WinresVersion

    # Patch binaries with resources
    Patch-Binaries

    # Save credentials
    Save-Credentials

    # Run healthcheck
    Invoke-Healthcheck

    # Sync certificates
    Sync-Certificates

    # Sign amd64 binary only
    $amd64Binary = Get-Item -Path "$BinDirectory\terragrunt_windows_amd64.exe" -ErrorAction SilentlyContinue

    if (-not $amd64Binary) {
        Write-Error "Binary not found: $BinDirectory\terragrunt_windows_amd64.exe"
        exit 1
    }

    Sign-Binary -BinaryPath $amd64Binary.FullName

    # Verify signature
    Verify-Signature -BinaryPath $amd64Binary.FullName

    Write-Host "Windows signing completed successfully"
}

Main
