# iPhone Use 画面服务

独立 Windows AirPlay 取帧服务，不包含串口控制、旧 Backend 或 iOS App。CLI/MCP 继续从 `ESP32AGENT_FRAME_URL` 获取截图，默认 `http://127.0.0.1:8892/frame`。

从项目根执行：

```powershell
./scripts/setup.ps1
./scripts/capture.ps1 start -UxPlayDir D:/path/to/uxplay-windows -Receiver attach
./scripts/capture.ps1 status
./scripts/capture.ps1 stop
```

若本机 PowerShell 策略禁止脚本，可直接调用已安装模块（无需修改系统执行策略）：

```powershell
.venv/Scripts/python.exe -m iphone_use_capture.control start --runtime "$PWD/.runtime/capture" --uxplay-dir D:/path/to/uxplay-windows --receiver attach
.venv/Scripts/python.exe -m iphone_use_capture.control status --runtime "$PWD/.runtime/capture"
.venv/Scripts/python.exe -m iphone_use_capture.control stop --runtime "$PWD/.runtime/capture"
```

`attach` 适用于已有接收器已配置 `\\.\pipe\hidagent-frames`、I420 496×1080 输出的环境。不改配置、不启动或重启 UxPlay。首次部署使用 `-Receiver start`：接收器不存在时生成配置并启动；已经运行时仍保留原会话。登录启动也使用此模式。名称默认 `iPhone Use`，UxPlay 配置中以 `iPhone-Use` 显示；已有进程不会即时改名。

先停用已确认的旧采集器再启动本服务；命名管道与 HTTP 端口冲突均明确失败，不能运行两份采集器。`stop` 只终止通过 PID、创建时间及模块命令行验证的本服务进程，绝不终止接收器或整个进程树。停止采集器会断开其视频管道；本机实测 UxPlay 随后自行退出，需要重新启动接收器并在手机重新投屏；控制 daemon 的重启没有这个影响。

UxPlay 外部依赖须包含现有 `uxplay-windows.exe`、引擎及 GStreamer 运行库。冷启动需要允许接收器写自身配置和网络发现。安装 Python 环境不会安装或升级 UxPlay，也不会刷写固件。

HTTP 仅监听本机：`GET /health` 报告进程归属、是否有快照和管道连接状态；`GET /frame` 返回 JPEG 或 204。没有控制动作路由。HTTP 健康不代表手机正在投屏，`pipe_connected` 也不等于手机连接状态。

验证入口：`.venv/Scripts/python.exe -m unittest discover -s capture/tests -v`。

需要登录启动时，执行 `./scripts/install-capture-startup.ps1 -UxPlayDir D:/path/to/uxplay-windows`。它只创建 `iPhone Use Capture.lnk`，不自动删除其他启动项。迁移旧项目时需另行核实并停用旧 Backend 启动项，避免下次登录争用8892/视频管道。

[设计与迁移约束](doc/design.md)。取帧核心来自旧本地 BLE 项目，保留 [MIT 许可证](LICENSE)；主项目上游来源和许可见根 README、LICENSE、NOTICE。
