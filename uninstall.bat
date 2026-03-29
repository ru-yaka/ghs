@echo off
title ghs uninstaller
powershell -ExecutionPolicy Bypass -NoProfile -Command ^
  "$dir='$env:LOCALAPPDATA\Programs\ghs'; " ^
  "if (Test-Path $dir) { " ^
  "  Remove-Item $dir -Recurse -Force; " ^
  "  $p=[Environment]::GetEnvironmentVariable('PATH','User'); " ^
  "  $p=$p.Split(';') | Where-Object { $_ -ne $dir }; " ^
  "  [Environment]::SetEnvironmentVariable('PATH',$p -join ';','User'); " ^
  "  Write-Host 'ghs uninstalled successfully.' -ForegroundColor Green; " ^
  "} else { Write-Host 'ghs is not installed.' -ForegroundColor Yellow }"
echo.
pause
