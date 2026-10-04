# Points Go's caches and temp at D:\ (faster than C:\); run before go-cache-restore, as paths version the cache.
$ErrorActionPreference = 'Stop'

$drive = 'D:'
$tmp = "$drive\tmp"
New-Item -ItemType Directory -Force -Path $tmp | Out-Null

@(
    "GOCACHE=$drive\go-build"
    "GOMODCACHE=$drive\go-mod"
    "TEMP=$tmp"
    "TMP=$tmp"
) | Out-File -Append -FilePath $env:GITHUB_ENV

Write-Output ('D:\ has {0:N1} GB free' -f ((Get-PSDrive -Name D).Free / 1GB))
