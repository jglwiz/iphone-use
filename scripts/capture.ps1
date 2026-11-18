param(
    [ValidateSet('start','status','stop')][string]$Action = 'status',
    [string]$UxPlayDir = '',
    [ValidateSet('attach','start')][string]$Receiver = 'attach',
    [int]$Port = 8892
)
$ErrorActionPreference = 'Stop'
$projectDir = Split-Path -Parent $PSScriptRoot
$pythonExe = Join-Path $projectDir '.venv/Scripts/python.exe'
$runtimeDir = Join-Path $projectDir '.runtime/capture'
$arguments = @('-m', 'iphone_use_capture.control', $Action, '--runtime', $runtimeDir, '--port', $Port)
if ($Action -eq 'start') {
    if (-not $UxPlayDir) { throw 'start requires -UxPlayDir' }
    $arguments += @('--uxplay-dir', (Resolve-Path -LiteralPath $UxPlayDir).Path, '--receiver', $Receiver)
}
& $pythonExe @arguments
if ($LASTEXITCODE -ne 0) { throw 'Capture operation failed; inspect .runtime/capture/capture.log' }
