# 串口双 HID 输出设计与构建

本文归属板端固件：维护报告、连接状态或动作逻辑时读取。PC配置及CLI/MCP参数见 [串口控制](../../../cli/doc/serial-control.md)，实机证据和未覆盖范围见 [验收记录](validation.md)。

## 为什么共用动作引擎

用户要求保留成熟USB控制方式，并让BLE模拟同样的键盘和鼠标。PC到ESP32仍为串口；新增的是ESP32到iPhone的标准BLE HID输出，不恢复电脑蓝牙命令服务，也不迁入参考Backend控制架构。

USB/BLE共用仓库中的 `hardware/shared_hid/actions.h`、`abs_mouse.h` 和pace实现，避免两条通路分别修改定位、点击及滚轮算法。原 `hid_bridge` 仅保留为历史固件及离线协议测试，默认AbsMouse输出仍为USB；串口入口通过发送回调选择输出。正式代码不依赖任务原型。

实测中合并位置/按钮报告没有产生完整点击，拆开后得到可信按下、松开和点击事件。因此位置、按钮和滚轮保持独立，切路不重发缓存位置。BLE Report Map直接由USB对象注册的描述符组成：1键盘8字节、4 Consumer 2字节、10绝对位置5字节、11按钮1字节、12滚轮4字节。BLE特征值不带Report ID前缀，ID由Report Reference标识。

键盘保留USB原生行为；BLE在同一接口下采用实测的先建立修饰键、后发送字符、按顺序释放字符与修饰键的节奏。每条BLE type限制16字节，由daemon拆分，避免同步处理越过串口命令时限。兼容动作语义不等于两个传输有相同延迟。

USB接口由首个`USBHID`对象决定，后创建的键盘对象不会覆盖它。`0.3.1-serial`将首个通用发送器显式声明为KEYBOARD协议，保留键盘报告内容、原生USB发送时序、BLE时序及动作API。此项接口修正的实验与正式回归见[USB Shift调查](usb-keyboard-shift.md)。

2026-11-18正式测试页首轮发现USB大写、下划线及显式Shift失败，最初按用户范围只记录。后续获准调查后，同一故障也在BLE复现；重连后原生方式又恢复。因此不能用早期USB基本ASCII验收证明Shift一直正常，也不能把200ms延迟或接口声明单独称为间歇性故障的已证实根因。首轮记录见[正式页面验收](../../tests/hid_acceptance/doc/validation.md)，后续结论以本模块调查为准。

## 单路发送与恢复

上电选择USB。`output usb` / `output ble` 明确切路，有按住状态时拒绝；正常切换释放旧路由，再释放新路由。两接口可以同时连接，但业务命令只发一路。松键/松按钮的零报告用于清理输入状态，不是补发旧动作。

BLE就绪必须满足连接、加密、全部输入报告订阅及未挂起；连接成功本身不等于可以操作。BLE HID的加密由蓝牙协议栈承担，没有另建应用层加密协议。BLE连接代次改变或报告失败后停止后续输入，不自动重放或回退USB，防止点击在新的页面被重复执行。有限press_for即使PC消失也到期释放；串口发送结果未知时仍要检查手机，不能用本地释放状态冒充实际效果。

串口沿用数字请求号+文本行，回执匹配同一编号。device_id、boot_id和connection_generation分别区分设备、启动及BLE连接；端口名不是永久身份。USBcalls统计库调用，BLEreports统计成功通知，它们可辅助检查路由隔离，不能直接比较吞吐或证明手机效果。

`usb`字段只反映TinyUSB挂载/挂起状态。手机端拔线时本机仍报告asleep；它不是独立的物理插线检测，不能据此要求改成off。BLE readiness独立判断。

## 构建与首次配对

正式入口是 `hardware/serial_hid`，共享 `hardware/shared_hid/actions.h`、`abs_mouse.h` 和 pace 模块；旧 `hid_bridge` 仅保留历史固件与协议测试，不是当前部署入口。新入口只有串口命令输入和标准 BLE HID 服务，不提供 PC 蓝牙命令服务。默认 USB；BLE 广播名为 `Handsy Serial HID`。沿用实验稳定地址与相同报告布局，已有手机可能缓存旧名称 `Handsy-BLE-Lab`。

已验证的依赖为 ESP32 Arduino 3.3.12、NimBLE-Arduino 2.3.8。在仓库根构建：

```powershell
./hardware/build-serial.ps1 -ToolsDir <Arduino工具目录> -NimBLE <NimBLE-Arduino-2.3.8目录> -OutputDir <输出目录>
```

ToolsDir 内为 arduino-cli/arduino-cli.exe、arduino-cli.yaml、arduino-data 与 arduino-downloads；脚本接受安装位置，不依赖实验目录或参考项目源码。当前目标为 ESP32-S3、16 MB Flash、OPI PSRAM。脚本只构建，不能把默认分区表写入已有设备：部署前核对板子和实际分区，取得有效备份；本机当前 app0 为 0x10000、上限 0x330000，仅写应用并独立校验，保留 NVS/bond 与 OTA 数据。上游 flash.sh/网页安装器不安装此串口双 HID 固件。

首次配对：以 USB 控制 iPhone，打开设置 → 辅助功能 → 触控 → 辅助触控 → 设备 → 蓝牙设备，选择本设备并确认系统配对弹窗；然后等 BLE 全部订阅及加密完成。只管理已核实的本设备。普通蓝牙设置列表不一定展示相同条目。USB 完成配对后，停止 daemon，设置 `ESP32AGENT_HID_OUTPUT=ble`，再运行原 CLI 或 MCP；切回 USB 同样先停止 daemon 再改为 usb。双接口可共存，但一个命令不会同时发送两路。

PnP采用开发身份，不声称商业设备身份。配对失败先保存实际状态；不能只凭手机列表“已连接”判断成功。
