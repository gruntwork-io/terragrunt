git config --global core.compression 9
git config --system core.longpaths true
git config --global core.longpaths true
git config --local core.longpaths true
Set-ItemProperty -Path 'HKLM:\SYSTEM\CurrentControlSet\Control\FileSystem' -Name 'LongPathsEnabled' -Value 1
reg add "HKEY_LOCAL_MACHINE\SOFTWARE\Microsoft\Windows\CurrentVersion\AppModelUnlock" /t REG_DWORD /f /v "AllowDevelopmentWithoutDevLicense" /d "1"

mkdir C:\bin
cmd /c mklink C:\bin\sh.exe "C:\Program Files\Git\usr\bin\bash.exe"
cmd /c mklink C:\bin\bash.exe "C:\Program Files\Git\usr\bin\bash.exe"
echo "C:\bin" | Out-File -Append -FilePath $env:GITHUB_PATH
