#Requires -Version 5.1
<#
.SYNOPSIS
  Chạy đúng MỘT instance thứ hai — để instance đầu bạn tự mở bằng `wails dev`.

.DESCRIPTION
  Giả định instance đầu (wails dev, cwd = app/):
    - db mặc định: app/.local/app.db
    - p2p-port mặc định: 4001

  Script này:
    - db: app/.local/dev-wails-sibling.db  (identity riêng, không đụng app.db)
    - p2p-port: 4002

  Bootstrap (tùy chọn, theo thứ tự):
    1) -Bootstrap / -BootstrapFile
    2) app/.local/dev-bootstrap.txt (một dòng multiaddr)
    3) $env:DATN_BOOTSTRAP

.EXAMPLE
  cd app
  wails dev -appargs "-write-bootstrap .local\dev-bootstrap.txt"

  # Terminal khác, từ gốc repo:
  .\scripts\dev-second-instance.ps1

.EXAMPLE
  .\scripts\dev-second-instance.ps1 -UseGoRun -NoNewWindow
#>
param(
    [string] $Bootstrap = '',

    [string] $BootstrapFile = '',

    [switch] $Headless,

    [string] $Exe = '',

    [switch] $UseGoRun,

    [switch] $NoNewWindow
)

$ErrorActionPreference = 'Stop'
$RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$AppDir = Join-Path $RepoRoot 'app'
$LocalInApp = [System.IO.Path]::GetFullPath((Join-Path $AppDir '.local'))
$db = [System.IO.Path]::GetFullPath((Join-Path $LocalInApp 'dev-wails-sibling.db'))
$port = 4002

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

if (-not $hasExe -and -not $UseGoRun) {
    Write-Host "Chưa thấy exe: $Exe" -ForegroundColor Yellow
    Write-Host "Build: cd app && wails build   hoặc thêm -UseGoRun" -ForegroundColor Yellow
    $UseGoRun = $true
}

Write-Host "Second instance: db=$db  port=$port  headless=$Headless" -ForegroundColor Cyan
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
