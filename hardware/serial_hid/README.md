# 串口双 HID 固件

PC → Serial0（115200）→ ESP32-S3 → USB HID 或 BLE HID → iPhone。

这是PC串口控制的正式固件入口。USB/BLE共用成熟的动作引擎，显式选择一路输出，CLI/MCP控制API保持一致。

- [输出设计与构建](doc/usb-ble-hid.md)：维护报告、动作逻辑、BLE状态、构建及配对时读取。
- [实机验收](doc/validation.md)：查看已验证效果、失败诊断、恢复依据和未覆盖范围。
- [PC配置与控制](../../cli/doc/serial-control.md)：配置串口、选择输出、调用CLI/MCP及解释截图元数据。

基于ESP32 iPhone Agent，Copyright (c) 2026 Henry Yuan，MIT；绝对鼠标布局保留源文件内jglwiz署名。正式代码不依赖任务目录。
