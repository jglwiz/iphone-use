param(
    [Parameter(Mandatory)][string]$ToolsDir,
    [Parameter(Mandatory)][string]$NimBLE,
    [Parameter(Mandatory)][string]$OutputDir
)
$ErrorActionPreference = 'Stop'
$cli = Join-Path $ToolsDir 'arduino-cli/arduino-cli.exe'
$env:ARDUINO_DIRECTORIES_DATA = Join-Path $ToolsDir 'arduino-data'
$env:ARDUINO_DIRECTORIES_DOWNLOADS = Join-Path $ToolsDir 'arduino-downloads'
$env:ARDUINO_DIRECTORIES_USER = Join-Path $OutputDir 'arduino-user'
New-Item -ItemType Directory -Force -Path $OutputDir | Out-Null
$platforms = & $cli --config-file (Join-Path $ToolsDir 'arduino-cli.yaml') core list --format json
if ($LASTEXITCODE -ne 0) { throw 'Cannot inspect Arduino toolchain' }
$esp32 = ($platforms | ConvertFrom-Json).platforms | Where-Object id -eq 'esp32:esp32'
if ($esp32.installed_version -ne '3.3.12') { throw 'Serial HID requires validated ESP32 Arduino 3.3.12' }
$libraryProperties = Get-Content -LiteralPath (Join-Path $NimBLE 'library.properties')
if (-not ($libraryProperties -match '^version=2\.3\.8\s*$')) { throw 'Serial HID requires validated NimBLE-Arduino 2.3.8' }
& $cli --config-file (Join-Path $ToolsDir 'arduino-cli.yaml') compile --fqbn 'esp32:esp32:esp32s3:USBMode=default,CDCOnBoot=default,FlashSize=16M,PSRAM=opi,PartitionScheme=default_8MB' --library $NimBLE --build-path (Join-Path $OutputDir 'work') --output-dir $OutputDir (Join-Path $PSScriptRoot 'serial_hid')
if ($LASTEXITCODE -ne 0) { throw 'Serial HID build failed.' }
# Build only. Deployment must separately verify the board and its actual partition layout.
