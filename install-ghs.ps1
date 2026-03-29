# ghs installer script
# Usage: Right-click -> "Run with PowerShell" or call from install.bat

$ErrorActionPreference = "Stop"
$InstallDir = "$env:LOCALAPPDATA\Programs\ghs"
$ExeName = "ghs.exe"

# Find the ghs.exe next to this script
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
if (-not $ScriptDir) { $ScriptDir = $PWD }
$SourceExe = Join-Path $ScriptDir $ExeName

if (-not (Test-Path $SourceExe)) {
    Write-Host ""
    Write-Host "  Error: $ExeName not found in $($ScriptDir)" -ForegroundColor Red
    Write-Host ""
    Read-Host "Press Enter to exit"
    exit 1
}

# Create install directory
if (-not (Test-Path $InstallDir)) {
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
}

# Copy ghs.exe
Copy-Item $SourceExe (Join-Path $InstallDir $ExeName) -Force
Write-Host ""

# Add to user PATH if not already there
$UserPath = [Environment]::GetEnvironmentVariable("PATH", "User")
if ($UserPath -notlike "*$InstallDir*") {
    [Environment]::SetEnvironmentVariable("PATH", "$UserPath;$InstallDir", "User")
    $AddedPath = $true
} else {
    $AddedPath = $false
}

# Refresh current session PATH
$env:PATH = "$env:PATH;$InstallDir"

Write-Host "  ghs installed successfully!" -ForegroundColor Green
Write-Host ""
Write-Host "  Location:    $(Join-Path $InstallDir $ExeName)" -ForegroundColor Cyan
Write-Host "  Version:     " -NoNewline
try {
    $ver = & (Join-Path $InstallDir $ExeName) version 2>&1
    Write-Host $ver -ForegroundColor Cyan
} catch {
    Write-Host "unknown" -ForegroundColor Yellow
}
if ($AddedPath) {
    Write-Host "  User PATH:   updated" -ForegroundColor Green
} else {
    Write-Host "  User PATH:   already configured" -ForegroundColor DarkGray
}
Write-Host ""
Write-Host "  Restart your terminal, then run:  ghs whoami" -ForegroundColor Yellow
Write-Host ""
