# ESP32 iPhone Agent — 串口双 HID Fork

这个 fork 将电脑作为控制端，通过 **CLI / MCP → 串口 → ESP32-S3** 控制真实 iPhone。ESP32 同时提供 USB HID 和 BLE HID 键盘、鼠标，显式选择一路输出。USB 和 BLE 共用动作逻辑，切换输出不改变 CLI/MCP 的控制参数。

Based on [ESP32 iPhone Agent](https://github.com/onehumanbeing/esp32-iphone-agent) by **Henry Yuan**. 保留原项目 MIT 许可证与版权声明，见 [LICENSE](LICENSE) 和 [NOTICE](NOTICE)。公开演示、视频及文章也应注明原项目来源。

## 与原始仓库有什么区别

| 项目 | 原始仓库的设计 | 本 fork 当前 PC 控制实现 |
|---|---|---|
| 控制入口 | iPhone App 驱动板子；上游 CLI 还支持电脑通过 BLE / Wi-Fi 控板 | 电脑上的 CLI/MCP，共用一个常驻串口 daemon |
| PC → ESP32 | BLE 命令通道及网络通道 | 独立 USB 转串口；移除 PC 蓝牙驱动与自动网络回退 |
| ESP32 → iPhone | 原生 USB HID | USB HID **或**标准 BLE HID；两接口可共存，每条命令只发一路 |
| 鼠标控制 | 上游包含相对移动、归位和校准流程 | PC 路径使用原生绝对坐标，位置、按钮和滚轮报告分离 |
| 屏幕返回 | 上游固件/App 的 AirPlay、视频传输路径 | iPhone AirPlay → PC 接收器 → 独立 HTTP 取帧服务 |
| 固件入口 | `hardware/hid_bridge` | `hardware/serial_hid`，复用共享 HID 动作模块 |
| 安装方式 | 上游发布包、网页烧录器、安装脚本 | 本 fork 从源码构建；上游安装器不会安装本实现 |

本 fork 已移除 iOS App、旧 Python server 及 App 专用调试工具和提示词。iPhone 无需安装本项目 App，使用系统 HID 与 AirPlay 即可。原 `hid_bridge` 仅保留用于历史固件及离线协议测试。上游 App、演示、TestFlight 和发布版本的功能，不应当作本 fork 已完成的部署或验收。参考项目的 Python 控制 Backend 没有迁入；目前只复用本机已有的独立画面服务。

## 当前通信链路

```text
                            ┌─ USB HID ─→ iPhone
PC CLI / MCP → Serial → ESP32
                            └─ BLE HID ─→ iPhone

iPhone → AirPlay → PC UxPlay / 取帧服务 → CLI / MCP 截图
```

这里的 BLE 是 **ESP32 模拟蓝牙键盘和鼠标供 iPhone 使用**，不是电脑通过蓝牙向开发板发送命令。电脑到开发板始终使用串口。

默认输出 USB。USB 可操作手机设置，完成本设备的 BLE 配对、取消配对及重新配对；之后显式切换到 BLE。BLE 不可用时拒绝输入，不自动转到 USB，也不自动重放结果未知的命令。两路不能同时发送同一动作。

USB 模式下，开发板的独立串口口接 PC，原生 USB/OTG 口接 iPhone；不能把同一个原生 USB 口同时接两个主机。BLE 模式下，板子通过无线 HID 向手机发送输入，PC 串口连接保持不变。

## 本 fork 修复和调整的 HID 行为

- **绝对定位与点击分离**：位置报告使用 0–32767 绝对坐标；按钮按下/释放不再附带缓存坐标，避免点击时重新发送位置。当前布局中位置、按钮、滚轮分别使用报告 10、11、12。
- **修复实测点击兼容问题**：本机对照中，上游合并报告布局没有产生完整点击事件；分离布局产生了可信的按下、松开和点击事件。普通点击与有限按压分别验证。
- **修复有滚轮事件但页面不滚动的问题**：将实测有效的连续滚轮节奏放入固件，提供 `scroll_by -4..4`。正数向下、负数向上；数值是动作量，不是像素。页面滚动与按住拖动物件分开处理。
- **共用动作模块**：USB/BLE 复用 `hardware/shared_hid/actions.h`、`abs_mouse.h` 和 pacing 实现。BLE Report Map 直接由同一组 USB 描述符组成，没有另建一套坐标或点击算法。
- **BLE 键盘时序适配**：先建立修饰键，再发送字符，按顺序释放字符与修饰键；BLE 文本每条最多16字节，daemon 自动分块。已验证 ASCII、Shift 字符、空格及退格；汉字转拼音仍依赖手机输入法。
- **连接和释放处理**：区分 USB 状态与 BLE 连接、加密、报告订阅状态；提供设备、启动和连接代次标识。有限按压到期释放，切换输出时不允许带着按下状态过去。

这些是有界实机验证支持的修正，不表示所有 iPhone、方向、应用和系统版本都已覆盖。

## 截图行为修正

静止画面超过5秒仍可供查看，保留真实 `frame_time` 和 `frame_id`，标记为 `observation_only`、`after_action=false`。旧帧不能冒充操作后的新帧，也不能作为自动重试输入的理由。

来源不可达、无帧以及来源明确报告投屏断开分别处理。目前取帧服务没有可靠的手机连接事件时，返回 `mirroring_state=unknown`；不能仅凭旧快照判断手机已断开或仍在投屏。截图新鲜也不自动证明输入生效。

## 实测硬件与软件

以下是2026-11-17双 HID 实验与正式 CLI/MCP 回归所用环境，不是通用兼容性承诺。

| 项目 | 实测配置 |
|---|---|
| 手机 | **iPhone 17e**（用户确认） |
| iOS 版本 / 构建号 | **iOS 26（用户回忆，待在本机确认）；具体小版本及构建号待补录** |
| 开发板 | ESP32-S3-WROOM-1 模组、双 Type-C（独立串口与原生 USB）；PCB 厂商及型号未确认 |
| 芯片与存储 | ESP32-S3 revision 0.2，16 MB Flash、8 MB PSRAM；容量对应 N16R8，完整模组订货型号尚未核对丝印 |
| PC 串口 | 当前为 COM24，115200、8N1；端口号不是永久设备身份 |
| 手机设置 | 辅助触控开启；精确 ASCII 测试临时使用 English (US)，结束恢复豆包中文输入法 |
| 双 HID 固件 | `hardware/serial_hid`，版本 `0.3.0-serial` |
| 构建依赖 | ESP32 Arduino 3.3.12、NimBLE-Arduino 2.3.8 |
| 画面服务 | Windows 上现有 UxPlay / HTTP 取帧服务；当前 `/frame` 地址为 `http://127.0.0.1:8892/frame` |

## 构建与使用

在 `cli/` 目录运行：

```powershell
go test ./...
go build -o esp32agent.exe ./cmd/esp32agent
$env:ESP32AGENT_PORT = 'COM24' # 先核对实际端口
$env:ESP32AGENT_HID_OUTPUT = 'usb' # usb 或 ble
$env:ESP32AGENT_FRAME_URL = 'http://127.0.0.1:8892/frame'
./esp32agent.exe device info
./esp32agent.exe screenshot --output screen.png
./esp32agent.exe mcp
```

CLI/MCP 复用同一 daemon，串口只能有一个拥有者。切换输出时先执行 `./esp32agent.exe daemon stop`，修改 `ESP32AGENT_HID_OUTPUT`，再启动 CLI/MCP。不要同时运行临时桥、串口监视器或另一配置目录的 daemon。

BLE 首次配对使用手机的“设置 → 辅助功能 → 触控 → 辅助触控 → 设备 → 蓝牙设备”，选择 `Handsy Serial HID`。已有实验配对可能缓存名称 `Handsy-BLE-Lab`。USB 可以完成这段手机操作；`device pair/unpair` 兼容入口不负责修改手机的 BLE HID 配对。

固件构建使用 [hardware/build-serial.ps1](hardware/build-serial.ps1)，具体参数、接线与部署步骤见 [串口控制说明](cli/doc/serial-control.md)。脚本只构建，不自动烧录；部署必须核对板子及实际分区、保留有效备份。本机验证仅更新应用分区，保留 NVS 配对资料，不能把本机地址直接用于其他板子。

现有 `tap`、`longpress`、`swipe`、`type_text`、`press_key`、`press_button`、`screenshot` 等控制 API 保留；新增 `scroll(amount)` 对应 `io scroll AMOUNT`。坐标按未缩放截图像素传入，底层转换为绝对 HID 坐标。`swipe` 是按住拖动，页面滚屏使用 `scroll`。

## 验证范围与记录

已完成 USB 自动引导 BLE 配对、清理和重配；正式 CLI/MCP 的 USB/BLE 点击及双向滚动通过。BLE 的多点定位、重复同点、键盘、20位文本分块、有限按压和拖动物件均有新帧或网页可信事件支持。BLE 用例中报告数增加、USB 调用数不变，确认输入没有走错通路。

正式整合时 Go 回归106项通过、3项因 ffmpeg/外部模拟器缺失跳过，`go vet`、Windows 构建及原 USB 固件编译回归通过。静止8秒后的 CLI/MCP 截图保留原帧时间，观察图标记复验通过。

断连后的实机负向验收以最新验证记录为准，不将自动化状态测试当作实机通过。全屏边缘、物理旋转、锁屏、长期稳定性以及跨设备/跨 iOS 版本覆盖仍需单独验收。命令回执仅表示固件处理结果，不等于手机效果已确认。

- [双 HID 验收记录](hardware/serial_hid/doc/validation.md)
- [共享 HID 模块设计](hardware/shared_hid/doc/design.md)
- [USB HID 实验记录](hardware/hid_bridge/doc/usb-hid-validation.md)
- [CLI 使用说明](cli/README.md)
- [通信协议、配置及截图语义](cli/doc/serial-control.md)

本 fork 保留上游源码与署名；上游项目及其历史介绍请参阅 [ESP32 iPhone Agent by Henry Yuan](https://github.com/onehumanbeing/esp32-iphone-agent)。本页描述的是当前 fork 的串口双 HID 实现。
