//go:build integration

package main

// 集成测试：起服务端子进程 + 连 CI 里的 frps，全链路 API/WS 断言。
// 断言项对应任务书 3.1 ⑥条：
// ① 后台登录+审核通过用户 ② 用户登录得 JWT ③ frp 房端口 20000-29999 且 used_ports 落库
// ④ vlan 房子网 25.10.x.0/24 + 16 位 hex 密钥 ⑤ WS room_list/room_joined 全字段
// ⑥ 房主断线→清扫删房→广播 room_removed（用缩短的清扫配置验证）

import (
        "database/sql"
        "encoding/json"
        "fmt"
        "io"
        "net/http"
        "os"
        "os/exec"
        "strings"
        "syscall"
        "testing"
        "time"

        "github.com/gorilla/websocket"
        _ "github.com/mattn/go-sqlite3"
)

const base = "http://127.0.0.1:7100"

func TestIntegration(t *testing.T) {
        // 构建服务端二进制
        bin := t.TempDir() + "/jdyc-server"
        build := exec.Command("go", "build", "-o", bin, ".")
        build.Dir = "."
        if out, err := build.CombinedOutput(); err != nil {
                t.Fatalf("构建服务端失败：%v\n%s", err, out)
        }

        // 建管理员
        admin := exec.Command(bin, "-createadmin", "admin", "AdminPass123", "初一1班", "管理员")
        if out, err := admin.CombinedOutput(); err != nil {
                t.Fatalf("创建管理员失败：%v\n%s", err, out)
        }

        // 启动服务端（缩短清扫与断线阈值，便于断言⑥）
        cmd := exec.Command(bin)
        cmd.Env = append(os.Environ(),
                "JDYC_SWEEP_SEC=2",
                "JDYC_HOST_DOWN_SEC=3",
        )
        cmd.Stdout = os.Stdout
        cmd.Stderr = os.Stderr
        if err := cmd.Start(); err != nil {
                t.Fatalf("服务端启动失败：%v", err)
        }
        t.Cleanup(func() {
                cmd.Process.Signal(syscall.SIGTERM)
                cmd.Wait()
        })

        // 等服务端就绪
        ready := false
        for i := 0; i < 50; i++ {
                resp, err := http.Get(base + "/")
                if err == nil {
                        resp.Body.Close()
                        ready = true
                        break
                }
                time.Sleep(200 * time.Millisecond)
        }
        if !ready {
                t.Fatal("服务端 10 秒内未就绪")
        }

        // ---- 注册两个用户（未审核状态） ----
        reg := func(username string) {
                body := fmt.Sprintf(`{"username":%q,"password":"Passw0rd8","nickname":"同学%s","class_name":"初一1班","real_name":"张三丰","student_no":"2026%03d"}`,
                        username, strings.TrimPrefix(username, "stu"), len(username))
                resp, err := http.Post(base+"/api/register", "application/json", strings.NewReader(body))
                if err != nil {
                        t.Fatalf("注册 %s 失败：%v", username, err)
                }
                defer resp.Body.Close()
                if resp.StatusCode != 201 {
                        b := make([]byte, 512)
                        n, _ := resp.Body.Read(b)
                        t.Fatalf("注册 %s 状态码 %d：%s", username, resp.StatusCode, b[:n])
                }
        }
        reg("stuhost")
        reg("stuguest")

        // ---- ① 管理后台登录 + 审核通过 ----
        jar := map[string]string{}
        client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}
        resp, err := client.PostForm(base+"/admin/login", map[string][]string{"username": {"admin"}, "password": {"AdminPass123"}})
        if err != nil {
                t.Fatalf("后台登录请求失败：%v", err)
        }
        for _, c := range resp.Cookies() {
                jar[c.Name] = c.Value
        }
        if jar["jdyc_admin_session"] == "" {
                t.Fatal("后台登录未下发会话 Cookie")
        }
        // 从审核页 HTML 抓两个用户 ID（手动带会话 Cookie，client 未挂 Jar）
        getAdmin := func(path string) (string, error) {
                req, _ := http.NewRequest("GET", base+path, nil)
                req.Header.Set("Cookie", "jdyc_admin_session="+jar["jdyc_admin_session"])
                r, err := client.Do(req)
                if err != nil {
                        return "", err
                }
                defer r.Body.Close()
                b, err := io.ReadAll(r.Body)
                return string(b), err
        }
        html, err := getAdmin("/admin/review")
        if err != nil {
                t.Fatal(err)
        }
        ids := []string{}
        for _, u := range []string{"stuhost", "stuguest"} {
                // 每行 <td>ID</td><td>username</td>，取 username 前最近的数字段
                idx := strings.Index(html, u)
                if idx < 0 {
                        t.Fatalf("审核页未找到用户 %s", u)
                }
                seg := html[max(idx-200, 0):idx]
                idFound := ""
                for i := len(seg) - 1; i >= 0; i-- {
                        if seg[i] >= '0' && seg[i] <= '9' {
                                idFound = string(seg[i]) + idFound
                        } else if idFound != "" {
                                break
                        }
                }
                if idFound == "" {
                        t.Fatalf("未解析到 %s 的用户 ID", u)
                }
                ids = append(ids, idFound)
        }
        if len(ids) != 2 {
                t.Fatalf("应解析出 2 个待审核 ID，实际 %v", ids)
        }
        for _, id := range ids {
                req, _ := http.NewRequest("POST", base+"/admin/review/"+id+"/approve", nil)
                req.AddCookie(&http.Cookie{Name: "jdyc_admin_session", Value: jar["jdyc_admin_session"]})
                r2, err := client.Do(req)
                if err != nil {
                        t.Fatal(err)
                }
                r2.Body.Close()
        }
        // 校验审核页已清空
        time.Sleep(200 * time.Millisecond)
        html2, _ := getAdmin("/admin/review")
        if strings.Contains(html2, "stuhost") {
                t.Error("审核通过后待审核列表应不含 stuhost")
        }

        // ---- ② 用户登录得 JWT ----
        login := func(username string) string {
                resp, err := http.Post(base+"/api/login", "application/json",
                        strings.NewReader(fmt.Sprintf(`{"username":%q,"password":"Passw0rd8"}`, username)))
                if err != nil {
                        t.Fatal(err)
                }
                defer resp.Body.Close()
                var out struct {
                        Token string `json:"token"`
                }
                if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || out.Token == "" {
                        t.Fatalf("%s 登录失败：token=%q err=%v", username, out.Token, err)
                }
                return out.Token
        }
        hostToken := login("stuhost")
        guestToken := login("stuguest")

        // GET /api/me
        req, _ := http.NewRequest("GET", base+"/api/me", nil)
        req.Header.Set("Authorization", "Bearer "+hostToken)
        r3, err := http.DefaultClient.Do(req)
        if err != nil || r3.StatusCode != 200 {
                t.Fatalf("/api/me 状态 %d err=%v", r3.StatusCode, err)
        }
        var me struct {
                Status string `json:"status"`
        }
        json.NewDecoder(r3.Body).Decode(&me)
        if me.Status != "approved" {
                t.Errorf("审核后 /api/me status = %s", me.Status)
        }
        r3.Body.Close()

        // ---- ③ frp 房：端口 20000-29999 + used_ports 落库 ----
        createRoom := func(token, name, mode, portType string, localPort int) map[string]any {
                body := fmt.Sprintf(`{"name":%q,"game":"minecraft","mode":%q,"port_type":%q,"local_port":%d}`,
                        name, mode, portType, localPort)
                req, _ := http.NewRequest("POST", base+"/api/rooms", strings.NewReader(body))
                req.Header.Set("Authorization", "Bearer "+token)
                req.Header.Set("Content-Type", "application/json")
                resp, err := http.DefaultClient.Do(req)
                if err != nil {
                        t.Fatal(err)
                }
                defer resp.Body.Close()
                var out map[string]any
                json.NewDecoder(resp.Body).Decode(&out)
                if resp.StatusCode != 200 {
                        t.Fatalf("建房 %s 状态 %d：%v", name, resp.StatusCode, out)
                }
                return out["room"].(map[string]any)
        }
        frpRoom := createRoom(hostToken, "MC端口映射房", "frp", "tcp", 25565)
        frpResp := frpRoom["frp"].(map[string]any)
        remotePort := int(frpResp["remote_port"].(float64))
        if remotePort < 20000 || remotePort > 29999 {
                t.Errorf("frp 端口 %d 不在 20000-29999", remotePort)
        }
        if frpRoom["frp_addr"] != fmt.Sprintf("127.0.0.1:%d", remotePort) {
                t.Errorf("frp_addr = %v", frpRoom["frp_addr"])
        }
        // used_ports 落库校验（直接查测试库）
        db, err := sql.Open("sqlite3", os.Getenv("JDYC_DB"))
        if err != nil {
                t.Fatal(err)
        }
        defer db.Close()
        var cnt int
        if err := db.QueryRow(`SELECT COUNT(*) FROM used_ports WHERE room_id=? AND port=?`,
                int64(frpRoom["id"].(float64)), remotePort).Scan(&cnt); err != nil || cnt != 1 {
                t.Errorf("used_ports 未落库：cnt=%d err=%v", cnt, err)
        }

        // ---- ④ vlan 房：子网 + 16 位 hex 密钥（房主为 stuhost，便于断言⑥） ----
        vlanRoom := createRoom(hostToken, "虚拟局域房", "vlan", "", 0)
        subnet := vlanRoom["vlan_subnet"].(string)
        if !strings.HasPrefix(subnet, "25.10.") || !strings.HasSuffix(subnet, ".0/24") {
                t.Errorf("vlan 子网 = %s", subnet)
        }
        vlanKey := vlanRoom["vlan_key"].(string)
        if len(vlanKey) != 16 {
                t.Errorf("vlan 密钥长度 = %d", len(vlanKey))
        }

        // ---- ⑤ WS：room_list + room_joined 全字段 ----
        wsDial := func(token string) *websocket.Conn {
                u := "ws://127.0.0.1:7100/lobby?token=" + token
                c, _, err := websocket.DefaultDialer.Dial(u, nil)
                if err != nil {
                        t.Fatalf("WS 连接失败：%v", err)
                }
                return c
        }
        readMsg := func(t *testing.T, c *websocket.Conn, wantType string, timeout time.Duration) map[string]any {
                t.Helper()
                deadline := time.Now().Add(timeout)
                for {
                        c.SetReadDeadline(deadline)
                        var msg map[string]any
                        if err := c.ReadJSON(&msg); err != nil {
                                t.Fatalf("等 %s 超时/出错：%v", wantType, err)
                        }
                        if msg["type"] == wantType {
                                return msg
                        }
                }
        }
        hostWS := wsDial(hostToken)
        defer hostWS.Close()
        guestWS := wsDial(guestToken)
        defer guestWS.Close()

        roomList := readMsg(t, hostWS, "room_list", 5*time.Second)
        rooms := roomList["rooms"].([]any)
        if len(rooms) < 2 {
                t.Errorf("room_list 应含 2 个房间，实际 %d", len(rooms))
        }

        // 客人进 frp 房 → room_joined 携带完整 frp 字段
        guestWS.WriteJSON(map[string]any{"type": "join_room", "room_id": frpRoom["id"]})
        joined := readMsg(t, guestWS, "room_joined", 5*time.Second)
        frpObj, ok := joined["frp"].(map[string]any)
        if !ok {
                t.Fatalf("room_joined 缺 frp 对象：%v", joined)
        }
        for _, field := range []string{"frpc_server", "frpc_port", "frpc_token", "proxy_name", "remote_port", "local_port", "port_type"} {
                if _, ok := frpObj[field]; !ok {
                        t.Errorf("frp 对象缺字段 %s", field)
                }
        }
        if frpObj["frpc_server"] != "127.0.0.1" || frpObj["frpc_port"] != float64(7000) ||
                frpObj["frpc_token"] != "test-token" || frpObj["local_port"] != float64(25565) ||
                frpObj["port_type"] != "tcp" {
                t.Errorf("frp 字段值不符：%v", frpObj)
        }

        // 客人进 vlan 房 → room_joined 携带完整 vlan 字段
        guestWS.WriteJSON(map[string]any{"type": "join_room", "room_id": vlanRoom["id"]})
        joinedV := readMsg(t, guestWS, "room_joined", 5*time.Second)
        vlanObj, ok := joinedV["vlan"].(map[string]any)
        if !ok {
                t.Fatalf("room_joined 缺 vlan 对象：%v", joinedV)
        }
        if vlanObj["network_community"] != fmt.Sprintf("jdyc-room-%d", int(vlanRoom["id"].(float64))) ||
                vlanObj["my_virt_ip"] == "" || vlanObj["host_virt_ip"] == "" ||
                vlanObj["supernode"] != "127.0.0.1:7777" {
                t.Errorf("vlan 字段值不符：%v", vlanObj)
        }
        if len(vlanKey) == 16 && vlanObj["vlan_key"] != vlanKey {
                t.Errorf("vlan_key 不一致：%v vs %s", vlanObj["vlan_key"], vlanKey)
        }

        // ---- ⑥ 房主断线 → 清扫删房 → 广播 room_removed ----
        // 先用 REST 关闭 frp 房（顺带验证 DELETE 接口），保证⑥只收到 vlan 房的广播
        delReq, _ := http.NewRequest("DELETE", fmt.Sprintf("/api/rooms/%d", int(frpRoom["id"].(float64))), nil)
        delReq.URL.Scheme = "http"
        delReq.URL.Host = "127.0.0.1:7100"
        delReq.Header.Set("Authorization", "Bearer "+hostToken)
        if r4, err := http.DefaultClient.Do(delReq); err != nil || r4.StatusCode != 200 {
                t.Fatalf("DELETE /api/rooms 失败：%v 状态=%v", err, r4)
        } else {
                r4.Body.Close()
        }
        // 删完再起监听器：只认 vlan 房的 room_removed（frp 房的广播还滞留在接收缓冲里）
        vlanRoomID := vlanRoom["id"]
        removedCh := make(chan map[string]any, 1)
        go func() {
                for {
                        guestWS.SetReadDeadline(time.Now().Add(15 * time.Second))
                        var msg map[string]any
                        if err := guestWS.ReadJSON(&msg); err != nil {
                                return
                        }
                        if msg["type"] == "room_removed" && msg["room_id"] == vlanRoomID {
                                removedCh <- msg
                                return
                        }
                }
        }()
        hostWS.Close() // 房主断开 WS

        select {
        case <-removedCh:
                // 收到 vlan 房的 room_removed，⑥ 通过
        case <-time.After(15 * time.Second):
                t.Error("15 秒内未收到 room_removed 广播（清扫=2s，断线阈值=3s）")
        }
}
