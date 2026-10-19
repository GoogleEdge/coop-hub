package main

// WebSocket 大厅：/lobby?token=<jwt>
// 服务端每 15 秒发 ping，客户端须回 pong；30 秒无任何消息断开。
// 每 JDYC_SWEEP_SEC 秒扫描：房主 WS 断开超过 JDYC_HOST_DOWN_SEC（默认 60）秒 → 删房、释放端口、广播 room_removed。

import (
        "encoding/json"
        "log"
        "net/http"
        "sync"
        "time"

        "github.com/gorilla/websocket"
)

const (
        pingInterval = 15 * time.Second
        pongTimeout  = 30 * time.Second
        writeTimeout = 5 * time.Second
        defaultSweep = 30 // 秒
        defaultDown  = 60 // 秒，房主断线判离阈值
)

var upgrader = websocket.Upgrader{
        ReadBufferSize:  1024,
        WriteBufferSize: 1024,
        CheckOrigin:     func(r *http.Request) bool { return true },
}

// Client 一个已登录的 WS 连接
type Client struct {
        uid      int64
        nickname string
        conn     *websocket.Conn
        send     chan []byte
        hub      *Hub
        closeCh  chan struct{}
}

// Hub 连接与房间运行期状态
type Hub struct {
        mu      sync.Mutex
        clients map[int64]*Client        // uid → 连接（同 uid 顶号）
        rt      map[int64]*RoomRuntime   // roomID → frp 运行参数
        down    map[int64]time.Time      // roomID → 房主断线起始时间（0 值未断）
        app     *App
}

func NewHub() *Hub {
        return &Hub{
                clients: map[int64]*Client{},
                rt:      map[int64]*RoomRuntime{},
                down:    map[int64]time.Time{},
        }
}

// SetRuntime 记录 frp 房运行参数
func (h *Hub) SetRuntime(roomID int64, rt *RoomRuntime) {
        h.mu.Lock()
        h.rt[roomID] = rt
        h.mu.Unlock()
}

// ClearRuntime 房间关闭时清理
func (h *Hub) ClearRuntime(roomID int64) {
        h.mu.Lock()
        delete(h.rt, roomID)
        delete(h.down, roomID)
        h.mu.Unlock()
}

// Runtime 取 frp 运行参数
func (h *Hub) Runtime(roomID int64) *RoomRuntime {
        h.mu.Lock()
        defer h.mu.Unlock()
        return h.rt[roomID]
}

// Broadcast 全体在线客户端广播
func (h *Hub) Broadcast(msg any) {
        b, err := json.Marshal(msg)
        if err != nil {
                return
        }
        h.mu.Lock()
        defer h.mu.Unlock()
        for _, c := range h.clients {
                c.trySend(b)
        }
}

// trySend 非阻塞投递，慢消费者直接踢下线
func (c *Client) trySend(b []byte) {
        select {
        case c.send <- b:
        default:
                go c.close()
        }
}

func (c *Client) close() {
        select {
        case <-c.closeCh:
        default:
                close(c.closeCh)
        }
}

// handleLobby GET /lobby
func (a *App) handleLobby(w http.ResponseWriter, r *http.Request) {
        token := r.URL.Query().Get("token")
        claims, err := ParseJWT(token, a.cfg.JWTSecret)
        if err != nil {
                writeErr(w, http.StatusUnauthorized, "unauthorized", "登录状态无效")
                return
        }
        u, err := a.store.GetUserByID(claims.UID)
        if err != nil || u == nil {
                writeErr(w, http.StatusUnauthorized, "unauthorized", "用户不存在")
                return
        }
        conn, err := upgrader.Upgrade(w, r, nil)
        if err != nil {
                return
        }
        c := &Client{
                uid:      u.ID,
                nickname: u.Nickname,
                conn:     conn,
                send:     make(chan []byte, 64),
                hub:      a.hub,
                closeCh:  make(chan struct{}),
        }
        // 顶号：同 uid 旧连接踢下线
        a.hub.mu.Lock()
        if old, ok := a.hub.clients[u.ID]; ok {
                old.trySend([]byte(`{"type":"error","code":"replaced","msg":"账号在其他地方登录"}`))
                go old.close()
        }
        a.hub.clients[u.ID] = c
        // 房主重新上线，清除其房间的断线计时
        for _, room := range a.mustHostRooms(u.ID) {
                delete(a.hub.down, room.ID)
        }
        a.hub.mu.Unlock()

        // 连接后全量房间列表
        a.sendRoomList(c)

        go c.writePump()
        go c.readPump()
}

// mustHostRooms 调用方需持 hub.mu；查询房主房间
func (h *Hub) mustHostRooms(uid int64) []*Room {
        rooms, err := h.app.store.HostRooms(uid)
        if err != nil {
                return nil
        }
        return rooms
}

// sendRoomList 向单个客户端推送房间列表
func (a *App) sendRoomList(c *Client) {
        rooms, err := a.store.ListRooms()
        if err != nil {
                return
        }
        list := []map[string]any{}
        for _, r := range rooms {
                list = append(list, a.roomJSON(r))
        }
        b, _ := json.Marshal(map[string]any{"type": "room_list", "rooms": list})
        c.trySend(b)
}

// broadcastRoomList 全体推送房间列表（成员数变化等增量用）
func (a *App) broadcastRoomList() {
        rooms, err := a.store.ListRooms()
        if err != nil {
                return
        }
        list := []map[string]any{}
        for _, r := range rooms {
                list = append(list, a.roomJSON(r))
        }
        a.hub.Broadcast(map[string]any{"type": "room_list", "rooms": list})
}

// writePump 出站：ping 定时器 + send 队列
func (c *Client) writePump() {
        ticker := time.NewTicker(pingInterval)
        defer ticker.Stop()
        for {
                select {
                case <-c.closeCh:
                        c.conn.Close()
                        return
                case b := <-c.send:
                        c.conn.SetWriteDeadline(time.Now().Add(writeTimeout))
                        if err := c.conn.WriteMessage(websocket.TextMessage, b); err != nil {
                                c.close()
                        }
                case <-ticker.C:
                        c.conn.SetWriteDeadline(time.Now().Add(writeTimeout))
                        if err := c.conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"ping"}`)); err != nil {
                                c.close()
                        }
                }
        }
}

// readPump 入站：pong / join_room / leave_room，30 秒无消息断开
func (c *Client) readPump() {
        defer c.hub.handleDisconnect(c)
        c.conn.SetReadLimit(4096)
        _ = c.conn.SetReadDeadline(time.Now().Add(pongTimeout))
        for {
                _, raw, err := c.conn.ReadMessage()
                if err != nil {
                        return
                }
                _ = c.conn.SetReadDeadline(time.Now().Add(pongTimeout))
                var msg struct {
                        Type   string `json:"type"`
                        RoomID int64  `json:"room_id"`
                }
                if err := json.Unmarshal(raw, &msg); err != nil {
                        continue
                }
                switch msg.Type {
                case "pong":
                        // 保活，无动作
                case "join_room":
                        c.hub.app.handleJoinRoom(c, msg.RoomID)
                case "leave_room":
                        c.hub.app.handleLeaveRoom(c)
                default:
                        c.trySend([]byte(`{"type":"error","code":"bad_request","msg":"未知消息类型"}`))
                }
        }
}

// handleJoinRoom 加入房间：approved → 房间存在 → 未满 → 分配 IP → room_joined
func (a *App) handleJoinRoom(c *Client, roomID int64) {
        u, err := a.store.GetUserByID(c.uid)
        if err != nil || u == nil {
                c.trySend([]byte(`{"type":"error","code":"unauthorized","msg":"登录状态无效"}`))
                return
        }
        if u.Status != "approved" {
                c.trySend([]byte(`{"type":"error","code":"not_approved","msg":"账号未通过实名审核"}`))
                return
        }
        room, err := a.store.GetRoom(roomID)
        if err != nil || room == nil {
                c.trySend([]byte(`{"type":"error","code":"room_not_found","msg":"房间不存在"}`))
                return
        }
        ip, joined, err := a.store.JoinMember(room, c.uid)
        if err == errRoomFull {
                b, _ := json.Marshal(map[string]any{"type": "room_full", "room_id": room.ID, "msg": "房间已满"})
                c.trySend(b)
                return
        }
        if err != nil {
                c.trySend([]byte(`{"type":"error","code":"server_error","msg":"加入失败"}`))
                return
        }
        // room_joined：携带 frp 或 vlan 完整字段
        msg := map[string]any{
                "type":          "room_joined",
                "room_id":       room.ID,
                "name":          room.Name,
                "game":          room.Game,
                "mode":          room.Mode,
                "host_nickname": room.HostNickname,
                "host_class":    room.HostClass,
                "host_uid":      room.HostUID,
        }
        if room.Mode == "frp" {
                rt := a.hub.Runtime(room.ID)
                if rt == nil {
                        rt = &RoomRuntime{}
                }
                msg["frp"] = frpDetail(room, a.cfg, rt)
        } else {
                msg["vlan"] = vlanDetail(room, a.cfg, ip)
                msg["members"] = a.memberList(room.ID)
        }
        b, _ := json.Marshal(msg)
        c.trySend(b)
        if joined {
                a.broadcastRoomList()
        }
}

// memberList vlan 房成员（昵称 + 虚拟 IP）
func (a *App) memberList(roomID int64) []map[string]any {
        members, err := a.store.ListMembers(roomID)
        if err != nil {
                return []map[string]any{}
        }
        out := []map[string]any{}
        for _, m := range members {
                out = append(out, map[string]any{
                        "uid":      m.UID,
                        "nickname": m.Nickname,
                        "virt_ip":  m.VirtIP,
                })
        }
        return out
}

// handleLeaveRoom 离开房间；房主离开即关房
func (a *App) handleLeaveRoom(c *Client) {
        rooms, _ := a.store.HostRooms(c.uid)
        for _, room := range rooms {
                // 房主发 leave_room → 关房
                a.closeRoom(room)
        }
        // 非房主身份：退出其所在房间
        allRooms, _ := a.store.ListRooms()
        for _, room := range allRooms {
                if room.HostUID == c.uid {
                        continue
                }
                members, _ := a.store.ListMembers(room.ID)
                for _, m := range members {
                        if m.UID == c.uid {
                                a.store.LeaveMember(room.ID, c.uid)
                                a.hub.Broadcast(map[string]any{
                                        "type": "member_left", "room_id": room.ID, "uid": c.uid, "nickname": c.nickname,
                                })
                                a.broadcastRoomList()
                        }
                }
        }
}

// handleDisconnect 连接断开：移除客户端；非房主成员即时退房；房主等待清扫
func (h *Hub) handleDisconnect(c *Client) {
        h.mu.Lock()
        if cur, ok := h.clients[c.uid]; ok && cur == c {
                delete(h.clients, c.uid)
        }
        hostRooms := h.app.mustHostRooms(c.uid)
        isHost := false
        for _, room := range hostRooms {
                isHost = true
                // 记录房主断线时刻，由清扫协程判定是否关房
                if _, ok := h.down[room.ID]; !ok {
                        h.down[room.ID] = time.Now()
                }
        }
        h.mu.Unlock()

        if isHost {
                return // 房间断线由 sweep 统一处理
        }
        // 非房主：即时退出所在房间并广播
        allRooms, _ := h.app.store.ListRooms()
        for _, room := range allRooms {
                members, _ := h.app.store.ListMembers(room.ID)
                for _, m := range members {
                        if m.UID == c.uid {
                                h.app.store.LeaveMember(room.ID, c.uid)
                                h.Broadcast(map[string]any{
                                        "type": "member_left", "room_id": room.ID, "uid": c.uid, "nickname": c.nickname,
                                })
                                h.app.broadcastRoomList()
                        }
                }
        }
}

// mustHostRooms 房主的房间列表（查询失败返回空）
func (a *App) mustHostRooms(uid int64) []*Room {
        rooms, err := a.store.HostRooms(uid)
        if err != nil {
                log.Printf("[大厅] 查询房主房间失败 uid=%d：%v", uid, err)
                return nil
        }
        return rooms
}

// sweepLoop 房间清扫协程：按 JDYC_SWEEP_SEC 周期检查房主断线时长
func (a *App) sweepLoop() {
        interval := time.Duration(a.cfg.SweepSec) * time.Second
        down := time.Duration(a.cfg.HostDownSec) * time.Second
        ticker := time.NewTicker(interval)
        defer ticker.Stop()
        for range ticker.C {
                rooms, err := a.store.ListRooms()
                if err != nil {
                        continue
                }
                now := time.Now()
                a.hub.mu.Lock()
                for _, room := range rooms {
                        _, online := a.hub.clients[room.HostUID]
                        if online {
                                delete(a.hub.down, room.ID)
                                continue
                        }
                        t, tracked := a.hub.down[room.ID]
                        if !tracked {
                                // 房主从未通过 WS 上线过（如服务重启），开始计时
                                a.hub.down[room.ID] = now
                                continue
                        }
                        if now.Sub(t) >= down {
                                roomID := room.ID
                                a.hub.mu.Unlock()
                                log.Printf("[清扫] 房主 %s 断线超时，关闭房间 %d", room.HostNickname, roomID)
                                a.closeRoom(room)
                                a.broadcastRoomList()
                                a.hub.mu.Lock()
                        }
                }
                a.hub.mu.Unlock()
        }
}
