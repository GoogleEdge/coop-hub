# 揭东一初联机大厅 部署文档（Linux 服务器）

## 一键 Docker Compose（推荐）

前置：服务器装有 Docker；域名 A 记录指向服务器 IP；80/443 空闲（caddy 自动签 TLS）。

1. 改配置：把占位符全部替换成真实值（对照 `config.example.toml`）：
   - `server/deploy/docker-compose.yml` 里的 `JDYC_JWT_SECRET`、`JDYC_FRPS_TOKEN`（两处要一致）、`JDYC_ADMIN_PASS_PEPPER`、域名
2. 启动：
   ```
   cd server/deploy
   docker compose up -d
   ```
3. 三个服务：
   - `frps`：FRP 服务端（7000 控制端口 + 20000-29999 玩家转发段，host 网络直通）
   - `server`：大厅服务端（7100）
   - `caddy`：反向代理 7100，自动 HTTPS
4. 建管理员：
   ```
   docker compose exec server jdyc-hub-server -createadmin 用户名 密码 班级 姓名
   ```
5. 验证：浏览器开 `https://你的域名/admin/login` 能登录即 OK。

## n2n supernode（vlan 模式）

服务端进程启动时会自动在 PATH 中找 `supernode` 并拉起（监听 7777）。
宿主机安装 n2n（编译或发行版包）确保 `which supernode` 可用即可；compose 内未包含。
找不到时只打日志告警，不影响 frp 模式。

## 裸机 systemd（不用 Docker）

1. 把 Release 里的 `jdyc-hub-server-linux-amd64` 放到 `/opt/jdyc/`
2. 安装 sqlite 依赖无需额外操作（CGO 静态编译）
3. 单元文件：`server/deploy/jdyc-server.service`，改好环境变量后：
   ```
   cp jdyc-server.service /etc/systemd/system/
   systemctl daemon-reload && systemctl enable --now jdyc-server
   ```
4. 反代建议 caddy：`caddy reverse-proxy --from jdyc.example.com --to 127.0.0.1:7100`

## 环境变量一览

| 变量 | 默认 | 说明 |
|---|---|---|
| JDYC_LISTEN | :7100 | HTTP 监听地址 |
| JDYC_DB | jdyc.db | SQLite 路径（放 /data 卷） |
| JDYC_JWT_SECRET | 随机 | JWT 签名密钥（多副本部署必须显式设置） |
| JDYC_FRPS_HOST | 127.0.0.1 | 玩家连的 frps 公网地址 |
| JDYC_FRPS_PORT | 7000 | frps 控制端口 |
| JDYC_FRPS_TOKEN | 空 | frps 认证 token（与 frps 启动参数一致） |
| JDYC_SUPERNODE | 127.0.0.1:7777 | 下发给客户端的 supernode 地址 |
| JDYC_ADMIN_PASS_PEPPER | 空 | 密码哈希 pepper |
| JDYC_SWEEP_SEC | 30 | 房间清扫周期（秒） |
| JDYC_HOST_DOWN_SEC | 60 | 房主断线判离阈值（秒，测试可调短） |

## 数据备份

只有一份数据：`/data/jdyc.db`（SQLite，WAL 模式）。直接 `sqlite3 jdyc.db ".backup 'bak.db'"` 热备。

## 免责声明

本项目为校内教学联机工具，仅供学习交流。使用 GitHub 提供 CDN/CI 属正常开源协作；请遵守微软服务条款，勿用于任何违反当地法规的用途。
