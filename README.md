# ESP32 iPhone Agent — serial USB/BLE HID

[简体中文](README.zh-CN.md)

This fork controls an iPhone through **PC CLI/MCP → serial → ESP32 → USB or BLE HID**. Both HID interfaces can coexist; commands go only to the explicitly selected output. CLI/MCP control APIs remain unchanged. BLE failure never automatically falls back to USB or replays rejected input.

No project iOS app is required. The iPhone uses system HID support and AirPlay. Screens come from an independent PC AirPlay receiver and HTTP frame service; removing the legacy Python server does not remove that service.

## Build and use

Build the CLI from `cli/` with `go build ./cmd/esp32agent`. Set `ESP32AGENT_PORT` to the verified serial port, `ESP32AGENT_HID_OUTPUT` to `usb` (default) or `ble`, and `ESP32AGENT_FRAME_URL` to the existing frame endpoint. Restart the daemon when changing output. Upstream installers do not install this fork's serial implementation.

- [CLI configuration and API semantics](cli/doc/serial-control.md)
- [Serial firmware build and pairing](hardware/serial_hid/doc/usb-ble-hid.md)
- [Shared HID design](hardware/shared_hid/doc/design.md)
- [Hardware validation and limits](hardware/serial_hid/doc/validation.md)
- [Contributing and checks](CONTRIBUTING.md)

## Repository scope

The iOS app, legacy Python server, app harness and app prompts have been removed. Their committed history remains in Git. Shared actions, mouse reports and pacing live in `hardware/shared_hid/`; `hardware/serial_hid/` is the maintained firmware entry point. The old `hardware/hid_bridge/` remains for historical firmware and offline protocol regression tests.

Based on [ESP32 iPhone Agent](https://github.com/onehumanbeing/esp32-iphone-agent) by **Henry Yuan**. Keep the original MIT copyright and [LICENSE](LICENSE), and credit the upstream project in public demos, videos and posts. See [NOTICE](NOTICE); source-specific licenses still apply.
