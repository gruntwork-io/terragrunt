# Install golang using Chocolatey
choco install golang --version 1.23.1 -y
# Verify installation
Get-Command go
go version

# configure git compression
git config --global core.compression 0

try {
    # Enable Developer Mode
    reg add "HKEY_LOCAL_MACHINE\SOFTWARE\Microsoft\Windows\CurrentVersion\AppModelUnlock" /t REG_DWORD /f /v "AllowDevelopmentWithoutDevLicense" /d "1"
    # Enable long paths in Git configuration
    Write-Output "Enabling long paths in Git..."
    git config --system core.longpaths true
    git config --global core.longpaths true
    git config --local core.longpaths true

    # Enable long paths in Windows Registry
    Write-Output "Enabling long paths in Windows Registry..."
    $regPath = "HKLM:\SYSTEM\CurrentControlSet\Control\FileSystem"
    Set-ItemProperty -Path $regPath -Name "LongPathsEnabled" -Value 1 -Type DWord

    $longPathsEnabled = Get-ItemProperty -Path $regPath -Name "LongPathsEnabled"
    $gitConfig = git config --system --get core.longpaths

    if ($longPathsEnabled.LongPathsEnabled -eq 1 -and $gitConfig -eq "true") {
        Write-Output "Successfully enabled long paths support"
        exit 0
    } else {
        Write-Error "Failed to verify changes"
        exit 1
    }
} catch {
    Write-Error "An error occurred: $_"
    exit 1
}
