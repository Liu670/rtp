package main

import (
	"github.com/Liu670/rtp/internal/core"
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ---------- 运行时对象 ----------

type Tunnel struct {
	TunnelConfig
	mu       sync.Mutex
	listener net.Listener
	udpConn  net.PacketConn
	c2s      int64 // 用户 -> 内网（下行）字节
	s2c      int64 // 内网 -> 用户（上行）字节
	traffic  *Traffic
}

func (t *Tunnel) counters() (c2s, s2c int64) {
	return atomic.LoadInt64(&t.c2s), atomic.LoadInt64(&t.s2c)
}

// stop 关闭该隧道的 TCP/UDP 监听（用于服务端整体停止）。
func (t *Tunnel) stop() {
	t.mu.Lock()
	ln, pc := t.listener, t.udpConn
	t.listener, t.udpConn = nil, nil
	t.mu.Unlock()
	if ln != nil {
		_ = ln.Close()
	}
	if pc != nil {
		_ = pc.Close()
	}
}

type ClientState struct {
	ClientConfig
	mu         sync.Mutex
	cc         *controlConn
	online     bool
	lastSeen   int64
	remoteAddr string
	traffic    *Traffic
	th         *Throttle
}

func (cs *ClientState) throttle() *Throttle {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return cs.th
}

type controlConn struct {
	conn     net.Conn
	w        *bufio.Writer
	mu       sync.Mutex
	clientID string
}

func (cc *controlConn) write(m Msg) error {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	cc.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	return core.WriteMsg(cc.w, m)
}

type udpRelay struct {
	conn net.Conn
	w    *bufio.Writer
	mu   sync.Mutex
}

func (r *udpRelay) write(m Msg) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	return core.WriteMsg(r.w, m)
}

// ---------- Server ----------

type Server struct {
	mu         sync.Mutex
	cfg        Config
	cfgPath    string
	tunnels    map[string]*Tunnel
	clients    map[string]*ClientState
	controlsMu sync.Mutex
	controls   map[string]*controlConn // clientID -> 当前控制连接
	pendingMu  sync.Mutex
	pending    map[string]chan net.Conn
	routeMu    sync.RWMutex
	httpRoutes map[string]string // host -> tunnelID
	httpsRoutes map[string]string
	udpMu      sync.Mutex
	udpRelays  map[string]*udpRelay // tunnelID -> relay
	// P2P 协调器状态
	p2pMu         sync.Mutex
	p2pCandidates map[string]net.Addr // clientID -> NAT 候选地址（信令看到的源地址）
	p2pStates     map[string]string    // tunnelID -> 状态: waiting/punching/direct/relay
	// Web 登录会话（账号密码鉴权）
	sessMu   sync.Mutex
	sessions map[string]sessionInfo // sessionID -> 会话
	startTime time.Time
	// 生命周期标记（GUI 模式下用于启停）
	running bool
}

func NewServer(cfg Config, cfgPath string) *Server {
	s := &Server{
		cfg:        cfg,
		cfgPath:    cfgPath,
		tunnels:    map[string]*Tunnel{},
		clients:    map[string]*ClientState{},
		controls:   map[string]*controlConn{},
		pending:    map[string]chan net.Conn{},
		httpRoutes: map[string]string{},
		httpsRoutes: map[string]string{},
		udpRelays:  map[string]*udpRelay{},
		p2pCandidates: map[string]net.Addr{},
		p2pStates:     map[string]string{},
		sessions:      map[string]sessionInfo{},
		startTime:     time.Now(),
	}
	s.registerWebhookHook()
	return s
}

// migrate 兼容旧配置：无客户端则建默认客户端；未归属隧道的归属默认客户端
func (s *Server) migrate() {
	if len(s.cfg.Clients) == 0 {
		s.cfg.Clients = []ClientConfig{{
			ID:        "default",
			Name:      "默认客户端",
			Token:     s.cfg.AdminToken,
			CreatedAt: time.Now().Unix(),
		}}
	}
	for i := range s.cfg.Tunnels {
		if s.cfg.Tunnels[i].ClientID == "" {
			s.cfg.Tunnels[i].ClientID = "default"
		}
		for _, c := range s.cfg.Clients {
			if c.ID == s.cfg.Tunnels[i].ClientID {
				s.cfg.Tunnels[i].ClientID = c.ID
				break
			}
		}
	}
	// 管理后台账号：首次运行创建默认 admin/admin123 并持久化
	if s.cfg.AdminUser == "" || s.cfg.AdminPassHash == "" || s.cfg.AdminSalt == "" {
		s.cfg.AdminUser = "admin"
		s.cfg.AdminSalt = core.RandID()[:16]
		s.cfg.AdminPassHash = hashPass(s.cfg.AdminSalt, "admin123")
		s.saveConfig()
		log.Printf("[auth] 已创建默认管理员账号 admin / admin123，请登录控制台后在「设置」中修改")
	}
}

func (s *Server) buildClients() {
	s.clients = map[string]*ClientState{}
	for _, c := range s.cfg.Clients {
		cs := &ClientState{ClientConfig: c, traffic: core.NewTraffic()}
		cs.th = core.NewThrottle(int64(c.BwLimit) * 1024 * 1024 / 8)
		s.clients[c.ID] = cs
	}
}

func (s *Server) clientOf(id string) *ClientState {
	s.controlsMu.Lock()
	defer s.controlsMu.Unlock()
	if cs, ok := s.clients[id]; ok {
		return cs
	}
	return nil
}

// loadConfig 读取配置，未指定时给默认值；文件不存在则生成随机管理员令牌
func loadConfig(path string) Config {
	cfg := Config{ControlPort: 7000, WorkPort: 7001, WebPort: 8080}
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &cfg)
	}
	if cfg.ControlPort == 0 {
		cfg.ControlPort = 7000
	}
	if cfg.WorkPort == 0 {
		cfg.WorkPort = 7001
	}
	if cfg.WebPort == 0 {
		cfg.WebPort = 8080
	}
	if cfg.P2pPort == 0 {
		cfg.P2pPort = cfg.ControlPort + 2
	}
	if cfg.AdminToken == "" {
		cfg.AdminToken = core.RandID()
	}
	return cfg
}

func (s *Server) rebuildRoutes() {
	s.routeMu.Lock()
	defer s.routeMu.Unlock()
	s.httpRoutes = map[string]string{}
	s.httpsRoutes = map[string]string{}
	for _, t := range s.tunnels {
		if !t.Enabled {
			continue
		}
		key := strings.ToLower(t.Domain)
		if key == "" && t.Subdomain != "" {
			if s.cfg.BaseDomain != "" {
				key = strings.ToLower(t.Subdomain + "." + s.cfg.BaseDomain)
			} else {
				key = strings.ToLower(t.Subdomain)
			}
		}
		if t.Proto == "http" && key != "" {
			s.httpRoutes[key] = t.ID
		} else if t.Proto == "https" && key != "" {
			s.httpsRoutes[key] = t.ID
		}
	}
}

func (s *Server) saveConfig() {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, _ := json.MarshalIndent(s.cfg, "", "  ")
	_ = os.WriteFile(s.cfgPath, data, 0644)
}

// ---------- 运行 ----------

// Stop 关闭所有隧道监听并标记服务停止（供 Windows GUI 停止服务使用）。
func (s *Server) Stop() {
	s.mu.Lock()
	s.running = false
	tunnels := make([]*Tunnel, 0, len(s.tunnels))
	for _, t := range s.tunnels {
		tunnels = append(tunnels, t)
	}
	s.tunnels = map[string]*Tunnel{}
	s.mu.Unlock()

	for _, t := range tunnels {
		t.stop()
	}
	core.LogEvent("admin", "warn", "服务端已停止（GUI 操作）")
}

func (s *Server) Run() {
	s.mu.Lock()
	s.running = true
	s.mu.Unlock()

	s.migrate()
	s.buildClients()
	s.rebuildRoutes()

	go s.serveControl(fmt.Sprintf(":%d", s.cfg.ControlPort))
	go s.serveWork(fmt.Sprintf(":%d", s.cfg.WorkPort))
	if s.cfg.HttpPort > 0 {
		go s.serveHTTP(fmt.Sprintf(":%d", s.cfg.HttpPort))
	}
	if s.cfg.HttpsPort > 0 {
		go s.serveHTTPS(fmt.Sprintf(":%d", s.cfg.HttpsPort))
	}
	if s.cfg.P2pPort > 0 {
		go s.startP2PListener(fmt.Sprintf(":%d", s.cfg.P2pPort))
	}

	s.mu.Lock()
	for _, tc := range s.cfg.Tunnels {
		if tc.Enabled {
			s.startTunnel(tc)
		}
	}
	s.mu.Unlock()

	go s.sampler()
	s.StartWeb(fmt.Sprintf(":%d", s.cfg.WebPort))
}

func (s *Server) sampler() {
	lastT := map[string][2]int64{}
	lastC := map[string][2]int64{}
	for range time.Tick(time.Second) {
		s.mu.Lock()
		tunnels := make([]*Tunnel, 0, len(s.tunnels))
		for _, t := range s.tunnels {
			tunnels = append(tunnels, t)
		}
		s.mu.Unlock()
		for _, t := range tunnels {
			c2s, s2c := t.counters()
			lt := lastT[t.ID]
			dC2s, dS2c := c2s-lt[0], s2c-lt[1]
			lastT[t.ID] = [2]int64{c2s, s2c}
			if dC2s <= 0 && dS2c <= 0 {
				continue
			}
			t.traffic.Add(dS2c, dC2s) // up=s2c(内网->用户), down=c2s(用户->内网)
			if cs := s.clientOf(t.ClientID); cs != nil {
				cs.traffic.Add(dS2c, dC2s)
			}
		}
		_ = lastC
	}
}

// ---------- 控制连接 ----------

func (s *Server) serveControl(addr string) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("控制端口 %s 监听失败: %v", addr, err)
	}
	log.Printf("[control] 监听 %s", addr)
	for {
		c, err := ln.Accept()
		if err != nil {
			continue
		}
		go s.handleControl(c)
	}
}

// pushSync 把某客户端相关的全部隧道（含其作为 target/visitor 的 P2P）热推给它，
// 用于隧道增删改后的免重启生效。客户端离线时静默跳过（重连时会拿到全量 sync）。
func (s *Server) pushSync(clientID string) {
	if clientID == "" {
		return
	}
	s.controlsMu.Lock()
	cs := s.clients[clientID]
	s.controlsMu.Unlock()
	if cs == nil {
		return
	}
	cs.mu.Lock()
	cc := cs.cc
	cs.mu.Unlock()
	if cc == nil {
		return
	}
	s.mu.Lock()
	ts := make([]TunnelConfig, 0)
	for _, t := range s.tunnels {
		if t.ClientID == clientID || t.VisitorClientID == clientID {
			ts = append(ts, t.TunnelConfig)
		}
	}
	s.mu.Unlock()
	_ = cc.write(Msg{Type: "sync", Tunnels: ts})
}

func (s *Server) handleControl(conn net.Conn) {
	r := bufio.NewReader(conn)
	w := bufio.NewWriter(conn)
	cc := &controlConn{conn: conn, w: w}

	m, err := core.ReadMsg(r)
	if err != nil {
		return
	}
	if m.Type != "auth" {
		return
	}
	// 解析客户端
	var cs *ClientState
	s.controlsMu.Lock()
	if m.Token == s.cfg.AdminToken {
		cs = s.clients["default"]
	} else {
		for _, c := range s.clients {
			if c.Token == m.Token {
				cs = c
				break
			}
		}
	}
	s.controlsMu.Unlock()
	if cs == nil {
		_ = cc.write(Msg{Type: "auth_result", Ok: false, Message: "令牌无效，请在控制台创建客户端"})
		return
	}
	_ = cc.write(Msg{Type: "auth_result", Ok: true, WorkPort: s.cfg.WorkPort, P2pPort: s.cfg.P2pPort, ClientID: cs.ID})
	cc.clientID = cs.ID

	cs.mu.Lock()
	cs.cc = cc
	cs.online = true
	cs.lastSeen = time.Now().Unix()
	cs.remoteAddr = conn.RemoteAddr().String()
	cs.mu.Unlock()

	s.controlsMu.Lock()
	s.controls[cs.ID] = cc
	s.controlsMu.Unlock()

	core.LogEvent("client_online", "ok", fmt.Sprintf("客户端上线: %s (%s)", cs.Name, cs.remoteAddr))
	log.Printf("[client] %s 上线", cs.Name)

	// 下发该客户端归属的隧道（含作为 target 或 visitor 的 P2P 隧道）
	s.mu.Lock()
	ts := make([]TunnelConfig, 0)
	for _, t := range s.tunnels {
		if t.ClientID == cs.ID || t.VisitorClientID == cs.ID {
			ts = append(ts, t.TunnelConfig)
		}
	}
	s.mu.Unlock()
	_ = cc.write(Msg{Type: "sync", Tunnels: ts})

	// 触发涉及该客户端的 P2P 隧道穿透
	s.onClientOnline(cs.ID)

	for {
		cm, err := core.ReadMsg(r)
		if err != nil {
			break
		}
		switch cm.Type {
		case "pong":
			cs.mu.Lock()
			cs.lastSeen = time.Now().Unix()
			cs.mu.Unlock()
		case "p2p_relay_open":
			s.handleP2PRelayOpen(cm.TunnelID, cm.ConnID)
		case "p2p_link":
			if cm.Ok {
				s.setP2PState(cm.TunnelID, "direct")
				core.LogEvent("p2p", "ok", fmt.Sprintf("P2P 直连成功: %s", cm.TunnelID))
			}
		}
	}

	s.controlsMu.Lock()
	if s.controls[cs.ID] == cc {
		delete(s.controls, cs.ID)
	}
	s.controlsMu.Unlock()
	cs.mu.Lock()
	if cs.cc == cc {
		cs.cc = nil
		cs.online = false
	}
	cs.mu.Unlock()
	// 重置涉及该客户端的 P2P 隧道状态，便于重连后重新穿透
	s.p2pMu.Lock()
	delete(s.p2pCandidates, cs.ID)
	for id, st := range s.p2pStates {
		if st != "waiting" {
			s.p2pStates[id] = "waiting"
		}
	}
	s.p2pMu.Unlock()
	core.LogEvent("client_offline", "warn", fmt.Sprintf("客户端离线: %s", cs.Name))
	conn.Close()
}

// ---------- 工作连接（TCP 风格 + UDP 中继） ----------

func (s *Server) serveWork(addr string) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("工作端口 %s 监听失败: %v", addr, err)
	}
	log.Printf("[work] 监听 %s", addr)
	for {
		c, err := ln.Accept()
		if err != nil {
			continue
		}
		go s.handleWork(c)
	}
}

func (s *Server) handleWork(conn net.Conn) {
	r := bufio.NewReader(conn)
	m, err := core.ReadMsg(r)
	if err != nil {
		conn.Close()
		return
	}
	switch m.Type {
	case "work_auth":
		if !s.validToken(m.Token) {
			conn.Close()
			return
		}
		s.pendingMu.Lock()
		ch, ok := s.pending[m.ConnID]
		s.pendingMu.Unlock()
		if !ok {
			conn.Close()
			return
		}
		select {
		case ch <- conn:
		default:
			conn.Close()
		}
	case "udp_register":
		t := s.tunnelByID(m.TunnelID)
		if t == nil || !s.validToken(m.Token) {
			conn.Close()
			return
		}
		relay := &udpRelay{conn: conn, w: bufio.NewWriter(conn)}
		s.udpMu.Lock()
		s.udpRelays[m.TunnelID] = relay
		s.udpMu.Unlock()
		log.Printf("[udp] 中继已建立: %s", t.Name)
		go s.udpRelayReader(relay, t)
	default:
		conn.Close()
	}
}

func (s *Server) udpRelayReader(relay *udpRelay, t *Tunnel) {
	r := bufio.NewReader(relay.conn)
	for {
		m, err := core.ReadMsg(r)
		if err != nil {
			break
		}
		if m.Type == "udp_data" && t.udpConn != nil && m.From != "" {
			addr, err := net.ResolveUDPAddr("udp", m.From)
			if err == nil {
				t.udpConn.WriteTo(m.Data, addr)
			}
		}
	}
	s.udpMu.Lock()
	if s.udpRelays[t.ID] == relay {
		delete(s.udpRelays, t.ID)
	}
	s.udpMu.Unlock()
	relay.conn.Close()
}

// ---------- 隧道公共逻辑 ----------

func (s *Server) tunnelByID(id string) *Tunnel {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.tunnels[id]
}

// validToken 工作/中继连接鉴权：管理员令牌或任意客户端令牌均可
func (s *Server) validToken(tok string) bool {
	if tok == s.cfg.AdminToken {
		return true
	}
	s.controlsMu.Lock()
	defer s.controlsMu.Unlock()
	for _, c := range s.clients {
		if c.Token == tok {
			return true
		}
	}
	return false
}

// openTunnelToClient 通知归属客户端建立工作连接，返回到客户端的连接
func (s *Server) openTunnelToClient(t *Tunnel, prepend []byte) (net.Conn, error) {
	cs := s.clientOf(t.ClientID)
	if cs == nil || !cs.online || cs.cc == nil {
		return nil, fmt.Errorf("客户端离线")
	}
	connID := core.RandID()
	ch := make(chan net.Conn, 1)
	s.pendingMu.Lock()
	s.pending[connID] = ch
	s.pendingMu.Unlock()
	defer func() {
		s.pendingMu.Lock()
		delete(s.pending, connID)
		s.pendingMu.Unlock()
	}()

	if err := cs.cc.write(Msg{Type: "new_conn", ConnID: connID, TunnelID: t.ID}); err != nil {
		return nil, err
	}
	select {
	case wc := <-ch:
		if len(prepend) > 0 {
			wc.Write(prepend)
		}
		return wc, nil
	case <-time.After(20 * time.Second):
		return nil, fmt.Errorf("等待客户端工作连接超时")
	}
}

// pipeStream 与客户端建立工作连接并双向透传。userSrc 是“用户->内网”方向的数据源
// （TCP 为原始连接 uc；HTTP/HTTPS 为已预读过头部的 bufio.Reader）。
func (s *Server) pipeStream(uc net.Conn, userSrc io.Reader, t *Tunnel, prepend []byte) {
	wc, err := s.openTunnelToClient(t, prepend)
	if err != nil {
		log.Printf("[tunnel %s] 打开失败: %v", t.Name, err)
		uc.Close()
		return
	}
	th := s.clientOf(t.ClientID).throttle()
	go core.Pump(wc, userSrc, &t.c2s, th) // 用户 -> 内网
	core.Pump(uc, wc, &t.s2c, th)         // 内网 -> 用户
	core.CloseBoth(uc, wc)
}

// ---------- TCP / STCP 公网监听 ----------

func (s *Server) servePublic(t *Tunnel, ln net.Listener) {
	for {
		uc, err := ln.Accept()
		if err != nil {
			return
		}
		go func(uc net.Conn) {
			if t.Proto == "stcp" {
				br := bufio.NewReader(uc)
				line, err := br.ReadString('\n')
				if err != nil {
					uc.Close()
					return
				}
				secret := strings.TrimSpace(strings.TrimPrefix(line, "secret:"))
				if secret != t.Secret {
					uc.Close()
					return
				}
				s.pipeStream(uc, br, t, nil)
				return
			}
			s.pipeStream(uc, uc, t, nil)
		}(uc)
	}
}

// ---------- HTTP 虚拟主机 ----------

func (s *Server) serveHTTP(addr string) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Printf("[http] 监听失败: %v", err)
		return
	}
	log.Printf("[http] 监听 %s", addr)
	for {
		c, err := ln.Accept()
		if err != nil {
			continue
		}
		go s.handleHTTP(c)
	}
}

func (s *Server) handleHTTP(uc net.Conn) {
	br := bufio.NewReader(uc)
	head, err := readHTTPHeader(br)
	if err != nil {
		uc.Close()
		return
	}
	host := parseHost(string(head))
	s.routeMu.RLock()
	id := s.httpRoutes[strings.ToLower(host)]
	s.routeMu.RUnlock()
	t := s.tunnelByID(id)
	if t == nil {
		uc.Write([]byte("HTTP/1.1 502 Bad Gateway\r\nContent-Length: 0\r\n\r\n"))
		uc.Close()
		return
	}
	s.pipeStream(uc, br, t, head)
}

// ---------- HTTPS SNI 透传 ----------

func (s *Server) serveHTTPS(addr string) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Printf("[https] 监听失败: %v", err)
		return
	}
	log.Printf("[https] 监听 %s", addr)
	for {
		c, err := ln.Accept()
		if err != nil {
			continue
		}
		go s.handleHTTPS(c)
	}
}

func (s *Server) handleHTTPS(uc net.Conn) {
	br := bufio.NewReader(uc)
	host, consumed, err := parseSNI(br)
	if err != nil {
		uc.Close()
		return
	}
	s.routeMu.RLock()
	id := s.httpsRoutes[strings.ToLower(host)]
	s.routeMu.RUnlock()
	t := s.tunnelByID(id)
	if t == nil {
		uc.Close()
		return
	}
	s.pipeStream(uc, br, t, consumed)
}

// ---------- UDP ----------

func (s *Server) serveUDP(t *Tunnel, pc net.PacketConn) {
	t.udpConn = pc
	defer pc.Close()
	buf := make([]byte, 65535)
	for {
		n, addr, err := pc.ReadFrom(buf)
		if err != nil {
			return
		}
		s.udpMu.Lock()
		relay := s.udpRelays[t.ID]
		s.udpMu.Unlock()
		if relay == nil {
			continue
		}
		data := make([]byte, n)
		copy(data, buf[:n])
		_ = relay.write(Msg{Type: "udp_data", TunnelID: t.ID, From: addr.String(), Data: data})
	}
}

// ---------- 隧道增删 ----------

func (s *Server) startTunnel(tc TunnelConfig) error {
	t := &Tunnel{TunnelConfig: tc, traffic: core.NewTraffic()}
	switch tc.Proto {
	case "tcp", "stcp":
		ln, err := net.Listen("tcp", fmt.Sprintf(":%d", tc.PublicPort))
		if err != nil {
			return err
		}
		if tc.PublicPort == 0 {
			tc.PublicPort = ln.Addr().(*net.TCPAddr).Port
			t.PublicPort = tc.PublicPort
		}
		t.listener = ln
		s.tunnels[tc.ID] = t
		go s.servePublic(t, ln)
	case "udp":
		pc, err := net.ListenPacket("udp", fmt.Sprintf(":%d", tc.PublicPort))
		if err != nil {
			return err
		}
		if tc.PublicPort == 0 {
			tc.PublicPort = pc.LocalAddr().(*net.UDPAddr).Port
			t.PublicPort = tc.PublicPort
		}
		t.udpConn = pc
		s.tunnels[tc.ID] = t
		go s.serveUDP(t, pc)
	case "http", "https":
		// 共享监听器，路由在 rebuildRoutes 中维护；仅登记对象
		s.tunnels[tc.ID] = t
	case "p2p":
		// P2P 隧道不监听端口，仅登记并触发穿透
		s.tunnels[tc.ID] = t
		s.setP2PState(tc.ID, "waiting")
		go s.tryP2POffer(tc)
	default:
		return fmt.Errorf("未知协议 %s", tc.Proto)
	}
	s.rebuildRoutes()
	return nil
}

func (s *Server) addTunnel(tc TunnelConfig) error {
	s.mu.Lock()
	for _, e := range s.tunnels {
		if (e.Proto == "tcp" || e.Proto == "stcp" || e.Proto == "udp") && e.PublicPort != 0 && e.PublicPort == tc.PublicPort {
			s.mu.Unlock()
			return fmt.Errorf("公网端口 %d 已被占用", tc.PublicPort)
		}
	}
	s.mu.Unlock()
	if err := s.startTunnel(tc); err != nil {
		return err
	}
	if tc.Proto == "p2p" {
		s.mu.Lock()
		s.cfg.Tunnels = append(s.cfg.Tunnels, tc)
		s.mu.Unlock()
		s.saveConfig()
		core.LogEvent("tunnel_create", "ok", fmt.Sprintf("新增 P2P 隧道: %s", tc.Name))
		go s.pushSync(tc.ClientID)
		go s.pushSync(tc.VisitorClientID)
		return nil
	}
	if tc.PublicPort == 0 {
		if t := s.tunnels[tc.ID]; t != nil {
			tc.PublicPort = t.PublicPort
		}
	}
	s.mu.Lock()
	s.cfg.Tunnels = append(s.cfg.Tunnels, tc)
	s.mu.Unlock()
	s.saveConfig()
	core.LogEvent("tunnel_create", "ok", fmt.Sprintf("新增隧道: %s (%s)", tc.Name, tc.Proto))
	go s.pushSync(tc.ClientID)
	if tc.Proto == "p2p" {
		go s.pushSync(tc.VisitorClientID)
	}
	return nil
}

func (s *Server) removeTunnel(id string) error {
	var notifyTo []string
	s.mu.Lock()
	t, ok := s.tunnels[id]
	if ok {
		notifyTo = append(notifyTo, t.ClientID, t.VisitorClientID)
		delete(s.tunnels, id)
		if t.listener != nil {
			t.listener.Close()
		}
		if t.udpConn != nil {
			t.udpConn.Close()
		}
	}
	s.mu.Unlock()
	if !ok {
		return fmt.Errorf("隧道不存在")
	}
	s.rebuildRoutes()
	s.mu.Lock()
	for i, x := range s.cfg.Tunnels {
		if x.ID == id {
			s.cfg.Tunnels = append(s.cfg.Tunnels[:i], s.cfg.Tunnels[i+1:]...)
			break
		}
	}
	s.mu.Unlock()
	s.saveConfig()
	core.LogEvent("tunnel_delete", "warn", fmt.Sprintf("删除隧道: %s", t.Name))
	for _, cid := range notifyTo {
		go s.pushSync(cid)
	}
	return nil
}

// ---------- HTTP 头/SNI 解析辅助 ----------

func readHTTPHeader(br *bufio.Reader) ([]byte, error) {
	var buf []byte
	for {
		line, err := br.ReadBytes('\n')
		buf = append(buf, line...)
		if err != nil {
			return buf, err
		}
		if len(line) == 2 && line[0] == '\r' && line[1] == '\n' {
			break
		}
		if len(line) == 1 && line[0] == '\n' {
			break
		}
	}
	return buf, nil
}

func parseHost(header string) string {
	for _, line := range strings.Split(header, "\r\n") {
		if len(line) > 5 && strings.EqualFold(line[:5], "host:") {
			h := strings.TrimSpace(line[5:])
			if i := strings.IndexByte(h, ':'); i >= 0 {
				h = h[:i]
			}
			return strings.ToLower(h)
		}
	}
	return ""
}

// parseSNI 解析 TLS ClientHello 中的 SNI，并返回已读取的字节（需转发给后端）
func parseSNI(br *bufio.Reader) (host string, consumed []byte, err error) {
	hdr := make([]byte, 5)
	if _, err = io.ReadFull(br, hdr); err != nil {
		return
	}
	if hdr[0] != 0x16 {
		err = fmt.Errorf("非 TLS handshake")
		return
	}
	recLen := int(hdr[3])<<8 | int(hdr[4])
	rec := make([]byte, recLen)
	if _, err = io.ReadFull(br, rec); err != nil {
		return
	}
	consumed = append(hdr, rec...)
	if len(rec) < 38 {
		err = fmt.Errorf("ClientHello 过短")
		return
	}
	p := 38 // 跳过 record/握手头 + version + random(32)
	if p >= len(rec) {
		err = fmt.Errorf("无 session_id")
		return
	}
	sidLen := int(rec[p])
	p += 1 + sidLen
	if p+2 > len(rec) {
		err = fmt.Errorf("无 cipher_suites")
		return
	}
	csLen := int(rec[p])<<8 | int(rec[p+1])
	p += 2 + csLen
	if p+2 > len(rec) {
		err = fmt.Errorf("无 extensions")
		return
	}
	extLen := int(rec[p])<<8 | int(rec[p+1])
	p += 2
	end := p + extLen
	for p+4 <= end && p+4 <= len(rec) {
		etype := int(rec[p])<<8 | int(rec[p+1])
		elen := int(rec[p+2])<<8 | int(rec[p+3])
		p += 4
		if etype == 0x00 { // server_name
			if p+2 > len(rec) {
				break
			}
			p += 2 // 跳过 list length
			if p+3 > len(rec) {
				break
			}
			nameType := rec[p]
			p++
			nlen := int(rec[p])<<8 | int(rec[p+1])
			p += 2
			if nameType == 0 && p+nlen <= len(rec) {
				host = string(rec[p : p+nlen])
				return host, consumed, nil
			}
			break
		}
		p += elen
	}
	err = fmt.Errorf("未找到 SNI")
	return
}
