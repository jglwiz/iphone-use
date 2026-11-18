# Installs this checkout's environment; never starts services or flashes hardware.
param([string]$Python = 'python', [string]$NimBLEArchive = '')
$ErrorActionPreference = 'Stop'
$projectDir = Split-Path -Parent $PSScriptRoot
$pythonExe = Join-Path $projectDir '.venv/Scripts/python.exe'
if (-not (Test-Path -LiteralPath $pythonExe)) {
    & $Python -m venv (Join-Path $projectDir '.venv')
    if ($LASTEXITCODE -ne 0) { throw 'venv creation failed' }
}
& $pythonExe -m pip install -e (Join-Path $projectDir 'capture') -r (Join-Path $PSScriptRoot 'requirements-dev.txt')
if ($LASTEXITCODE -ne 0) { throw 'Dependency installation failed' }
$depsDir = Join-Path $projectDir '.runtime/deps'
New-Item -ItemType Directory -Force -Path $depsDir | Out-Null
$archive = Join-Path $depsDir 'nimble-2.3.8.zip'
if ($NimBLEArchive) {
    if ((Resolve-Path -LiteralPath $NimBLEArchive).Path -ne $archive) {
        Copy-Item -LiteralPath $NimBLEArchive -Destination $archive
    }
} elseif (-not (Test-Path -LiteralPath $archive)) {
    Invoke-WebRequest 'https://codeload.github.com/h2zero/NimBLE-Arduino/zip/refs/tags/2.3.8' -OutFile $archive
}
$expected = '4849BA65F187EA237A5C8DE7FD0916692CFD058771EB8E8BDAB3135AEFAA78AF'
if ((Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash -ne $expected) { throw 'NimBLE archive checksum mismatch' }
Expand-Archive -LiteralPath $archive -DestinationPath $depsDir -Force
Write-Output "Environment ready: $pythonExe"
Write-Output "NimBLE: $(Join-Path $depsDir 'NimBLE-Arduino-2.3.8')"
Write-Output 'Use hardware/build-serial.ps1 with Arduino ESP32 3.3.12. No hardware was flashed.'
