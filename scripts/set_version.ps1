<#
.SYNOPSIS
    Updates and synchronizes the ActonOS Plugin SDK version across the repository.

.DESCRIPTION
    Sets the version in root VERSION, sdk/VERSION, and optionally all plugin manifest.json files.

.PARAMETER NewVersion
    The new semantic version (e.g. 2.0.0, 2.1.0, 2.0.1-rc1)

.PARAMETER SyncPlugins
    Also update the "version" field in all plugins/ manifest.json files.

.EXAMPLE
    .\scripts\set_version.ps1 2.0.0
    .\scripts\set_version.ps1 2.1.0 -SyncPlugins
#>

[CmdletBinding()]
param(
    [Parameter(Mandatory = $true, Position = 0)]
    [string]$NewVersion,

    [switch]$SyncPlugins = $false
)

$ErrorActionPreference = "Stop"
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$RootDir = Split-Path -Parent $ScriptDir
if (!(Test-Path "$RootDir\go.mod")) {
    $RootDir = $ScriptDir
}

$NewVersion = $NewVersion.Trim()
if ($NewVersion -match "^v") {
    $NewVersion = $NewVersion.Substring(1)
}

# Validate SemVer syntax (e.g. 2.0.0, 2.0.0-beta.1)
if ($NewVersion -notmatch "^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$") {
    Write-Error "Invalid semantic version format: '$NewVersion'. Expected format: X.Y.Z (e.g. 2.0.0)"
    exit 1
}

Write-Host "=================================================================" -ForegroundColor Cyan
Write-Host "🏷️ ActonOS Plugin SDK - Version Synchronization Tool" -ForegroundColor Cyan
Write-Host "=================================================================" -ForegroundColor Cyan
Write-Host "Target Version: v$NewVersion" -ForegroundColor Yellow
Write-Host ""

# 1. Update root VERSION
$rootVersionFile = Join-Path $RootDir "VERSION"
Set-Content -Path $rootVersionFile -Value $NewVersion -NoNewline
Write-Host "✅ Updated root VERSION -> $NewVersion" -ForegroundColor Green

# 2. Update sdk/VERSION
$sdkVersionFile = Join-Path $RootDir "sdk\VERSION"
Set-Content -Path $sdkVersionFile -Value $NewVersion -NoNewline
Write-Host "✅ Updated sdk/VERSION  -> $NewVersion" -ForegroundColor Green

# 3. Update plugins manifests if requested
if ($SyncPlugins) {
    $pluginsDir = Join-Path $RootDir "plugins"
    $manifests = Get-ChildItem -Path $pluginsDir -Filter "manifest.json" -Recurse
    foreach ($m in $manifests) {
        $json = Get-Content -Raw -Path $m.FullName | ConvertFrom-Json
        $oldVer = $json.version
        $json.version = $NewVersion
        $json | ConvertTo-Json -Depth 10 | Set-Content -Path $m.FullName
        Write-Host "   Updated $($json.id) manifest ($oldVer -> $NewVersion)" -ForegroundColor Gray
    }
    Write-Host "✅ Updated $($manifests.Count) plugin manifest(s)" -ForegroundColor Green
}

Write-Host ""
Write-Host "Version successfully set to v$NewVersion!" -ForegroundColor Cyan
Write-Host "Run 'go build -o ./build/acton-plugin.exe ./cmd/acton-plugin' to compile CLI with the new version." -ForegroundColor Gray
Write-Host "=================================================================" -ForegroundColor Cyan
