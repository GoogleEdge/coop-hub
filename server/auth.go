package main

// 鉴权：argon2id 密码哈希、JWT 签发校验、登录限速、参数校验。

import (
        "errors"
        "regexp"
        "sync"
        "time"
        "unicode/utf8"

        "github.com/alexedwards/argon2id"
        "github.com/golang-jwt/jwt/v5"
)

var (
        reUsername  = regexp.MustCompile(`^[a-zA-Z0-9_]{4,20}$`)
        reHasLetter = regexp.MustCompile(`[a-zA-Z]`)
        reHasDigit  = regexp.MustCompile(`[0-9]`)
        reHanzi     = regexp.MustCompile(`^[\p{Han}]{2,4}$`)
)

// ValidateUsername 4-20 位字母数字下划线
func ValidateUsername(s string) bool { return reUsername.MatchString(s) }

// ValidatePassword 8-64 位且同时含字母与数字
func ValidatePassword(s string) bool {
        if l := utf8.RuneCountInString(s); l < 8 || l > 64 {
                return false
        }
        return reHasLetter.MatchString(s) && reHasDigit.MatchString(s)
}

// ValidateClassName 班级 ≤20 字
func ValidateClassName(s string) bool { return utf8.RuneCountInString(s) >= 1 && utf8.RuneCountInString(s) <= 20 }

// ValidateRealName 2-4 个汉字
func ValidateRealName(s string) bool { return reHanzi.MatchString(s) }

// ValidateNickname 1-16 字
func ValidateNickname(s string) bool {
        l := utf8.RuneCountInString(s)
        return l >= 1 && l <= 16
}

// HashPassword 加盐 pepper 后做 argon2id
func HashPassword(password, pepper string) (string, error) {
        return argon2id.CreateHash(password+pepper, argon2id.DefaultParams)
}

// CheckPassword 校验密码
func CheckPassword(hash, password, pepper string) bool {
        ok, err := argon2id.ComparePasswordAndHash(password+pepper, hash)
        return err == nil && ok
}

// Claims JWT claims：uid、role
type Claims struct {
        UID  int64  `json:"uid"`
        Role string `json:"role"`
        jwt.RegisteredClaims
}

// IssueJWT 签发 7 天有效 HS256 token
func IssueJWT(uid int64, role, secret string) (string, error) {
        claims := Claims{
                UID:  uid,
                Role: role,
                RegisteredClaims: jwt.RegisteredClaims{
                        ExpiresAt: jwt.NewNumericDate(time.Now().Add(7 * 24 * time.Hour)),
                        IssuedAt:  jwt.NewNumericDate(time.Now()),
                },
        }
        return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
}

// ParseJWT 校验并解析 token
func ParseJWT(tokenStr, secret string) (*Claims, error) {
        tok, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
                if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
                        return nil, errors.New("bad signing method")
                }
                return []byte(secret), nil
        })
        if err != nil || !tok.Valid {
                return nil, errors.New("invalid token")
        }
        claims, ok := tok.Claims.(*Claims)
        if !ok {
                return nil, errors.New("invalid claims")
        }
        return claims, nil
}

// RateLimiter 登录限速：同 IP 每分钟 5 次（滑动窗口，内存 map）
type RateLimiter struct {
        mu sync.Mutex
        m  map[string][]time.Time
}

func NewRateLimiter() *RateLimiter {
        return &RateLimiter{m: map[string][]time.Time{}}
}

// Allow 判断该 IP 本分钟内是否还允许登录
func (r *RateLimiter) Allow(ip string) bool {
        now := time.Now()
        r.mu.Lock()
        defer r.mu.Unlock()
        win := r.m[ip]
        fresh := win[:0]
        for _, t := range win {
                if now.Sub(t) < time.Minute {
                        fresh = append(fresh, t)
                }
        }
        if len(fresh) >= 5 {
                r.m[ip] = fresh
                return false
        }
        r.m[ip] = append(fresh, now)
        return true
}
