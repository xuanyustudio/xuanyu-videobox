$ErrorActionPreference = "Stop"

$Root = Split-Path -Parent $PSScriptRoot
Set-Location $Root

if (-not (Test-Path "dist")) {
    New-Item -ItemType Directory -Path "dist" | Out-Null
}

# Prefer portable toolchain under tools/w64devkit (no system install required).
$w64Bin = Join-Path $Root "tools\w64devkit\bin"
if (Test-Path (Join-Path $w64Bin "gcc.exe")) {
    $env:Path = "$w64Bin;$env:Path"
    Write-Host "Using portable gcc: $w64Bin"
}

$gcc = Get-Command gcc -ErrorAction SilentlyContinue
if (-not $gcc) {
    Write-Host "ERROR: gcc not found. Extract tools/w64devkit first (see README)."
    exit 1
}

Write-Host "gcc: $($gcc.Source)"
$env:CGO_ENABLED = "1"

Write-Host "Building native Fyne GUI -> dist/narrato.exe ..."
go build -tags fyne_gui -ldflags "-H windowsgui -s -w" -o dist/narrato.exe ./cmd/narrato
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

Write-Host "Building CLI helper -> dist/narrato-cli.exe ..."
$env:CGO_ENABLED = "0"
go build -ldflags "-s -w" -o dist/narrato-cli.exe ./cmd/narrato
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

# Browser UI fallback (no CGO) for machines without gcc
Write-Host "Building browser-UI fallback -> dist/narrato-web.exe ..."
go build -ldflags "-H windowsgui -s -w" -o dist/narrato-web.exe ./cmd/narrato
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

Write-Host ""
Write-Host "Build complete:"
Write-Host "  dist/narrato.exe     (native Fyne window)"
Write-Host "  dist/narrato-cli.exe (console)"
Write-Host "  dist/narrato-web.exe (browser UI fallback)"
Write-Host "Reminder: copy ffmpeg.exe and ffprobe.exe into dist/ before distributing."
