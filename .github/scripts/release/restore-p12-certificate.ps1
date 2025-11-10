# Script to restore P12 client certificate from base64
# Usage: restore-p12-certificate.ps1
# Environment variables:
#   WINDOWS_SIGNING_P12_BASE64: Base64 encoded P12 certificate (required)
#   RUNNER_TEMP: Temporary directory for certificate file
#   GITHUB_ENV: Path to GitHub environment file

$ErrorActionPreference = 'Stop'

Write-Host "Restoring P12 client certificate from base64..."

# Get certificate from environment variable
$base64Certificate = $env:WINDOWS_SIGNING_P12_BASE64
if ([string]::IsNullOrEmpty($base64Certificate)) {
    Write-Error "ERROR: Required environment variable WINDOWS_SIGNING_P12_BASE64 not set."
    exit 1
}

# Decode base64 certificate
$bytes = [Convert]::FromBase64String($base64Certificate)

# Generate output path
$path = Join-Path $env:RUNNER_TEMP "sm_client_auth.p12"

# Write certificate to file
[IO.File]::WriteAllBytes($path, $bytes)

# Verify file was created
if (Test-Path $path) {
    Write-Host "Certificate file created: $path"
    $fileInfo = Get-Item $path
    Write-Host "Size: $($fileInfo.Length) bytes"
} else {
    Write-Error "Failed to create certificate file"
    exit 1
}

# Export to GitHub environment
echo "SM_CLIENT_CERT_FILE=$path" | Out-File -FilePath $env:GITHUB_ENV -Encoding utf8 -Append

Write-Host ""
Write-Host "SM_CLIENT_CERT_FILE set to: $path"
Write-Host "Certificate restored successfully"
