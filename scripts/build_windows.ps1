$ErrorActionPreference = 'Stop'

$Root = Split-Path -Parent $PSScriptRoot
Set-Location $Root

Write-Host "ROOT=$Root"

if (-not (Test-Path (Join-Path $Root 'dist'))) {
    New-Item -ItemType Directory -Path (Join-Path $Root 'dist') | Out-Null
}

$w64Bin = Join-Path $Root 'tools\w64devkit\bin'
$gccPath = $null

$gccInPath = Get-Command gcc -ErrorAction SilentlyContinue
if ($gccInPath) {
    $gccPath = $gccInPath.Source
    Write-Host "Using system gcc: $gccPath"
} elseif (Test-Path (Join-Path $w64Bin 'gcc.exe')) {
    Write-Host "Using portable gcc: $w64Bin"
    $env:PATH = "$w64Bin;$env:PATH"
    $gccPath = Join-Path $w64Bin 'gcc.exe'
}

if (-not $gccPath) {
    Write-Host "ERROR: gcc not found. Extract tools\w64devkit first (see README)." -ForegroundColor Red
    exit 1
}

Write-Host "gcc: $gccPath"
$env:CGO_ENABLED = '1'
$env:GOOS      = 'windows'
$env:GOARCH    = 'amd64'

Write-Host ""
Write-Host "[1/3] Building native Fyne GUI -> dist\玄宇视频工具箱.exe" -ForegroundColor Cyan
go build -tags fyne_gui -ldflags "-H windowsgui -s -w" -o (Join-Path $Root 'dist\玄宇视频工具箱.exe') (Join-Path $Root '.\cmd\narrato')
if ($LASTEXITCODE -ne 0) {
    Write-Host "[FAIL] GUI build failed, exit=$LASTEXITCODE" -ForegroundColor Red
    exit $LASTEXITCODE
}
Write-Host "   OK" -ForegroundColor Green

Write-Host ""
Write-Host "[2/3] Building CLI helper -> dist\narrato-cli.exe" -ForegroundColor Cyan
$env:CGO_ENABLED = '0'
go build -ldflags "-s -w" -o (Join-Path $Root 'dist\narrato-cli.exe') (Join-Path $Root '.\cmd\narrato')
if ($LASTEXITCODE -ne 0) {
    Write-Host "[FAIL] CLI build failed, exit=$LASTEXITCODE" -ForegroundColor Red
    exit $LASTEXITCODE
}
Write-Host "   OK" -ForegroundColor Green

Write-Host ""
Write-Host "[3/3] Building browser-UI fallback -> dist\narrato-web.exe" -ForegroundColor Cyan
go build -ldflags "-H windowsgui -s -w" -o (Join-Path $Root 'dist\narrato-web.exe') (Join-Path $Root '.\cmd\narrato')
if ($LASTEXITCODE -ne 0) {
    Write-Host "[FAIL] Web build failed, exit=$LASTEXITCODE" -ForegroundColor Red
    exit $LASTEXITCODE
}
Write-Host "   OK" -ForegroundColor Green

Write-Host ""
Write-Host "============================================================"
Write-Host "  Build complete:" -ForegroundColor Green
Write-Host "    dist\玄宇视频工具箱.exe (native Fyne window)"
Write-Host "    dist\narrato-cli.exe (console)"
Write-Host "    dist\narrato-web.exe (browser UI fallback)"
Write-Host "============================================================"
Write-Host "  Reminder: copy ffmpeg.exe and ffprobe.exe into dist\"
Write-Host "            before distributing."
Write-Host "============================================================"
