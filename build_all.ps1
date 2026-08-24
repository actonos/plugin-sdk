<#
.SYNOPSIS
    Root wrapper to batch build and package plugins into dist/
#>

[CmdletBinding()]
param(
    [Parameter(Position = 0, ValueFromRemainingArguments = $false)]
    [Alias("Name", "Target", "Filter")]
    [string]$Plugin = "",
    [switch]$Clean = $false,
    [string]$DownloadBaseURL = "https://github.com/actonos/plugin-sdk/releases/latest/download"
)

$params = @{}
if ($Plugin -ne "") { $params["Plugin"] = $Plugin }
if ($Clean) { $params["Clean"] = $true }
if ($PSBoundParameters.ContainsKey("DownloadBaseURL")) { $params["DownloadBaseURL"] = $DownloadBaseURL }

& "$PSScriptRoot\scripts\build_all.ps1" @params
