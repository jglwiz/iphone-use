# Security

This project drives a real phone, so bugs can have real effects. Please report security issues privately via
GitHub's "Report a vulnerability" (Security tab) instead of a public issue. Include steps to reproduce and the
version (commit) you tested.

The maintained serial path includes PC serial access, CLI/MCP input authorization, USB/BLE HID pairing and report handling, and the independent AirPlay frame source. It does not expose the old PC Bluetooth command or hotspot control service. Historical bridge code remains for regression tests; report which firmware entry point is affected.
