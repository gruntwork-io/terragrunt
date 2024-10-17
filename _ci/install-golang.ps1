# Install golang using Chocolatey
choco install golang --version 1.23.1 -y
# Verify installation
Get-Command go
go version

# Configure long paths
New-ItemProperty -Path "HKLM:\SYSTEM\CurrentControlSet\Control\FileSystem" `
    -Name "LongPathsEnabled" -Value 1 -PropertyType DWORD -Force


git config --system core.longpaths true

