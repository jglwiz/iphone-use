# Install only the new capture entry. Legacy entries are reviewed/retired separately.
param([Parameter(Mandatory)][string]$UxPlayDir)
$ErrorActionPreference = 'Stop'
$projectDir = Split-Path -Parent $PSScriptRoot
$pythonExe = Join-Path $projectDir '.venv/Scripts/pythonw.exe'
if (-not (Test-Path -LiteralPath $pythonExe)) { throw 'Run scripts/setup.ps1 first' }
$receiverDir = (Resolve-Path -LiteralPath $UxPlayDir).Path
if (-not (Test-Path -LiteralPath (Join-Path $receiverDir 'uxplay-windows.exe'))) { throw 'UxPlay executable missing' }
$runtimeDir = Join-Path $projectDir '.runtime/capture'
$startupDir = [Environment]::GetFolderPath('Startup')
$shortcutPath = Join-Path $startupDir 'iPhone Use Capture.lnk'
$shell = New-Object -ComObject WScript.Shell
if (Test-Path -LiteralPath $shortcutPath) {
    $existing = $shell.CreateShortcut($shortcutPath)
    if ($existing.TargetPath -ne $pythonExe) { throw 'Existing shortcut belongs to another checkout; inspect before replacing' }
}
$shortcut = $shell.CreateShortcut($shortcutPath)
$shortcut.TargetPath = $pythonExe
$shortcut.Arguments = "-m iphone_use_capture.control start --runtime `"$runtimeDir`" --uxplay-dir `"$receiverDir`" --receiver start"
$shortcut.WorkingDirectory = $projectDir
$shortcut.Description = 'iPhone Use independent AirPlay capture (no legacy Backend)'
$shortcut.WindowStyle = 7
$shortcut.Save()
Write-Output $shortcutPath
