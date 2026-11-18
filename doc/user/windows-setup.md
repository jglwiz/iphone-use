# Windows 主项目环境

在 `iphone-use/` 中使用 Python 3.11+、Go 以及独立 Arduino CLI 工具目录。旧 BLE 项目的 `.venv`、Backend 和 `.build-deps` 不作为部署依赖。

```powershell
./scripts/setup.ps1
# 已缓存依赖时可以显式指定归档；脚本仍会校验 SHA256。
# ./scripts/setup.ps1 -NimBLEArchive D:/cache/nimble-2.3.8.zip
```

脚本建立本项目 `.venv`（不继承系统 site-packages），安装取帧及测试工具，并准备 `.runtime/deps/NimBLE-Arduino-2.3.8`。库源为 h2zero/NimBLE-Arduino 的2.3.8发布标签归档，校验值固定在脚本中；校验失败时停止。Python 直接依赖使用迁移时已验证的版本。环境安装不启动服务、不刷板。

Arduino CLI 可使用共享工具目录，但 ESP32 core 固定3.3.12。本机共享工具位于工作区 `.tools`，不是旧 BLE 项目的子目录。在已安装该 core 的工具目录上构建：

```powershell
./hardware/build-serial.ps1 -ToolsDir D:/path/to/tools `
  -NimBLE "$PWD/.runtime/deps/NimBLE-Arduino-2.3.8" `
  -OutputDir "$PWD/.runtime/firmware"
Push-Location cli
go build -o ../.runtime/bin/esp32agent.exe ./cmd/esp32agent
Pop-Location
```

该固件脚本只构建。烧录必须另外核实目标板、真实分区、配对状态和备份，不能拿构建默认分区直接覆盖整片 Flash。

画面服务配置与启动见 [capture 使用说明](../../capture/README.md)，控制服务配置见 [串口控制](../../cli/doc/serial-control.md)。UxPlay 仍是外部依赖，其安装目录需包含原有引擎和运行库。日常通过 CLI/MCP 控制；不启动旧 Backend 的 `/action` 服务。

迁移环境的检查：

```powershell
.venv/Scripts/python.exe -m unittest discover -s capture/tests -v
.venv/Scripts/python.exe -m unittest discover -s hardware/tests/hid_acceptance -p test_server.py -v
Push-Location cli
go test ./...
Pop-Location
```

本机首次迁移的真实链路证据和尚未完成事项，以 [迁移验收](../dev/reference-project-retirement.md) 为准。
