# 测试归属与迁移清单

2026-11-18，实施中。删除门槛：新的网页离线/浏览器检查、串口契约与正式网页实机矩阵通过后才删除候选。此清单不是“已删除”声明；原 iOS App、Python server、App harness/prompts 已由2913c06清理，与本轮测试整理分开。

| 现有断言/位置（仓库相对路径） | 处置及替代依据 |
|---|---|
| cli/internal/seriallink/serial_test.go | 保留：字节分片、编号匹配、日志过滤、短写、超时、输入校验与无重放 |
| cli/internal/seriallink/ownership_test.go | 新增：排队取消不发送；发送后取消、迟到成功不能解除故障或重放 |
| cli/internal/daemon/serial_test.go、serial_output_test.go | 保留：绝对动作、整动作互斥、释放、USB/BLE选路与会话身份门禁 |
| cli/internal/daemon/serial_ownership_test.go | 新增：环境/显式端口、默认USB、并发唯一拥有者、占用失败不猜端口或回退 |
| cli/internal/app/serial_test.go | 保留：CLI/MCP请求形状、参数提前拒绝 |
| cli/internal/app/serial_e2e_test.go | 新增：真实CLI与MCP JSON-RPC→daemon socket→seriallink→假串口；USB/BLE、分片回执、拒绝、文本分块、绝对点击、pair/unpair及废弃网络入口 |
| cli/internal/daemon/wifi_test.go | 删除候选：TestSessionMovesToWiFiAndBack；由串口独占/拒绝网络及不回退测试替代，不再要求自动切Wi-Fi |
| cli/internal/daemon/server_test.go | 旧配对窗口、密钥和自动重连断言删除候选；owner拒绝、截图来源错误、参数/封装等有效断言先保留到串口用例 |
| cli/internal/daemon/record_test.go | 迁移：动作历史、键盘节奏、汉字转拼音有效；改用串口夹具，移除相对归位/旧配对前提 |
| cli/internal/daemon/listen_test.go | 分拆：旧App听写联调废弃，串口路径应明确拒绝phone_audio；纯音频转换断言若仍有消费者独立保留 |
| cli/internal/app/app_test.go | 保留输出格式、参数、MCP握手/工具schema；旧sim配对端到端用例的截图、文本、日志有效断言迁移后再移除 |
| cli/internal/daemon/observation_test.go、logs_test.go | 保留：真实帧时间、来源失联、静止旧图、后续帧及通用日志解析 |
| cli/internal/proto、simboard、hardware/tests/sim及CI启动步骤 | 必须核对非测试消费者后清理；当前daemon/app仍引用模拟传输，不能先删目录造成编译失败。旧PC协议专用断言不计为串口覆盖；若需解除运行代码依赖，列明最小变更及回归范围，不能隐蔽恢复两套PC控制 |
| hardware/shared_hid、hardware/tests/Makefile/test_main.c | 保留共用算法及固件纯C断言，Host C使用mbedTLS3和ASan/UBSan；不当作iPhone效果证明 |
| CLI其他history/intent/memory/pointer/图像测试 | 保留公共功能；不得因整文件夹具来自旧协议而连带删除 |

假串口验证通信协议和软件独占；不证明Windows实际DTR/RTS电平、不证明设备枚举或手机HID效果。实际输出隔离、物理拔线和输入行为由新页面实测。当前串口打开配置仍为115200/8N1、DTR=false/RTS=false，不为测试改动。

本轮旧测试和专用辅助代码尚未删除。迁移验收完成后在此更新最终删除/保留清单及理由，并更新CI入口；未测必需项不能标记完成。

本轮新页面实机发现的阻塞项：English (US) 下 USB `text 1a_A-1` 实际为 `1a-a-1`；显式 `keys shift+a`、`keys shift+-` 和单字符 `text A` 也未产生预期修饰效果。页面可信键盘事件与码点均吻合失败结果，不能只按串口成功回执计通过。旧 USB 验收覆盖了小写、空格和退格，没有证明此 Shift 用例；BLE 历史同字符串通过也不能替代 USB。用户明确本轮只记录问题，不修改固件，保留旧测试及专用辅助代码；清理门槛未满足，后续修复需单独验证。

后续进展（2026-11-18）：[iPhone Use 独立环境迁移回归](../../../../doc/dev/reference-project-retirement.md)已补验USB/BLE主要输入、静止帧和BLE断连/重配闭环。此前“BLE未恢复”并非当前状态，精确Shift文本本轮也通过；仍不据此宣称偶发问题根治。物理USB拔线、多点返回及完整入口交叉组合仍需补齐，候选文件尚未删除。
