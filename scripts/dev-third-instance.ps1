#Requires -Version 5.1
<#
.SYNOPSIS
  Chạy instance thứ ba (peer thứ 3). Instance đầu: `wails dev` trong app/.

.DESCRIPTION
  Giả định instance đầu (wails dev, cwd = app/):
    - db mặc định: app/.local/app.db
    - p2p-port mặc định: 4001

  Script này:
    - db: app/.local/dev-wails-peer3.db
    - p2p-port: 4003

  Bootstrap (tùy chọn, giống dev-second-instance.ps1):
    1) -Bootstrap / -BootstrapFile
    2) app/.local/dev-bootstrap.txt
    3) $env:DATN_BOOTSTRAP

.EXAMPLE
  .\scripts\dev-third-instance.cmd

.EXAMPLE
  .\scripts\dev-third-instance.ps1 -UseGoRun -NoNewWindow
#>
param(
    [string] $Bootstrap = '',

    [string] $BootstrapFile = '',

    [switch] $Headless,

    [string] $Exe = '',

    [switch] $UseGoRun,

    [switch] $AutoBuild,

    [switch] $NoNewWindow
)

$ErrorActionPreference = 'Stop'
$RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$AppDir = Join-Path $RepoRoot 'app'
$LocalInApp = [System.IO.Path]::GetFullPath((Join-Path $AppDir '.local'))
$db = [System.IO.Path]::GetFullPath((Join-Path $LocalInApp 'dev-wails-peer3.db'))
$port = 4003
if (-not $PSBoundParameters.ContainsKey('AutoBuild')) {
    $AutoBuild = $true
}

function Resolve-FromAppDir([string] $PathLike) {
    if ([string]::IsNullOrWhiteSpace($PathLike)) { return $PathLike }
    if ([System.IO.Path]::IsPathRooted($PathLike)) { return [System.IO.Path]::GetFullPath($PathLike) }
    return [System.IO.Path]::GetFullPath((Join-Path $AppDir $PathLike.TrimStart('.\')))
}

if (-not (Test-Path -LiteralPath $LocalInApp)) {
    New-Item -ItemType Directory -Path $LocalInApp -Force | Out-Null
}

if ($BootstrapFile -ne '') {
    $BootstrapFile = Resolve-FromAppDir $BootstrapFile
}
if ($BootstrapFile -ne '' -and (Test-Path -LiteralPath $BootstrapFile)) {
    $Bootstrap = (Get-Content -LiteralPath $BootstrapFile -TotalCount 1).Trim()
}

$defaultBootstrap = Join-Path $LocalInApp 'dev-bootstrap.txt'
if ($Bootstrap -eq '' -and (Test-Path -LiteralPath $defaultBootstrap)) {
    $Bootstrap = (Get-Content -LiteralPath $defaultBootstrap -TotalCount 1).Trim()
}

if ($Bootstrap -eq '' -and $env:DATN_BOOTSTRAP) {
    $Bootstrap = $env:DATN_BOOTSTRAP.Trim()
}

$runArgs = @('-db', $db, '-p2p-port', "$port")
if ($Bootstrap -ne '') {
    $runArgs += @('-bootstrap', $Bootstrap)
}
if ($Headless) {
    $runArgs += '-headless'
}

if ($Exe -eq '') {
    $Exe = Join-Path $AppDir 'build\bin\SecureP2P.exe'
}

$hasExe = Test-Path -LiteralPath $Exe

if (-not $UseGoRun -and $AutoBuild) {
    Write-Host "Auto build: wails build (instance 3)" -ForegroundColor DarkGray
    Push-Location $AppDir
    try {
        & wails build
    }
    finally {
        Pop-Location
    }
    $hasExe = Test-Path -LiteralPath $Exe
}

if (-not $hasExe -and -not $UseGoRun) {
    Write-Host "Chưa thấy exe: $Exe" -ForegroundColor Yellow
    Write-Host "Build: cd app && wails build   hoặc thêm -UseGoRun" -ForegroundColor Yellow
    $UseGoRun = $true
}

Write-Host "Third instance: db=$db  port=$port  headless=$Headless" -ForegroundColor Cyan
if ($Bootstrap -ne '') {
    Write-Host "Bootstrap: $Bootstrap" -ForegroundColor DarkGray
}

if ($hasExe -and -not $UseGoRun) {
    if ($NoNewWindow) {
        & $Exe @runArgs
    }
    else {
        Start-Process -FilePath $Exe -WorkingDirectory $AppDir -ArgumentList $runArgs
    }
    exit 0
}

Push-Location $AppDir
try {
    $goArgs = @('run', '.', '--') + $runArgs
    if ($NoNewWindow) {
        & go @goArgs
    }
    else {
        Start-Process -FilePath 'go' -WorkingDirectory $AppDir -ArgumentList $goArgs
    }
}
finally {
    Pop-Location
}
