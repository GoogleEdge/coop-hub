# 揭东一初联机大厅（jdyc-hub）

<!-- TODO：仓库地址待定，推送后替换 YOUR_GITHUB_OWNER 并打开 badge
[![server](https://github.com/YOUR_GITHUB_OWNER/jdyc-hub/actions/workflows/ci-server.yml/badge.svg)]()
[![clients](https://github.com/YOUR_GITHUB_OWNER/jdyc-hub/actions/workflows/ci-clients.yml/badge.svg)]()
-->

校内 Minecraft / 泰拉瑞亚 / CS1.6 等游戏的联机大厅：注册审核实名（班级+姓名）、两种联机模式（FRP 端口转发 / n2n 虚拟局域网）、双 Windows 客户端（Win7 专用版 + Win10/11 主流版），服务端单二进制，全部产物由 GitHub Actions 公共仓库构建。

## 目录结构

```
jdyc-hub/
├── .github/workflows/   ci-server.yml / ci-clients.yml / release.yml
├── server/              Go 1.21 服务端（REST+WebSocket+管理后台+集成测试）
│   └── deploy/          Dockerfile / docker-compose.yml / systemd 单元
├── core/                jdyc-core.dll（C ABI，子进程管理 + TAP 检测安装）
├── client-qt/           Win7 专用版（Qt 5.15 QML + /MT + windeployqt）
├── client-flutter/      主流版（Flutter stable Windows desktop）
├── installer/           NSIS 脚本 ×2
├── assets-src/          图标生成脚本（Pillow，CI 运行）
├── docs/                API.md / DEPLOY.md / TEST-REPORT.md
├── config.example.toml  所有域名/密钥占位符集中于此
└── README.md
```

## 快速开始

**服务端**（详见 [docs/DEPLOY.md](docs/DEPLOY.md)）：
```
cd server/deploy && docker compose up -d
docker compose exec server jdyc-hub-server -createadmin 用户名 密码 班级 姓名
```

**客户端**：Release 页下载 `JDYC-Hub-Win7-v1.0.0.exe`（Win7 SP1 x64）或 `JDYC-Hub-Win10-v1.0.0.exe`（Win8.1/10/11 x64），安装后登录、注册待审核。

**玩法**：登录 → 大厅建房（frp 填本机游戏端口；vlan 自动进虚拟局域网）→ 成员从房间页复制地址直接连。

## 开发

```
server:      cd server && go test ./...        # 集成：go test -tags=integration
core:        cmake -S core -B core/build && cmake --build core/build
client-qt:   cmake -S client-qt -B client-qt/build（CI 内完成）
client-flutter: flutter pub get && flutter build windows（CI 内完成）
```

推送 tag `v*` 自动发 Release。域名字段一律走 `config.example.toml` 占位符，禁止硬编码。

## 免责声明

本项目为校内教学联机工具，仅供学习交流；请遵守微软服务条款与当地法规。
