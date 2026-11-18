# PC 串口控制

本分支的正式链路为 **CLI/MCP → daemon → Serial Port → ESP32 → USB / BLE HID → iPhone**。基于 Henry Yuan 的 ESP32 iPhone Agent；保留原 MIT 许可证与署名，公开演示、视频和文章也应注明原项目。

USB HID 与 BLE HID 同时存在，每次只向显式选中的通路发送，沿用同一 CLI/MCP。PC 不再通过蓝牙连接开发板。本轮没有迁移旧 Python Backend；旧 iOS App 与 Python server 已移除；当前不需要安装本项目 App。

## Windows 构建与启动

在 `cli/` 中运行：

```powershell
go test ./...
go build -o esp32agent.exe ./cmd/esp32agent
$env:ESP32AGENT_PORT = 'COM24'
$env:ESP32AGENT_HID_OUTPUT = 'usb' # 默认 usb；也可设为 ble
$env:ESP32AGENT_FRAME_URL = 'http://127.0.0.1:8892/frame'
./esp32agent.exe device info
./esp32agent.exe screenshot --output screen.png
./esp32agent.exe mcp
```

COM24 只是当前实测连接。先用 `devices` 枚举并核对设备；枚举不打开端口，也不把所有串口冒充已验证的 ESP32。可用 `--device COM24` 覆盖环境变量。不指定端口时明确报错，避免连接其他开发板。Windows 打开端口时设置 115200、8N1、DTR=false、RTS=false，不主动复位。PC 始终通过独立 UART/USB 串口连接开发板并保证供电。USB HID 模式另用板子的原生 USB 接手机；BLE HID 模式不需要手机端 USB 线。不能将同一个原生 USB 口同时插入两个主机。

CLI 与 MCP 自动复用同一配置目录的 daemon。`ESP32AGENT_HOME` 可指定独立运行目录；同一硬件只能有一个拥有者，不同时运行临时桥、串口监视器或另一配置目录的 daemon。修改配置后先 `daemon stop` 再启动。遇到打开失败先检查占用，不自动杀进程或复位板子。

此版本是本地分支构建。npm、上游安装脚本和 `upgrade` 指向上游发布版本，不能用来安装或更新本串口实现。

## 固件与配对

PC控制使用 `hardware/serial_hid`。固件构建、共享动作的设计原因和USB引导配对见 [板端双HID说明](../../hardware/serial_hid/doc/usb-ble-hid.md)；原 `hardware/hid_bridge` 仅保留为历史固件及离线协议测试。使用BLE前先在手机完成目标设备配对并确认BLE就绪，电脑本身无需蓝牙配对。

新固件中的 USB/BLE 设备名为 `iPhone Agent HID`。命名变更只完成了源码修改和构建，尚未刷写；旧固件或手机缓存仍可能显示 `Handsy Serial HID` / `Handsy-BLE-Lab`。按已核实的设备身份操作，不因名称不同直接删除配对；`device info` 的 `device_id` 用于核对板子，显示名称不能代替设备身份。CLI 命令、MCP 工具和环境变量名称不随项目改名改变。项目目录及 AirPlay 的切换状态见[项目说明](../../README.md)。

## 输出选择与接线

切换需要明确配置，不由拔线自动触发。例如切到 BLE：

```powershell
./esp32agent.exe daemon stop
$env:ESP32AGENT_HID_OUTPUT = 'ble'
./esp32agent.exe device info
```

切回 USB 使用相同顺序并设为 `usb`。环境变量由启动 daemon 的进程继承；只修改客户端环境而复用旧 daemon 不会更换输出。运行中不会自动将 BLE 命令改走 USB，也不会在重连后补发被拒绝的动作。切换时发送的释放报告仅用于松开按键和按钮，不是旧动作重放。

| 状态 | 判定 |
|---|---|
| `control_transport=serial` | PC 到开发板始终为串口 |
| `hid_output=usb` | USB 业务输入要求 `usb=ok` |
| `hid_output=ble` | 要求 BLE 已连接、加密、全部输入报告已订阅、未挂起且无故障 |
| `usb=asleep` | USB 协议栈处于挂起；不是独立的物理插线检测 |
| `held=0` | 本地记录无按住的键或按钮；实际效果仍需看手机 |

本机手机端拔线后 `usb` 仍报告 `asleep`，BLE 输入仍可用。不要把它改写为 `off`，也不要用它否认物理拔线。BLE HID 的加密由蓝牙协议栈完成，配对后自动建立；无需另设应用层密码。BLE 连接代次改变或结果未知时，先观察手机，再显式恢复 daemon，失败动作不会排队等待重放。

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
| `device info` | `device_info` | 原固件状态加串口、真实 HID 输出及截图源字段 |

`scroll_by` 使用已经实测的固件实现，节奏、次数由板子控制，最大量耗时约 3.2 秒；正式串口命令预算 10 秒，长按为动作等待时间。固件不支持时返回明确错误，不改写成单发滚轮，也不切换所选 HID 输出。距离不是像素或固定屏幕比例。

`calibrate` 在串口路径报告原生绝对模式，不复用相对鼠标校准，也不伪造一次新校准验收。普通 tap 保留原来的默认直接执行语义；CLI `--aim` 使用绝对移动前后画面检查，不确认则阻止点击、无相对纠偏。参考 Backend 的 guard v9、完整校准审核和作用域门禁没有移植，不能宣称这些能力已由 CLI 等价实现。当前绝对映射采用已实验的 0..32767 坐标；全屏边缘、横屏、多设备及长期稳定性仍需分别验收。

`pair` 为 PC 串口连接检查，`unpair` 不清除手机 BLE HID 或 iOS App 配对资料；`net` 仅展示串口状态，`net via/join/hotspot` 拒绝，已移除 PC 蓝牙驱动、依赖和真实网络控制回退。`internal/simboard` 及模拟会话代码仅供显式模拟器/离线测试，不能访问真实蓝牙。

## 回执、故障和互斥

沿用当前板已实测的文本协议：

```text
<递增请求号> abs 16384 16384\n
<同一请求号> ok\r\n
<递增请求号> scroll_by 1\n
<同一请求号> ok\r\n
0 status\n
0 {"fw":"0.3.0-serial","abs":1,"hid_output":"ble","usb":"asleep","ble_ready":true,...}\r\n
```

开机日志、诊断行、其他编号回执不能完成当前请求；不靠静默窗口猜测完成。同一动作的定位、按压、等待、释放不可被其他输入插入。串口错误、短写、响应超时后结果未知，停止后续输入；先观察手机，必要时显式重启 daemon，不自动重放。PC 进程消失时固件有界按压到期释放；取消收尾另用独立时间预算尝试 reset 释放。固件报告其他控制端拥有设备时拒绝输入。

正式串口固件提供 device_id、boot_id、hid_output、connection_generation。daemon 固定本次打开时选定的输出，检测设备/启动标识改变、BLE 连接代次变化或输出被外部改变后停止输入；检查手机后显式重启 daemon，不自动重放。BLE readiness 必须同时满足连接、加密和所有输入报告订阅，USB 插着不能替代 BLE readiness。仍兼容旧 USB 固件：缺少输出字段按 USB 处理，重启只能按 uptime 回退检测，请求 BLE 则明确拒绝。

BLE 的同步 type 命令最多 16 字节，daemon 自动分块，避免超过串口命令时限；USB 保留既有分块。BLE 修饰键建立、字符与修饰键释放顺序采用实机验证的节奏。API 中的文本、坐标和按键语义保持不变。

动作成功附加 `effect_verified=false`：表示命令得到固件回执，不表示手机已生效。`swipe` 拖物件可用与拖动页面不滚屏的实验结论保持分开。详见 [USB 验证记录](../../hardware/hid_bridge/doc/usb-hid-validation.md)。

## 独立画面通道

默认复用现有 AirPlay HTTP 服务的 `/frame`，只读，不接入旧 Backend 控制路由。要求 HTTP 200、可解码 JPEG/PNG、`X-Frame-Id`、`X-Frame-Time`（Unix 秒）。超过 5 秒的有效截图仍可查看，返回 `capture.state=observation_only`、`after_action=false`，保留来源帧时间和 ID；重复 ID 不刷新时间。CLI JSON 与 MCP 截图文字均附带 capture 元数据，图像与元数据取自同一快照。

操作后首次截图最多等待 4 秒，要求真实采集时间晚于动作完成至少 300 ms；未等到时可以返回旧图供观察，不能冒充操作后的新证据。只有满足时间条件且年龄不超过 5 秒的帧才标记 `after_action_frame`。`effect_verified` 始终为 false，新帧本身不证明动作成功；视觉定位的严格新帧等待也不接受旧帧或失联后的缓存。

连接与画面年龄分别处理：来源不可达、204 无帧、错误 HTTP 状态或无效帧元数据仍报错，不用缓存掩盖来源故障。来源显式发送 `X-Mirroring-State: disconnected` 时报告投屏断开。现有服务没有可靠的手机连接事件，因此有效旧快照返回 `source_state=available`、`mirroring_state=unknown`，年龄超过五秒只标记 `freshness=unchanged_or_stale`，不推断手机已断开或仍连接。仅凭该服务保留的旧快照无法完全区分正常静止与未报告的断开。

HTTP 图像源支持原 screenshot 与 MJPEG 输出；不提供 H.264 与手机音频，相应 API 明确报不支持。现有 UxPlay/取帧继续运行，不在 CLI 内重启。新机器仍需单独准备画面服务；其独立迁移不属于本轮输入通道实现。

## 已验证范围

2026-11-17 的正式构建为 `hardware/serial_hid`（0.3.0-serial）及源码构建的 CLI/MCP。固件与原 USB 固件编译回归、Windows CLI 构建、`go vet` 均通过；Go 测试106项通过，3项因缺少 ffmpeg/外部模拟器跳过。计数包含测试子项，不代表全部面向新链路的覆盖率；未运行 race。

| 验收 | 已观察到的结果 |
|---|---|
| USB 与 BLE 控制 | 正式 CLI/MCP 点击、双向滚轮；BLE 多点/重复点定位、ASCII/Shift/空格/退格、20字符分块、有界按压和物件拖动 |
| USB 引导配对 | 仅操作目标 Handsy HID，USB 自动完成配对、取消配对及重新配对，无人工点击手机 |
| BLE 断连拒绝 | 正式 CLI/MCP 及固件直接命令拒绝输入；USB 可用时也没有自动回退 |
| 重连后无重放 | 重连及选路后的静置窗口无迟到动作，新的 probe/CLI/MCP 点击各生效一次 |
| 手机端 USB 拔线 | PC 串口保留，纯 BLE 完成定位、点击、滚轮、键盘、按压和拖动；测试期 USB 调用128不变，BLE报告214→325 |
| 静止截图 | CLI/MCP 返回超过8秒的原帧时间/ID，标记 observation_only、after_action=false，没有伪造操作后新帧 |

本机任务目录中的测试网页是本地验收工具，用于观察可信 DOM 事件、计数和文字；不是正式控制服务的运行依赖，日常控制手机 App 不需要启动它。正式固件和 daemon 不依赖 task/lab，冻结的原型仅保存实验依据。

构建哈希、故障诊断、实机步骤与边界统一保存在 [板端验收记录](../../hardware/serial_hid/doc/validation.md)。原始截图、串口日志、实验计划和完整Flash保存在本机，不是正式运行或构建依赖，也不纳入本次提交。

最后一次实机验收保持 BLE 模式、原输入法、held=0，正式 daemon 与投屏服务继续运行，临时桥及测试页服务器已停止；运行配置目录为工作区 `.runtime/serial-usb-cli/`。目录名称来自USB首轮，不决定当前输出。下一次启动仍须显式设置 `ESP32AGENT_HID_OUTPUT=ble`。

尚未覆盖动作发送中途断链、全屏边缘、物理旋转、锁屏、长期稳定性及跨设备/系统完整兼容性；不能把有界实测扩大成上述保证。
