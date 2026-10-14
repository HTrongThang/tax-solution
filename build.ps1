###############################################
# TaxSolution - Build & Package Script
# Chay: powershell -ExecutionPolicy Bypass -File build.ps1
###############################################

$ErrorActionPreference = "Stop"
$ROOT = $PSScriptRoot
$OUTPUT = "$ROOT\dist"

Write-Host ""
Write-Host "========================================" -ForegroundColor Cyan
Write-Host "  TaxSolution - Build Script" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
Write-Host ""

# 1. Xoa thu muc cu
if (Test-Path $OUTPUT) {
    Write-Host "[1/5] Xoa ban build cu trong dist..." -ForegroundColor Yellow
    Get-ChildItem -Path $OUTPUT -Recurse | Remove-Item -Force -Recurse -ErrorAction SilentlyContinue
} else {
    New-Item -ItemType Directory -Path $OUTPUT -Force | Out-Null
}

# 2. Build Frontend (C# -> self-contained single file)
Write-Host "[2/5] Build Frontend (C# .NET 8 Self-Contained)..." -ForegroundColor Green
Push-Location "$ROOT\Frontend"
dotnet publish -c Release -r win-x64 --self-contained true -p:PublishSingleFile=true -p:IncludeNativeLibrariesForSelfExtract=true -p:EnableCompressionInSingleFile=true -o "$OUTPUT"
if ($LASTEXITCODE -ne 0) { Write-Host ".NET build failed!" -ForegroundColor Red; exit 1 }
Write-Host "  OK: Frontend.exe (self-contained)" -ForegroundColor DarkGreen
Pop-Location

# 3. Build Backend (Go -> TaxBot.exe)
Write-Host "[3/5] Build Backend (Go)..." -ForegroundColor Green
Push-Location "$ROOT\Backend"
$env:CGO_ENABLED = "0"
go build -ldflags="-s -w" -o "$OUTPUT\TaxBot.exe" ./cmd
if ($LASTEXITCODE -ne 0) { Write-Host "Go build failed!" -ForegroundColor Red; exit 1 }
Copy-Item "$OUTPUT\TaxBot.exe" "$ROOT\Backend\TaxBot.exe" -Force
Write-Host "  OK: TaxBot.exe" -ForegroundColor DarkGreen
Pop-Location

# 4. Copy WebResources
Write-Host "[4/5] Copy WebResources..." -ForegroundColor Green
if (Test-Path "$OUTPUT\WebResources") { Remove-Item "$OUTPUT\WebResources" -Recurse -Force }
Copy-Item -Path "$ROOT\Frontend\WebResources" -Destination "$OUTPUT\WebResources" -Recurse -Force
Write-Host "  OK: WebResources/" -ForegroundColor DarkGreen

# Xoa file khong can thiet
Get-ChildItem "$OUTPUT" -Filter "*.pdb" -Recurse | Remove-Item -Force -ErrorAction SilentlyContinue
Get-ChildItem "$OUTPUT" -Filter "*.xml" -Recurse | Remove-Item -Force -ErrorAction SilentlyContinue

# 5. Build bo cai dat Inno Setup
Write-Host "[5/5] Dong goi Inno Setup Installer..." -ForegroundColor Green
$ISCC = "C:\Program Files (x86)\Inno Setup 6\ISCC.exe"
if (Test-Path $ISCC) {
    & $ISCC "$ROOT\installer.iss"
    if ($LASTEXITCODE -eq 0) {
        Write-Host "  OK: Tao installer thanh cong trong output_installer/" -ForegroundColor DarkGreen
    } else {
        Write-Host "  WARNING: Inno Setup build that bai!" -ForegroundColor Red
    }
} else {
    Write-Host "  Khong tim thay Inno Setup tai $ISCC, vui long bien dich installer.iss thu cong." -ForegroundColor Yellow
}

# Ket qua
$size = [math]::Round((Get-ChildItem $OUTPUT -Recurse | Measure-Object -Property Length -Sum).Sum / 1MB, 1)
Write-Host ""
Write-Host "========================================" -ForegroundColor Cyan
Write-Host "  BUILD THANH CONG!" -ForegroundColor Green
Write-Host "  Output dist: $OUTPUT" -ForegroundColor White
Write-Host "  Kich thuoc dist: $size MB" -ForegroundColor White
Write-Host "  Chay: Frontend.exe (Self-contained, khach khong can cai .NET)" -ForegroundColor White
Write-Host "========================================" -ForegroundColor Cyan
