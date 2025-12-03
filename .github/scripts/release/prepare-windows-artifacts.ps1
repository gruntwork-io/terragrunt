# Script to prepare Windows artifacts for signing
# Usage: prepare-windows-artifacts.ps1 -ArtifactsDirectory <path> -BinDirectory <path>

param(
    [Parameter(Mandatory=$false)]
    [string]$ArtifactsDirectory = "artifacts",

    [Parameter(Mandatory=$false)]
    [string]$BinDirectory = "bin"
)

$ErrorActionPreference = 'Stop'

Write-Host "Preparing Windows build artifacts..."

# Create bin directory
New-Item -ItemType Directory -Force -Path $BinDirectory | Out-Null

# Check if artifacts directory exists
if (-not (Test-Path $ArtifactsDirectory)) {
    Write-Error "Artifacts directory not found: $ArtifactsDirectory"
    exit 1
}

# Copy Windows binaries to bin directory
Get-ChildItem -Path $ArtifactsDirectory -Filter "terragrunt_windows_*" -Recurse -File |
ForEach-Object {
    Copy-Item $_.FullName -Destination $BinDirectory/
    Write-Host "Copied: $($_.Name)"
}

Write-Host ""
Write-Host "Binary files to sign:"
Get-ChildItem -Path $BinDirectory | ForEach-Object { Write-Host $_.FullName }

Write-Host ""
Write-Host "Artifacts prepared successfully"
