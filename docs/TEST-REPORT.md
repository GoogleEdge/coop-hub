# 测试报告（v1.0.0）

> 说明：本地 Agent 环境无 GitHub 仓库凭据，本轮交付以「代码全部就绪 + 本地可验证项全部通过」为准。
> push 到 GitHub 后，任务书执行顺序里的「push → 查 Actions → 修到绿」循环尚未执行，下表"CI 已验证项"以本地等价验证为准，push 后以 Actions 结果为准更新本报告。

## CI 已验证项（本地等价验证）

| 项 | 验证方式 | 结果 |
|---|---|---|
| 服务端单元测试（校验器/端口分配/vlan子网/限速/密码哈希） | `go test ./...` 本地 Go 1.27 | ✅ 9 项全过 |
| 服务端静态检查 | `go vet ./...` | ✅ 通过 |
| 服务端构建 | `go build`（CGO+sqlite3） | ✅ 成功 |
| `-createadmin` 建管理员 | 本地运行 | ✅ 成功 |
| 注册 → 管理员审核 → 登录 JWT 链路 | 本地起服 + curl | ✅ 全链路通 |
| 登录限速（5次/分/IP） | 单测 TestRateLimiter | ✅ |
| frp 端口分配 20000-29999 + used_ports 落库 | 单测 TestPortAllocation | ✅ |
| vlan 子网 25.10.x.0/24 + 16 位 hex 密钥 | 单测 TestVlanSubnetFor/TestRandHexKey/TestCreateRoomAndVlan | ✅ |
| jdyc-core 三个 C 源文件语法 | gcc -fsyntax-only（桩 windows.h） | ✅ 通过 |
| 集成测试脚本本身 | 编译通过（`go vet` 覆盖） | ✅；**实际断言需 CI 跑**（依赖 docker frps） |
| Qt 客户端 / Flutter 客户端 / NSIS | 无本地工具链，未验证 | ⏳ 待 CI |

集成测试断言项（integration_test.go，对应任务书 3.1 ①-⑥）：
① 建管理员→后台登录→审核通过 ✅已写 ② 登录 JWT ✅ ③ frp 房端口+落库 ✅ ④ vlan 子网+密钥 ✅ ⑤ WS room_list/room_joined 全字段 ✅ ⑥ 房主断线→清扫→room_removed ✅（用 JDYC_SWEEP_SEC=2 / JDYC_HOST_DOWN_SEC=3 缩短验证）

## 需用户真机验证项

| 项 | 说明 |
|---|---|
| Win7 裸机安装 | JDYC-Hub-Win7-v1.0.0.exe 安装、UAC 提权、桌面快捷方式 |
| Win7 Qt 客户端启动 | Qt 5.15 DLL 齐全（windeployqt 产物）、QML 渲染正常 |
| 实际 MC 联机（frp 模式） | 建房→房主开本地服→成员 `域名:2xxxx` 直连 |
| 实际联机（vlan 模式） | edge 连 supernode 成功、TAP 虚拟网卡拿到 25.10.x.x、游戏局域网发现 |
| TAP 驱动安装 UAC | 静默 /S 安装弹 UAC，装后 tap0901 出现 |
| Win10/11 主流版 | Flutter 安装包同样流程 |

## 遗留问题（未定义情况记录，不做猜测实现）

1. **install-qt-action 可能缺 QML 模块**：`jurplel/install-qt-action` 默认只装 qtbase；本 Qt 版 UI 需要 qtdeclarative（Quick/QML）与 qtwebsockets。若 CI 报 `find_package(Qt5Quick)` 失败，按任务书"报缺包则最小调整"精神，在 install-qt-action 的 `modules:` 里追加 `qtdeclarative qtwebsockets`（唯一改动）。
2. **JDYC_HOST_DOWN_SEC 为新增可配置项**：任务书 ⑥ 要求"房主 WS 断开 60 秒后清理，用缩短的心跳间隔配置验证"，60 秒阈值本身需可缩短才能快速测试，故新增该环境变量（默认 60，行为不变），已同步写入 docs/API.md。
3. **图标资产未被客户端 job 下载**：任务书第 10 章要求"生成物被 Qt/Flutter job 下载后放入各自 assets"，但第 3.2 节给的两个客户端 job 并无 download-artifact 步骤。按"工作流逐字写入禁止改动"优先，本方案把图标（app.ico）直接提交进仓库供 .rc/Runner 引用，CI 生成物仅用于文档/发布说明。
4. **release.yml 跨 run artifact**：按第 3.3 节备注，若 GITHUB_TOKEN 无法跨 run 下载，需按备注改为 workflow_dispatch + 内联编译（任务书明确允许的备选）。
5. **frp 最新版 toml 字段**：frpc TOML 模板按 serverAddr/serverPort/auth.token 写定；若最新 frp 改字段名，按第 13 章对照该版 README 仅改拼写。
6. **n2n 3.1.1 编译**：若 MSYS2 下 autogen/configure 失败，按第 13 章依次试 3.1.0、3.0.1，仍失败改用 ntop 官方 release 的 Windows edge.exe。
