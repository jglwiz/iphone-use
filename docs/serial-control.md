# PC 串口控制

本分支的正式链路为 **CLI/MCP → daemon → Serial Port → ESP32 → USB HID → iPhone**。基于 Henry Yuan 的 ESP32 iPhone Agent；保留原 MIT 许可证与署名，公开演示、视频和文章也应注明原项目。

第二阶段将加入 ESP32→BLE HID→iPhone，沿用同一 CLI/MCP。PC 不再通过蓝牙连接开发板。本轮没有迁移旧 Python Backend；原 iOS App 的 BLE 功能与 PC 控制是不同路径。

## Windows 构建与启动

在 `cli/` 中运行：

```powershell
go test ./...
go build -o esp32agent.exe ./cmd/esp32agent
$env:ESP32AGENT_PORT = 'COM24'
$env:ESP32AGENT_FRAME_URL = 'http://127.0.0.1:8892/frame'
./esp32agent.exe device info
./esp32agent.exe screenshot --output screen.png
./esp32agent.exe mcp
```

COM24 只是当前实测连接。先用 `devices` 枚举并核对设备；枚举不打开端口，也不把所有串口冒充已验证的 ESP32。可用 `--device COM24` 覆盖环境变量。不指定端口时明确报错，避免连接其他开发板。Windows 打开端口时设置 115200、8N1、DTR=false、RTS=false，不主动复位。主机需有独立 UART/USB 串口连接，板子的原生 USB 接手机；不能将同一个原生 USB 口同时插入两个主机。

CLI 与 MCP 自动复用同一配置目录的 daemon。`ESP32AGENT_HOME` 可指定独立运行目录；同一硬件只能有一个拥有者，不同时运行临时桥、串口监视器或另一配置目录的 daemon。修改配置后先 `daemon stop` 再启动。遇到打开失败先检查占用，不自动杀进程或复位板子。

此版本是本地分支构建。npm、上游安装脚本和 `upgrade` 指向上游发布版本，不能用来安装或更新本串口实现。

## 控制 API

保留原 CLI/MCP 命令和 JSON 成功/失败封装：

| CLI | MCP | 实际行为 |
|---|---|---|
| `screenshot` | `screenshot` | 原有截图；坐标以未缩放原图像素为准 |
| `io tap X,Y [--aim] [--count N]` | `tap(x,y,count)` | `abs` 定位后点击，最多 10 次，不发送相对归位 |
| `io longpress X,Y --duration MS` | `longpress(x,y,duration_ms)` | 有界 `press_for`，结束/取消后释放 |
| `io swipe X1,Y1,X2,Y2` | `swipe(x1,y1,x2,y2)` | 绝对指针按住拖动；不保证页面滚屏 |
| `io text TEXT` | `type_text(text)` | 原 ASCII/汉字转拼音规则；空格单独按键，保留键盘节奏 |
| `io keys COMBO` | `press_key(key)` | 原组合键语法 |
| `io button HOME` | `press_button(button)` | 发送一次 Home，不自动追加其他快捷键或手势 |
| `io scroll AMOUNT` | `scroll(amount)` | 新增单参数滚屏，整数 -4..4，正下负上，零不动 |
| `device info` | `device_info` | 原固件状态加串口、USB 输出及截图源字段 |

`scroll_by` 使用已经实测的固件实现，节奏、次数由板子控制，最大量耗时约 3.2 秒；正式串口命令预算 10 秒，长按为动作等待时间。固件不支持时返回明确错误，不回退为单发滚轮或蓝牙。距离不是像素或固定屏幕比例。

`calibrate` 在串口路径报告原生绝对模式，不复用相对鼠标校准，也不伪造一次新校准验收。普通 tap 保留原来的默认直接执行语义；CLI `--aim` 使用绝对移动前后画面检查，不确认则阻止点击、无相对纠偏。参考 Backend 的 guard v9、完整校准审核和作用域门禁没有移植，不能宣称这些能力已由 CLI 等价实现。当前绝对映射采用已实验的 0..32767 坐标；全屏边缘、横屏、多设备及长期稳定性仍需分别验收。

`pair` 为串口连接检查，`unpair` 提示串口无需配对，不清除 iOS App 的配对资料；`net` 仅展示串口状态，`net via/join/hotspot` 拒绝，已移除 PC 蓝牙驱动、依赖和真实网络控制回退。`internal/simboard` 及模拟会话代码仅供显式模拟器/离线测试，不能访问真实蓝牙。

## 回执、故障和互斥

沿用当前板已实测的文本协议：

```text
<递增请求号> abs 16384 16384\n
<同一请求号> ok\r\n
<递增请求号> scroll_by 1\n
<同一请求号> ok\r\n
0 status\n
0 {"fw":"0.2.5","abs":1,"usb":"ok",...}\r\n
```

开机日志、诊断行、其他编号回执不能完成当前请求；不靠静默窗口猜测完成。同一动作的定位、按压、等待、释放不可被其他输入插入。串口错误、短写、响应超时后结果未知，停止后续输入；先观察手机，必要时显式重启 daemon，不自动重放。PC 进程消失时固件有界按压到期释放；取消收尾另用独立时间预算尝试 reset 释放。固件报告其他控制端拥有设备时拒绝输入。

当前旧固件只有特殊 `0 status`，没有可靠唯一设备/boot ID。服务检测 uptime 回退并阻止后续输入，但它不是完整重启证明；端口号也不是设备身份。现阶段以显式端口绑定、串口独占和故障后人工检查为边界。下一阶段协议应增加独立启动标识、输出模式与连接代次；不能以 USB 状态代替 BLE HID readiness。

动作成功附加 `effect_verified=false`：表示命令得到固件回执，不表示手机已生效。`swipe` 拖物件可用与拖动页面不滚屏的实验结论保持分开。详见 [USB 验证记录](../hardware/hid_bridge/doc/usb-hid-validation.md)。

## 独立画面通道

默认复用现有 AirPlay HTTP 服务的 `/frame`，只读，不接入旧 Backend 控制路由。要求 HTTP 200、可解码 JPEG/PNG、`X-Frame-Id`、`X-Frame-Time`（Unix 秒）。超过 5 秒的有效截图仍可查看，返回 `capture.state=observation_only`、`after_action=false`，保留来源帧时间和 ID；重复 ID 不刷新时间。CLI JSON 与 MCP 截图文字均附带 capture 元数据，图像与元数据取自同一快照。

操作后首次截图最多等待 4 秒，要求真实采集时间晚于动作完成至少 300 ms；未等到时可以返回旧图供观察，不能冒充操作后的新证据。只有满足时间条件且年龄不超过 5 秒的帧才标记 `after_action_frame`。`effect_verified` 始终为 false，新帧本身不证明动作成功；视觉定位的严格新帧等待也不接受旧帧或失联后的缓存。

连接与画面年龄分别处理：来源不可达、204 无帧、错误 HTTP 状态或无效帧元数据仍报错，不用缓存掩盖来源故障。来源显式发送 `X-Mirroring-State: disconnected` 时报告投屏断开。现有服务没有可靠的手机连接事件，因此有效旧快照返回 `source_state=available`、`mirroring_state=unknown`，年龄超过五秒只标记 `freshness=unchanged_or_stale`，不推断手机已断开或仍连接。仅凭该服务保留的旧快照无法完全区分正常静止与未报告的断开。

HTTP 图像源支持原 screenshot 与 MJPEG 输出；不提供 H.264 与手机音频，相应 API 明确报不支持。现有 UxPlay/取帧继续运行，不在 CLI 内重启。新机器仍需单独准备画面服务；其独立迁移不属于本轮输入通道实现。

## 当前验收

2026-11-17 首轮：Go 回归 81 项通过、3 项因缺少 ffmpeg/外部模拟器而跳过，`go vet` 与 Windows 构建通过；正式 CLI `device info/net/devices` 和 MCP `initialize/device_info` 经 COM24 读到 fw 0.2.5、usb=ok、control_transport=serial。已交接临时串口桥。用户恢复投屏后，正式 CLI/MCP 的点击、双向滚屏、150 ms 按压、物件拖动及单次 --aim 通过网页可信事件和截图回归；英文键盘下精确 ASCII、退格与空格输入通过，已恢复原豆包输入法。详见 [首轮实机回归](../task/serial-control/live-validation.md)。静止图观察修复及后续验收见 [静止截图回归](../task/serial-control/static-frame-validation.md)。此前串口→USB HID 的既有实测继续有效。

本地证据位于工作区 `.runtime/serial-usb-validation/`；完整计划在 [实施计划](../task/serial-control/plan.md)。

当前包含静止截图修复的构建：`task/serial-control/esp32agent-static.exe`（忽略 Git），SHA-256 `FC03EADB9C2D89B28DEE23D37685F0133405047A0E90E22A70778E53B76FBA8A`；88 项测试通过、3 项跳过，go vet 与构建通过，CLI/MCP 静止截图实机复验通过。当前实机配置目录为工作区 `.runtime/serial-usb-cli/`，不要与其他配置目录同时占用 COM24。
