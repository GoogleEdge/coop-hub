package main

// 单元测试：参数校验 / 端口分配 / vlan 子网 / 限速器

import (
        "path/filepath"
        "testing"
)

func TestValidateUsername(t *testing.T) {
        cases := map[string]bool{
                "abc1": true, "ab_01": true, "a": false, "Abcdefghij0123456789X": false,
                "中文": false, "abc": false, "": false, "user name": false,
        }
        for s, want := range cases {
                if got := ValidateUsername(s); got != want {
                        t.Errorf("ValidateUsername(%q) = %v, 期望 %v", s, got, want)
                }
        }
}

func TestValidatePassword(t *testing.T) {
        cases := map[string]bool{
                "abcd1234": true, "abcd123": false, "aaaaaaaa": false, "12345678": false,
                "": false, "a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6q7r8s9t0u1v2w3x4y5z6A1B2C3D4E5F6G7H8": false,
        }
        for s, want := range cases {
                if got := ValidatePassword(s); got != want {
                        t.Errorf("ValidatePassword(%q) = %v, 期望 %v", s, got, want)
                }
        }
}

func TestValidateRealName(t *testing.T) {
        cases := map[string]bool{"张三": true, "欧阳娜娜": true, "李": false, "王五五五五": false, "Tom": false, "张三丰!": false}
        for s, want := range cases {
                if got := ValidateRealName(s); got != want {
                        t.Errorf("ValidateRealName(%q) = %v, 期望 %v", s, got, want)
                }
        }
}

func TestValidateClassName(t *testing.T) {
        if !ValidateClassName("初一（3）班") {
                t.Error("合法班级被拒")
        }
        if ValidateClassName("") {
                t.Error("空班级应被拒")
        }
        long := ""
        for i := 0; i < 21; i++ {
                long += "班"
        }
        if ValidateClassName(long) {
                t.Error("超 20 字班级应被拒")
        }
}

func TestVlanSubnetFor(t *testing.T) {
        if got := vlanSubnetFor(1); got != "25.10.2.0/24" {
                t.Errorf("vlanSubnetFor(1) = %s", got)
        }
        if got := vlanSubnetFor(249); got != "25.10.250.0/24" {
                t.Errorf("vlanSubnetFor(249) = %s", got)
        }
        if got := vlanSubnetFor(250); got != "25.10.1.0/24" {
                t.Errorf("vlanSubnetFor(250) = %s", got)
        }
        if got := vlanSubnetFor(300); got != "25.10.51.0/24" {
                t.Errorf("vlanSubnetFor(300) = %s", got)
        }
        if got := subnetPrefix("25.10.3.0/24"); got != "25.10.3." {
                t.Errorf("subnetPrefix = %s", got)
        }
}

func TestRandHexKey(t *testing.T) {
        k, err := randHexKey()
        if err != nil || len(k) != 16 {
                t.Fatalf("randHexKey 长度应 16，实际 %d（err=%v）", len(k), err)
        }
        for _, c := range k {
                if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
                        t.Fatalf("密钥含非 hex 字符：%s", k)
                }
        }
}

func TestPortAllocation(t *testing.T) {
        store := openTestStore(t)
        // 逐个分配，落在 20000-29999 且不重复
        seen := map[int]bool{}
        for i := 0; i < 5; i++ {
                p, err := store.AllocPort(0)
                if err != nil {
                        t.Fatalf("第 %d 次分配失败：%v", i, err)
                }
                if p < 20000 || p > 29999 {
                        t.Fatalf("端口 %d 超出范围", p)
                }
                if seen[p] {
                        t.Fatalf("端口 %d 重复分配", p)
                }
                seen[p] = true
        }
}

func openTestStore(t *testing.T) *Store {
        t.Helper()
        path := filepath.Join(t.TempDir(), "test.db")
        store, err := openStore(path)
        if err != nil {
                t.Fatalf("打开测试库失败：%v", err)
        }
        t.Cleanup(func() { store.DB.Close() })
        return store
}

func TestCreateRoomAndVlan(t *testing.T) {
        store := openTestStore(t)
        if err := store.CreateUser(&User{Username: "host1", PasswordHash: "x", Nickname: "房主",
                ClassName: "初一1班", RealName: "张三", Role: "user", Status: "approved"}); err != nil {
                t.Fatal(err)
        }
        u1, _ := store.GetUserByUsername("host1")
        uid := u1.ID
        room := &Room{Name: "测试房", Game: "minecraft", Mode: "vlan", HostUID: uid,
                HostNickname: "房主", HostClass: "初一1班", MaxMembers: 10, VlanKey: "0123456789abcdef", IpCursor: 2}
        id, err := store.CreateRoom(room)
        if err != nil {
                t.Fatal(err)
        }
        got, _ := store.GetRoom(id)
        if got.VlanSubnet != vlanSubnetFor(id) {
                t.Errorf("子网 %s 应为 %s", got.VlanSubnet, vlanSubnetFor(id))
        }
        // 房主占 .1
        ip, _, err := store.JoinMember(got, uid)
        if err != nil {
                t.Fatal(err)
        }
        if ip != subnetPrefix(got.VlanSubnet)+"1" {
                t.Errorf("房主 IP = %s", ip)
        }
        // 成员从 .2 起
        if err := store.CreateUser(&User{Username: "host2", PasswordHash: "x", Nickname: "成员",
                ClassName: "初一1班", RealName: "李四", Role: "user", Status: "approved"}); err != nil {
                t.Fatal(err)
        }
        u2, _ := store.GetUserByUsername("host2")
        uid2 := u2.ID
        ip2, _, err := store.JoinMember(got, uid2)
        if err != nil {
                t.Fatal(err)
        }
        if ip2 != subnetPrefix(got.VlanSubnet)+"2" {
                t.Errorf("成员 IP = %s", ip2)
        }
}

func TestRateLimiter(t *testing.T) {
        rl := NewRateLimiter()
        for i := 0; i < 5; i++ {
                if !rl.Allow("1.2.3.4") {
                        t.Fatalf("第 %d 次应放行", i+1)
                }
        }
        if rl.Allow("1.2.3.4") {
                t.Error("第 6 次应限速")
        }
        if !rl.Allow("5.6.7.8") {
                t.Error("其他 IP 不应受影响")
        }
}

func TestPasswordHash(t *testing.T) {
        h, err := HashPassword("abcd1234", "pepper")
        if err != nil {
                t.Fatal(err)
        }
        if !CheckPassword(h, "abcd1234", "pepper") {
                t.Error("正确密码校验失败")
        }
        if CheckPassword(h, "abcd1234", "") {
                t.Error("pepper 不同应校验失败")
        }
        if CheckPassword(h, "wrong1234", "pepper") {
                t.Error("错误密码应校验失败")
        }
}
