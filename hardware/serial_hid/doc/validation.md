# 串口 USB / BLE HID 验收

日期：2026-11-17。正式入口为 `hardware/serial_hid`，版本 `0.3.0-serial`；PC 始终通过串口连接 ESP32，显式选择 USB HID 或标准 BLE HID 向 iPhone 输出。CLI/MCP 动作 API 不变。配置与操作语义见 [串口控制说明](../../../cli/doc/serial-control.md)。

后续的`0.3.1-serial`接口修正、Shift复发及对照证据单独记录于[USB Shift调查](usb-keyboard-shift.md)。下文保留11月17日的局部验收事实，不作为以后每次连接的输入保证。

## 实现与测试范围

先完成独立原型，再整合正式代码。USB/BLE 共用 `shared_hid/actions.h`、`abs_mouse.h` 和 pace；BLE Report Map 直接由 USB 对象的描述符组成，不另建坐标或点击算法。实读 Report Map 为191字节，输入报告为：键盘1/8字节、Consumer 4/2字节、绝对位置10/5字节、独立按钮11/1字节、滚轮12/4字节。

使用 COM24 的 ESP32-S3 revision 0.2、16 MB Flash、8 MB OPI PSRAM；原生 USB 接手机，独立 USB 串口接 PC。工具链为 ESP32 Arduino 3.3.12，串口双 HID 使用 NimBLE-Arduino 2.3.8；原 `hid_bridge` 另用既有 NimBLE 2.5.1 构建回归。测试只读复用现有 AirPlay `/frame`，没有调用参考 Backend 输入接口，也没有重启 UxPlay。

所有手机操作由测试执行代理完成，主代理实现代码、设计用例并复核截图、原始回执和网页事件。测试网页记录 `isTrusted` 输入事件；操作截图保留来源 `frame_time`、`frame_id`。回执、订阅和报告计数是辅助证据，不能单独证明手机效果。

## 正式构建与自动化

| 项目 | 结果 |
|---|---|
| 双 HID 固件 | 构建通过，应用镜像669696字节 |
| 原 USB 固件 | ABS_POINTER=1、HID_TRACE=1及既有nimble_cfg配置编译通过 |
| Go | 106项通过、3项因缺少ffmpeg/外部模拟器跳过；含测试子项，不等于新链路覆盖率 |
| 静态检查与客户端 | go vet、Windows CGO_ENABLED=0构建通过；未运行race |
| 提交前独立快照 | 同样106项通过、3项跳过，vet通过；导出快照无Git元数据，使用-buildvcs=false构建通过 |

自动化覆盖BLE不依赖USB就绪、USB可用不能替代BLE连接/加密/订阅、身份/启动/输出/连接代次变化阻止后续输入、旧USB固件拒绝BLE选择、非法配置拒绝及20字符文本按16+4拆分。PC蓝牙命令通道没有恢复。

实机使用的固件SHA-256为 `739458ed9e7f4d71e944abbc5ef0d825f4a5b55283e13930c81484ad269727cc`；CLI为 `6de9c8fdcd34540532b905ced524079833bdf0acb854bc5ee1849d3b9d11e2a8`。提交前仅清理文档及文件尾空白，运行逻辑未变。

## USB 引导配对与共存

USB自动完成设置 → 辅助功能 → 触控 → 辅助触控 → 设备 → 蓝牙设备中的目标设备配对、取消配对和重新配对，无需用户点击手机；没有删除无关配对。普通蓝牙列表未稳定显示名称，改用 [Apple 指针设备入口](https://support.apple.com/en-hk/111775)。

原型名为 `Handsy-BLE-Lab`，正式名为 `Handsy Serial HID`。正式固件保留稳定地址和相同报告布局，保留NVS后手机自动重连、加密及全报告订阅恢复，bond_count=1，无需重新配对。USB和BLE可同时连接，每条业务输入只走选中的一路。

## 正式 CLI/MCP 实机结果

| 用例 | 实际结果 |
|---|---|
| USB点击 | CLI/MCP使网页计数2→3→4，可信pointerup、buttons=0 |
| USB双向滚动 | scrollY 0→637→11 |
| BLE定位 | 屏内(150,200)→(350,700)→(150,200)，实际箭头正向返回 |
| BLE点击与滚动 | CLI/MCP计数4→5→6，scrollY 11→638→11 |
| BLE键盘 | English (US)下精确1a_A-1、空格U+0020、退格；20位文本12345678901234567890完整进入输入框 |
| BLE有限按压 | 150ms请求，正式回归观察到149ms down/up，buttons=0、held=0 |
| BLE拖动 | 物件CSS(140,306)→(56,319)，可信pointerup且buttons=0 |
| 静止截图 | CLI/MCP返回同一原始帧时间和ID；年龄约8.12/8.24秒，observation_only、after_action=false |

上述BLE阶段报告数3→136、USB调用保持35。两个计数层次不同：USB是库调用数，BLE是成功通知数；用于确认通路隔离，不用于比较吞吐。滚轮单位不是像素，双向滚动不保证恰好抵消。文本框会横向裁切显示，20位完整文本由可信事件确认。

## BLE 断连负向与重连

1. USB仅对明确的Handsy设备执行手机“忽略此设备”，然后回同一本地测试页，指针移至计数按钮。BLE连接/加密/ready=false、订阅0、连接代次2，USB仍可用。板端bond_count=1只是保留的密钥记录，不表示连接。
2. 显式选择BLE后，基线USB调用86、BLE报告188。正式CLI tap、MCP tap、CLI scroll各一次均报 `phone BLE HID is not ready; no USB fallback`。各自等待后页面计数和滚动仍0、无新输入事件；两路计数不变，输出仍BLE。
3. 交接串口给实验桥，直接向同一正式固件发送一次带编号click，返回 `err BLE HID not ready (connection/encryption/subscriptions)`。5秒后页面及计数仍不变，证明固件层同样拒绝。
4. 明确选USB并重新配对。连接代次升到3，加密和全部订阅恢复；同一boot_id，未重启。保持USB输出观察约5.25秒，BLE188不变，没有补发。
5. 返回同一网页后显式选BLE，仅产生3个松键/松按钮报告，188→191；再静置5秒不变且无点击。新发一次click使计数0→1、BLE191→193、USB121不变；交回正式daemon后，CLI/MCP新tap各使计数1→2→3，USB仍121，按钮释放。

手机列表曾显示“已连接”，而板端仍断开；返回上级再进入刷新并完成新配对后，才按加密、订阅和实际效果确认恢复。没有把显示文字当作成功证据。此用例验证执行前已经断连及显式恢复，不是动作正在发送时中途断链的测试。

## 手机端 USB 拔线

用户确认线仍插在ESP32端，另一端已与iPhone断开；PC串口保留。固件报告usb=asleep，而不是off。该字段由TinyUSB挂载/挂起状态生成，不是独立的物理插线检测；保留原值，未据此否认用户的接线确认或修改固件伪造off。

明确选择BLE后，基线USB调用128、BLE报告214；纯BLE通过以下实测：

- 两个屏内绝对点及返回同点，CLI/MCP点击计数3→4→5。
- CLI/MCP双向滚轮，scrollY 0→647→21。
- 150ms按压实测159ms，计数5→6、buttons=0；拖动物件CSS(43,277)→(120.578125,289.84375)，最终held=0。
- English键盘下精确1a_A-1，MCP空格及CLI退格符合可信文本事件。

整个切路后观察期USB调用128不变，BLE报告214→325。结束恢复豆包中文输入法，保留正式BLE daemon、投屏服务，停止临时测试页。默认上电模式仍是USB；后续启动daemon需明确配置 `ESP32AGENT_HID_OUTPUT=ble`。

## 诊断经验、恢复依据与边界

原型早期PnP ID为空，后续补齐7字节开发身份；广播名称及配对确认时序也有调整。最终连接成功不能单独归因于其中一项。NimBLE-Arduino 2.3.8的getScanData返回广播数据，不能用它证明空中扫描响应；设备广播另有PC只读扫描证据，PC未连接或配对，扫描不是控制链路。系统配对弹窗有时限，看到明确目标后应及时完成确认，不在中间插入长时间轮询。

部署前读取当前16 MiB有效Flash，SHA-256 `551b297dea925f4b8ee92e607b5389140136b60f5dc3469c28b4a3268b9975a5`，不能使用早期全FF备份。实际OTA记录指向app0 0x10000、上限0x330000；仅写应用并独立verify_flash，保留bootloader、分区表、NVS和OTA数据。地址只适用于已核对的本板，不能直接用于其他设备。

原始截图、串口回执、DOM事件、备份与构建日志仅在本机工作区 `.runtime/serial-ble-validation/`，断连补测位于其 `disconnect-negative/`，手机拔线补测位于 `usb-unplugged/`，提交快照检查位于 `commit-review/`。冻结原型和详细过程保留本地任务目录，不是正式构建依赖；原始运行产物不入库。

尚未覆盖全屏边缘、物理旋转、锁屏、长期稳定性、动作中途断链、Unicode以及跨设备/系统/应用完整兼容性。静止帧保留真实时间，不能冒充操作后新证据；来源未报告连接事件时，旧快照也不能证明投屏仍连通或已经断开。
