# 揭东一初联机大厅 服务端 API 文档（v1.0.0）

所有请求/响应均为 UTF-8 JSON（`Content-Type: application/json`）。
鉴权方式：登录后拿 JWT，请求头 `Authorization: Bearer <token>`。

## REST

### POST /api/register
请求：`{username, password, nickname, class_name, real_name, student_no?}`
- username：4-20 位字母数字下划线；password：8-64 位且含字母和数字
- class_name：≤20 字；real_name：2-4 个汉字；student_no：选填
- 201 `{"ok":true}`；409 `{"error":"username_exists",...}`；400 `{"error":"invalid_params",...}`
- 注册后状态 pending，需管理员审核通过才能建房/进房

### POST /api/login
请求：`{username, password}`
- 200 `{"token":"<JWT>","user":{id,username,nickname,class_name,role,status,reject_reason}}`
- 同一 IP 每分钟最多 5 次，超出返回 429 `{"error":"rate_limited",...}`

### GET /api/me（鉴权）
- 200 返回当前用户对象（同 login.user）

### GET /api/rooms（鉴权）
- 200 `{"rooms":[{id,name,game,mode,host_nickname,host_class,members,max_members,created_at,frp_addr?}]}`
- frp_addr 仅 frp 房有，形如 `jdyc.example.com:20001`（游戏内直连地址）

### POST /api/rooms（鉴权，须 approved）
请求：
- 公共：`{name, game, mode, max_members?}`；game ∈ minecraft|terraria|cs16|yuzu|custom；mode ∈ frp|vlan；max_members 默认 20
- frp 追加：`local_port`（本机游戏端口）、`port_type`（tcp|udp|both）
- 200 `{"room":{...同列表行, "frp":{...} 或 "vlan":{...}}}`
- 403 `{"error":"not_approved",...}`（未审核通过）

frp 对象：`{frpc_server, frpc_port, frpc_token, proxy_name, remote_port, local_port, port_type}`
vlan 对象：`{network_community, vlan_key, my_virt_ip, host_virt_ip, supernode}`
- vlan 子网规则：`25.10.<房间ID%250+1>.0/24`；密钥为 16 位随机 hex；房主固定 `.1`，成员从 `.2` 起分配，上限 `.254`

### DELETE /api/rooms/{id}（鉴权，仅房主）
- 200 `{"ok":true}`；403 `{"error":"not_host",...}`；404 `{"error":"room_not_found",...}`

## WebSocket

### GET /lobby?token=<jwt>
- 连接成功后服务端立刻下发 `{"type":"room_list","rooms":[...]}`（全量）
- 之后事件增量下发：

| type | 载荷 | 说明 |
|---|---|---|
| room_list | rooms[] | 房间变化时重发全量快照 |
| room_created | room{} | 新房 |
| room_removed | room_id | 房关闭（房主离开/关服/清扫） |
| room_joined | room_id, room_name, mode, host_nickname, host_class, vlan{...} 或 frp{...}, members[] | 加入成功（frp/vlan 对象同 REST） |
| member_left | room_id, uid, nickname | 成员退出 |
| room_full | room_id | 房满拒入 |
| error | code, msg | 如 not_approved |

客户端→服务端：
- `{"type":"join_room","room_id":<id>}`（校验顺序：approved → 房间存在 → 未满）
- `{"type":"leave_room","room_id":<id>}`（房主发=关房）

心跳：服务端每 15 秒发 `{"type":"ping"}`，客户端回 `{"type":"pong"}`；30 秒无消息断开。
清扫：每 JDYC_SWEEP_SEC（默认 30）秒扫描，房主 WS 断开超过 JDYC_HOST_DOWN_SEC（默认 60）秒 → 删房、释放端口、广播 room_removed。（JDYC_HOST_DOWN_SEC 为测试便利新增的可配置项，见 TEST-REPORT 遗留问题。）

## 管理后台（HTML 表单，浏览器操作）

- GET/POST `/admin/login`：管理员账号登录（Session Cookie：jdyc_admin_session）
- GET `/admin/review`：待审核用户列表
- POST `/admin/review/{uid}/approve`：通过
- POST `/admin/review/{uid}/reject`（表单 reason）：拒绝并记录原因
- GET `/admin/rooms`：房间总览
- POST `/admin/rooms/{id}/close`：强制关房
- 命令行建管理员：`./jdyc-hub-server -createadmin 用户名 密码 班级 姓名`
