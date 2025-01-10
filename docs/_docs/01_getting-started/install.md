---
layout: collection-browser-doc
title: Install
category: getting-started
excerpt: Learn how to install Terragrunt on Windows, Mac OS, Linux, FreeBSD and manually from source.
tags: ["install"]
order: 103
nav_title: Documentation
nav_title_link: /docs/
---

## Download from releases page

1. Go to the [Releases Page](https://github.com/gruntwork-io/terragrunt/releases).
2. Downloading the binary for your operating system: e.g., if you're on a Mac, download `terragrunt_darwin_amd64`; if you're on Windows, download `terragrunt_windows_amd64.exe`, etc.
3. Optionally, follow the instructions below on [verifying the checksum](#verifying-the-checksum).
4. Rename the downloaded file to `terragrunt`.
5. Add execute permissions to the binary: e.g., On Linux and Mac: `chmod u+x terragrunt`.
6. Put the binary somewhere on your `PATH`: e.g., On Linux and Mac: `mv terragrunt /usr/local/bin/terragrunt`.

### Verifying the checksum

When you download the binary from the releases page, you can also use the checksum file to verify the integrity of the binary. This can be useful for ensuring that you have an intact binary and that it has not been tampered with.

To verify the integrity of the file, do the following:

1. Have the binary downloaded, and accessible.
2. Generate the SHA256 checksum of the binary.
3. Download the `SHA256SUMS` file from the releases page.
4. Find the expected checksum for the binary you downloaded.
5. If the checksums match, the binary is intact and has not been tampered with.

Here is a basic bash script that does that for an AMD64 Linux environment:

```bash
#!/usr/bin/env bash

set -euo pipefail

OS="linux"
ARCH="amd64"
VERSION="v0.69.10"
BINARY_NAME="terragrunt_${OS}_${ARCH}"

# Download the binary
curl -sL "https://github.com/gruntwork-io/terragrunt/releases/download/$VERSION/$BINARY_NAME" -o "$BINARY_NAME"

# Generate the checksum
CHECKSUM="$(sha256sum "$BINARY_NAME" | awk '{print $1}')"

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
```

Aside from adjusting the `OS` and `ARCH` variables above for different operating systems, you may also need to use different utilities to generate the checksum.

In MacOS environments, you can use the `shasum` command instead of `sha256sum`, if you don't have `sha256sum` installed.

```bash
CHECKSUM="$(shasum -a 256 "$BINARY_NAME" | awk '{print $1}')"
```

In Windows environments, you can either use Windows Subsystem for Linux (WSL) or use `Get-FileHash` in PowerShell.

```powershell
$os = "windows"
$arch = "amd64"
$version = "v0.69.10"
$binaryName = "terragrunt_${os}_${arch}.exe"

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
}
```

## Install via a package manager

Note that all the different package managers are third party. The third party Terragrunt packages may not be updated with the latest version, but are often close. Please check your version against the latest available on the [Releases Page](https://github.com/gruntwork-io/terragrunt/releases).
If you  want the latest version, the recommended installation option is to [download from the releases page](https://github.com/gruntwork-io/terragrunt/releases).

* **Windows**: You can install Terragrunt on Windows using [Chocolatey](https://chocolatey.org/): `choco install terragrunt`.

* **macOS**: You can install Terragrunt on macOS using [Homebrew](https://brew.sh/): `brew install terragrunt`.

* **Linux**: Most Linux users can use [Homebrew](https://docs.brew.sh/Homebrew-on-Linux): `brew install terragrunt`. Arch Linux users can use `pacman -S terragrunt` to install it [`community-terragrunt`](https://archlinux.org/packages/extra/x86_64/terragrunt/). Gentoo users can use `emerge -a app-admin/terragrunt-bin` on Guru, [see for other systems](https://repology.org/project/terragrunt/versions).

* **FreeBSD**: You can install Terragrunt on FreeBSD using [Pkg](https://www.freebsd.org/cgi/man.cgi?pkg(7)): `pkg install terragrunt`.

## Install via tool manager

A best practice when using Terragrunt is to pin the version you are using to ensure that you, your colleagues and your CI/CD pipelines are all using the same version. This also allows you to easily upgrade to new versions and rollback to previous versions if needed.

You can use a tool manager to install and manage Terragrunt versions.

* **mise**: You can install Terragrunt using [mise](https://mise.jdx.dev): `mise install terragrunt <version>`.
* **asdf**: You can install Terragrunt using [asdf](https://asdf-vm.com): `asdf plugin add terragrunt && asdf install terragrunt <version>`.

Both of these tools allow you to pin the version of Terragrunt you are using in a `.tool-versions` (and `.mise.toml` for mise) file in your project directory.

Colleagues and CI/CD pipelines can then install the associated tool manager, and run using the pinned version.

Note that the tools Terragrunt integrates with, such as OpenTofu and Terraform, can also be managed by these tool managers, so you can also pin the versions of those tools in the same file.

Also note that the asdf plugin that `asdf` relies on is maintained by a third party:
<https://github.com/ohmer/asdf-terragrunt>

Gruntwork makes no guarantees about the safety or reliability of third-party plugins.

The asdf plugin relied upon by `mise` is maintained by Gruntwork, as requested by the community:
<https://github.com/gruntwork-io/asdf-terragrunt>

## Building from source

If you'd like to build from source, you can use `go` to build Terragrunt yourself, and install it:

```shell
git clone https://github.com/gruntwork-io/terragrunt.git
cd terragrunt
# Feel free to checkout a particular tag, etc if you want here.
go install
```

## Enable tab completion

If you use either Bash or Zsh, you can enable tab completion for Terragrunt commands. To enable autocomplete, first ensure that a config file exists for your chosen shell.

For Bash shell.

```shell
touch ~/.bashrc
```

For Zsh shell.

```shell
touch ~/.zshrc
```

Then install the autocomplete package.

``` shell
terragrunt --install-autocomplete
```

Once the autocomplete support is installed, you will need to restart your shell.

## Terragrunt GitHub Action

Terragrunt is also available as a GitHub Action. Instructions on how to use it can be found at [https://github.com/gruntwork-io/terragrunt-action](https://github.com/gruntwork-io/terragrunt-action).
