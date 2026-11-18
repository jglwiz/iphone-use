# 旧 BLE 参考项目退役

2026-11-18，运行入口与自启动已切换，迁移范围内的实机回归通过。唯一维护项目已命名 `iphone-use`；历史 `handsy-upstream` 路径仅用于解释旧证据。旧 `esp32-iphone-agent-windows-ble-hid` 保留源码和记录，退出运行、构建和测试依赖。

## 已迁移边界

主项目一直使用串口 CLI/MCP 和双 HID 固件。本轮补齐独立 Python 环境与 NimBLE 缓存，把已验证的 AirPlay 快照/命名管道模块迁入 `capture/`，不迁入旧动作 API、校准或串口 worker。新模块保留旧项目 MIT 许可；外部 UxPlay 不并入仓库。

只读盘点确认旧服务为 manager/API/capture 三组 Python 进程，8892 转发8891快照；用户 Startup 的 `iPhone Backend.lnk` 是发现的直接自启动入口。任务计划、Run 注册表和公共 Startup 未发现直接匹配，不能据此排除未知间接脚本。

旧 API 的基线故障为 `Timeout on hello; outcome unknown; not replayed`，队列为空、没有活动动作。新 CLI 连接 COM24，USB/BLE ready且held=0。不能调用旧 API 重放控制或让其旧协议参与新链路。

## 验证与状态

- 独立 `.venv`：Python3.11.10，无 system-site-packages。
- 离线：取帧8项（含进程身份保护）、正式网页9项、Go全包测试通过，Windows CLI和ESP32固件构建通过。
- 固件构建使用ESP32 Arduino3.3.12和主项目自己的NimBLE2.3.8缓存。本轮未刷写板子。
- 真实切换已完成：旧 manager/API/capture 进程退出，8891/8894无监听，8892由主项目capture直接提供。新服务PID63464，UxPlay PID84048（仅本次证据标识，不可用于后续硬编码停止）。
- 旧视频管道关闭导致原UxPlay自行退出；主代理没有直接杀接收器，但切换确实中断了画面。新服务启动iPhone-Use接收器，用户手动重选投屏后恢复。一次USB点击尝试被CLI无帧门禁拒绝，未发送输入；没有伪造截图或绕过门禁。
- USB正向页面：CLI/MCP点击、双向滚动、`1a_A-1A_A`、空格退格、150ms按压及拖动通过。BLE同组键盘与空格退格、点击/滚动、150ms按压和MCP拖动也通过；BLE窗口USBcalls=214不变，BLEreports15→115、held=0。静止帧和BLE断连恢复矩阵通过。
- BLE断连负向：系统没有单设备断开按钮，因此仅忘记iPhone Use HID；显式BLE的CLI tap/scroll和MCP tap均拒绝，USBcalls280、BLEreports124以及页面ack293/count6/scrollY0不变。
- USB重新配对同一目标后，状态connected/encrypted/ready恢复、订阅7186，BLEreports仍124；切回BLE产生中性释放报告后，静置6秒仍reports127、USBcalls305、页面count6。仅新发的一次点击使count6→7、BLEreports127→130，USBcalls305不变、held=0，未观察到旧命令重放。
- 控制daemon重启切换USB/BLE后，新采集器与UxPlay PID均保持不变。
- 静止实机：CLI/MCP均返回原frame_id `a6a957878b104d9c9e79f2b60382ec97:3216`、frame_time `1794973921.426`，帧龄约17秒，`observation_only`、`after_action=false`。初次等待仍有系统画面变化产生真实新帧，没有把那次误算为静止通过。
- 旧用户Startup `iPhone Backend.lnk` 已核实目标、备份后移除，替换为 `iPhone Use Capture.lnk`，仅指向主项目`.venv`和capture模块。未重启Windows验证登录；新服务实际冷启动与人工恢复投屏已完成。

工作区本地证据在 `.runtime/iphone-use-migration/`；任务清单在本项目忽略的 `task/serial-control/deprecate-reference-project-plan.md`。这些本机产物不作为可分发运行依赖。

## 保留的问题与清理边界

键盘 Shift 偶发失败根因仍未知；构造器接口修正有通过证据但不等于根治。BLE连接/加密成功但订阅为零的恢复问题也不能由此次迁移宣称解决。见 [键盘调查](../../hardware/serial_hid/doc/usb-keyboard-shift.md)。

旧PC通信测试与专用辅助代码的删除仍受 [测试迁移清单](../../hardware/tests/hid_acceptance/doc/test-migration.md) 的覆盖门槛约束。退役参考项目不意味着立即删除共享HID、Host C测试、历史固件或未完成替代的断言。本轮没有进行手机端USB物理拔线、多点返回和每一路/每个入口的完整交叉组合验收；旧专用测试仍保留，清理仍由原测试整理任务继续推进。

## 回退与保留

本机旧快捷方式、接收器arguments/rc配置和服务身份保存在工作区 `.runtime/iphone-use-migration/cutover/`，不含管理token；原旧项目配置未删除。需要明确回退时先停止新capture，恢复原接收器配置，再用旧项目原部署入口启动，避免两套同时占用8892/管道；手机可能需要重新选择旧接收器。正常使用不执行回退入口。

实机正向证据使用正式网页run `b7114957db3a408897d7bf01703da1b7`、page `d5d9b6b011ab4a4798ad48e3edc3d6ed`；USB文本为 `1a_A-1A_A`，BLE追加同样字符串。可信事件、截图与通道计数互相核对，ack/HTTP成功不单独作为效果证明。具体证据为工作区 `live/usb-text-page.json`、`ble-text-page.json`、`ble-drag-page.json`（位于迁移证据目录内）。

本轮代码未改变CLI/MCP控制API或HID动作/报告；固件只重新构建，没有刷写。新增的是独立取帧模块、环境/生命周期/登录启动工具、固件依赖版本检查和Windows取帧CI任务。GitHub CI尚未远程运行；本机对应8项测试已通过。实现验收后，用户另行授权仅提交本次迁移及停用相关内容；已有无关修改和暂存项保留，不推送。

## 收尾

最终已恢复豆包输入法（主代理复核菜单高亮）和USB输出，USB/BLE均ready、订阅7186、held=0，固件仍为0.3.1-serial；正式daemon保留。本轮8906测试服务已核实身份并停止，独立capture和UxPlay保持运行，8892/frame返回200，8891/8894无监听。最终状态见本机证据 `live/final-status.json`。

构建版本门禁加入后固件缓存构建通过；capture最终8项通过，CI YAML结构可解析，12份文档43条本地链接存在，Git空白检查通过。未执行Windows注销/重启登录验收，不把快捷方式目标核对当作登录实测。
