# 共享 HID 动作模块

USB 与 BLE 需要相同的坐标映射、独立按钮报告和滚轮节奏，因此共用 actions.h、abs_mouse.h、pace.h/c。此模块从旧 hid_bridge 中原样迁出，避免正式串口固件依赖旧 App 架构。发送回调选择 USB 或 BLE，动作语义保持一致。

串口入口在 [serial_hid](../../serial_hid/README.md)，实测依据见 [验收记录](../../serial_hid/doc/validation.md)。旧 hid_bridge 对应源文件仅作为 include 适配层，让历史固件和离线协议测试继续使用同一实现。Arduino 分别通过各 sketch 的编译入口编译 pace.c，不在两个位置复制实现。

清理 iOS App、旧 server、App harness 和提示词不会删除独立 AirPlay 接收/取帧服务；CLI 仍从 ESP32AGENT_FRAME_URL 获取画面。旧 App 源码可从 Git 历史追溯，清理前未提交内容另有本机备份。

Arduino 会将旧 sketch 的 src 子目录复制到临时目录，因此适配层不能依赖相对路径回溯到仓库。旧 flash.sh 为 C/C++ 显式加入 hardware/ 头文件搜索路径，host Makefile 同样加入该路径；自定义旧固件构建必须带上对应 -I 参数。串口入口的直接共享引用已单独构建验证。

## 清理后的回归（2026-11-17）

CLI 106项通过、3项因外部依赖跳过，go vet 与 Windows 构建通过。串口双 HID 和旧 hid_bridge 固件均构建通过；后者先复现 Arduino 临时目录相对引用失败，再验证显式 include 路径修复。共享动作实现未变，本轮不刷机或重做实机验收，既有实机边界仍见串口模块验收文档。

CI YAML、模块文档链接、插件图标及 diff 空白检查通过。Host C 测试随后在 Ubuntu 22.04 WSL 的隔离快照通过，启用 AddressSanitizer/UndefinedBehaviorSanitizer，输出 all tests passed。系统 mbedTLS 2.28 与测试使用的 3.x API 不兼容；从官方 release 构建独立的 mbedTLS 3.6.2，使用 make MBEDTLS=<隔离前缀>，未替换系统依赖。原始结果保存在本机工作区 `.runtime/remove-legacy-app-20261117/test-results.json`。

Host C 补测日志：本机 `.runtime/remove-legacy-app-20261117/host-c-test-final.log`。测试工具和依赖仅存在本地，不属于运行时依赖。
