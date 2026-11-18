# iPhone Agent CLI — esp32agent

English · [简体中文](README.zh-CN.md)

This branch uses **PC CLI/MCP → Serial Port → ESP32 → USB / BLE HID → iPhone**. USB and BLE HID coexist with one explicitly selected output and the same control API. The PC Bluetooth driver and automatic network fallback have been removed. The reference Python control Backend is not imported.

The project is named **iPhone Agent**; the executable remains `esp32agent`, with unchanged MCP tool names and `ESP32AGENT_*` configuration. USB/BLE firmware now uses `iPhone Agent HID`; deployment and cached names are covered in [device naming and pairing](doc/serial-control.md#固件与配对). See the [project README](../README.md) for directory and AirPlay naming status.

## Build and configure

```powershell
go test ./...
go build -o esp32agent.exe ./cmd/esp32agent
$env:ESP32AGENT_PORT = 'COM24'
$env:ESP32AGENT_HID_OUTPUT = 'usb' # usb (default) or ble; restart daemon after changing
$env:ESP32AGENT_FRAME_URL = 'http://127.0.0.1:8892/frame'
./esp32agent.exe device info
./esp32agent.exe screenshot --output screen.png
./esp32agent.exe mcp
```

COM24 is this setup's current port, not a permanent board identity. `devices` lists ports without probing them. CLI/MCP share one daemon; release the diagnostic bridge first. Build this branch from source: npm, upstream installers and `upgrade` do not install this implementation.

## API

- Existing `io tap X,Y [--aim] [--count N]`, `io longpress X,Y --duration MS`, and `io swipe X1,Y1,X2,Y2` use original screenshot pixels and native absolute HID, without relative homing.
- `io text TEXT`, `io keys COMBO`, and `io button HOME` remain. Chinese is transliterated to pinyin and needs the phone's corresponding keyboard. Home sends one action.
- New `io scroll AMOUNT` / MCP `scroll(amount)`: integer -4..4, positive down, negative up, zero no-op. Firmware owns wheel timing.
- Screenshot format, scaling, quality and output options remain. `screencapture --format mjpeg` uses the HTTP source; H.264 and phone audio are explicitly unsupported by that source.
- Existing MCP screenshot, tap, longpress, swipe, type_text, press_key, press_button and device_info argument shapes and response envelopes remain.
- `calibrate` reports native absolute mode, not a newly verified calibration. `pair/unpair` are PC serial compatibility commands and do not change iPhone BLE HID pairing; `net` shows serial and cannot switch back to PC Bluetooth.
- Local history, intent, memory and report features remain.

A reply confirms firmware submission, not an observed phone effect. Uncertain inputs are never replayed. Pointer dragging is distinct from content scrolling. Screenshots retain capture timestamps; an old still frame is observation-only, never post-action evidence. The reference Backend's full guard v9 and calibration review have not been ported or inherited.

See [serial control design and validation](doc/serial-control.md) for protocol details, screen service requirements and USB/BLE HID output selection.

Based on [ESP32 iPhone Agent](https://github.com/onehumanbeing/esp32-iphone-agent) by Henry Yuan. Keep the MIT license and credit the upstream project in public demos.
