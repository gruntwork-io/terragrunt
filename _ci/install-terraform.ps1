# Install terraform using Chocolatey
$TerraformVersion = $Env:TERRAFORM_VERSION
choco install terraform --version $TerraformVersion -y
# Verify installation
Get-Command terraform
terraform version
