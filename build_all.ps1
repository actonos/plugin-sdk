<#
.SYNOPSIS
    Root wrapper to batch build and package all plugins into dist/
#>

[CmdletBinding()]
param(
    [switch]$Clean = $false
)

$params = @{}
if ($Clean) { $params["Clean"] = $true }

& "$PSScriptRoot\scripts\build_all.ps1" @params
