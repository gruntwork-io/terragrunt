param (
    [string]$Path = ".",
    [int]$MaxDepth = 2
)

function Show-Tree {
    param (
        [string]$CurrentPath,
        [int]$Depth,
        [int]$MaxDepth,
        [string]$Prefix = ""
    )

    if ($Depth -gt $MaxDepth) { return }

    $items = Get-ChildItem -Path $CurrentPath -Directory -Force | Sort-Object Name

    foreach ($item in $items) {
        Write-Host "$Prefix|-- $($item.Name)"
        Show-Tree -CurrentPath $item.FullName -Depth ($Depth + 1) -MaxDepth $MaxDepth -Prefix ("$Prefix|   ")
    }
}

Write-Host "`n📁 Tree view of '$Path' up to depth $MaxDepth`n"
Show-Tree -CurrentPath (Resolve-Path $Path) -Depth 1 -MaxDepth $MaxDepth
