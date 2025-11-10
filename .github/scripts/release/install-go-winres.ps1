# Script to install go-winres tool
# Usage: install-go-winres.ps1

$ErrorActionPreference = 'Stop'

Write-Host "Installing go-winres..."

# Install go-winres
go install github.com/tc-hib/go-winres@latest

if ($LASTEXITCODE -ne 0) {
    Write-Error "Failed to install go-winres"
    exit 1
}

# Add Go bin to PATH
$goPath = & go env GOPATH
$goBinPath = Join-Path $goPath "bin"
$env:PATH = "$goBinPath;$env:PATH"

# Export PATH to GitHub environment
echo "$goBinPath" | Out-File -FilePath $env:GITHUB_PATH -Encoding utf8 -Append

Write-Host "go-winres installed to: $goBinPath"
Write-Host ""
Write-Host "Verifying go-winres installation..."

& go-winres help

if ($LASTEXITCODE -ne 0) {
    Write-Error "go-winres verification failed"
    exit 1
}

Write-Host ""
Write-Host "go-winres installed successfully"
