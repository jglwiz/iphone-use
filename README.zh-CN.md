# iPhone Use — 串口双 HID

[项目概览](README.md)

当前方案无需安装 iOS App，PC 通过串口控制 ESP32，再显式选择 USB 或 BLE HID 控制 iPhone；画面由独立 AirPlay 服务提供。

- [CLI 使用与控制语义](cli/doc/serial-control.md)
- [Windows 独立环境](doc/user/windows-setup.md)
- [AirPlay 画面服务](capture/README.md)
- [固件设计与构建](hardware/serial_hid/doc/usb-ble-hid.md)
- [实机验收与边界](hardware/serial_hid/doc/validation.md)

Based on [ESP32 iPhone Agent](https://github.com/onehumanbeing/esp32-iphone-agent) by Henry Yuan. 保留 MIT 许可证与版权声明，公开演示及文章也应注明来源。
