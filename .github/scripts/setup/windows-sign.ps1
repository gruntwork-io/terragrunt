# Windows Code Signing Script for DigiCert KeyLocker
# This script signs Windows binaries using DigiCert's cloud-based signing service

param(
    [Parameter(Mandatory=$true)]
    [string[]]$Binaries,

    [Parameter(Mandatory=$false)]
    [string]$KeypairAlias,

    [Parameter(Mandatory=$false)]
    [string]$Description = "Terragrunt - Terraform Wrapper",

    [Parameter(Mandatory=$false)]
    [switch]$Help
)

function Show-Usage {
    Write-Host ""
    Write-Host "Usage: $($MyInvocation.MyCommand.Name) -Binaries <paths> [OPTIONS]"
    Write-Host ""
    Write-Host "Required Environment Variables:"
    Write-Host "  DIGICERT_API_KEY               DigiCert ONE API key for authentication"
    Write-Host "  DIGICERT_P12_BASE64             Client certificate in P12 format, base64 encoded"
    Write-Host "  DIGICERT_P12_PASSWORD           Password for the client certificate"
    Write-Host "  DIGICERT_KEYPAIR_ALIAS          The keypair alias name from KeyLocker portal"
    Write-Host ""
    Write-Host "Optional Parameters:"
    Write-Host "  -KeypairAlias <alias>           Override the DIGICERT_KEYPAIR_ALIAS env var"
    Write-Host "  -Description <text>             Description to show in UAC prompt (default: 'Terragrunt - Terraform Wrapper')"
    Write-Host "  -Help                           Show this help text and exit"
    Write-Host ""
    Write-Host "Examples:"
    Write-Host "  .\windows-sign.ps1 -Binaries bin\terragrunt_windows_amd64.exe"
    Write-Host "  .\windows-sign.ps1 -Binaries bin\*.exe -Description 'My App'"
    Write-Host ""
}

function Test-EnvironmentVariable {
    param([string]$Name)

    $value = [Environment]::GetEnvironmentVariable($Name)
    if ([string]::IsNullOrEmpty($value)) {
        Write-Error "ERROR: Required environment variable $Name is not set."
        return $false
    }
    return $true
}

function Install-Prerequisites {
    Write-Host "Checking prerequisites..."

    # Check for signtool.exe
    $signtoolPath = Get-Command signtool.exe -ErrorAction SilentlyContinue
    if (-not $signtoolPath) {
        Write-Host "signtool.exe not found. Searching in Windows SDK..."
        $signtoolPath = Get-ChildItem -Path "C:\Program Files (x86)\Windows Kits\10\bin" -Filter "signtool.exe" -Recurse -ErrorAction SilentlyContinue | Select-Object -First 1

        if (-not $signtoolPath) {
            Write-Error "signtool.exe not found. Please install Windows SDK."
            Write-Host "Install with: choco install windows-sdk-10-version-2004-all"
            exit 1
        }

        # Add to PATH for this session
        $env:PATH += ";$($signtoolPath.DirectoryName)"
        Write-Host "Found signtool.exe at: $($signtoolPath.FullName)"
    } else {
        Write-Host "Found signtool.exe: $($signtoolPath.Source)"
    }

    # Check for smctl.exe
    $smctlPath = Get-Command smctl.exe -ErrorAction SilentlyContinue
    if (-not $smctlPath) {
        Write-Host "smctl.exe not found. Checking default installation path..."
        $defaultSmctlPath = "C:\Program Files\DigiCert\DigiCert Signing Manager Tools\smctl.exe"

        if (Test-Path $defaultSmctlPath) {
            $smctlDir = Split-Path $defaultSmctlPath
            $env:PATH += ";$smctlDir"
            Write-Host "Found smctl.exe at: $defaultSmlPath"
        } else {
            Write-Error "smctl.exe not found. Please install DigiCert Signing Manager Tools."
            Write-Host "Download from: https://one.digicert.com/signingmanager/api-ui/v1/releases/smtools-windows-x64.msi"
            exit 1
        }
    } else {
        Write-Host "Found smctl.exe: $($smctlPath.Source)"
    }
}

#
# --- THIS FUNCTION HAS BEEN FIXED ---
#
function Initialize-DigiCertAuth {
    Write-Host "`nInitializing DigiCert authentication..."

    # Validate required environment variables
    $requiredVars = @("DIGICERT_API_KEY", "DIGICERT_P12_BASE64", "DIGICERT_P12_PASSWORD")
    foreach ($var in $requiredVars) {
        if (-not (Test-EnvironmentVariable -Name $var)) {
            exit 1
        }
    }

    # Decode and save client certificate
    $certPath = Join-Path $env:TEMP "digicert_client_auth_$(Get-Random).p12"
    $certBytes = [System.Convert]::FromBase64String($env:DIGICERT_P12_BASE64)
    [System.IO.File]::WriteAllBytes($certPath, $certBytes)

    Write-Host "Client certificate saved to temporary location"

    # 1. Set the API key
    Write-Host "Configuring smctl API key..."
    & smctl.exe config --api-key $env:DIGICERT_API_KEY
    if ($LASTEXITCODE -ne 0) {
        Write-Error "Failed to set smctl API key!"
        Remove-Item $certPath -Force -ErrorAction SilentlyContinue
        exit 1
    }

    # 2. Set the client certificate
    Write-Host "Configuring smctl client certificate..."
    & smctl.exe config --client-cert $certPath --client-cert-pass $env:DIGICERT_P12_PASSWORD
    if ($LASTEXITCODE -ne 0) {
        Write-Error "Failed to set smctl client certificate!"
        Remove-Item $certPath -Force -ErrorAction SilentlyContinue
        exit 1
    }

    # 3. Synchronize the certificates
    Write-Host "Synchronizing DigiCert certificates..."
    & smctl.exe windows certsync
    if ($LASTEXITCODE -ne 0) {
        Write-Error "Certificate synchronization failed!"
        Remove-Item $certPath -Force -ErrorAction SilentlyContinue
        exit 1
    }

    Write-Host "Certificate synchronization completed successfully"

    # Clean up the temporary certificate file
    Remove-Item $certPath -Force -ErrorAction SilentlyContinue

    return $true
}

function Sign-Binary {
    param(
        [string]$BinaryPath,
        [string]$KeypairAlias,
        [string]$Description
    )

    if (-not (Test-Path $BinaryPath)) {
        Write-Error "Binary not found: $BinaryPath"
        return $false
    }

    Write-Host "`nSigning: $BinaryPath"

    # Sign using signtool with DigiCert KSP
    # The /sha1 parameter uses the keypair alias (certificate thumbprint or alias)
    $signtoolArgs = @(
        "sign",
        "/sha1", $KeypairAlias,
        "/tr", "http://timestamp.digicert.com",
        "/td", "SHA256",
        "/fd", "SHA256",
        "/d", $Description,
        $BinaryPath
    )

    & signtool.exe $signtoolArgs

    if ($LASTEXITCODE -ne 0) {
        Write-Error "Signing failed for: $BinaryPath"
        return $false
    }

    Write-Host "Successfully signed: $BinaryPath"

    # Verify the signature
    Write-Host "Verifying signature..."
    & signtool.exe verify /pa /v $BinaryPath

    if ($LASTEXITCODE -ne 0) {
        Write-Warning "Signature verification failed for: $BinaryPath"
        return $false
    }

    Write-Host "Signature verified successfully"
    return $true
}

function Main {
    if ($Help) {
        Show-Usage
        exit 0
    }

    if ($Binaries.Count -eq 0) {
        Write-Error "No binaries specified to sign."
        Show-Usage
        exit 1
    }

    # Determine keypair alias
    $alias = $KeypairAlias
    if ([string]::IsNullOrEmpty($alias)) {
        $alias = $env:DIGICERT_KEYPAIR_ALIAS
        if ([string]::IsNullOrEmpty($alias)) {
            Write-Error "Keypair alias not specified. Provide -KeypairAlias or set DIGICERT_KEYPAIR_ALIAS environment variable."
            exit 1
        }
    }

    Write-Host "Windows Code Signing Script"
    Write-Host "============================"
    Write-Host "Keypair Alias: $alias"
    Write-Host "Description: $Description"
    Write-Host "Binaries to sign: $($Binaries.Count)"
    Write-Host ""

    # Install and verify prerequisites
    Install-Prerequisites

    # Initialize DigiCert authentication
    if (-not (Initialize-DigiCertAuth)) {
        exit 1
    }

    # Expand wildcards and sign each binary
    $allBinaries = @()
    foreach ($pattern in $Binaries) {
        $expandedFiles = Get-Item $pattern -ErrorAction SilentlyContinue
        if ($expandedFiles) {
            $allBinaries += $expandedFiles
        } else {
            Write-Warning "No files matched pattern: $pattern"
        }
    }

    if ($allBinaries.Count -eq 0) {
        Write-Error "No binaries found to sign!"
        exit 1
    }

    $successCount = 0
    $failCount = 0

    foreach ($binary in $allBinaries) {
        if (Sign-Binary -BinaryPath $binary.FullName -KeypairAlias $alias -Description $Description) {
            $successCount++
        } else {
            $failCount++
        }
    }

    Write-Host "`n============================"
    Write-Host "Signing Summary"
    Write-Host "============================"
    Write-Host "Total binaries: $($allBinaries.Count)"
    Write-Host "Successfully signed: $successCount"
    Write-Host "Failed: $failCount"
    Write-Host ""

    if ($failCount -gt 0) {
        exit 1
    }

    Write-Host "All binaries signed successfully!"
    exit 0
}

# Run main function
Main