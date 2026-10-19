package main

// 管理后台：html/template 内嵌，Session Cookie 鉴权。
// 路由：GET/POST /admin/login、GET /admin/review、POST /admin/review/{uid}/approve|reject、GET /admin/rooms、POST /admin/rooms/{id}/close

import (
        "crypto/rand"
        "encoding/hex"
        "html/template"
        "net/http"
        "strconv"
        "strings"
        "sync"
)

const (
        adminCookie = "jdyc_admin_session"
)

// AdminSessions 内存 Session
type AdminSessions struct {
        mu       sync.Mutex
        sessions map[string]int64 // token → uid
}

func NewAdminSessions() *AdminSessions {
        return &AdminSessions{sessions: map[string]int64{}}
}

func (s *AdminSessions) newSession(uid int64) string {
        b := make([]byte, 16)
        rand.Read(b)
        tok := hex.EncodeToString(b)
        s.mu.Lock()
        s.sessions[tok] = uid
        s.mu.Unlock()
        return tok
}

func (s *AdminSessions) get(r *http.Request) (int64, bool) {
        c, err := r.Cookie(adminCookie)
        if err != nil {
                return 0, false
        }
        s.mu.Lock()
        defer s.mu.Unlock()
        uid, ok := s.sessions[c.Value]
        return uid, ok
}

func (s *AdminSessions) del(r *http.Request) {
        if c, err := r.Cookie(adminCookie); err == nil {
                s.mu.Lock()
                delete(s.sessions, c.Value)
                s.mu.Unlock()
        }
}

const tplLogin = `<!DOCTYPE html>
<html lang="zh-CN"><head><meta charset="utf-8"><title>管理后台登录</title>
<style>body{font-family:system-ui;max-width:22rem;margin:6rem auto;padding:0 1rem}
input{width:100%;padding:.5rem;margin:.3rem 0;box-sizing:border-box}
button{width:100%;padding:.5rem;background:#1E6FFF;color:#fff;border:0;border-radius:.3rem}
h1{font-size:1.2rem}</style></head><body>
<h1>揭东一初联机大厅 · 管理后台</h1>
{{if .Error}}<p style="color:#c00">{{.Error}}</p>{{end}}
<form method="post" action="/admin/login">
<input name="username" placeholder="用户名" required>
<input name="password" type="password" placeholder="密码" required>
<button type="submit">登录</button>
</form></body></html>`

const tplReview = `<!DOCTYPE html>
<html lang="zh-CN"><head><meta charset="utf-8"><title>实名审核</title>
<style>body{font-family:system-ui;max-width:48rem;margin:2rem auto;padding:0 1rem}
table{width:100%;border-collapse:collapse}td,th{border:1px solid #ddd;padding:.5rem;text-align:left}
nav a{margin-right:1rem}.rj{color:#c00}</style></head><body>
<nav><a href="/admin/review">实名审核</a><a href="/admin/rooms">房间管理</a><a href="/admin/logout">退出</a></nav>
<h1>待审核用户（{{len .Users}}）</h1>
<table><tr><th>ID</th><th>用户名</th><th>昵称</th><th>班级</th><th>真实姓名</th><th>学号</th><th>操作</th></tr>
{{range .Users}}<tr>
<td>{{.ID}}</td><td>{{.Username}}</td><td>{{.Nickname}}</td><td>{{.ClassName}}</td><td>{{.RealName}}</td><td>{{.StudentNo}}</td>
<td><form method="post" action="/admin/review/{{.ID}}/approve" style="display:inline"><button>通过</button></form>
<form method="post" action="/admin/review/{{.ID}}/reject" style="display:inline">
<input name="reason" placeholder="拒绝原因" required><button class="rj">拒绝</button></form></td>
</tr>{{else}}<tr><td colspan="7">暂无待审核用户</td></tr>{{end}}
</table></body></html>`

const tplRooms = `<!DOCTYPE html>
<html lang="zh-CN"><head><meta charset="utf-8"><title>房间管理</title>
<style>body{font-family:system-ui;max-width:48rem;margin:2rem auto;padding:0 1rem}
table{width:100%;border-collapse:collapse}td,th{border:1px solid #ddd;padding:.5rem;text-align:left}
nav a{margin-right:1rem}.cls{color:#c00}</style></head><body>
<nav><a href="/admin/review">实名审核</a><a href="/admin/rooms">房间管理</a><a href="/admin/logout">退出</a></nav>
<h1>当前房间（{{len .Rooms}}）</h1>
<table><tr><th>ID</th><th>房间名</th><th>游戏</th><th>模式</th><th>房主</th><th>班级</th><th>操作</th></tr>
{{range .Rooms}}<tr><td>{{.ID}}</td><td>{{.Name}}</td><td>{{.Game}}</td><td>{{.Mode}}</td>
<td>{{.HostNickname}}</td><td>{{.HostClass}}</td>
<td><form method="post" action="/admin/rooms/{{.ID}}/close"><button class="cls">强制关闭</button></form></td>
</tr>{{else}}<tr><td colspan="7">当前没有房间</td></tr>{{end}}
</table></body></html>`

var (
        tplLoginV  = template.Must(template.New("login").Parse(tplLogin))
        tplReviewV = template.Must(template.New("review").Parse(tplReview))
        tplRoomsV  = template.Must(template.New("rooms").Parse(tplRooms))
)

// requireAdmin 管理员会话校验
func (a *App) requireAdmin(w http.ResponseWriter, r *http.Request) (int64, bool) {
        uid, ok := a.adminSessions.get(r)
        if !ok {
                http.Redirect(w, r, "/admin/login", http.StatusFound)
                return 0, false
        }
        u, err := a.store.GetUserByID(uid)
        if err != nil || u == nil || u.Role != "admin" {
                a.adminSessions.del(r)
                http.Redirect(w, r, "/admin/login", http.StatusFound)
                return 0, false
        }
        return uid, true
}

// handleAdminLogin GET+POST /admin/login
func (a *App) handleAdminLogin(w http.ResponseWriter, r *http.Request) {
        if r.Method == http.MethodGet {
                tplLoginV.Execute(w, map[string]any{"Error": ""})
                return
        }
        username := strings.TrimSpace(r.FormValue("username"))
        password := r.FormValue("password")
        u, err := a.store.GetUserByUsername(username)
        if err != nil || u == nil || u.Role != "admin" || !CheckPassword(u.PasswordHash, password, a.cfg.PassPepper) {
                tplLoginV.Execute(w, map[string]any{"Error": "用户名或密码错误"})
                return
        }
        if u.Status == "banned" {
                tplLoginV.Execute(w, map[string]any{"Error": "账号已被封禁"})
                return
        }
        http.SetCookie(w, &http.Cookie{
                Name: adminCookie, Value: a.adminSessions.newSession(u.ID),
                Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode,
        })
        http.Redirect(w, r, "/admin/review", http.StatusFound)
}

// handleAdminLogout GET /admin/logout
func (a *App) handleAdminLogout(w http.ResponseWriter, r *http.Request) {
        a.adminSessions.del(r)
        http.Redirect(w, r, "/admin/login", http.StatusFound)
}

// handleAdminReview GET /admin/review
func (a *App) handleAdminReview(w http.ResponseWriter, r *http.Request) {
        if _, ok := a.requireAdmin(w, r); !ok {
                return
        }
        users, err := a.store.ListUsersByStatus("pending")
        if err != nil {
                http.Error(w, "数据库错误", http.StatusInternalServerError)
                return
        }
        if users == nil {
                users = []*User{}
        }
        tplReviewV.Execute(w, map[string]any{"Users": users})
}

// handleAdminApprove POST /admin/review/{uid}/approve
func (a *App) handleAdminApprove(w http.ResponseWriter, r *http.Request) {
        adminUID, ok := a.requireAdmin(w, r)
        if !ok {
                return
        }
        uid, err := strconv.ParseInt(strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/admin/review/"), "/approve"), 10, 64)
        if err != nil {
                http.Error(w, "参数错误", http.StatusBadRequest)
                return
        }
        a.store.SetUserStatus(uid, "approved", "", adminUID)
        http.Redirect(w, r, "/admin/review", http.StatusFound)
}

// handleAdminReject POST /admin/review/{uid}/reject（表单带 reason）
func (a *App) handleAdminReject(w http.ResponseWriter, r *http.Request) {
        adminUID, ok := a.requireAdmin(w, r)
        if !ok {
                return
        }
        uid, err := strconv.ParseInt(strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/admin/review/"), "/reject"), 10, 64)
        if err != nil {
                http.Error(w, "参数错误", http.StatusBadRequest)
                return
        }
        reason := strings.TrimSpace(r.FormValue("reason"))
        if reason == "" {
                reason = "信息不符合要求"
        }
        a.store.SetUserStatus(uid, "rejected", reason, adminUID)
        http.Redirect(w, r, "/admin/review", http.StatusFound)
}

// handleAdminRooms GET /admin/rooms
func (a *App) handleAdminRooms(w http.ResponseWriter, r *http.Request) {
        if _, ok := a.requireAdmin(w, r); !ok {
                return
        }
        rooms, err := a.store.ListRooms()
        if err != nil {
                http.Error(w, "数据库错误", http.StatusInternalServerError)
                return
        }
        if rooms == nil {
                rooms = []*Room{}
        }
        tplRoomsV.Execute(w, map[string]any{"Rooms": rooms})
}

// handleAdminCloseRoom POST /admin/rooms/{id}/close
func (a *App) handleAdminCloseRoom(w http.ResponseWriter, r *http.Request) {
        if _, ok := a.requireAdmin(w, r); !ok {
                return
        }
        id, err := strconv.ParseInt(strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/admin/rooms/"), "/close"), 10, 64)
        if err != nil {
                http.Error(w, "参数错误", http.StatusBadRequest)
                return
        }
        room, err := a.store.GetRoom(id)
        if err == nil && room != nil {
                a.closeRoom(room)
        }
        http.Redirect(w, r, "/admin/rooms", http.StatusFound)
}
