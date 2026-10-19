package main

// 揭东一初联机大厅 服务端入口
// 环境变量见 README / docs/DEPLOY.md；管理员用 -createadmin 创建。

import (
        "context"
        "crypto/rand"
        "encoding/hex"
        "encoding/json"
        "flag"
        "fmt"
        "log"
        "net"
        "net/http"
        "os"
        "os/exec"
        "strconv"
        "strings"
        "unicode/utf8"
)

// Config 全部配置来自环境变量
type Config struct {
        Listen     string
        DBPath     string
        JWTSecret  string
        FrpsHost   string
        FrpsPort   int
        FrpsToken  string
        Supernode  string
        PassPepper string
        SweepSec   int
        HostDownSec int
}

func loadConfig() *Config {
        c := &Config{
                Listen:      envOr("JDYC_LISTEN", ":7100"),
                DBPath:      envOr("JDYC_DB", "jdyc.db"),
                JWTSecret:   os.Getenv("JDYC_JWT_SECRET"),
                FrpsHost:    envOr("JDYC_FRPS_HOST", "127.0.0.1"),
                FrpsToken:   envOr("JDYC_FRPS_TOKEN", ""),
                Supernode:   envOr("JDYC_SUPERNODE", "127.0.0.1:7777"),
                PassPepper:  envOr("JDYC_ADMIN_PASS_PEPPER", ""),
                SweepSec:    envIntOr("JDYC_SWEEP_SEC", defaultSweep),
                HostDownSec: envIntOr("JDYC_HOST_DOWN_SEC", defaultDown),
        }
        c.FrpsPort = envIntOr("JDYC_FRPS_PORT", 7000)
        if c.JWTSecret == "" {
                // 未配置密钥时随机生成并打印，重启后旧 token 失效
                b := make([]byte, 32)
                if _, err := rand.Read(b); err != nil {
                        log.Fatal("无法生成随机密钥，请显式设置 JDYC_JWT_SECRET")
                }
                c.JWTSecret = hex.EncodeToString(b)
                log.Printf("[警告] 未设置 JDYC_JWT_SECRET，已随机生成，重启后旧 token 失效")
        }
        return c
}

func envOr(k, def string) string {
        if v := os.Getenv(k); v != "" {
                return v
        }
        return def
}

func envIntOr(k string, def int) int {
        if v := os.Getenv(k); v != "" {
                if n, err := strconv.Atoi(v); err == nil {
                        return n
                }
        }
        return def
}

// App 应用聚合
type App struct {
        store          *Store
        cfg            *Config
        hub            *Hub
        adminSessions  *AdminSessions
        limiter        *RateLimiter
        superCmd       *exec.Cmd
}

func main() {
        createAdmin := flag.Bool("createadmin", false, "创建管理员：-createadmin 用户名 密码 班级 姓名")
        flag.Parse()

        cfg := loadConfig()

        // -createadmin 分支：建完即退出
        if *createAdmin {
                args := flag.Args()
                if len(args) != 4 {
                        log.Fatal("用法：-createadmin 用户名 密码 班级 姓名")
                }
                if err := runCreateAdmin(cfg, args[0], args[1], args[2], args[3]); err != nil {
                        log.Fatal("创建管理员失败：", err)
                }
                log.Printf("管理员 %s 创建成功", args[0])
                return
        }

        store, err := openStore(cfg.DBPath)
        if err != nil {
                log.Fatal("打开数据库失败：", err)
        }
        app := &App{
                store:         store,
                cfg:           cfg,
                hub:           NewHub(),
                adminSessions: NewAdminSessions(),
                limiter:       NewRateLimiter(),
        }
        app.hub.app = app

        // 尝试拉起 supernode（n2n），找不到二进制则仅记录
        app.spawnSupernode()

        go app.sweepLoop()

        mux := http.NewServeMux()
        mux.HandleFunc("/api/register", app.handleRegister)
        mux.HandleFunc("/api/login", app.handleLogin)
        mux.HandleFunc("/api/me", app.authRequired(app.handleMe))
        mux.HandleFunc("/api/rooms", app.authRequired(app.roomsHandler))
        mux.HandleFunc("/api/rooms/", app.authRequired(app.roomsHandler))
        mux.HandleFunc("/lobby", app.handleLobby)
        mux.HandleFunc("/admin/login", app.handleAdminLogin)
        mux.HandleFunc("/admin/logout", app.handleAdminLogout)
        mux.HandleFunc("/admin/review", app.handleAdminReview)
        mux.HandleFunc("/admin/review/", func(w http.ResponseWriter, r *http.Request) {
                switch {
                case strings.HasSuffix(r.URL.Path, "/approve"):
                        app.handleAdminApprove(w, r)
                case strings.HasSuffix(r.URL.Path, "/reject"):
                        app.handleAdminReject(w, r)
                default:
                        http.NotFound(w, r)
                }
        })
        mux.HandleFunc("/admin/rooms", app.handleAdminRooms)
        mux.HandleFunc("/admin/rooms/", app.handleAdminCloseRoom)
        mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
                writeJSON(w, http.StatusOK, map[string]string{"name": "揭东一初联机大厅", "ok": "true"})
        })

        log.Printf("服务端监听 %s", cfg.Listen)
        if err := http.ListenAndServe(cfg.Listen, mux); err != nil {
                log.Fatal("HTTP 服务失败：", err)
        }
}

// runCreateAdmin 创建管理员账号
func runCreateAdmin(cfg *Config, username, password, class, realname string) error {
        if !ValidateUsername(username) || !ValidatePassword(password) {
                return fmt.Errorf("用户名需 4-20 位字母数字下划线，密码需 8-64 位且含字母和数字")
        }
        hash, err := HashPassword(password, cfg.PassPepper)
        if err != nil {
                return err
        }
        store, err := openStore(cfg.DBPath)
        if err != nil {
                return err
        }
        defer store.DB.Close()
        return store.CreateUser(&User{
                Username: username, PasswordHash: hash, Nickname: realname,
                ClassName: class, RealName: realname, Role: "admin", Status: "approved",
        })
}

// spawnSupernode 拉起 n2n supernode 子进程（存在才拉）
func (a *App) spawnSupernode() {
        path, err := exec.LookPath("supernode")
        if err != nil {
                log.Printf("[提示] 未找到 supernode，虚拟局域网功能将在部署层单独运行（见 docs/DEPLOY.md）")
                return
        }
        // 从 JDYC_SUPERNODE 解析监听端口
        _, port, err := net.SplitHostPort(a.cfg.Supernode)
        if err != nil {
                port = "7777"
        }
        cmd := exec.Command(path, "-l", port) // 参数数组启动，不经 shell
        cmd.Stdout = os.Stdout
        cmd.Stderr = os.Stderr
        if err := cmd.Start(); err != nil {
                log.Printf("[提示] supernode 启动失败：%v", err)
                return
        }
        a.superCmd = cmd
        go func() {
                cmd.Wait()
        }()
        log.Printf("supernode 已启动，监听 %s", port)
}

// ---- REST 处理器 ----

type ctxKeyType int

const ctxUser ctxKeyType = 1

// authRequired JWT 鉴权中间件
func (a *App) authRequired(next http.HandlerFunc) http.HandlerFunc {
        return func(w http.ResponseWriter, r *http.Request) {
                auth := r.Header.Get("Authorization")
                if !strings.HasPrefix(auth, "Bearer ") {
                        writeErr(w, http.StatusUnauthorized, "unauthorized", "请先登录")
                        return
                }
                claims, err := ParseJWT(strings.TrimPrefix(auth, "Bearer "), a.cfg.JWTSecret)
                if err != nil {
                        writeErr(w, http.StatusUnauthorized, "unauthorized", "登录状态无效")
                        return
                }
                u, err := a.store.GetUserByID(claims.UID)
                if err != nil || u == nil {
                        writeErr(w, http.StatusUnauthorized, "unauthorized", "用户不存在")
                        return
                }
                next(w, r.WithContext(context.WithValue(r.Context(), ctxUser, u)))
        }
}

// handleRegister POST /api/register
func (a *App) handleRegister(w http.ResponseWriter, r *http.Request) {
        var body struct {
                Username   string `json:"username"`
                Password   string `json:"password"`
                Nickname   string `json:"nickname"`
                ClassName  string `json:"class_name"`
                RealName   string `json:"real_name"`
                StudentNo  string `json:"student_no"`
        }
        if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
                writeErr(w, http.StatusBadRequest, "invalid_params", "请求体格式错误")
                return
        }
        switch {
        case !ValidateUsername(body.Username):
                writeErr(w, http.StatusBadRequest, "invalid_params", "用户名需 4-20 位字母、数字或下划线")
                return
        case !ValidatePassword(body.Password):
                writeErr(w, http.StatusBadRequest, "invalid_params", "密码需 8-64 位且同时包含字母和数字")
                return
        case !ValidateClassName(body.ClassName):
                writeErr(w, http.StatusBadRequest, "invalid_params", "班级不能超过 20 字")
                return
        case !ValidateRealName(body.RealName):
                writeErr(w, http.StatusBadRequest, "invalid_params", "真实姓名需 2-4 个汉字")
                return
        case !ValidateNickname(body.Nickname):
                writeErr(w, http.StatusBadRequest, "invalid_params", "昵称需 1-16 字")
                return
        }
        hash, err := HashPassword(body.Password, a.cfg.PassPepper)
        if err != nil {
                writeErr(w, http.StatusInternalServerError, "server_error", "服务器内部错误")
                return
        }
        err = a.store.CreateUser(&User{
                Username: body.Username, PasswordHash: hash, Nickname: body.Nickname,
                ClassName: body.ClassName, RealName: body.RealName, StudentNo: body.StudentNo,
                Role: "user", Status: "pending",
        })
        if err == ErrUsernameExists {
                writeErr(w, http.StatusConflict, "username_exists", "用户名已被注册")
                return
        }
        if err != nil {
                writeErr(w, http.StatusInternalServerError, "server_error", "服务器内部错误")
                return
        }
        writeJSON(w, http.StatusCreated, map[string]any{"ok": true})
}

// handleLogin POST /api/login（同 IP 每分钟 5 次）
func (a *App) handleLogin(w http.ResponseWriter, r *http.Request) {
        ip, _, err := net.SplitHostPort(r.RemoteAddr)
        if err != nil {
                ip = r.RemoteAddr
        }
        if !a.limiter.Allow(ip) {
                writeErr(w, http.StatusTooManyRequests, "rate_limited", "登录过于频繁，请稍后再试")
                return
        }
        var body struct {
                Username string `json:"username"`
                Password string `json:"password"`
        }
        if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
                writeErr(w, http.StatusBadRequest, "invalid_params", "请求体格式错误")
                return
        }
        u, err := a.store.GetUserByUsername(body.Username)
        if err != nil || u == nil || !CheckPassword(u.PasswordHash, body.Password, a.cfg.PassPepper) {
                writeErr(w, http.StatusUnauthorized, "wrong_password", "用户名或密码错误")
                return
        }
        if u.Status == "banned" {
                writeErr(w, http.StatusForbidden, "banned", "账号已被封禁")
                return
        }
        token, err := IssueJWT(u.ID, u.Role, a.cfg.JWTSecret)
        if err != nil {
                writeErr(w, http.StatusInternalServerError, "server_error", "服务器内部错误")
                return
        }
        writeJSON(w, http.StatusOK, map[string]any{"token": token, "user": u})
}

// handleMe GET /api/me
func (a *App) handleMe(w http.ResponseWriter, r *http.Request) {
        u := r.Context().Value(ctxUser).(*User)
        writeJSON(w, http.StatusOK, u)
}

// roomsHandler GET/POST /api/rooms 与 DELETE /api/rooms/{id}
func (a *App) roomsHandler(w http.ResponseWriter, r *http.Request) {
        if r.URL.Path != "/api/rooms" {
                // /api/rooms/{id} 仅支持 DELETE
                if r.Method == http.MethodDelete {
                        a.handleDeleteRoom(w, r)
                        return
                }
                writeErr(w, http.StatusNotFound, "not_found", "接口不存在")
                return
        }
        switch r.Method {
        case http.MethodGet:
                a.handleListRooms(w, r)
        case http.MethodPost:
                a.handleCreateRoom(w, r)
        default:
                writeErr(w, http.StatusMethodNotAllowed, "bad_method", "不支持的请求方法")
        }
}

// ---- 通用工具 ----

func writeJSON(w http.ResponseWriter, status int, v any) {
        w.Header().Set("Content-Type", "application/json; charset=utf-8")
        w.WriteHeader(status)
        json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, code, msg string) {
        writeJSON(w, status, map[string]string{"error": code, "msg": msg})
}

func runeCount(s string) int { return utf8.RuneCountInString(s) }
