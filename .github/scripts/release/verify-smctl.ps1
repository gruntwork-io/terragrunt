# Script to verify smctl installation
# Usage: verify-smctl.ps1

$ErrorActionPreference = 'Stop'

Write-Host "Checking smctl installation..."

# Check if smctl is in PATH
$smctlPath = Get-Command smctl.exe -ErrorAction SilentlyContinue

if (-not $smctlPath) {
    Write-Error "smctl.exe not found in PATH"
    exit 1
}

Write-Host "smctl found at: $($smctlPath.Source)"

Write-Host "Checking smctl version..."
& smctl.exe --version

if ($LASTEXITCODE -ne 0) {
    Write-Warning "smctl --version returned non-zero exit code"
}

Write-Host ""
Write-Host "smctl is ready"
