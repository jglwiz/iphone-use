# Contributing

本 fork 维护 PC CLI/MCP → 串口 → ESP32 → USB/BLE HID 通路。无需 iOS App 或旧 Python server。

修改 CLI 后在 `cli/` 运行 `go test ./...`、`go vet ./...` 和 `go build ./cmd/esp32agent`。串口固件使用 [构建脚本](hardware/build-serial.ps1)；工具版本和部署边界见 [模块说明](hardware/serial_hid/doc/usb-ble-hid.md)。CI 同时保留旧固件及离线协议测试，防止共享 HID 代码影响这些消费者。

画面服务是独立依赖；测试不要重启现有投屏服务。不要提交密钥、设备配置、Flash 备份和运行日志。公开源码检查使用 `scripts/check-public.sh`。

原项目 MIT 版权和署名必须保留。`hardware/hid_bridge/src/airplay/` 是 GPL-3.0 代码，修改该部分仍须遵守其许可证。推送和创建 PR 前需维护者明确同意。

Host C 回归入口是 `make -C hardware/tests`，默认启用 ASan/UBSan，需要 C 编译器及 mbedTLS 3.x。若系统版本不同，可用 `make -C hardware/tests MBEDTLS=/path/to/mbedtls3` 指向独立安装前缀，不必替换系统库。
