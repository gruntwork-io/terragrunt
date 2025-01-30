import React from 'react';
import CodeBlock from '@theme/CodeBlock';

export default function Install({ os, arch, version }) {
	if (os === 'windows') {
		return (
			<div>
				<CodeBlock
					language="powershell">
					{`$os = "windows"
$arch = "amd64"
$version = "v0.72.5"
$binaryName = "terragrunt_\${os}_\${arch}.exe"

try {
    $ProgressPreference = 'SilentlyContinue'

    # Download binary and checksum
    $baseUrl = "https://github.com/gruntwork-io/terragrunt/releases/download/$version"
    Write-Host "Downloading Terragrunt $version..."

    Invoke-WebRequest -Uri "$baseUrl/$binaryName" -OutFile $binaryName -UseBasicParsing
    Invoke-WebRequest -Uri "$baseUrl/SHA256SUMS" -OutFile "SHA256SUMS" -UseBasicParsing

    $actualChecksum = (Get-FileHash -Algorithm SHA256 $binaryName).Hash.ToLower()
    $expectedChecksum = (Get-Content "SHA256SUMS" | Select-String -Pattern $binaryName).Line.Split()[0].ToLower()

    if ($actualChecksum -ne $expectedChecksum) {
        Write-Error "Checksum verification failed"
        exit 1
    }

    Write-Host "Terragrunt $version has been downloaded and verified successfully"
}
catch {
    Write-Error "Failed to download: $_"
    exit 1
}
finally {
    $ProgressPreference = 'Continue'
}`}
				</CodeBlock>
			</div>
		);
	}

	const checksumCommand = os === 'darwin' ? 'shasum -a 256' : 'sha256sum';

	return (
		<div>
			<CodeBlock
				language="bash">
				{`set -euo pipefail

OS="${os}"
ARCH="${arch}"
VERSION="v0.72.5"
BINARY_NAME="terragrunt_\${OS}_\${ARCH}"

# Download the binary
curl -sL "https://github.com/gruntwork-io/terragrunt/releases/download/$VERSION/$BINARY_NAME" -o "$BINARY_NAME"

# Generate the checksum
CHECKSUM="$(${checksumCommand} "$BINARY_NAME" | awk '{print $1}')"

# Download the checksum file
curl -sL "https://github.com/gruntwork-io/terragrunt/releases/download/$VERSION/SHA256SUMS" -o SHA256SUMS

# Grab the expected checksum
EXPECTED_CHECKSUM="$(grep "$BINARY_NAME" <SHA256SUMS | awk '{print $1}')"

# Compare the checksums
if [ "$CHECKSUM" == "$EXPECTED_CHECKSUM" ]; then
 echo "Checksums match!"
else
 echo "Checksums do not match!"
fi
`}
			</CodeBlock>
		</div>
	);
}
