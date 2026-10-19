package main

// 数据库层：SQLite，Schema 与任务书第 4 章逐字一致。

import (
        "database/sql"
        "errors"
        "time"

        _ "github.com/mattn/go-sqlite3"
)

const schema = `PRAGMA journal_mode=WAL;
CREATE TABLE IF NOT EXISTS users (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  username      TEXT UNIQUE NOT NULL,
  password_hash TEXT NOT NULL,
  nickname      TEXT NOT NULL,
  class_name    TEXT NOT NULL,
  real_name     TEXT NOT NULL,
  student_no    TEXT NOT NULL DEFAULT '',
  role          TEXT NOT NULL DEFAULT 'user',       -- user | admin
  status        TEXT NOT NULL DEFAULT 'pending',    -- pending|approved|rejected|banned
  reject_reason TEXT NOT NULL DEFAULT '',
  created_at    INTEGER NOT NULL,
  reviewed_at   INTEGER NOT NULL DEFAULT 0,
  reviewed_by   INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS rooms (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  name          TEXT NOT NULL,
  game          TEXT NOT NULL,          -- minecraft|terraria|cs16|yuzu|custom
  mode          TEXT NOT NULL,          -- vlan | frp
  host_uid      INTEGER NOT NULL,
  host_nickname TEXT NOT NULL,
  host_class    TEXT NOT NULL,
  max_members   INTEGER NOT NULL DEFAULT 20,
  frp_remote_port INTEGER NOT NULL DEFAULT 0,
  vlan_subnet   TEXT NOT NULL DEFAULT '',
  vlan_key      TEXT NOT NULL DEFAULT '',
  ip_cursor     INTEGER NOT NULL DEFAULT 1,
  created_at    INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS room_members (
  room_id INTEGER NOT NULL,
  uid     INTEGER NOT NULL,
  virt_ip TEXT NOT NULL DEFAULT '',
  joined_at INTEGER NOT NULL,
  PRIMARY KEY (room_id, uid)
);
CREATE TABLE IF NOT EXISTS used_ports (
  port INTEGER PRIMARY KEY,
  room_id INTEGER NOT NULL,
  created_at INTEGER NOT NULL
);`

// ErrUsernameExists 用户名重复
var ErrUsernameExists = errors.New("username_exists")

// User 用户表映射
type User struct {
        ID           int64  `json:"id"`
        Username     string `json:"username"`
        PasswordHash string `json:"-"`
        Nickname     string `json:"nickname"`
        ClassName    string `json:"class_name"`
        RealName     string `json:"-"`
        StudentNo    string `json:"-"`
        Role         string `json:"role"`
        Status       string `json:"status"`
        RejectReason string `json:"reject_reason"`
        CreatedAt    int64  `json:"-"`
        ReviewedAt   int64  `json:"-"`
        ReviewedBy   int64  `json:"-"`
}

// Room 房间表映射
type Room struct {
        ID            int64  `json:"id"`
        Name          string `json:"name"`
        Game          string `json:"game"`
        Mode          string `json:"mode"`
        HostUID       int64  `json:"-"`
        HostNickname  string `json:"host_nickname"`
        HostClass     string `json:"host_class"`
        MaxMembers    int    `json:"max_members"`
        FrpRemotePort int    `json:"-"`
        VlanSubnet    string `json:"-"`
        VlanKey       string `json:"-"`
        IpCursor      int    `json:"-"`
        CreatedAt     int64  `json:"created_at"`
}

// Member 房间成员
type Member struct {
        RoomID   int64
        UID      int64
        VirtIP   string
        JoinedAt int64
        Nickname string
}

// Store 数据库封装
type Store struct {
        DB *sql.DB
}

func openStore(path string) (*Store, error) {
        db, err := sql.Open("sqlite3", path)
        if err != nil {
                return nil, err
        }
        // SQLite 单写多读，用单连接避免锁竞争
        db.SetMaxOpenConns(1)
        if _, err := db.Exec(schema); err != nil {
                db.Close()
                return nil, err
        }
        return &Store{DB: db}, nil
}

// CreateUser 新建用户
func (s *Store) CreateUser(u *User) error {
        u.CreatedAt = time.Now().Unix()
        _, err := s.DB.Exec(`INSERT INTO users(username,password_hash,nickname,class_name,real_name,student_no,role,status,created_at)
                VALUES(?,?,?,?,?,?,?,?,?)`,
                u.Username, u.PasswordHash, u.Nickname, u.ClassName, u.RealName, u.StudentNo, u.Role, u.Status, u.CreatedAt)
        if err != nil && isUniqueErr(err) {
                return ErrUsernameExists
        }
        return err
}

func isUniqueErr(err error) bool {
        return err != nil && (errors.Is(err, sql.ErrNoRows) == false) &&
                (containsStr(err.Error(), "UNIQUE constraint failed"))
}

func containsStr(s, sub string) bool {
        for i := 0; i+len(sub) <= len(s); i++ {
                if s[i:i+len(sub)] == sub {
                        return true
                }
        }
        return false
}

const userCols = `id,username,password_hash,nickname,class_name,real_name,student_no,role,status,reject_reason,created_at,reviewed_at,reviewed_by`

func scanUser(row interface{ Scan(...any) error }) (*User, error) {
        u := &User{}
        err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Nickname, &u.ClassName,
                &u.RealName, &u.StudentNo, &u.Role, &u.Status, &u.RejectReason,
                &u.CreatedAt, &u.ReviewedAt, &u.ReviewedBy)
        if err != nil {
                return nil, err
        }
        return u, nil
}

// GetUserByUsername 按用户名查
func (s *Store) GetUserByUsername(username string) (*User, error) {
        u, err := scanUser(s.DB.QueryRow(`SELECT `+userCols+` FROM users WHERE username=?`, username))
        if errors.Is(err, sql.ErrNoRows) {
                return nil, nil
        }
        return u, err
}

// GetUserByID 按 ID 查
func (s *Store) GetUserByID(id int64) (*User, error) {
        u, err := scanUser(s.DB.QueryRow(`SELECT `+userCols+` FROM users WHERE id=?`, id))
        if errors.Is(err, sql.ErrNoRows) {
                return nil, nil
        }
        return u, err
}

// ListUsersByStatus 按状态列出用户（审核页用）
func (s *Store) ListUsersByStatus(status string) ([]*User, error) {
        rows, err := s.DB.Query(`SELECT `+userCols+` FROM users WHERE status=? ORDER BY created_at`, status)
        if err != nil {
                return nil, err
        }
        defer rows.Close()
        var out []*User
        for rows.Next() {
                u, err := scanUser(rows)
                if err != nil {
                        return nil, err
                }
                out = append(out, u)
        }
        return out, rows.Err()
}

// SetUserStatus 审核动作
func (s *Store) SetUserStatus(uid int64, status, reason string, reviewedBy int64) error {
        _, err := s.DB.Exec(`UPDATE users SET status=?, reject_reason=?, reviewed_at=?, reviewed_by=? WHERE id=?`,
                status, reason, time.Now().Unix(), reviewedBy, uid)
        return err
}

// CreateRoom 建房（含 frp 端口分配或 vlan 信息写入）
func (s *Store) CreateRoom(r *Room) (int64, error) {
        r.CreatedAt = time.Now().Unix()
        res, err := s.DB.Exec(`INSERT INTO rooms(name,game,mode,host_uid,host_nickname,host_class,max_members,frp_remote_port,vlan_subnet,vlan_key,ip_cursor,created_at)
                VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
                r.Name, r.Game, r.Mode, r.HostUID, r.HostNickname, r.HostClass, r.MaxMembers,
                r.FrpRemotePort, r.VlanSubnet, r.VlanKey, r.IpCursor, r.CreatedAt)
        if err != nil {
                return 0, err
        }
        id, err := res.LastInsertId()
        if err != nil {
                return 0, err
        }
        r.ID = id
        // vlan 房：子网依赖房间 ID，插入后回填
        if r.Mode == "vlan" {
                r.VlanSubnet = vlanSubnetFor(id)
                sub := subnetPrefix(r.VlanSubnet) // "25.10.x."
                if _, err := s.DB.Exec(`UPDATE rooms SET vlan_subnet=?, ip_cursor=? WHERE id=?`, r.VlanSubnet, 2, id); err != nil {
                        return 0, err
                }
                // 房主占 .1
                if _, err := s.DB.Exec(`INSERT INTO room_members(room_id,uid,virt_ip,joined_at) VALUES(?,?,?,?)`,
                        id, r.HostUID, sub+"1", time.Now().Unix()); err != nil {
                        return 0, err
                }
        }
        return id, nil
}

// GetRoom 查房间
func (s *Store) GetRoom(id int64) (*Room, error) {
        r := &Room{}
        err := s.DB.QueryRow(`SELECT id,name,game,mode,host_uid,host_nickname,host_class,max_members,frp_remote_port,vlan_subnet,vlan_key,ip_cursor,created_at FROM rooms WHERE id=?`, id).
                Scan(&r.ID, &r.Name, &r.Game, &r.Mode, &r.HostUID, &r.HostNickname, &r.HostClass,
                        &r.MaxMembers, &r.FrpRemotePort, &r.VlanSubnet, &r.VlanKey, &r.IpCursor, &r.CreatedAt)
        if errors.Is(err, sql.ErrNoRows) {
                return nil, nil
        }
        if err != nil {
                return nil, err
        }
        return r, nil
}

// ListRooms 全量房间
func (s *Store) ListRooms() ([]*Room, error) {
        rows, err := s.DB.Query(`SELECT id,name,game,mode,host_uid,host_nickname,host_class,max_members,frp_remote_port,vlan_subnet,vlan_key,ip_cursor,created_at FROM rooms ORDER BY created_at DESC`)
        if err != nil {
                return nil, err
        }
        defer rows.Close()
        var out []*Room
        for rows.Next() {
                r := &Room{}
                if err := rows.Scan(&r.ID, &r.Name, &r.Game, &r.Mode, &r.HostUID, &r.HostNickname, &r.HostClass,
                        &r.MaxMembers, &r.FrpRemotePort, &r.VlanSubnet, &r.VlanKey, &r.IpCursor, &r.CreatedAt); err != nil {
                        return nil, err
                }
                out = append(out, r)
        }
        return out, rows.Err()
}

// DeleteRoom 删房（连带成员与占用端口）
func (s *Store) DeleteRoom(id int64) error {
        if _, err := s.DB.Exec(`DELETE FROM room_members WHERE room_id=?`, id); err != nil {
                return err
        }
        if _, err := s.DB.Exec(`DELETE FROM used_ports WHERE room_id=?`, id); err != nil {
                return err
        }
        _, err := s.DB.Exec(`DELETE FROM rooms WHERE id=?`, id)
        return err
}

// AllocPort 从 20000-29999 分配一个空闲远端端口并落库
func (s *Store) AllocPort(roomID int64) (int, error) {
        rows, err := s.DB.Query(`SELECT port FROM used_ports`)
        if err != nil {
                return 0, err
        }
        used := map[int]bool{}
        for rows.Next() {
                var p int
                if err := rows.Scan(&p); err != nil {
                        rows.Close()
                        return 0, err
                }
                used[p] = true
        }
        rows.Close()
        for p := 20000; p <= 29999; p++ {
                if !used[p] {
                        if _, err := s.DB.Exec(`INSERT INTO used_ports(port,room_id,created_at) VALUES(?,?,?)`, p, roomID, time.Now().Unix()); err != nil {
                                return 0, err
                        }
                        return p, nil
                }
        }
        return 0, errors.New("no_free_port")
}

// BindPort 回填端口归属的房间 ID
func (s *Store) BindPort(port, roomID int64) error {
        _, err := s.DB.Exec(`UPDATE used_ports SET room_id=? WHERE port=?`, roomID, port)
        return err
}

// JoinMember 成员加入，vlan 房自动分配虚拟 IP；返回成员虚拟 IP
func (s *Store) JoinMember(r *Room, uid int64) (string, bool, error) {
        // 已在房内则直接返回原 IP
        var existing string
        err := s.DB.QueryRow(`SELECT virt_ip FROM room_members WHERE room_id=? AND uid=?`, r.ID, uid).Scan(&existing)
        if err == nil {
                return existing, false, nil
        }
        if !errors.Is(err, sql.ErrNoRows) {
                return "", false, err
        }
        cnt, err := s.MemberCount(r.ID)
        if err != nil {
                return "", false, err
        }
        if cnt >= r.MaxMembers {
                return "", false, errRoomFull
        }
        ip := ""
        if r.Mode == "vlan" {
                if r.IpCursor > 254 {
                        return "", false, errRoomFull
                }
                ip = subnetPrefix(r.VlanSubnet) + itoa(r.IpCursor)
                if _, err := s.DB.Exec(`UPDATE rooms SET ip_cursor=? WHERE id=?`, r.IpCursor+1, r.ID); err != nil {
                        return "", false, err
                }
        }
        if _, err := s.DB.Exec(`INSERT INTO room_members(room_id,uid,virt_ip,joined_at) VALUES(?,?,?,?)`, r.ID, uid, ip, time.Now().Unix()); err != nil {
                return "", false, err
        }
        return ip, true, nil
}

// LeaveMember 成员退出
func (s *Store) LeaveMember(roomID, uid int64) error {
        _, err := s.DB.Exec(`DELETE FROM room_members WHERE room_id=? AND uid=?`, roomID, uid)
        return err
}

// MemberCount 成员数
func (s *Store) MemberCount(roomID int64) (int, error) {
        var n int
        err := s.DB.QueryRow(`SELECT COUNT(*) FROM room_members WHERE room_id=?`, roomID).Scan(&n)
        return n, err
}

// ListMembers 成员列表
func (s *Store) ListMembers(roomID int64) ([]*Member, error) {
        rows, err := s.DB.Query(`SELECT m.room_id,m.uid,m.virt_ip,m.joined_at,u.nickname FROM room_members m LEFT JOIN users u ON u.id=m.uid WHERE m.room_id=? ORDER BY m.virt_ip, m.joined_at`, roomID)
        if err != nil {
                return nil, err
        }
        defer rows.Close()
        var out []*Member
        for rows.Next() {
                m := &Member{}
                if err := rows.Scan(&m.RoomID, &m.UID, &m.VirtIP, &m.JoinedAt, &m.Nickname); err != nil {
                        return nil, err
                }
                out = append(out, m)
        }
        return out, rows.Err()
}

// HostRooms 某用户当房主的所有房间
func (s *Store) HostRooms(uid int64) ([]*Room, error) {
        all, err := s.ListRooms()
        if err != nil {
                return nil, err
        }
        var out []*Room
        for _, r := range all {
                if r.HostUID == uid {
                        out = append(out, r)
                }
        }
        return out, nil
}

func itoa(n int) string {
        if n == 0 {
                return "0"
        }
        neg := n < 0
        if neg {
                n = -n
        }
        var buf [20]byte
        i := len(buf)
        for n > 0 {
                i--
                buf[i] = byte('0' + n%10)
                n /= 10
        }
        if neg {
                i--
                buf[i] = '-'
        }
        return string(buf[i:])
}
