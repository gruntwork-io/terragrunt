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

# Capture stderr and stdout
$output = & smctl.exe --version 2>&1

# Check exit code
if ($LASTEXITCODE -ne 0) {
    Write-Error "smctl --version failed with exit code $LASTEXITCODE. Output: $output"
    exit $LASTEXITCODE
}

# Display output if successful
Write-Host $output

Write-Host ""
Write-Host "smctl is ready"
