###############################################
# TaxSolution - Build & Package Script
# Chay: powershell -ExecutionPolicy Bypass -File build.ps1
###############################################

$ErrorActionPreference = "Stop"
$ROOT = $PSScriptRoot
$OUTPUT = "$ROOT\dist\TaxSolution"

Write-Host ""
Write-Host "========================================" -ForegroundColor Cyan
Write-Host "  TaxSolution - Build Script" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
Write-Host ""

# 1. Xoa thu muc cu
if (Test-Path $OUTPUT) {
    Write-Host "[1/4] Xoa ban build cu..." -ForegroundColor Yellow
    Remove-Item -Path $OUTPUT -Recurse -Force
}
New-Item -ItemType Directory -Path $OUTPUT -Force | Out-Null

# 2. Build Backend (Go -> TaxBot.exe)
Write-Host "[2/4] Build Backend (Go)..." -ForegroundColor Green
Push-Location "$ROOT\Backend"
$env:CGO_ENABLED = "0"
go build -ldflags="-s -w" -o "$OUTPUT\TaxBot.exe" ./cmd
if ($LASTEXITCODE -ne 0) { Write-Host "Go build failed!" -ForegroundColor Red; exit 1 }
Write-Host "  OK: TaxBot.exe" -ForegroundColor DarkGreen
Pop-Location

# 3. Build Frontend (C# -> self-contained single file)
Write-Host "[3/4] Build Frontend (C# .NET 8)..." -ForegroundColor Green
Push-Location "$ROOT\Frontend"
dotnet publish -c Release -r win-x64 --self-contained true -p:PublishSingleFile=true -p:IncludeNativeLibrariesForSelfExtract=true -o "$OUTPUT"
if ($LASTEXITCODE -ne 0) { Write-Host ".NET build failed!" -ForegroundColor Red; exit 1 }
Write-Host "  OK: Frontend.exe (self-contained)" -ForegroundColor DarkGreen
Pop-Location

# 4. Copy WebResources
Write-Host "[4/4] Copy WebResources..." -ForegroundColor Green
if (Test-Path "$OUTPUT\WebResources") { Remove-Item "$OUTPUT\WebResources" -Recurse -Force }
Copy-Item -Path "$ROOT\Frontend\WebResources" -Destination "$OUTPUT\WebResources" -Recurse -Force
Write-Host "  OK: WebResources/" -ForegroundColor DarkGreen

# Xoa file khong can thiet
Get-ChildItem "$OUTPUT" -Filter "*.pdb" -Recurse | Remove-Item -Force -ErrorAction SilentlyContinue

# Ket qua
$size = [math]::Round((Get-ChildItem $OUTPUT -Recurse | Measure-Object -Property Length -Sum).Sum / 1MB, 1)
Write-Host ""
Write-Host "========================================" -ForegroundColor Cyan
Write-Host "  BUILD THANH CONG!" -ForegroundColor Green
Write-Host "  Output: $OUTPUT" -ForegroundColor White
Write-Host "  Kich thuoc: $size MB" -ForegroundColor White
Write-Host "  Chay: Frontend.exe" -ForegroundColor White
Write-Host "========================================" -ForegroundColor Cyan
