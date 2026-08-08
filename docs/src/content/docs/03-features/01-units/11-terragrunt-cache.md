---
title: Terragrunt Cache
description: Learn what the `.terragrunt-cache` directory is and how to manage it.
slug: features/units/terragrunt-cache
sidebar:
  order: 11
---

Terragrunt uses a cache directory (`.terragrunt-cache`) as the working directory for every run.

Terragrunt copies the configuration a run operates on into this directory and runs your OpenTofu/Terraform commands there. It also holds any modules and providers those commands download.

## Why it is smaller than it looks

A project with many units looks like it should hold many copies of the same providers and modules. It usually does not, because the two things that dominate the size are shared rather than duplicated.

**Provider binaries are symlinked.** [Automatic Provider Cache Dir](/features/caching/auto-provider-cache-dir) (on by default with OpenTofu 1.10 and later) and the [Provider Cache Server](/features/caching/provider-cache-server) both keep provider plugins in a single directory outside your project. The `.terraform/providers` directory inside each unit's working directory holds symlinks into it, so a provider is stored on disk once no matter how many units require it.

**Module source is hard linked from the CAS.** The [Content Addressable Store](/features/caching/cas) hashes the content it fetches and hard links it into the working directory. Identical files occupy disk space once, however many units use them. Where the filesystem does not support hard links, Terragrunt falls back to copying.

This makes per-directory measurements misleading. Two units sourcing the same module can each report the same size when measured on their own, yet cost barely more than one of them when measured together, because a hard linked file is counted once per traversal:

```bash
# Measure the whole tree at once, so shared content is counted once
du -sh .

# Follow symlinks to see what providers would cost
# if every unit kept its own copy
du -shL .
```

To see the total Terragrunt is holding on disk, measure the shared caches rather than your project. They live under your user cache directory:

| Platform | Location |
| --- | --- |
| Linux | `$XDG_CACHE_HOME/terragrunt`, or `~/.cache/terragrunt` |
| macOS | `~/Library/Caches/terragrunt` |
| Windows | `%LOCALAPPDATA%\terragrunt` |

On Linux and macOS:

```bash
du -sh "${XDG_CACHE_HOME:-$HOME/.cache}/terragrunt"   # Linux
du -sh ~/Library/Caches/terragrunt                    # macOS
```

On Windows, using PowerShell:

```powershell
"{0:N1} GB" -f ((Get-ChildItem -Recurse -File "$env:LOCALAPPDATA\terragrunt" |
  Measure-Object -Property Length -Sum).Sum / 1GB)
```

## Clearing the Terragrunt cache

The cache is scratch space. You can safely delete it any time, and Terragrunt will recreate it as necessary.

If you need to clean up a lot of these folders (e.g., after `terragrunt run --all apply`), first list every `.terragrunt-cache` folder below the current one:

``` bash
find . -type d -name ".terragrunt-cache"
```

``` powershell
Get-ChildItem -Path . -Filter .terragrunt-cache -Recurse -Directory
```

If you are **SURE** you want to delete all the folders that come up in the previous command, you can recursively delete all of them as follows:

``` bash
find . -type d -name ".terragrunt-cache" -prune -exec rm -rf {} \;
```

``` powershell
Get-ChildItem -Path . -Filter .terragrunt-cache -Recurse -Directory | Remove-Item -Recurse -Force
```

Also consider setting the `TG_DOWNLOAD_DIR` environment variable if you wish to place the cache directories somewhere else.

If you are clearing the cache because you are running out of disk space, check that provider sharing is in effect first. On OpenTofu 1.10 and later it is on by default, and each unit's providers are symlinks into a shared directory. If you use Terraform or an older version of OpenTofu, providers are copied into every unit instead, and enabling the [Provider Cache Server](/features/caching/provider-cache-server) will reclaim the most space. Note that deleting `.terragrunt-cache` does not reclaim the shared provider cache or the CAS store, which live under your user cache directory.
