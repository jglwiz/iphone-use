# Handsy HID 验收

USB/BLE 共用的本地测试页面和事件记录服务。仅观察浏览器输入，不打开串口、不发送手机控制指令，也不参与日常控制服务。

```powershell
python hardware/tests/hid_acceptance/serve.py --host <PC局域网IP> --port 8905 --output <新的证据目录>
python -m unittest discover -s hardware/tests/hid_acceptance -p test_server.py -v
```

运行需要 Python 3.10+，仅使用标准库，无 CDN、前端构建或旧 Backend 依赖。证据目录必须尚不存在；端口占用时明确失败。手机通过同一局域网访问启动时打印的 URL。

- [使用、用例及证据判定](doc/usage.md)
- [测试归属与迁移清单](doc/test-migration.md)
- [正式页面验收结果与已知问题](doc/validation.md)
- [串口控制配置](../../../cli/doc/serial-control.md)

基于本项目早期 usb_probe.py 验收页整理。Based on ESP32 iPhone Agent by Henry Yuan, MIT；保留仓库 LICENSE 和 NOTICE。网页正式化的验收状态单独记录，不继承旧页面实机通过结论。
