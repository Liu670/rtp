package main

import (
	"github.com/Liu670/rtp/internal/core"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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
	mux.HandleFunc("/api/events", s.handleEvents)
	mux.HandleFunc("/api/wizard", s.handleWizard)
	mux.HandleFunc("/api/stream", s.handleStream)
	mux.HandleFunc("/api/settings", s.handleSettings)
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
