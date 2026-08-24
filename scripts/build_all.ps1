<#
.SYNOPSIS
    Builds and packages all ActonOS plugins inside the 'plugins' directory into 'dist/' as .actonpkg bundles
    and generates the plugin-registry.json catalog.

.DESCRIPTION
    Scans the ./plugins directory (channels & saas connectors), validates manifests,
    compiles Go code into WebAssembly (GOOS=wasip1 GOARCH=wasm), packages them
    into production-ready .actonpkg archives, and generates plugin-registry.json with download URLs.

.PARAMETER Clean
    Clean the ./dist directory before building.

.PARAMETER DownloadBaseURL
    Base URL for release downloads (default: https://github.com/actonos/plugin-sdk/releases/latest/download)

.EXAMPLE
    .\scripts\build_all.ps1
    .\build_all.ps1 -Clean
#>

[CmdletBinding()]
param(
    [switch]$Clean = $false,
    [string]$DownloadBaseURL = "https://github.com/actonos/plugin-sdk/releases/latest/download"
)

$ErrorActionPreference = "Stop"
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$RootDir = Split-Path -Parent $ScriptDir
if (!(Test-Path "$RootDir\go.mod")) {
    $RootDir = $ScriptDir # In case script is in root
}

Set-Location $RootDir

$DistDir = Join-Path $RootDir "dist"
if ($Clean -and (Test-Path $DistDir)) {
    Write-Host "🧹 Cleaning $DistDir ..." -ForegroundColor Yellow
    Remove-Item -Path "$DistDir\*" -Recurse -Force -ErrorAction SilentlyContinue
}

if (!(Test-Path $DistDir)) {
    New-Item -ItemType Directory -Path $DistDir -Force | Out-Null
}

$sdkVersion = "1.0.0"
$versionFile = Join-Path $RootDir "VERSION"
if (Test-Path $versionFile) {
    $sdkVersion = (Get-Content -Raw -Path $versionFile).Trim()
}

Write-Host "=================================================================" -ForegroundColor Cyan
Write-Host "🚀 ActonOS Plugin SDK - Batch Build & Package Toolchain" -ForegroundColor Cyan
Write-Host "=================================================================" -ForegroundColor Cyan
Write-Host "Root Directory: $RootDir" -ForegroundColor Gray
Write-Host "Output Dist:    $DistDir" -ForegroundColor Gray
Write-Host "SDK Version:    v$sdkVersion" -ForegroundColor Gray
Write-Host "Download Base:  $DownloadBaseURL" -ForegroundColor Gray
Write-Host ""

$SearchPaths = @("plugins")

$PluginManifests = @()
foreach ($p in $SearchPaths) {
    $targetPath = Join-Path $RootDir $p
    if (Test-Path $targetPath) {
        $found = Get-ChildItem -Path $targetPath -Filter "manifest.json" -Recurse
        $PluginManifests += $found
    }
}

if ($PluginManifests.Count -eq 0) {
    Write-Error "No plugins found in search paths: $($SearchPaths -join ', ')"
    exit 1
}

Write-Host "Found $($PluginManifests.Count) plugin(s) to process." -ForegroundColor Green
Write-Host ""

$Results = @()
$RegistryEntries = @()
$OverallStart = [System.Diagnostics.Stopwatch]::StartNew()
$SuccessCount = 0
$FailureCount = 0

foreach ($manifestItem in $PluginManifests) {
    $pluginDir = $manifestItem.DirectoryName
    $manifestPath = $manifestItem.FullName
    $relPath = $pluginDir.Substring($RootDir.Length + 1).Replace("\", "/")

    try {
        $manifestContent = Get-Content -Raw -Path $manifestPath | ConvertFrom-Json
        $pluginId = $manifestContent.id
        $pluginName = $manifestContent.name
        $pluginVer = $manifestContent.version
        $pluginDesc = $manifestContent.description
        $pluginAuthor = $manifestContent.author
        $pluginLicense = $manifestContent.license
        $pluginCaps = $manifestContent.capabilities
    } catch {
        Write-Host "❌ [$relPath] Failed to parse manifest.json: $_" -ForegroundColor Red
        $FailureCount++
        $Results += [PSCustomObject]@{
            ID      = "unknown"
            Path    = $relPath
            Status  = "FAILED (Manifest Error)"
            WasmKB  = "-"
            PkgKB   = "-"
            TimeMs  = "-"
        }
        continue
    }

    Write-Host "▶ Building [$pluginId] ($pluginName v$pluginVer)..." -ForegroundColor Yellow

    $wasmOutput = Join-Path $DistDir "$pluginId.wasm"
    $pkgFilename = "$pluginId.actonpkg"
    $pkgOutput  = Join-Path $DistDir $pkgFilename
    $localDist  = Join-Path $pluginDir "dist"
    if (!(Test-Path $localDist)) {
        New-Item -ItemType Directory -Path $localDist -Force | Out-Null
    }
    $localWasmOutput = Join-Path $localDist "plugin.wasm"

    $sw = [System.Diagnostics.Stopwatch]::StartNew()

    # 1. Compile to WebAssembly (wasip1)
    $env:GOOS = "wasip1"
    $env:GOARCH = "wasm"

    $buildCmd = "go build -buildmode=c-shared -trimpath -o `"$wasmOutput`" `"$pluginDir`""
    Invoke-Expression $buildCmd

    if ($LASTEXITCODE -ne 0 -or !(Test-Path $wasmOutput)) {
        Write-Host "❌ [$pluginId] Compilation failed!" -ForegroundColor Red
        $FailureCount++
        $Results += [PSCustomObject]@{
            ID      = $pluginId
            Path    = $relPath
            Status  = "FAILED (Compile Error)"
            WasmKB  = "-"
            PkgKB   = "-"
            TimeMs  = "-"
        }
        continue
    }

    # Copy local wasm inside plugin/dist for tests
    Copy-Item -Path $wasmOutput -Destination $localWasmOutput -Force

    $wasmSizeKB = [math]::Round(((Get-Item $wasmOutput).Length / 1024), 1)

    # 2. Package into .actonpkg (Zip bundle)
    if (Test-Path $pkgOutput) {
        Remove-Item -Path $pkgOutput -Force
    }

    $zipStream = [System.IO.File]::Create($pkgOutput)
    $zipArchive = New-Object System.IO.Compression.ZipArchive($zipStream, [System.IO.Compression.ZipArchiveMode]::Create)

    # Add manifest.json
    $manifestEntry = $zipArchive.CreateEntry("manifest.json", [System.IO.Compression.CompressionLevel]::Optimal)
    $entryStream = $manifestEntry.Open()
    $fileBytes = [System.IO.File]::ReadAllBytes($manifestPath)
    $entryStream.Write($fileBytes, 0, $fileBytes.Length)
    $entryStream.Close()

    # Add plugin.wasm
    $wasmEntry = $zipArchive.CreateEntry("plugin.wasm", [System.IO.Compression.CompressionLevel]::Optimal)
    $entryStream = $wasmEntry.Open()
    $fileBytes = [System.IO.File]::ReadAllBytes($wasmOutput)
    $entryStream.Write($fileBytes, 0, $fileBytes.Length)
    $entryStream.Close()

    # Add signature if present
    $sigPath = Join-Path $pluginDir "dist\signature.sig"
    if (Test-Path $sigPath) {
        $sigEntry = $zipArchive.CreateEntry("signature.sig", [System.IO.Compression.CompressionLevel]::Optimal)
        $entryStream = $sigEntry.Open()
        $fileBytes = [System.IO.File]::ReadAllBytes($sigPath)
        $entryStream.Write($fileBytes, 0, $fileBytes.Length)
        $entryStream.Close()
    }

    # Add README.md if present
    $readmePath = Join-Path $pluginDir "README.md"
    if (Test-Path $readmePath) {
        $readmeEntry = $zipArchive.CreateEntry("README.md", [System.IO.Compression.CompressionLevel]::Optimal)
        $entryStream = $readmeEntry.Open()
        $fileBytes = [System.IO.File]::ReadAllBytes($readmePath)
        $entryStream.Write($fileBytes, 0, $fileBytes.Length)
        $entryStream.Close()
    }

    $zipArchive.Dispose()
    $zipStream.Dispose()

    $sw.Stop()
    $pkgItem = Get-Item $pkgOutput
    $pkgSizeKB = [math]::Round(($pkgItem.Length / 1024), 1)

    # 3. Compute SHA256 Hash
    $sha256 = (Get-FileHash -Path $pkgOutput -Algorithm SHA256).Hash.ToLower()
    $downloadUrl = "$($DownloadBaseURL.TrimEnd('/'))/$pkgFilename"

    $regEntry = [ordered]@{
        id           = $pluginId
        name         = $pluginName
        version      = $pluginVer
        description  = $pluginDesc
        author       = $pluginAuthor
        license      = $pluginLicense
        capabilities = $pluginCaps
        filename     = $pkgFilename
        download_url = $downloadUrl
        size_bytes   = $pkgItem.Length
        sha256       = $sha256
    }
    $RegistryEntries += $regEntry

    Write-Host "   ✅ Compiled & Packaged -> dist/$pkgFilename ($pkgSizeKB KB) in $($sw.ElapsedMilliseconds)ms" -ForegroundColor Green

    $SuccessCount++
    $Results += [PSCustomObject]@{
        ID      = $pluginId
        Path    = $relPath
        Status  = "SUCCESS"
        WasmKB  = "$wasmSizeKB KB"
        PkgKB   = "$pkgSizeKB KB"
        TimeMs  = "$($sw.ElapsedMilliseconds)ms"
    }
}

# 4. Write plugin-registry.json
$registryData = [ordered]@{
    schema_version    = "1.0.0"
    generated_at      = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
    sdk_version       = $sdkVersion
    total_plugins     = $RegistryEntries.Count
    download_base_url = $DownloadBaseURL
    plugins           = $RegistryEntries
}

$registryJson = $registryData | ConvertTo-Json -Depth 10
$distRegistryPath = Join-Path $DistDir "plugin-registry.json"
Set-Content -Path $distRegistryPath -Value $registryJson

$OverallStart.Stop()

Write-Host ""
Write-Host "📄 Generated Registry Catalog: dist/plugin-registry.json ($($RegistryEntries.Count) plugins)" -ForegroundColor Green

Write-Host ""
Write-Host "=================================================================" -ForegroundColor Cyan
Write-Host "📊 Build & Packaging Summary" -ForegroundColor Cyan
Write-Host "=================================================================" -ForegroundColor Cyan

$Results | Format-Table -AutoSize -Property ID, Path, Status, WasmKB, PkgKB, TimeMs

Write-Host "Total Processed: $($PluginManifests.Count) | Success: $SuccessCount | Failed: $FailureCount | Total Time: $([math]::Round($OverallStart.Elapsed.TotalSeconds, 2))s" -ForegroundColor $(if ($FailureCount -eq 0) { "Green" } else { "Red" })
Write-Host "All distributable packages and registry are located in: $DistDir" -ForegroundColor Cyan
Write-Host "=================================================================" -ForegroundColor Cyan
