# remove the old go installation
$goPath = "C:\Go"
$golangURI = "https://golang.org/dl/go1.18.8.windows-amd64.zip"
$output = "go1.18.8.zip"
if (Test-Path -Path $goPath)
{
    Remove-Item $goPath -Recurse
}
# Download golang, unpack it, and then update the PATH to include gobin
# The SilentlyContinue is needed to handle access denied error. See
# https://discuss.circleci.com/t/access-denied-error-while-trying-to-download-software-on-windows-cirlcleci-environment/32809/2
$ProgressPreference = "SilentlyContinue"
Invoke-WebRequest -Uri $golangURI -OutFile $output
Expand-Archive -LiteralPath $output -DestinationPath "C:\Gotmp"
Move-Item "C:\Gotmp\go" "C:\Go"
# Verify installation
go version
Get-Command go
