<#
.SYNOPSIS
  Creates a Startup-folder shortcut that launches the NFC Time Tracking Server via VBS (no console window).

.PARAMETER InstallDir
  Full path to the deployment folder containing start-nfc-time-tracking.vbs and nfc-time-tracker-server.exe.

.EXAMPLE
  powershell -ExecutionPolicy Bypass -File install-startup-shortcut.ps1 -InstallDir "C:\Program Files\NfcTimeTracking"
#>
param(
  [Parameter(Mandatory = $true)]
  [string] $InstallDir
)

$ErrorActionPreference = "Stop"

if (-not (Test-Path -LiteralPath $InstallDir -PathType Container)) {
  Write-Error "InstallDir is not a directory: $InstallDir"
  exit 1
}

$installDirFull = (Resolve-Path -LiteralPath $InstallDir).Path
$vbs = Join-Path $installDirFull "start-nfc-time-tracking.vbs"

if (-not (Test-Path -LiteralPath $vbs -PathType Leaf)) {
  Write-Error "Missing launcher script: $vbs (copy scripts from the repo into InstallDir first)."
  exit 1
}

$exe = Join-Path $installDirFull "nfc-time-tracker-server.exe"
if (-not (Test-Path -LiteralPath $exe -PathType Leaf)) {
  Write-Error "Missing server binary: $exe"
  exit 1
}

$startup = [Environment]::GetFolderPath("Startup")
$lnkPath = Join-Path $startup "NFC Time Tracking Server.lnk"

$wsh = New-Object -ComObject WScript.Shell
$shortcut = $wsh.CreateShortcut($lnkPath)
# Ziel direkt die .vbs (nicht wscript.exe), damit Windows beim Autostart „NFC Time Tracking Server“
# statt „wscript.exe“ anzeigt.
$shortcut.TargetPath = $vbs
$shortcut.WorkingDirectory = $installDirFull
$shortcut.Description = "NFC Time Tracking Server"
# Icon aus der EXE (eingebettetes Anwendungs-Icon), nicht VBS-Standard-Skriptsymbol
$shortcut.IconLocation = "$exe,0"
$shortcut.Save()

Write-Host "Shortcut created: $lnkPath"
