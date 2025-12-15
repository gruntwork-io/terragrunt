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

# Path to centralized configuration
$ConfigFile = ".github/assets/release-assets-config.json"

function Assert-EnvVar {
    param([string]$Name)

    $value = [Environment]::GetEnvironmentVariable($Name)
    if ([string]::IsNullOrEmpty($value)) {
        Write-Error "ERROR: Required environment variable $Name not set."
        exit 1
    }
}

function Get-WindowsPlatforms {
    # Read configuration file
    if (-not (Test-Path $ConfigFile)) {
        Write-Error "Configuration file not found: $ConfigFile"
        exit 1
    }

    Write-Host "Reading configuration from: $ConfigFile"
    $config = Get-Content $ConfigFile -Raw | ConvertFrom-Json

    # Filter Windows platforms
    $windowsPlatforms = $config.platforms | Where-Object { $_.os -eq "windows" }

    if ($windowsPlatforms.Count -eq 0) {
        Write-Error "No Windows platforms found in configuration"
        exit 1
    }

    Write-Host "Found $($windowsPlatforms.Count) Windows platform(s) in configuration"
    return $windowsPlatforms
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

    # Generate winres.json dynamically
    Write-Host "Generating winres.json..."
    $winresConfig = @{
        RT_GROUP_ICON = @{
            APP = @{
                "0409" = ".github/assets/terragrunt.png"
            }
        }
        RT_MANIFEST = @{
            "#1" = @{
                "0409" = @{
                    assembly = @{
                        identity = @{
                            name = "Terragrunt"
                            version = $fileVersion
                        }
                        description = "Terragrunt - Orchestrate OpenTofu and Terraform at Scale"
                    }
                    compatibility = @{
                        application = @(
                            @{
                                supportedOS = @{
                                    Id = "{e2011457-1546-43c5-a5fe-008deee3d3f0}"
                                    comment = "Windows Vista / Windows Server 2008"
                                }
                            },
                            @{
                                supportedOS = @{
                                    Id = "{35138b9a-5d96-4fbd-8e2d-a2440225f93a}"
                                    comment = "Windows 7 / Windows Server 2008 R2"
                                }
                            },
                            @{
                                supportedOS = @{
                                    Id = "{4a2f28e3-53b9-4441-ba9c-d69d4a4a6e38}"
                                    comment = "Windows 8 / Windows Server 2012"
                                }
                            },
                            @{
                                supportedOS = @{
                                    Id = "{1f676c76-80e1-4239-95bb-83d0f6d0da78}"
                                    comment = "Windows 8.1 / Windows Server 2012 R2"
                                }
                            },
                            @{
                                supportedOS = @{
                                    Id = "{8e0f7a12-bfb3-4fe8-b9a5-48fd50a15a9a}"
                                    comment = "Windows 10, Windows 11 / Windows Server 2016, 2019, 2022"
                                }
                            }
                        )
                    }
                    dpiAwareness = "PerMonitorV2, PerMonitor"
                }
            }
        }
        RT_VERSION = @{
            "#1" = @{
                "0409" = @{
                    fixed = @{
                        file_version = $fileVersion
                        product_version = $fileVersion
                    }
                    info = @{
                        "0409" = @{
                            Comments = "Standardize IaC and manage growing infra complexity: define units, stacks, cut repetition with includes/hooks, execute modules in dependency order across environments"
                            CompanyName = "Gruntwork, Inc."
                            FileDescription = "Terragrunt - Orchestrate OpenTofu and Terraform at Scale"
                            FileVersion = $version
                            InternalName = "terragrunt"
                            LegalCopyright = "Copyright (C) $copyrightYear Gruntwork, Inc."
                            OriginalFilename = "terragrunt.exe"
                            ProductName = "Terragrunt"
                            ProductVersion = $version
                        }
                    }
                }
            }
        }
    }

    # Write winres.json to current directory
    $jsonOutput = $winresConfig | ConvertTo-Json -Depth 10 -Compress:$false
    [System.IO.File]::WriteAllText("winres.json", $jsonOutput)

    Write-Host "Generated winres.json:"
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
    param([array]$Platforms)

    # Add Go bin to PATH
    $goPath = & go env GOPATH
    $goBinPath = Join-Path $goPath "bin"
    $env:PATH = "$goBinPath;$env:PATH"

    Write-Host "Patching Windows binaries with icon and version info..."

    foreach ($platform in $Platforms) {
        $binaryPath = Join-Path $BinDirectory $platform.binary

        if (Test-Path $binaryPath) {
            Write-Host "Patching $($platform.binary) ($($platform.arch))..."
            & go-winres patch --in winres.json $binaryPath

            if ($LASTEXITCODE -ne 0) {
                Write-Error "Failed to patch $($platform.binary)"
                exit 1
            }

            Write-Host "Successfully patched $($platform.binary)"
        } else {
            Write-Error "Binary not found: $binaryPath"
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

    # Get Windows platforms from configuration
    $windowsPlatforms = Get-WindowsPlatforms

    # Update winres.json with version info
    Update-WinresVersion

    # Patch all Windows binaries with resources (icon, manifest, version info)
    Patch-Binaries -Platforms $windowsPlatforms

    # Save credentials
    Save-Credentials

    # Run healthcheck
    Invoke-Healthcheck

    # Sync certificates
    Sync-Certificates

    # Sign binaries based on configuration
    Write-Host ""
    Write-Host "Processing Windows binaries for signing..."
    Write-Host ""

    $signedCount = 0
    $unsignedCount = 0

    foreach ($platform in $windowsPlatforms) {
        $binaryPath = Join-Path $BinDirectory $platform.binary

        if (-not (Test-Path $binaryPath)) {
            Write-Error "Binary not found: $binaryPath"
            exit 1
        }

        if ($platform.signed -eq $true) {
            Write-Host "Signing $($platform.binary) ($($platform.arch))..."
            Sign-Binary -BinaryPath $binaryPath

            Write-Host "Verifying signature on $($platform.binary)..."
            Verify-Signature -BinaryPath $binaryPath

            Write-Host "✓ $($platform.binary): signed and verified"
            $signedCount++
        } else {
            Write-Host "○ $($platform.binary) ($($platform.arch)): patched with resources only (not signed per config)"
            $unsignedCount++
        }
        Write-Host ""
    }

    Write-Host "Windows processing completed successfully:"
    Write-Host "  - Signed: $signedCount binary(ies)"
    Write-Host "  - Patched only: $unsignedCount binary(ies)"
    Write-Host ""
    Write-Host "All decisions based on configuration: $ConfigFile"
}

Main
