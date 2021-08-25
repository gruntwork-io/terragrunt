TerraformInstallPath = "C:\Program Files\Terraform\terraform.exe"
$TerraformTmpPath = "C:\Terraformtmp"
$TerraformTmpBinaryPath = "C:\Terraformtmp\terraform.exe"
$TerraformPath = "C:\Program Files\Terraform"
# Remove any old terraform installation, if present
if (Test-Path -Path $TerraformInstallPath)
{
	Remove-Item $TerraformInstallPath -Recurse
}
# Download terraform and unpack it
$terraformURI = "https://releases.hashicorp.com/terraform/1.0.4/terraform_1.0.4_windows_amd64.zip"
$output = "terraform.1.0.4.zip"
$ProgressPreference = "SilentlyContinue"
Invoke-WebRequest -Uri $terraformURI -OutFile $output
New-Item -ItemType "directory" -Path $TerraformTmpPath
# Unpack Terraform to temp directory
Expand-Archive -LiteralPath $output -DestinationPath $TerraformTmpPath
# Make new Terraform directory to hold binary
New-Item -ItemType "directory" -Path $TerraformPath
Move-Item $TerraformTmpBinaryPath $TerraformPath
# Add new Terraform path to system
$OldPath = [System.Environment]::GetEnvironmentVariable('PATH', "Machine")
$NewPath = "$OldPath;$TerraformPath"
[Environment]::SetEnvironmentVariable("PATH", "$NewPath", "Machine")
# Load System and User PATHs into latest $env:Path, which has the effect of "refreshing" the latest path
# in the current PowerShell session
$env:Path = [System.Environment]::GetEnvironmentVariable("Path","Machine") + ";" + [System.Environment]::GetEnvironmentVariable("Path","User")
# Verify installation
terraform version
