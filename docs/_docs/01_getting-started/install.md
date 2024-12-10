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
3. Rename the downloaded file to `terragrunt`.
4. Add execute permissions to the binary: e.g., On Linux and Mac: `chmod u+x terragrunt`.
5. Put the binary somewhere on your `PATH`: e.g., On Linux and Mac: `mv terragrunt /usr/local/bin/terragrunt`.

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
