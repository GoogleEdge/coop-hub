package main

// 房间 REST 接口：列表 / 创建 / 删除，frp 与 vlan 详情构造。

import (
        "crypto/rand"
        "encoding/hex"
        "encoding/json"
        "errors"
        "fmt"
        "net"
        "net/http"
        "strconv"
        "strings"
)

// errRoomFull 房间已满
var errRoomFull = errors.New("room_full")

// RoomRuntime 房间运行期信息（frp 本地端口与协议，不落库）
type RoomRuntime struct {
        LocalPort int
        PortType  string
}

// vlanSubnetFor 房间 ID → 25.10.<id%250+1>.0/24
func vlanSubnetFor(roomID int64) string {
        return fmt.Sprintf("25.10.%d.0/24", roomID%250+1)
}

// subnetPrefix 子网 → 主机号前缀（"25.10.x."，可直接拼主机号 1~254）
func subnetPrefix(subnet string) string {
        ip, _, err := net.ParseCIDR(subnet)
        if err != nil {
                return ""
        }
        s := ip.String() // 25.10.x.0（/24 基址恒以 .0 结尾）
        return s[:len(s)-1]
}

// randHexKey 16 位随机 hex（8 字节）
func randHexKey() (string, error) {
        b := make([]byte, 8)
        if _, err := rand.Read(b); err != nil {
                return "", err
        }
        return hex.EncodeToString(b), nil
}

// frpDetail frp 连接参数（room_joined 的 "frp" 对象）
func frpDetail(r *Room, cfg *Config, rt *RoomRuntime) map[string]any {
        return map[string]any{
                "frpc_server": cfg.FrpsHost,
                "frpc_port":   cfg.FrpsPort,
                "frpc_token":  cfg.FrpsToken,
                "proxy_name":  fmt.Sprintf("jdyc-room-%d", r.ID),
                "remote_port": r.FrpRemotePort,
                "local_port":  rt.LocalPort,
                "port_type":   rt.PortType,
        }
}

// vlanDetail vlan 连接参数（room_joined 的 "vlan" 对象）
func vlanDetail(r *Room, cfg *Config, myVirtIP string) map[string]any {
        base := subnetPrefix(r.VlanSubnet)
        return map[string]any{
                "network_community": fmt.Sprintf("jdyc-room-%d", r.ID),
                "vlan_key":          r.VlanKey,
                "my_virt_ip":        myVirtIP,
                "host_virt_ip":      base + "1",
                "supernode":         cfg.Supernode,
        }
}

// roomJSON 房间列表行（REST 与 WS 通用）
func (a *App) roomJSON(r *Room) map[string]any {
        members, _ := a.store.MemberCount(r.ID)
        m := map[string]any{
                "id":            r.ID,
                "name":          r.Name,
                "game":          r.Game,
                "mode":          r.Mode,
                "host_nickname": r.HostNickname,
                "host_class":    r.HostClass,
                "members":       members,
                "max_members":   r.MaxMembers,
                "created_at":    r.CreatedAt,
        }
        if r.Mode == "frp" {
                m["frp_addr"] = fmt.Sprintf("%s:%d", a.cfg.FrpsHost, r.FrpRemotePort)
        }
        return m
}

// handleListRooms GET /api/rooms
func (a *App) handleListRooms(w http.ResponseWriter, r *http.Request) {
        rooms, err := a.store.ListRooms()
        if err != nil {
                writeErr(w, http.StatusInternalServerError, "server_error", "服务器内部错误")
                return
        }
        list := []map[string]any{}
        for _, room := range rooms {
                list = append(list, a.roomJSON(room))
        }
        writeJSON(w, http.StatusOK, map[string]any{"rooms": list})
}

// handleCreateRoom POST /api/rooms
func (a *App) handleCreateRoom(w http.ResponseWriter, r *http.Request) {
        u := r.Context().Value(ctxUser).(*User)
        if u.Status != "approved" {
                writeErr(w, http.StatusForbidden, "not_approved", "账号未通过实名审核")
                return
        }
        var body struct {
                Name      string `json:"name"`
                Game      string `json:"game"`
                Mode      string `json:"mode"`
                LocalPort int    `json:"local_port"`
                PortType  string `json:"port_type"`
        }
        if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
                writeErr(w, http.StatusBadRequest, "invalid_params", "请求体格式错误")
                return
        }
        if runeCount(body.Name) < 1 || runeCount(body.Name) > 20 {
                writeErr(w, http.StatusBadRequest, "invalid_params", "房间名需为 1-20 字")
                return
        }
        switch body.Game {
        case "minecraft", "terraria", "cs16", "yuzu", "custom":
        default:
                writeErr(w, http.StatusBadRequest, "invalid_params", "不支持的游戏类型")
                return
        }
        if body.Mode != "frp" && body.Mode != "vlan" {
                writeErr(w, http.StatusBadRequest, "invalid_params", "模式仅支持 frp 或 vlan")
                return
        }
        room := &Room{
                Name: body.Name, Game: body.Game, Mode: body.Mode,
                HostUID: u.ID, HostNickname: u.Nickname, HostClass: u.ClassName,
                MaxMembers: 20,
        }
        if body.Mode == "frp" {
                if body.LocalPort <= 0 || body.LocalPort > 65535 {
                        writeErr(w, http.StatusBadRequest, "invalid_params", "本地端口无效")
                        return
                }
                if body.PortType != "tcp" && body.PortType != "udp" && body.PortType != "both" {
                        writeErr(w, http.StatusBadRequest, "invalid_params", "协议仅支持 tcp/udp/both")
                        return
                }
                port, err := a.store.AllocPort(0)
                if err != nil {
                        writeErr(w, http.StatusInternalServerError, "server_error", "端口分配失败："+err.Error())
                        return
                }
                room.FrpRemotePort = port
                room.IpCursor = 1
        } else {
                // vlan 房：生成 16 位 hex 密钥，房主占 .1（ip_cursor 从 2 起分给成员）
                key, kerr := randHexKey()
                if kerr != nil {
                        writeErr(w, http.StatusInternalServerError, "server_error", "密钥生成失败")
                        return
                }
                room.VlanKey = key
                room.IpCursor = 2
        }
        if _, err := a.store.CreateRoom(room); err != nil {
                writeErr(w, http.StatusInternalServerError, "server_error", "建房失败："+err.Error())
                return
        }
        rt := &RoomRuntime{LocalPort: body.LocalPort, PortType: body.PortType}
        if body.Mode == "frp" {
                // 端口已随建房落库，房间 ID 现在才确定，回填 used_ports 的归属
                if err := a.store.BindPort(int64(room.FrpRemotePort), room.ID); err != nil {
                        writeErr(w, http.StatusInternalServerError, "server_error", "端口登记失败："+err.Error())
                        return
                }
        }
        a.hub.SetRuntime(room.ID, rt)
        a.hub.Broadcast(map[string]any{"type": "room_created", "room": a.roomJSON(room)})
        resp := a.roomJSON(room)
        if room.Mode == "frp" {
                resp["frp"] = frpDetail(room, a.cfg, rt)
        } else {
                resp["vlan"] = vlanDetail(room, a.cfg, subnetPrefix(room.VlanSubnet)+"1")
                // 顶层附子网与密钥，便于建房方直接记录
                resp["vlan_subnet"] = room.VlanSubnet
                resp["vlan_key"] = room.VlanKey
        }
        writeJSON(w, http.StatusOK, map[string]any{"room": resp})
}

// handleDeleteRoom DELETE /api/rooms/{id}
func (a *App) handleDeleteRoom(w http.ResponseWriter, r *http.Request) {
        u := r.Context().Value(ctxUser).(*User)
        id, err := strconv.ParseInt(strings.TrimPrefix(r.URL.Path, "/api/rooms/"), 10, 64)
        if err != nil {
                writeErr(w, http.StatusBadRequest, "invalid_params", "房间 ID 无效")
                return
        }
        room, err := a.store.GetRoom(id)
        if err != nil || room == nil {
                writeErr(w, http.StatusNotFound, "room_not_found", "房间不存在")
                return
        }
        if room.HostUID != u.ID {
                writeErr(w, http.StatusForbidden, "not_host", "只有房主可以关闭房间")
                return
        }
        a.closeRoom(room)
        writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// closeRoom 删房并广播
func (a *App) closeRoom(room *Room) {
        a.store.DeleteRoom(room.ID)
        a.hub.ClearRuntime(room.ID)
        a.hub.Broadcast(map[string]any{"type": "room_removed", "room_id": room.ID})
}
