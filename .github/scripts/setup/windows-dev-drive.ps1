# Creates a ReFS Dev Drive and points Go's caches and the temp directory at it.
#
# Go's caches and test temp directories hold many small files, which unpack
# and churn far faster on a Dev Drive than on the NTFS C: drive. Run this
# before go-cache-restore: changing GOCACHE and GOMODCACHE changes the cache
# version, so the restore must see the new paths.
$ErrorActionPreference = 'Stop'

# D:\ keeps the backing file outside the checkout.
$vhd = 'D:\dev_drive.vhdx'

$partition = New-VHD -Path $vhd -SizeBytes 16GB -Dynamic |
    Mount-VHD -PassThru |
    Initialize-Disk -PassThru |
    New-Partition -AssignDriveLetter -UseMaximumSize
$partition | Format-Volume -DevDrive -Confirm:$false -Force | Out-Null

$drive = "$($partition.DriveLetter):"
$tmp = "$drive\tmp"
New-Item -ItemType Directory -Force -Path $tmp | Out-Null

@(
    "GOCACHE=$drive\go-build"
    "GOMODCACHE=$drive\go-mod"
    "TEMP=$tmp"
    "TMP=$tmp"
) | Out-File -Append -FilePath $env:GITHUB_ENV

Write-Output "Dev Drive mounted at $drive"
