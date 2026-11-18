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

正式入口是 `hardware/serial_hid`，共享 `hardware/shared_hid/actions.h`、`abs_mouse.h` 和 pace 模块；旧 `hid_bridge` 仅保留历史固件与协议测试，不是当前部署入口。新入口只有串口命令输入和标准 BLE HID 服务，不提供 PC 蓝牙命令服务。默认 USB；USB 产品名、BLE 广播名与状态中的设备名称统一为 `iPhone Use HID`，由 `HID_DEVICE_NAME` 常量维护。沿用实验稳定地址与相同报告布局，新名称需刷写新固件后生效；已有手机可能缓存旧名称 `iPhone Agent HID`、`Handsy Serial HID` 或 `Handsy-BLE-Lab`。本次命名不改变设备地址、配对存储及 HID 报告布局。

已验证的依赖为 ESP32 Arduino 3.3.12、NimBLE-Arduino 2.3.8。在仓库根构建：

```powershell
./hardware/build-serial.ps1 -ToolsDir <Arduino工具目录> -NimBLE <NimBLE-Arduino-2.3.8目录> -OutputDir <输出目录>
```

ToolsDir 内为 arduino-cli/arduino-cli.exe、arduino-cli.yaml、arduino-data 与 arduino-downloads；脚本接受安装位置，不依赖实验目录或参考项目源码。当前目标为 ESP32-S3、16 MB Flash、OPI PSRAM。脚本只构建，不能把默认分区表写入已有设备：部署前核对板子和实际分区，取得有效备份；本机当前 app0 为 0x10000、上限 0x330000，仅写应用并独立校验，保留 NVS/bond 与 OTA 数据。上游 flash.sh/网页安装器不安装此串口双 HID 固件。

首次配对：以 USB 控制 iPhone，打开设置 → 辅助功能 → 触控 → 辅助触控 → 设备 → 蓝牙设备，选择本设备并确认系统配对弹窗；然后等 BLE 全部订阅及加密完成。只管理已核实的本设备。普通蓝牙设置列表不一定展示相同条目。USB 完成配对后，停止 daemon，设置 `ESP32AGENT_HID_OUTPUT=ble`，再运行原 CLI 或 MCP；切回 USB 同样先停止 daemon 再改为 usb。双接口可共存，但一个命令不会同时发送两路。

PnP采用开发身份，不声称商业设备身份。配对失败先保存实际状态；不能只凭手机列表“已连接”判断成功。


## 设备命名变更的验证边界

2026-11-18将产品名称统一为 `iPhone Agent HID`，BLE Manufacturer 字符串改为 `iPhone Agent`。USB 产品名、BLE 初始化／广播名及状态返回使用同一常量，避免界面名称与诊断信息不一致。上游版权及许可证保留，历史验收中的 Handsy 名称不回写成新名称。

本次使用上述 ESP32 Arduino 3.3.12 与 NimBLE-Arduino 2.3.8 构建通过，差异空白检查通过。产物为669696字节，SHA256 `2a4f8ae892fd84ebf1f647b7089f21d4815d86a7c3465330d8fd280d513af2ca`；本地构建目录为工作区 `.runtime/iphone-agent-naming-build/`。版本字段仍为 `0.3.1-serial`，因此区分此前实机验证镜像与命名变更镜像时应核对哈希，不能只看版本号。

初次命名提交只完成构建，未部署。后续部署记录见下文。其他设备部署后仍需核对串口状态中的名称和设备身份、实际 BLE 广播／手机显示，以及 USB/BLE 基本控制。若手机仍显示旧名，先区分旧固件与名称缓存；不为改显示名称自动清除配对。AirPlay 命名属于外部接收服务配置，不由本固件控制。


### 2026-11-18中间名称 iPhone Agent 的部署与重新配对

用户明确要求删除旧 BLE 设备、重新烧录并重新连接。通过正式 CLI 的 USB 输出在 iPhone 辅助触控蓝牙列表中“忽略”目标 `Handsy Serial HID`，随后板端确认 BLE 断开。只操作该 HID 配对，没有修改其他设备或关闭手机蓝牙。

核对 COM24 芯片 MAC `cc:68:c7:b0:85:18`、16 MB Flash，读取当前分区表及 OTA 数据，与既有完整备份一致；核验备份及上述新镜像哈希后，仅写 `app0`（0x10000），独立 `verify-flash` digest 匹配。NVS、分区表、OTA 数据保留。新启动 `2577771f3bb42a3d`，设备身份 `cc68c7b08519` 不变，状态与广播数据均为 `iPhone Agent HID`。

USB 控制完成新的配对弹窗确认。最初蓝牙列表及弹窗仍显示缓存旧名；重新配对并再次进入列表后，实际显示 `iPhone Agent HID`、已连接。无需改变 BLE 地址或重启手机蓝牙。最终 BLE 已连接、加密、订阅7186、ready=true、bond_count=1、held=0。显式切换到 BLE 后点击进入蓝牙列表成功，BLE 报告3→6，USB 调用27不变；当前保留 BLE 输出，USB 仍就绪，原 AirPlay 未重启。该检查是命名部署后的基本连接／点击验证，不代替完整键盘、滚动或稳定性回归。

本机证据位于工作区 `.runtime/iphone-agent-rename-deploy/`：`write.log`、`verify.log`、`flashed-status.json`、`paired-ready.json`、`ble-before.json`、`ble-after.json`、`24-final.png` 及 `final-frame.json`。名称变更未改变 CLI/MCP API，本地仓库目录已改为 `iphone-agent/`；AirPlay 服务名称尚未切换。


### 最终名称 iPhone Use 与再次部署

用户最终选定 **iPhone Use**，项目目录为 `iphone-use/`，USB/BLE 名称为 `iPhone Use HID`，BLE Manufacturer 为 `iPhone Use`。名称表达供 AI 调用的手机操作能力，ESP32 保留为实现说明；CLI 可执行命令 `esp32agent`、MCP 工具、控制参数与环境变量均不变。前文 iPhone Agent 镜像和截图是中间阶段证据，不能当作最终名称的验证结果。

本地目录改名后，已核实的旧测试服务 `task/usb-hid-drag/lab/probe.py` 从新路径启动，192.168.100.10:8902 返回 HTTP 200；Git 历史和原有未提交／暂存状态保留，远程仓库未改名。AirPlay 的目标名称为 `iPhone Use`，当前外部服务仍为 `iPhone-Agent`，本次未重启或迁移它。

使用相同工具链构建通过，镜像669696字节，SHA256 `0b2af972603d75df8f437d9591b090bc5a48f74e7c5dfeff6685a3dd5e9afc11`。先用 USB 在手机上删除中间名称的 BLE 配对，核对同一芯片、分区表、OTA 选择和恢复备份，仅烧录 app0 后独立校验通过。设备身份及 BLE 地址不变，新 boot 为 `09e03dc8fdbdd2b0`。再次通过 USB 确认配对，手机列表从缓存旧名刷新为 `iPhone Use HID`、已连接。

最终 BLE 连接、加密和全部订阅7186就绪，held=0。BLE 点击打开蓝牙列表成功，报告3→6、USB调用21不变。当前保留 BLE 输出，USB 同时就绪，AirPlay 返回新帧。该轮只验证改名后的构建、部署、配对和基本点击，未重复完整键盘／滚动回归。

本地证据：工作区 `.runtime/iphone-use-deploy/` 内的 `build.json`、`partition-read.log`、`write.log`、`verify.log`、`flashed.json`、`paired.json`、`ble-before.json`、`ble-after.json`、`14-final.png` 和 `final-frame.json`；构建日志为 `.runtime/iphone-use-build.log`。历史证据路径保留原名，不批量重写。
