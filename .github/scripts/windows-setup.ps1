git config --global core.compression 0
git config --global gc.auto 0
git config --global http.postBuffer 2147483648
git config --global core.packedGitLimit 2048m
git config --global core.packedGitWindowSize 2048m
git config --global pack.windowMemory 2048m
git config --global pack.packSizeLimit 2048m
git config --system core.longpaths true
git config --global core.longpaths true
git config --local core.longpaths true
Set-ItemProperty -Path 'HKLM:\SYSTEM\CurrentControlSet\Control\FileSystem' -Name 'LongPathsEnabled' -Value 1
reg add "HKEY_LOCAL_MACHINE\SOFTWARE\Microsoft\Windows\CurrentVersion\AppModelUnlock" /t REG_DWORD /f /v "AllowDevelopmentWithoutDevLicense" /d "1"

mkdir C:\bin
cmd /c mklink C:\bin\sh.exe "C:\Program Files\Git\usr\bin\bash.exe"
cmd /c mklink C:\bin\bash.exe "C:\Program Files\Git\usr\bin\bash.exe"
echo "C:\bin" | Out-File -Append -FilePath $env:GITHUB_PATH
