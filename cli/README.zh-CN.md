# esp32agent：PC 串口控制

[English](README.md) · 简体中文

本地分支以 **PC CLI/MCP → Serial Port → ESP32 → USB / BLE HID → iPhone** 为主通道。USB 与 BLE HID 共用动作引擎，控制 API 保持一致。已移除 PC 蓝牙驱动与自动网络回退，不迁入参考项目的 Python 控制 Backend。

## 构建与配置

```powershell
go test ./...
go build -o esp32agent.exe ./cmd/esp32agent
$env:ESP32AGENT_PORT = 'COM24'
$env:ESP32AGENT_HID_OUTPUT = 'usb' # 默认 usb，也可用 ble；修改后重启 daemon
$env:ESP32AGENT_FRAME_URL = 'http://127.0.0.1:8892/frame'
./esp32agent.exe device info
./esp32agent.exe screenshot --output screen.png
./esp32agent.exe mcp
```

COM24 是本机当前连接，不是固定设备标识；用 `devices` 枚举后核对。CLI/MCP 复用同一 daemon，临时桥必须释放串口。当前分支需源码构建，npm、上游安装器与 upgrade 不会安装本实现。

## 调用方式

- `io tap X,Y [--aim] [--count N]`、`io longpress X,Y --duration MS`、`io swipe X1,Y1,X2,Y2` 保留，坐标使用未缩放截图像素；底层使用原生绝对 HID，不发送相对归位。
- `io text TEXT`、`io keys COMBO`、`io button HOME` 保留。汉字转拼音需要手机对应输入法；Home 仅执行一次。
- 新增 `io scroll AMOUNT` / MCP `scroll(amount)`：整数 -4..4，正下负上、零不动，固件负责滚轮节奏。
- `screenshot` 的格式、缩放、质量和输出选项保留；`screencapture --format mjpeg` 可用，HTTP 图像源不支持 H.264/phone_audio。
- MCP 原有 screenshot、tap、longpress、swipe、type_text、press_key、press_button、device_info 参数与封装保留。
- `calibrate` 报告原生绝对模式，不自动生成已验证校准；`pair/unpair` 为 PC 串口兼容入口，不操作手机 BLE HID 配对；`net` 展示串口，不能切回 PC 蓝牙。
- history、intent、memory、report 等本地功能保留。带外导出/发送仍由调用者明确操作。

动作回执不是实际效果证明；结果未知不自动重放。拖动与页面滚屏是不同操作。截图保留实际捕获时间；没有操作后新帧时，旧图只能作为观察图，不能证明动作效果。参考 Backend 的完整 guard v9 未移植，不能声称其验收被 CLI 继承。

完整协议、验收边界、截图服务与 USB/BLE HID 输出选择见 [PC 串口控制](doc/serial-control.md)。

Based on [ESP32 iPhone Agent](https://github.com/onehumanbeing/esp32-iphone-agent) by Henry Yuan. 保留 MIT 许可证；公开演示也应注明来源。
