# Zip đồ án bỏ qua build artifacts, dependencies, cache, runtime data
# Chạy: pwsh -File zip-project.ps1
# Output: DATN-submission.zip (ước tính < 10MB)

$ErrorActionPreference = "Stop"

$projectRoot = "e:\Projects\DATN"
$outputZip   = "e:\Projects\DATN\DATN_LeTienDung_SourceCode.zip"

# Xoá zip cũ nếu có
if (Test-Path $outputZip) { Remove-Item $outputZip -Force }

# Danh sách thư mục/file cần EXCLUDE (không đưa vào zip)
$excludes = @(
    # Rust build artifacts — 3.3 GB
    "crypto-engine\target"

    # Go temp cache — 524 MB
    ".tmp-go-cache"
    ".tmp-go-tmp"

    # npm dependencies — 276 MB
    "app\frontend\node_modules"
    "demo-control\frontend\node_modules"

    # Compiled binaries — 153 MB
    "app\build\bin"
    "demo-control\build\bin"
    "app\app"
    "app\coordination_test.exe"
    "app\coordination.test.exe"
    "app\bench_test.exe"
    "demo-control\demo-control.exe"

    # Runtime data — 23 MB
    "app\.local"
    ".demo-control"

    # Agent tooling assets — 42 MB
    "thesis_drafts\paper_project\.agents"

    # LaTeX PDF outputs — 4 MB (compile lại được)
    "thesis_drafts\paper_project\DoAn.pdf"
    "thesis_drafts\paper_project\DoAn_test.pdf"

    # Wails generated bindings — tạo lại khi build
    "app\frontend\wailsjs"
    "demo-control\frontend\wailsjs"

    # Frontend dist — tạo lại khi build
    "app\frontend\dist"
    "demo-control\frontend\dist"

    # Temp/build logs
    "app\bench_full.log"
    "app\bench.log"
    "app\bench_final_sizes.log"
    "demo-control\wails-dev.log"
    "demo-control\.wails-dev.pid"

    # Temp inspect folders
    "app\.tmp_inspect"
    "app\.tmp_dbinspect"
    "app\.tmp_go"

    # Git metadata — 342 MB, không cần nộp
    ".git"

    # Script này
    "zip-project.ps1"

    # Refactor scripts (temp, không phải đồ án chính)
    "refactor.py"
    "refactor2.py"
    "refactor3.py"
    "refactor4.py"
    "refactor5.py"
    "refactor6.py"
    "refactor7.py"
    "refactor8.py"
    "refactor9.py"

    # Temp test files ở root
    "test_fallback.go"
    "test_idem.go"
    "test_mls.go"
)

Write-Host "Dang tao zip: $outputZip" -ForegroundColor Cyan
Write-Host "Exclude:" -ForegroundColor Yellow
$excludes | ForEach-Object { Write-Host "  - $_" }
Write-Host ""

# Dùng .NET ZipFile.CreateFromDirectory khong co option exclude,
# nen dung Compress-Archive voi danh sach file include
# Cach khac: dung 7zip neu co, hoac dung tar

# Approach: lay danh sach file, loai bo exclude, roi zip
$allFiles = Get-ChildItem -Path $projectRoot -Recurse -File -Force | Where-Object {
    $relative = $_.FullName.Substring($projectRoot.Length + 1)
    $shouldExclude = $false
    foreach ($ex in $excludes) {
        if ($relative -like "$ex" -or $relative -like "$ex\*" -or $relative -like "$ex\*") {
            $shouldExclude = $true
            break
        }
    }
    -not $shouldExclude
}

$totalSize = ($allFiles | Measure-Object -Property Length -Sum).Sum
Write-Host "So file se zip: $($allFiles.Count)" -ForegroundColor Green
Write-Host "Tong dung luong: $([math]::Round($totalSize/1MB, 2)) MB" -ForegroundColor Green
Write-Host ""

# Tao thu muc tam de copy file can zip
$tempDir = Join-Path $env:TEMP "DATN-zip-temp"
if (Test-Path $tempDir) { Remove-Item $tempDir -Recurse -Force }
New-Item -ItemType Directory -Path $tempDir -Force | Out-Null

foreach ($file in $allFiles) {
    $relative = $file.FullName.Substring($projectRoot.Length + 1)
    $dest = Join-Path $tempDir $relative
    $destDir = Split-Path $dest -Parent
    if (-not (Test-Path $destDir)) { New-Item -ItemType Directory -Path $destDir -Force | Out-Null }
    Copy-Item $file.FullName $dest -Force
}

# Zip
Write-Host "Dang nen..." -ForegroundColor Cyan
Compress-Archive -Path "$tempDir\*" -DestinationPath $outputZip -CompressionLevel Optimal

# Don dep
Remove-Item $tempDir -Recurse -Force

$zipSize = (Get-Item $outputZip).Length
Write-Host ""
Write-Host "Hoan thanh!" -ForegroundColor Green
Write-Host "File: $outputZip" -ForegroundColor Green
Write-Host "Dung luong: $([math]::Round($zipSize/1MB, 2)) MB" -ForegroundColor Green
