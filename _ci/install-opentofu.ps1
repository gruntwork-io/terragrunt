OpenTofuInstallPath = "C:\Program Files\tofu\tofu.exe"
$OpenTofuTmpPath = "C:\OpenTofutmp"
$OpenTofuTmpBinaryPath = "C:\OpenTofutmp\tofu.exe"
$OpenTofuPath = "C:\Program Files\tofu"
# Remove any old OpenTofu installation, if present
if (Test-Path -Path $OpenTofuInstallPath)
{
	Remove-Item $OpenTofuInstallPath -Recurse
}
# Download OpenTofu and unpack it
$OpenTofuURI = "https://github.com/opentofu/opentofu/releases/download/v1.6.0-alpha1/tofu_1.6.0-alpha1_windows_amd64.zip"
$output = "tofu_1.6.0-alpha1_windows_amd64.zip"
$ProgressPreference = "SilentlyContinue"
Invoke-WebRequest -Uri $OpenTofuURI -OutFile $output
New-Item -ItemType "directory" -Path $OpenTofuTmpPath
# Unpack OpenTofu to temp directory
Expand-Archive -LiteralPath $output -DestinationPath $OpenTofuTmpPath
# Make new OpenTofu directory to hold binary
New-Item -ItemType "directory" -Path $OpenTofuPath
Move-Item $OpenTofuTmpBinaryPath $OpenTofuPath
# Add new OpenTofu path to system
$OldPath = [System.Environment]::GetEnvironmentVariable('PATH', "Machine")
$NewPath = "$OldPath;$OpenTofuPath"
[Environment]::SetEnvironmentVariable("PATH", "$NewPath", "Machine")
# Load System and User PATHs into latest $env:Path, which has the effect of "refreshing" the latest path
# in the current PowerShell session
$env:Path = [System.Environment]::GetEnvironmentVariable("Path","Machine") + ";" + [System.Environment]::GetEnvironmentVariable("Path","User")
# Verify installation
tofu version
