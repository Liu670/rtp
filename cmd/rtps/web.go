package main

import (
	"bytes"
	"github.com/Liu670/rtp/internal/core"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

func (s *Server) StartWeb(addr string) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	// 品牌 Logo（公开，登录页也需要）
	mux.HandleFunc("/logo.svg", s.handleLogo)
	mux.HandleFunc("/favicon.ico", s.handleLogo)
	mux.HandleFunc("/api/overview", s.handleOverview)
	mux.HandleFunc("/api/clients", s.handleClients)
	mux.HandleFunc("/api/tunnels", s.handleTunnels)
	mux.HandleFunc("/api/traffic", s.handleTraffic)
	mux.HandleFunc("/api/traffic/summary", s.handleTrafficSummary)
	mux.HandleFunc("/api/traffic/breakdown", s.handleTrafficBreakdown)
	mux.HandleFunc("/api/events", s.handleEvents)
	mux.HandleFunc("/api/wizard", s.handleWizard)
	mux.HandleFunc("/api/stream", s.handleStream)
	mux.HandleFunc("/api/settings", s.handleSettings)
	mux.HandleFunc("/api/proxy", s.handleProxy)
	mux.HandleFunc("/api/proxy/clash", s.handleProxyClash)
	mux.HandleFunc("/api/proxy/geo", s.handleProxyGeo)
	mux.HandleFunc("/api/client_cmd", s.handleClientCmd)
	mux.HandleFunc("/api/download", s.handleDownload)
	// 账号密码鉴权
	mux.HandleFunc("/api/login", s.handleLogin)
	mux.HandleFunc("/api/logout", s.handleLogout)
	mux.HandleFunc("/api/me", s.handleMe)
	// 运行状态 / 工具
	mux.HandleFunc("/api/stats", s.handleStats)
	mux.HandleFunc("/api/probe", s.handleProbe)
	mux.HandleFunc("/api/backup", s.handleBackup)
	mux.HandleFunc("/api/restore", s.handleRestore)
	// 带 ID 的子路由
	mux.HandleFunc("/api/clients/", s.handleClientOne)
	mux.HandleFunc("/api/tunnels/", s.handleTunnelOne)
	mux.HandleFunc("/api/webhooks/", s.handleWebhookOne)
	mux.HandleFunc("/api/webhooks", s.handleWebhooks)
	log.Printf("[web] 控制台 http://0.0.0.0%s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("[web] 启动失败: %v", err)
	}
}

func (s *Server) tokenOK(r *http.Request) bool {
	tok := r.URL.Query().Get("token")
	if tok == "" {
		tok = r.Header.Get("X-Token")
	}
	if tok == "" {
		if c, err := r.Cookie("rtp_token"); err == nil {
			tok = c.Value
		}
	}
	return tok == s.cfg.AdminToken
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	// 带 token 的直达链接：写入 HttpOnly cookie 后跳转到干净地址，之后免登录
	if tok := r.URL.Query().Get("token"); tok != "" {
		if tok == s.cfg.AdminToken {
			http.SetCookie(w, &http.Cookie{
				Name: "rtp_token", Value: tok, Path: "/",
				HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 30 * 24 * 3600,
			})
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, uiHTML, s.cfg.WebPort, s.cfg.ControlPort, s.cfg.WorkPort, s.cfg.HttpPort, s.cfg.HttpsPort, s.cfg.P2pPort)
}

// ---------- 概览 ----------

type clientView struct {
	ClientConfig
	Online     bool   `json:"online"`
	RemoteAddr string `json:"remote_addr"`
	Up         int64  `json:"up"`
	Down       int64  `json:"down"`
	UpRate     int64  `json:"up_rate"`
	DownRate   int64  `json:"down_rate"`
	Conns      int    `json:"conns"`
}

type tunnelView struct {
	TunnelConfig
	ClientName string `json:"client_name"`
	Up         int64  `json:"up"`
	Down       int64  `json:"down"`
	Online     bool   `json:"online"`
}

func (s *Server) handleOverview(w http.ResponseWriter, r *http.Request) {
	if !s.authed(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	s.controlsMu.Lock()
	clients := make([]clientView, 0, len(s.clients))
	totalUp, totalDown := int64(0), int64(0)
	for _, cs := range s.clients {
		cs.mu.Lock()
		up, down, _ := cs.traffic.Snapshot()
		cv := clientView{
			ClientConfig: cs.ClientConfig,
			Online:       cs.online,
			RemoteAddr:   cs.remoteAddr,
			Up:           up, Down: down,
		}
		cs.mu.Unlock()
		clients = append(clients, cv)
		totalUp += up
		totalDown += down
	}
	s.controlsMu.Unlock()

	s.mu.Lock()
	tunnels := make([]tunnelView, 0, len(s.tunnels))
	for _, t := range s.tunnels {
		up, down, _ := t.traffic.Snapshot()
		tc := t.TunnelConfig
		if tc.Proto == "p2p" {
			tc.P2PMode = s.getP2PState(t.ID)
		}
		tunnels = append(tunnels, tunnelView{
			TunnelConfig: tc,
			ClientName:   s.clientName(t.ClientID),
			Up:           up, Down: down,
			Online:       s.clientOnline(t.ClientID),
		})
	}
	s.mu.Unlock()

	onlineClients := 0
	for _, c := range clients {
		if c.Online {
			onlineClients++
		}
	}
	json.NewEncoder(w).Encode(map[string]any{
		"admin_token": s.cfg.AdminToken,
		"clients":     clients,
		"tunnels":     tunnels,
		"online_clients": onlineClients,
		"total_clients":  len(clients),
		"total_tunnels":  len(tunnels),
		"total_up":       totalUp,
		"total_down":     totalDown,
		"server": map[string]any{
			"control_port": s.cfg.ControlPort,
			"work_port":    s.cfg.WorkPort,
			"web_port":     s.cfg.WebPort,
			"http_port":    s.cfg.HttpPort,
			"https_port":   s.cfg.HttpsPort,
			"base_domain":  s.cfg.BaseDomain,
		},
	})
}

func (s *Server) clientName(id string) string {
	if cs, ok := s.clients[id]; ok {
		return cs.Name
	}
	return id
}
func (s *Server) clientOnline(id string) bool {
	if cs, ok := s.clients[id]; ok {
		return cs.online
	}
	return false
}

// ---------- 客户端 CRUD ----------

func (s *Server) handleClients(w http.ResponseWriter, r *http.Request) {
	if !s.authed(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.controlsMu.Lock()
		list := make([]clientView, 0, len(s.clients))
		for _, cs := range s.clients {
			cs.mu.Lock()
			up, down, _ := cs.traffic.Snapshot()
			list = append(list, clientView{
				ClientConfig: cs.ClientConfig,
				Online:       cs.online,
				RemoteAddr:   cs.remoteAddr,
				Up:           up, Down: down,
			})
			cs.mu.Unlock()
		}
		s.controlsMu.Unlock()
		json.NewEncoder(w).Encode(list)
	case http.MethodPost:
		var in ClientConfig
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		if in.Name == "" {
			in.Name = "客户端"
		}
		if in.Token != "" {
			if !validTokenFormat(in.Token) {
				http.Error(w, "自定义令牌需为 8-64 位字母、数字、- 或 _", http.StatusBadRequest)
				return
			}
			if !s.tokenFree(in.Token, "") {
				http.Error(w, "令牌已被占用（其他客户端或管理员令牌）", http.StatusBadRequest)
				return
			}
		} else {
			in.Token = core.RandID() + core.RandID()
		}
		in.ID = "c-" + core.RandID()[:8]
		in.CreatedAt = time.Now().Unix()
		s.controlsMu.Lock()
		s.clients[in.ID] = &ClientState{ClientConfig: in, traffic: core.NewTraffic(), th: core.NewThrottle(int64(in.BwLimit) * 1024 * 1024 / 8)}
		s.cfg.Clients = append(s.cfg.Clients, in)
		s.controlsMu.Unlock()
		s.saveConfig()
		core.LogEvent("client_create", "ok", "创建客户端: "+in.Name)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(in)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleClientOne(w http.ResponseWriter, r *http.Request) {
	if !s.authed(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/clients/")
	switch r.Method {
	case http.MethodDelete:
		s.controlsMu.Lock()
		delete(s.clients, id)
		for i, c := range s.cfg.Clients {
			if c.ID == id {
				s.cfg.Clients = append(s.cfg.Clients[:i], s.cfg.Clients[i+1:]...)
				break
			}
		}
		s.controlsMu.Unlock()
		// 级联删除该客户端名下的隧道（removeTunnel 内部自行加锁，避免锁嵌套）
		s.mu.Lock()
		var tids []string
		for _, t := range s.tunnels {
			if t.ClientID == id {
				tids = append(tids, t.ID)
			}
		}
		s.mu.Unlock()
		for _, tid := range tids {
			_ = s.removeTunnel(tid)
		}
		s.saveConfig()
		w.WriteHeader(http.StatusNoContent)
	case http.MethodPut:
		var in struct {
			Name    *string `json:"name"`
			Note    *string `json:"note"`
			BwLimit *int    `json:"bw_limit_mbps"`
			Token   *string `json:"token"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		if in.Token != nil && *in.Token != "" {
			if !validTokenFormat(*in.Token) {
				http.Error(w, "自定义令牌需为 8-64 位字母、数字、- 或 _", http.StatusBadRequest)
				return
			}
			if !s.tokenFree(*in.Token, id) {
				http.Error(w, "令牌已被占用（其他客户端或管理员令牌）", http.StatusBadRequest)
				return
			}
		}
		s.controlsMu.Lock()
		cs, ok := s.clients[id]
		tokenChanged := false
		cname := ""
		if ok {
			cname = cs.Name
			if in.Name != nil && *in.Name != "" {
				cs.Name = *in.Name
			}
			if in.Note != nil {
				cs.Note = *in.Note
			}
			if in.BwLimit != nil && *in.BwLimit >= 0 {
				cs.BwLimit = *in.BwLimit
				cs.th.SetRate(int64(*in.BwLimit) * 1024 * 1024 / 8)
			}
			if in.Token != nil && *in.Token != "" && *in.Token != cs.Token {
				cs.Token = *in.Token
				tokenChanged = true
			}
		}
		for i := range s.cfg.Clients {
			if s.cfg.Clients[i].ID == id {
				if in.Name != nil && *in.Name != "" {
					s.cfg.Clients[i].Name = *in.Name
				}
				if in.Note != nil {
					s.cfg.Clients[i].Note = *in.Note
				}
				if in.BwLimit != nil && *in.BwLimit >= 0 {
					s.cfg.Clients[i].BwLimit = *in.BwLimit
				}
				if in.Token != nil && *in.Token != "" {
					s.cfg.Clients[i].Token = *in.Token
				}
				break
			}
		}
		s.controlsMu.Unlock()
		if !ok {
			http.Error(w, "客户端不存在", http.StatusNotFound)
			return
		}
		if tokenChanged {
			core.LogEvent("client_update", "warn", "客户端令牌已修改: "+cname+"（旧令牌将在其重连后失效）")
		}
		s.saveConfig()
		s.controlsMu.Lock()
		updated := cs.ClientConfig
		s.controlsMu.Unlock()
		json.NewEncoder(w).Encode(updated)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// ---------- 隧道 CRUD ----------

func (s *Server) handleTunnels(w http.ResponseWriter, r *http.Request) {
	if !s.authed(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.mu.Lock()
		list := make([]TunnelConfig, 0, len(s.tunnels))
		for _, t := range s.tunnels {
			list = append(list, t.TunnelConfig)
		}
		s.mu.Unlock()
		json.NewEncoder(w).Encode(list)
	case http.MethodPost:
		var in TunnelConfig
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		if in.Proto == "" {
			in.Proto = "tcp"
		}
		if in.LocalHost == "" {
			in.LocalHost = "127.0.0.1"
		}
		if in.ClientID == "" {
			in.ClientID = "default"
		}
		if !s.clientExists(in.ClientID) {
			http.Error(w, "客户端不存在", http.StatusBadRequest)
			return
		}
		if in.Proto == "p2p" {
			if in.VisitorClientID == "" {
				in.VisitorClientID = in.ClientID
			}
			if in.VisitorPort == 0 {
				http.Error(w, "P2P 隧道需要填写访问端本地端口(visitor_port)", http.StatusBadRequest)
				return
			}
			if !s.clientExists(in.VisitorClientID) {
				http.Error(w, "访问端客户端不存在", http.StatusBadRequest)
				return
			}
		}
		in.ID = "t-" + core.RandID()[:8]
		if in.Proto == "stcp" && in.Secret == "" {
			in.Secret = core.RandID()[:12]
		}
		in.Enabled = true
		if err := s.addTunnel(in); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(in)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) clientExists(id string) bool {
	s.controlsMu.Lock()
	defer s.controlsMu.Unlock()
	_, ok := s.clients[id]
	return ok
}

// validTokenFormat 令牌格式：8-64 位字母/数字/-/_
func validTokenFormat(t string) bool {
	if len(t) < 8 || len(t) > 64 {
		return false
	}
	for _, c := range t {
		ok := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_'
		if !ok {
			return false
		}
	}
	return true
}

// tokenFree 检查令牌未被其他客户端占用，也不与管理员令牌冲突；excludeID 用于编辑自身
func (s *Server) tokenFree(tok, excludeID string) bool {
	if tok == s.cfg.AdminToken {
		return false
	}
	s.controlsMu.Lock()
	defer s.controlsMu.Unlock()
	for id, c := range s.clients {
		if id != excludeID && c.Token == tok {
			return false
		}
	}
	return true
}

func (s *Server) handleTunnelOne(w http.ResponseWriter, r *http.Request) {
	if !s.authed(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/tunnels/")
	switch r.Method {
	case http.MethodDelete:
		if err := s.removeTunnel(id); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case http.MethodPut:
		var in struct {
			Name            *string `json:"name"`
			Note            *string `json:"note"`
			Enabled         *bool   `json:"enabled"`
			LocalHost       *string `json:"local_host"`
			LocalPort       *int    `json:"local_port"`
			PublicPort      *int    `json:"public_port"`
			Domain          *string `json:"domain"`
			Subdomain       *string `json:"subdomain"`
			Secret          *string `json:"secret"`
			VisitorClientID *string `json:"visitor_client_id"`
			VisitorPort     *int    `json:"visitor_port"`
			Reload          bool    `json:"reload"` // true = 强制重建监听/重新打洞
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		s.mu.Lock()
		t, ok := s.tunnels[id]
		if !ok {
			s.mu.Unlock()
			http.Error(w, "隧道不存在", http.StatusNotFound)
			return
		}
		oldPP := t.PublicPort
		wasListening := t.listener != nil || t.udpConn != nil
		if in.Name != nil && *in.Name != "" {
			t.Name = *in.Name
		}
		if in.Note != nil {
			t.Note = *in.Note
		}
		if in.LocalHost != nil && *in.LocalHost != "" {
			t.LocalHost = *in.LocalHost
		}
		if in.LocalPort != nil && *in.LocalPort > 0 {
			t.LocalPort = *in.LocalPort
		}
		if in.PublicPort != nil && *in.PublicPort >= 0 {
			t.PublicPort = *in.PublicPort
		}
		if in.Domain != nil {
			t.Domain = *in.Domain
		}
		if in.Subdomain != nil {
			t.Subdomain = *in.Subdomain
		}
		if in.Secret != nil && *in.Secret != "" {
			t.Secret = *in.Secret
		}
		if in.VisitorClientID != nil && *in.VisitorClientID != "" {
			t.VisitorClientID = *in.VisitorClientID
		}
		if in.VisitorPort != nil && *in.VisitorPort > 0 {
			t.VisitorPort = *in.VisitorPort
		}
		if in.Enabled != nil {
			t.Enabled = *in.Enabled
		}
		// 公网端口占用检查（排除自身）
		if (t.Proto == "tcp" || t.Proto == "stcp" || t.Proto == "udp") && t.PublicPort != 0 {
			for _, e := range s.tunnels {
				if e.ID != id && (e.Proto == "tcp" || e.Proto == "stcp" || e.Proto == "udp") && e.PublicPort == t.PublicPort {
					s.mu.Unlock()
					http.Error(w, fmt.Sprintf("公网端口 %d 已被其他隧道占用", t.PublicPort), http.StatusBadRequest)
					return
				}
			}
		}
		visitorID := t.VisitorClientID
		portChanged := oldPP != t.PublicPort
		needListen := t.Enabled && (t.Proto == "tcp" || t.Proto == "stcp" || t.Proto == "udp") &&
			(in.Reload || portChanged || !wasListening)
		needP2PRestart := t.Enabled && t.Proto == "p2p" && (in.Reload || in.VisitorClientID != nil || in.VisitorPort != nil)
		// 写回配置
		for i := range s.cfg.Tunnels {
			if s.cfg.Tunnels[i].ID == id {
				s.cfg.Tunnels[i] = t.TunnelConfig
				break
			}
		}
		if !t.Enabled || needListen {
			if t.listener != nil {
				t.listener.Close()
				t.listener = nil
			}
			if t.udpConn != nil {
				t.udpConn.Close()
				t.udpConn = nil
			}
		}
		var tc TunnelConfig
		if needListen {
			tc = t.TunnelConfig
		}
		cur := t.TunnelConfig // 锁内快照，锁外使用
		s.mu.Unlock()
		if needListen {
			if err := s.startTunnel(tc); err != nil {
				http.Error(w, "重建监听失败: "+err.Error(), http.StatusInternalServerError)
				return
			}
			// 自动分配的端口回写
			if tc.PublicPort == 0 {
				s.mu.Lock()
				if t2 := s.tunnels[tc.ID]; t2 != nil {
					for i := range s.cfg.Tunnels {
						if s.cfg.Tunnels[i].ID == tc.ID {
							s.cfg.Tunnels[i].PublicPort = t2.PublicPort
							break
						}
					}
				}
				s.mu.Unlock()
			}
		}
		if needP2PRestart {
			s.setP2PState(id, "waiting")
			go s.tryP2POffer(cur)
		}
		if visitorID != "" && !s.clientExists(visitorID) {
			http.Error(w, "访问端客户端不存在", http.StatusBadRequest)
			return
		}
		s.rebuildRoutes()
		s.saveConfig()
		core.LogEvent("tunnel_update", "ok", fmt.Sprintf("隧道已更新: %s (%s)", cur.Name, cur.Proto))
		// 热推送新配置给服务方与访问方
		go s.pushSync(cur.ClientID)
		if cur.Proto == "p2p" {
			go s.pushSync(visitorID)
		}
		json.NewEncoder(w).Encode(cur)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// ---------- 流量时序 ----------

func (s *Server) handleTraffic(w http.ResponseWriter, r *http.Request) {
	if !s.authed(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	out := map[string]any{}
	s.mu.Lock()
	tunnels := map[string]any{}
	totalUp, totalDown := int64(0), int64(0)
	for id, t := range s.tunnels {
		up, down, series := t.traffic.Snapshot()
		tunnels[id] = map[string]any{"up": up, "down": down, "series": series}
		totalUp += up
		totalDown += down
	}
	clients := map[string]any{}
	for id, cs := range s.clients {
		up, down, series := cs.traffic.Snapshot()
		clients[id] = map[string]any{"up": up, "down": down, "series": series}
	}
	s.mu.Unlock()
	out["tunnels"] = tunnels
	out["clients"] = clients
	out["total_up"] = totalUp
	out["total_down"] = totalDown
	json.NewEncoder(w).Encode(out)
}

// handleTrafficSummary 返回按时间段与粒度汇总的流量统计（累计 / 当月 / 上月 / 区间 / 按日或按月明细）。
// 查询参数：from=YYYY-MM-DD，to=YYYY-MM-DD，gran=day|month（默认 day）。
func (s *Server) handleTrafficSummary(w http.ResponseWriter, r *http.Request) {
	if !s.authed(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	q := r.URL.Query()
	rep := s.ledger.Query(q.Get("from"), q.Get("to"), q.Get("gran"))
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(rep)
}

// handleTrafficBreakdown 返回按隧道或客户端维度的区间流量归因。
// 查询参数：from=YYYY-MM-DD，to=YYYY-MM-DD，by=tunnel|client（默认 tunnel）。
func (s *Server) handleTrafficBreakdown(w http.ResponseWriter, r *http.Request) {
	if !s.authed(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	q := r.URL.Query()
	by := q.Get("by")
	if by != "client" {
		by = "tunnel"
	}
	rep := s.ledger.Breakdown(q.Get("from"), q.Get("to"), by)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(rep)
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	if !s.authed(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	json.NewEncoder(w).Encode(core.RecentEvents())
}

// ---------- 向导 ----------

func (s *Server) handleWizard(w http.ResponseWriter, r *http.Request) {
	if !s.authed(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var in struct {
		Preset          string `json:"preset"`
		ClientID        string `json:"client_id"`
		VisitorClientID string `json:"visitor_client_id"`
		LocalHost       string `json:"local_host"`
		LocalPort       int    `json:"local_port"`
		PublicPort      int    `json:"public_port"`
		VisitorPort     int    `json:"visitor_port"`
		Domain          string `json:"domain"`
		Subdomain       string `json:"subdomain"`
		Name            string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	if in.ClientID == "" {
		in.ClientID = "default"
	}
	if in.LocalHost == "" {
		in.LocalHost = "127.0.0.1"
	}
	tc := TunnelConfig{ClientID: in.ClientID, LocalHost: in.LocalHost, Enabled: true}
	switch in.Preset {
	case "rdp":
		tc.Proto, tc.Name, tc.LocalPort, tc.PublicPort = "tcp", "远程桌面(RDP)", 3389, orDefault(in.PublicPort, 33890)
	case "ssh":
		tc.Proto, tc.Name, tc.LocalPort, tc.PublicPort = "tcp", "SSH", 22, orDefault(in.PublicPort, 22022)
	case "web":
		tc.Proto, tc.Name, tc.LocalPort = "http", "Web 网站", orDefault(in.LocalPort, 80)
		tc.Domain, tc.Subdomain = in.Domain, in.Subdomain
	case "https":
		tc.Proto, tc.Name, tc.LocalPort = "https", "HTTPS 网站", orDefault(in.LocalPort, 443)
		tc.Domain, tc.Subdomain = in.Domain, in.Subdomain
	case "game-udp":
		tc.Proto, tc.Name, tc.LocalPort, tc.PublicPort = "udp", "游戏(UDP)", orDefault(in.LocalPort, 27015), orDefault(in.PublicPort, 27015)
	case "game-tcp":
		tc.Proto, tc.Name, tc.LocalPort, tc.PublicPort = "tcp", "游戏(TCP)", orDefault(in.LocalPort, 25565), orDefault(in.PublicPort, 25565)
	case "p2p":
		if in.VisitorClientID == "" {
			in.VisitorClientID = in.ClientID
		}
		tc.Proto, tc.Name = "p2p", orDefaultName(in.Name, "P2P 直连")
		tc.LocalPort = orDefault(in.LocalPort, 3389)
		tc.VisitorClientID = in.VisitorClientID
		tc.VisitorPort = orDefault(in.VisitorPort, 23389)
	default:
		tc.Proto, tc.Name, tc.LocalPort, tc.PublicPort = "tcp", in.Name, in.LocalPort, in.PublicPort
	}
	if in.Name != "" && in.Preset == "" {
		tc.Name = in.Name
	}
	tc.ID = "t-" + core.RandID()[:8]
	if err := s.addTunnel(tc); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(tc)
}

func orDefault(v, d int) int {
	if v == 0 {
		return d
	}
	return v
}

func orDefaultName(v, d string) string {
	if v == "" {
		return d
	}
	return v
}

// ---------- SSE 实时推送 ----------

func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	if !s.authed(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "stream unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	for {
		// 复用 overview 的数据
		snap := s.snapshotForStream()
		b, _ := json.Marshal(snap)
		fmt.Fprintf(w, "data: %s\n\n", b)
		flusher.Flush()
		select {
		case <-r.Context().Done():
			return
		case <-time.After(1 * time.Second):
		}
	}
}

func (s *Server) snapshotForStream() map[string]any {
	s.controlsMu.Lock()
	clients := make([]map[string]any, 0, len(s.clients))
	totalUp, totalDown := int64(0), int64(0)
	for _, cs := range s.clients {
		cs.mu.Lock()
		up, down, _ := cs.traffic.Snapshot()
		clients = append(clients, map[string]any{
			"id": cs.ID, "name": cs.Name, "online": cs.online,
			"up": up, "down": down, "remote_addr": cs.remoteAddr,
		})
		cs.mu.Unlock()
		totalUp += up
		totalDown += down
	}
	s.controlsMu.Unlock()
	s.mu.Lock()
	tunnels := make([]map[string]any, 0, len(s.tunnels))
	for _, t := range s.tunnels {
		up, down, _ := t.traffic.Snapshot()
		mode := ""
		if t.Proto == "p2p" {
			mode = s.getP2PState(t.ID)
		}
		tunnels = append(tunnels, map[string]any{
			"id": t.ID, "name": t.Name, "proto": t.Proto,
			"client_id": t.ClientID, "up": up, "down": down,
			"public_port": t.PublicPort, "online": s.clientOnline(t.ClientID),
			"enabled": t.Enabled,
			"visitor_client_id": t.VisitorClientID, "visitor_port": t.VisitorPort,
			"p2p_mode": mode,
		})
	}
	s.mu.Unlock()
	return map[string]any{
		"clients": clients, "tunnels": tunnels,
		"total_up": totalUp, "total_down": totalDown,
		"admin_token": s.cfg.AdminToken,
		"events": core.RecentEvents(),
	}
}

// ---------- 客户端下载 ----------
// 从可执行文件所在目录安全地提供预编译客户端（文件名白名单，防目录穿越）
func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	if !s.authed(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	name := r.URL.Query().Get("name")
	allowed := map[string]bool{
		// 客户端
		"rtpc":                        true,
		"rtpc.exe":                    true,
		"rtpc-linux":                  true,
		"rtpc-darwin":                 true,
		"rtpc-android-arm64":          true,
		"rtp-android.apk":             true,
		"rtpc-windows-amd64.exe":      true,
		"rtpc-windows-cli-amd64.exe":  true,
		"rtpc-windows-arm64.exe":      true,
		"rtpc-windows-cli-arm64.exe":  true,
		// 服务端
		"rtps-linux":                  true,
		"rtps-darwin":                 true,
		"rtps-windows-amd64.exe":      true,
		"rtps-windows-cli-amd64.exe":  true,
	}
	if !allowed[name] {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	exe, err := os.Executable()
	if err != nil {
		http.Error(w, "internal", http.StatusInternalServerError)
		return
	}
	p := filepath.Join(filepath.Dir(exe), name)
	if _, err := os.Stat(p); err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+name+"\"")
	http.ServeFile(w, r, p)
}

// ---------- 代理网关（SOCKS5 / HTTP） ----------

// handleProxy GET 返回代理配置+运行状态+实时统计；PUT 保存配置并热生效。
func (s *Server) handleProxy(w http.ResponseWriter, r *http.Request) {
	if !s.authed(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.proxyView(r))
	case http.MethodPut:
		var in struct {
			Enabled   *bool  `json:"enabled"`
			SocksPort *int   `json:"socks_port"`
			HTTPPort  *int   `json:"http_port"`
			Mode      string `json:"mode"` // token / custom
			User      string `json:"user"`
			Pass      string `json:"pass"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		socksPort, httpPort := s.cfg.ProxyPort, s.cfg.ProxyHTTPPort
		if in.SocksPort != nil {
			if *in.SocksPort < 0 || *in.SocksPort > 65535 {
				http.Error(w, "SOCKS5 端口无效", http.StatusBadRequest)
				return
			}
			socksPort = *in.SocksPort
		}
		if in.HTTPPort != nil {
			if *in.HTTPPort < 0 || *in.HTTPPort > 65535 {
				http.Error(w, "HTTP 代理端口无效", http.StatusBadRequest)
				return
			}
			httpPort = *in.HTTPPort
		}
		// 端口冲突检查：与其他固定端口及已有隧道公网端口互斥
		taken := map[int]string{
			s.cfg.WebPort: "Web 控制台", s.cfg.ControlPort: "隧道控制",
			s.cfg.WorkPort: "工作端口", s.cfg.P2pPort: "P2P",
		}
		if s.cfg.HttpPort > 0 {
			taken[s.cfg.HttpPort] = "HTTP 网站隧道"
		}
		if s.cfg.HttpsPort > 0 {
			taken[s.cfg.HttpsPort] = "HTTPS 网站隧道"
		}
		for _, t := range s.tunnels {
			if t.PublicPort > 0 {
				taken[t.PublicPort] = "隧道 " + t.Name
			}
		}
		delete(taken, s.cfg.ProxyPort)     // 原值自身不算冲突
		delete(taken, s.cfg.ProxyHTTPPort) // 端口对调的合法场景
		if socksPort > 0 {
			if other, ok := taken[socksPort]; ok && socksPort != httpPort {
				http.Error(w, fmt.Sprintf("SOCKS5 端口 %d 已被占用（%s）", socksPort, other), http.StatusBadRequest)
				return
			}
		}
		if httpPort > 0 {
			if other, ok := taken[httpPort]; ok && httpPort != socksPort {
				http.Error(w, fmt.Sprintf("HTTP 代理端口 %d 已被占用（%s）", httpPort, other), http.StatusBadRequest)
				return
			}
		}
		if socksPort > 0 && httpPort > 0 && socksPort == httpPort {
			http.Error(w, "SOCKS5 与 HTTP 代理端口不能相同", http.StatusBadRequest)
			return
		}
		user, pass := s.cfg.ProxyUser, s.cfg.ProxyPass
		if in.Mode == "custom" {
			if strings.TrimSpace(in.User) == "" || strings.TrimSpace(in.Pass) == "" {
				http.Error(w, "自定义鉴权需要填写账号与密码", http.StatusBadRequest)
				return
			}
			if len(in.Pass) < 6 {
				http.Error(w, "自定义密码至少 6 位", http.StatusBadRequest)
				return
			}
			user, pass = strings.TrimSpace(in.User), in.Pass
		} else {
			user, pass = "", "" // 使用管理员令牌
		}
		enabled := s.cfg.ProxyEnabled
		if in.Enabled != nil {
			enabled = *in.Enabled
		}
		if enabled && socksPort == 0 && httpPort == 0 {
			http.Error(w, "已开启代理网关但未配置任何端口", http.StatusBadRequest)
			return
		}

		s.mu.Lock()
		s.cfg.ProxyEnabled = enabled
		s.cfg.ProxyPort = socksPort
		s.cfg.ProxyHTTPPort = httpPort
		s.cfg.ProxyUser = user
		s.cfg.ProxyPass = pass
		s.mu.Unlock()
		s.applyProxyConfig()
		s.saveConfig()
		core.LogEvent("admin", "ok", fmt.Sprintf("代理网关配置已更新：开关=%v SOCKS5=%d HTTP=%d 鉴权=%s", enabled, socksPort, httpPort, orDefaultName(in.Mode, "token")))
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// proxyView 汇总配置、运行状态、统计与客户端接入材料（Clash 配置 / 订阅链接）
func (s *Server) proxyView(r *http.Request) map[string]any {
	s.mu.Lock()
	enabled := s.cfg.ProxyEnabled
	socksPort, httpPort := s.cfg.ProxyPort, s.cfg.ProxyHTTPPort
	user, pass := s.cfg.ProxyUser, s.cfg.ProxyPass
	webPort := s.cfg.WebPort
	socksRunning := s.socksLn != nil
	httpRunning := s.httpLn != nil
	s.mu.Unlock()

	host := publicHostFromReq(r)
	if host == "" {
		host = s.cfg.BaseDomain
	}
	out := map[string]any{
		"enabled":       enabled,
		"socks_port":    socksPort,
		"http_port":     httpPort,
		"user":          user,
		"pass":          pass,
		"use_token":     user == "" && pass == "",
		"socks_running": socksRunning,
		"http_running":  httpRunning,
		"web_port":      webPort,
		"public_host":   host,
		"stats":         s.px.snapshot(),
	}
	if host != "" {
		u, p := user, pass
		if u == "" {
			u = s.cfg.AdminToken
		}
		if p == "" {
			p = s.cfg.AdminToken
		}
		out["clash_yaml"] = clashConfigYAML(host, socksPort, httpPort, u, p)
		out["sub_url"] = fmt.Sprintf("http://%s/api/proxy/clash?token=%s", host, s.cfg.AdminToken)
	}
	return out
}

// handleProxyClash 返回 Clash 配置 YAML；?download=1 时作为附件下载。
// 该 URL 即「订阅链接」：Clash → Profiles → URL 导入。
func (s *Server) handleProxyClash(w http.ResponseWriter, r *http.Request) {
	if !s.authed(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	host := publicHostFromReq(r)
	if host == "" {
		host = s.cfg.BaseDomain
	}
	u, p := s.proxyCreds()
	yaml := clashConfigYAML(host, s.cfg.ProxyPort, s.cfg.ProxyHTTPPort, u, p)
	w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
	if r.URL.Query().Get("download") == "1" {
		w.Header().Set("Content-Disposition", "attachment; filename=\"clash-rtp.yaml\"")
	}
	_, _ = w.Write([]byte(yaml))
}

// ---------- IP 归属地 ----------

type pxGeoEntry struct {
	Loc     string `json:"loc"`     // 归属地描述，如 "中国 上海"
	CN      bool   `json:"cn"`      // 是否中国大陆
	Private bool   `json:"private"` // 内网/回环地址
	Expire  int64  `json:"-"`
}

var geoCache = struct {
	sync.Mutex
	m map[string]pxGeoEntry
}{m: map[string]pxGeoEntry{}}

// handleProxyGeo 批量查询 IP/域名归属地。私有/回环地址直接标内网；
// 查询失败按「未知」处理且视为中国大陆（避免误标红）。缓存 24 小时。
func (s *Server) handleProxyGeo(w http.ResponseWriter, r *http.Request) {
	if !s.authed(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var in struct {
		Ips []string `json:"ips"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	now := time.Now().Unix()
	res := map[string]pxGeoEntry{}
	geoCache.Lock()
	var need []string
	for _, raw := range in.Ips {
		ip := strings.TrimSpace(raw)
		if ip == "" {
			continue
		}
		if e, ok := geoCache.m[ip]; ok && e.Expire > now {
			res[ip] = pxGeoEntry{Loc: e.Loc, CN: e.CN, Private: e.Private}
			continue
		}
		need = append(need, ip)
	}
	geoCache.Unlock()
	if len(need) > 0 {
		got := queryGeoBatch(need)
		geoCache.Lock()
		for ip, e := range got {
			geoCache.m[ip] = pxGeoEntry{Loc: e.Loc, CN: e.CN, Private: e.Private, Expire: now + 86400}
			res[ip] = pxGeoEntry{Loc: e.Loc, CN: e.CN, Private: e.Private}
		}
		geoCache.Unlock()
	}
	writeJSON(w, http.StatusOK, map[string]any{"geo": res})
}

func isPrivateIPStr(s string) bool {
	ip := net.ParseIP(s)
	return ip != nil && (ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast())
}

// queryGeoBatch 通过 ip-api.com 免费接口批量查询归属地（单次最多 100 条，失败返回「未知」）。
func queryGeoBatch(ips []string) map[string]pxGeoEntry {
	out := map[string]pxGeoEntry{}
	var remote []string
	for _, ip := range ips {
		if isPrivateIPStr(ip) {
			out[ip] = pxGeoEntry{Loc: "内网/本地", CN: true, Private: true}
			continue
		}
		remote = append(remote, ip)
	}
	if len(remote) == 0 {
		return out
	}
	type ipApiResp struct {
		Status      string `json:"status"`
		Country     string `json:"country"`
		CountryCode string `json:"countryCode"`
		City        string `json:"city"`
		Query       string `json:"query"`
	}
	payload, _ := json.Marshal(remote)
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Post("http://ip-api.com/batch?fields=status,country,countryCode,city,query&lang=zh-CN",
		"application/json", bytes.NewReader(payload))
	if err != nil {
		for _, ip := range remote {
			out[ip] = pxGeoEntry{Loc: "未知", CN: true}
		}
		return out
	}
	defer resp.Body.Close()
	var arr []ipApiResp
	if err := json.NewDecoder(resp.Body).Decode(&arr); err != nil {
		for _, ip := range remote {
			out[ip] = pxGeoEntry{Loc: "未知", CN: true}
		}
		return out
	}
	byQuery := map[string]ipApiResp{}
	for _, e := range arr {
		byQuery[e.Query] = e
	}
	for _, ip := range remote {
		e, ok := byQuery[ip]
		if !ok || e.Status != "success" {
			out[ip] = pxGeoEntry{Loc: "未知", CN: true}
			continue
		}
		loc := strings.TrimSpace(e.Country)
		if e.City != "" && e.City != e.Country {
			loc = strings.TrimSpace(e.Country + " " + e.City)
		}
		out[ip] = pxGeoEntry{Loc: loc, CN: e.CountryCode == "CN"}
	}
	return out
}

// publicHostFromReq 从请求 Host 推断服务端对外地址（去掉端口；回环地址回退 base_domain）
func publicHostFromReq(r *http.Request) string {
	h := r.Host
	if i := strings.LastIndex(h, ":"); i >= 0 && !strings.Contains(strings.TrimPrefix(h, "["), "]") {
		// IPv6 [::1]:8080 或 host:port
		if !strings.HasPrefix(h, "[") || strings.Contains(h, "]") {
			h = h[:i]
		}
	}
	h = strings.Trim(h, "[]")
	if h == "" || h == "127.0.0.1" || h == "localhost" || h == "::1" {
		return ""
	}
	return h
}

// clashConfigYAML 生成可直接导入 Clash 的配置
func clashConfigYAML(host string, socksPort, httpPort int, user, pass string) string {
	var names []string
	var b strings.Builder
	b.WriteString("# rtp 代理网关 · Clash 配置（由服务端自动生成）\n")
	b.WriteString("# 导入后请选中 RTP-Server-LAN 节点（建议开启全局模式）\n")
	b.WriteString("mixed-port: 7890\nmode: global\nlog-level: info\n\nproxies:\n")
	if socksPort > 0 {
		names = append(names, "RTP-Server-LAN")
		fmt.Fprintf(&b, "  - name: \"RTP-Server-LAN\"\n    type: socks5\n    server: %s\n    port: %d\n    username: \"%s\"\n    password: \"%s\"\n    udp: true\n\n", host, socksPort, user, pass)
	}
	if httpPort > 0 {
		// HTTP 代理为免密直连，Clash 节点不带账密
		names = append(names, "RTP-Server-HTTP")
		fmt.Fprintf(&b, "  - name: \"RTP-Server-HTTP\"\n    type: http\n    server: %s\n    port: %d\n\n", host, httpPort)
	}
	if len(names) == 0 {
		return "# 代理网关未启用：请先在「代理」页开启并配置端口\n"
	}
	b.WriteString("proxy-groups:\n  - name: PROXY\n    type: select\n    proxies:\n")
	for _, n := range names {
		fmt.Fprintf(&b, "      - \"%s\"\n", n)
	}
	b.WriteString("\nrules:\n  - \"MATCH,PROXY\"\n")
	return b.String()
}

// ---------- 设置 ----------

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	if !s.authed(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if r.Method == http.MethodPut {
		var in struct {
			BaseDomain   *string `json:"base_domain"`
			Announcement *string `json:"announcement"`
			CurPass      string  `json:"cur_pass"`
			NewUser      string  `json:"new_user"`
			NewPass      string  `json:"new_pass"`
			NewToken     string  `json:"new_token"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err == nil {
			// 修改管理员令牌：需要当前密码验证，且不能与客户端令牌冲突
			if in.NewToken != "" {
				if hashPass(s.cfg.AdminSalt, in.CurPass) != s.cfg.AdminPassHash {
					writeJSON(w, http.StatusForbidden, map[string]any{"ok": false, "error": "当前密码不正确"})
					return
				}
				if !validTokenFormat(in.NewToken) {
					writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "令牌需为 8-64 位字母、数字、- 或 _"})
					return
				}
				if in.NewToken == s.cfg.AdminToken {
					// 与当前相同，视为无操作
				} else if !s.tokenFree(in.NewToken, "") {
					writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "令牌已被某个客户端占用"})
					return
				} else {
					s.mu.Lock()
					s.cfg.AdminToken = in.NewToken
					s.mu.Unlock()
					core.LogEvent("admin", "warn", "管理员令牌已修改（旧令牌链接即日失效）")
				}
			}
			s.mu.Lock()
			if in.BaseDomain != nil {
				s.cfg.BaseDomain = *in.BaseDomain
			}
			if in.Announcement != nil {
				s.cfg.Announcement = *in.Announcement
			}
			if in.NewPass != "" || in.NewUser != "" {
				if hashPass(s.cfg.AdminSalt, in.CurPass) != s.cfg.AdminPassHash {
					s.mu.Unlock()
					writeJSON(w, http.StatusForbidden, map[string]any{"ok": false, "error": "当前密码不正确"})
					return
				}
				if len(in.NewPass) > 0 && len(in.NewPass) < 6 {
					s.mu.Unlock()
					writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "新密码至少 6 位"})
					return
				}
				if in.NewUser != "" {
					s.cfg.AdminUser = in.NewUser
				}
				if in.NewPass != "" {
					s.cfg.AdminPassHash = hashPass(s.cfg.AdminSalt, in.NewPass)
					core.LogEvent("admin", "ok", "管理后台密码已修改")
				}
			}
			s.mu.Unlock()
			s.rebuildRoutes()
			s.saveConfig()
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"base_domain":  s.cfg.BaseDomain,
		"http_port":    s.cfg.HttpPort,
		"https_port":   s.cfg.HttpsPort,
		"p2p_port":     s.cfg.P2pPort,
		"control_port": s.cfg.ControlPort,
		"work_port":    s.cfg.WorkPort,
		"web_port":     s.cfg.WebPort,
		"admin_user":   s.cfg.AdminUser,
		"admin_token":  s.cfg.AdminToken,
		"announcement": s.cfg.Announcement,
		"version":      RtpVersion,
		"default_pass": s.cfg.AdminPassHash == hashPass(s.cfg.AdminSalt, "admin123"),
	})
}

// ---------- 客户端连接命令 ----------

func (s *Server) handleClientCmd(w http.ResponseWriter, r *http.Request) {
	if !s.authed(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	id := r.URL.Query().Get("id")
	cs, ok := s.clients[id]
	if !ok {
		http.Error(w, "client not found", http.StatusNotFound)
		return
	}
	// 服务端公网地址从请求 Host 推断
	host := r.Host
	port := s.cfg.ControlPort
	cmd := fmt.Sprintf("rtpc -server %s:%d -token %s -name %s", host, port, cs.Token, cs.Name)
	json.NewEncoder(w).Encode(map[string]any{
		"cmd":    cmd,
		"token":  cs.Token,
		"server": fmt.Sprintf("%s:%d", host, port),
	})
}
