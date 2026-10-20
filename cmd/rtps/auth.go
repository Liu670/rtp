package main

import (
	"github.com/Liu670/rtp/internal/core"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"runtime"
	"strconv"
	"time"
)

// RtpVersion 当前版本号，展示在控制台「关于」中
const RtpVersion = "1.5.0"

const sessionCookie = "rtp_session"
const sessionMaxAge = 7 * 24 * 3600 // 会话有效期（秒）

type sessionInfo struct {
	user    string
	expires time.Time
}

// hashPass sha256(salt:pass)，std-only，足够管理后台本地防护
func hashPass(salt, pass string) string {
	h := sha256.Sum256([]byte(salt + ":" + pass))
	return hex.EncodeToString(h[:])
}

// authed 统一鉴权：管理员令牌 或 有效登录会话，二者其一即可
func (s *Server) authed(r *http.Request) bool {
	if s.tokenOK(r) {
		return true
	}
	_, ok := s.sessionUser(r)
	return ok
}

// sessionUser 从 cookie 解析登录会话
func (s *Server) sessionUser(r *http.Request) (string, bool) {
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return "", false
	}
	s.sessMu.Lock()
	defer s.sessMu.Unlock()
	si, ok := s.sessions[c.Value]
	if !ok {
		return "", false
	}
	if time.Now().After(si.expires) {
		delete(s.sessions, c.Value)
		return "", false
	}
	return si.user, true
}

func (s *Server) newSession(user string) string {
	sid := core.RandID()
	s.sessMu.Lock()
	s.sessions[sid] = sessionInfo{user: user, expires: time.Now().Add(time.Duration(sessionMaxAge) * time.Second)}
	s.sessMu.Unlock()
	return sid
}

func (s *Server) dropSession(sid string) {
	s.sessMu.Lock()
	delete(s.sessions, sid)
	s.sessMu.Unlock()
}

// ---------- 登录 / 登出 / 当前身份 ----------

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "请求格式错误"})
		return
	}
	if in.Username == "" || in.Password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "请输入用户名和密码"})
		return
	}
	s.mu.Lock()
	user, salt, hash := s.cfg.AdminUser, s.cfg.AdminSalt, s.cfg.AdminPassHash
	s.mu.Unlock()
	if user == "" || in.Username != user || hashPass(salt, in.Password) != hash {
		time.Sleep(300 * time.Millisecond) // 轻微延迟，减缓爆破
		writeJSON(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "用户名或密码错误"})
		return
	}
	sid := s.newSession(user)
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: sid, Path: "/",
		HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: sessionMaxAge,
	})
	core.LogEvent("admin", "ok", fmt.Sprintf("管理后台登录: %s (%s)", user, r.RemoteAddr))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "user": user})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		s.dropSession(c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", HttpOnly: true, MaxAge: -1})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleMe 无需登录：前端据此判断展示登录页还是控制台
func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	resp := map[string]any{
		"authed":       false,
		"via":          "",
		"user":         "",
		"announcement": "",
		"version":      RtpVersion,
		"default_pass": false,
	}
	s.mu.Lock()
	resp["announcement"] = s.cfg.Announcement
	user := s.cfg.AdminUser
	resp["default_pass"] = s.cfg.AdminPassHash == hashPass(s.cfg.AdminSalt, "admin123")
	s.mu.Unlock()
	if s.tokenOK(r) {
		resp["authed"] = true
		resp["via"] = "token"
		resp["user"] = user
	} else if u, ok := s.sessionUser(r); ok {
		resp["authed"] = true
		resp["via"] = "session"
		resp["user"] = u
	}
	writeJSON(w, http.StatusOK, resp)
}

// ---------- 运行状态 ----------

func humanDur(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%d 秒", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%d 分 %d 秒", int(d.Minutes()), int(d.Seconds())%60)
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%d 小时 %d 分", int(d.Hours()), int(d.Minutes())%60)
	}
	return fmt.Sprintf("%d 天 %d 小时", int(d.Hours())/24, int(d.Hours())%24)
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	if !s.authed(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	s.mu.Lock()
	nClients, nTunnels, online := len(s.cfg.Clients), len(s.cfg.Tunnels), 0
	s.mu.Unlock()
	for _, cs := range s.clients {
		if cs.online {
			online++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"version":      RtpVersion,
		"go_version":   runtime.Version(),
		"uptime_sec":   int(time.Since(s.startTime).Seconds()),
		"uptime_human": humanDur(time.Since(s.startTime)),
		"goroutines":   runtime.NumGoroutine(),
		"mem_alloc_mb": fmt.Sprintf("%.1f", float64(m.HeapAlloc)/1024/1024),
		"mem_sys_mb":   fmt.Sprintf("%.1f", float64(m.Sys)/1024/1024),
		"gc_count":     int(m.NumGC),
		"clients":      nClients,
		"tunnels":      nTunnels,
		"online":       online,
	})
}

// ---------- 连通性测试（从服务端发起 TCP 探测） ----------

func (s *Server) handleProbe(w http.ResponseWriter, r *http.Request) {
	if !s.authed(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var in struct {
		Host string `json:"host"`
		Port int    `json:"port"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "请求格式错误"})
		return
	}
	if len(in.Host) == 0 || len(in.Host) > 253 || in.Port <= 0 || in.Port > 65535 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "主机或端口不合法"})
		return
	}
	addr := net.JoinHostPort(in.Host, strconv.Itoa(in.Port))
	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	ms := time.Since(start).Milliseconds()
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "ms": ms, "addr": addr, "error": err.Error()})
		return
	}
	conn.Close()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "ms": ms, "addr": addr})
}

// ---------- 配置备份 / 恢复 ----------

func (s *Server) handleBackup(w http.ResponseWriter, r *http.Request) {
	if !s.authed(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	s.mu.Lock()
	data, _ := json.MarshalIndent(s.cfg, "", "  ")
	s.mu.Unlock()
	name := "rtp-config-" + time.Now().Format("20060102-150405") + ".json"
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+name+"\"")
	w.Write(data)
}

// handleRestore 用上传的配置覆盖 客户端/隧道/公告/域名/账号；端口类变更仅落盘，重启后生效
func (s *Server) handleRestore(w http.ResponseWriter, r *http.Request) {
	if !s.authed(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var nc Config
	if err := json.NewDecoder(r.Body).Decode(&nc); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "JSON 解析失败: " + err.Error()})
		return
	}
	if nc.ControlPort <= 0 || nc.WorkPort <= 0 || nc.WebPort <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "配置缺少必要端口（control_port/work_port/web_port）"})
		return
	}
	restartRequired := false
	s.mu.Lock()
	if nc.ControlPort != s.cfg.ControlPort || nc.WorkPort != s.cfg.WorkPort ||
		nc.WebPort != s.cfg.WebPort || nc.P2pPort != s.cfg.P2pPort ||
		nc.HttpPort != s.cfg.HttpPort || nc.HttpsPort != s.cfg.HttpsPort {
		restartRequired = true
	}
	if nc.AdminToken == "" {
		nc.AdminToken = s.cfg.AdminToken
	}
	if nc.AdminUser == "" || nc.AdminPassHash == "" || nc.AdminSalt == "" {
		nc.AdminUser, nc.AdminSalt, nc.AdminPassHash = s.cfg.AdminUser, s.cfg.AdminSalt, s.cfg.AdminPassHash
	}
	// 停掉现有隧道监听
	for id, t := range s.tunnels {
		if t.listener != nil {
			t.listener.Close()
		}
		if t.udpConn != nil {
			t.udpConn.Close()
		}
		delete(s.tunnels, id)
	}
	s.cfg.Clients = nc.Clients
	s.cfg.Tunnels = nc.Tunnels
	s.cfg.BaseDomain = nc.BaseDomain
	s.cfg.Announcement = nc.Announcement
	s.cfg.ControlPort, s.cfg.WorkPort, s.cfg.WebPort = nc.ControlPort, nc.WorkPort, nc.WebPort
	s.cfg.HttpPort, s.cfg.HttpsPort, s.cfg.P2pPort = nc.HttpPort, nc.HttpsPort, nc.P2pPort
	s.cfg.AdminToken, s.cfg.AdminUser, s.cfg.AdminSalt, s.cfg.AdminPassHash = nc.AdminToken, nc.AdminUser, nc.AdminSalt, nc.AdminPassHash
	tunnels := make([]TunnelConfig, len(nc.Tunnels))
	copy(tunnels, nc.Tunnels)
	s.mu.Unlock()

	s.buildClients()
	s.rebuildRoutes()
	failed := 0
	for _, tc := range tunnels {
		if !tc.Enabled {
			continue
		}
		if err := s.addTunnel(tc); err != nil {
			failed++
			log.Printf("[restore] 隧道 %s 启动失败: %v", tc.Name, err)
		}
	}
	s.saveConfig()
	core.LogEvent("admin", "ok", fmt.Sprintf("配置已恢复: %d 客户端 / %d 隧道", len(nc.Clients), len(nc.Tunnels)))
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "clients": len(nc.Clients), "tunnels": len(nc.Tunnels),
		"failed": failed, "restart_required": restartRequired,
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
