package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"sync"
	"time"
)

// udpRelayClient 客户端侧 UDP 中继连接（每隧道一条长连接，双向复用）
type udpRelayClient struct {
	conn net.Conn
	w    *bufio.Writer
	mu   sync.Mutex
}

func (u *udpRelayClient) writeMsg(m Msg) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	return writeMsg(u.w, m)
}

// Client 客户端（对应 frpc/npc）：持客户端令牌接入，规则由服务端热推送
type Client struct {
	serverHost string // 仅主机名/IP
	serverAddr string // host:controlPort
	token      string
	name       string

	mu       sync.Mutex
	tunnels  map[string]TunnelConfig
	relays   map[string]*udpRelayClient
	workPort int
	p2pPort  int

	p2pConn       *net.UDPConn
	p2pServerAddr net.Addr
	p2pLinks      map[string]*p2pLink
	p2pPeer       map[string]string
	p2pRelayLocal map[string]net.Conn
	p2pListeners  map[string]net.Listener

	ctrlSend chan Msg
}

func (c *Client) Run() {
	for {
		if err := c.connectOnce(); err != nil {
			log.Printf("[client] 连接断开/失败: %v", err)
		}
		log.Println("[client] 3 秒后重连...")
		time.Sleep(3 * time.Second)
	}
}

func (c *Client) connectOnce() error {
	conn, err := net.DialTimeout("tcp", c.serverAddr, 10*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()
	r := bufio.NewReader(conn)
	w := bufio.NewWriter(conn)

	if err := writeMsg(w, Msg{Type: "auth", Token: c.token, ClientID: c.name}); err != nil {
		return err
	}
	m, err := readMsg(r)
	if err != nil {
		return err
	}
	if !m.Ok {
		return fmt.Errorf("认证失败: %s", m.Message)
	}
	c.mu.Lock()
	c.workPort = m.WorkPort
	c.p2pPort = m.P2pPort
	c.tunnels = map[string]TunnelConfig{}
	c.relays = map[string]*udpRelayClient{}
	c.ctrlSend = make(chan Msg, 16)
	ch := c.ctrlSend
	c.mu.Unlock()
	go c.ctrlWriter(ch, w)
	log.Printf("[client] 已连接，工作端口=%d，P2P 信令端口=%d，等待服务端推送隧道规则", m.WorkPort, m.P2pPort)
	// 立即启动 P2P 信令，让服务端记录本端候选地址（供打洞）
	go c.ensureP2P()

	for {
		cm, err := readMsg(r)
		if err != nil {
			return err
		}
		switch cm.Type {
		case "sync":
			c.mu.Lock()
			newSet := map[string]bool{}
			for _, t := range cm.Tunnels {
				newSet[t.ID] = true
			}
			// 清理已删除隧道的 UDP 中继与 P2P 本地监听
			for id, rc := range c.relays {
				if !newSet[id] {
					rc.conn.Close()
					delete(c.relays, id)
				}
			}
			for id, ln := range c.p2pListeners {
				if !newSet[id] {
					ln.Close()
					delete(c.p2pListeners, id)
				}
			}
			c.tunnels = map[string]TunnelConfig{}
			for _, t := range cm.Tunnels {
				c.tunnels[t.ID] = t
				if t.Proto == "udp" && c.relays[t.ID] == nil {
					go c.openUDPRelay(t)
				}
			}
			c.mu.Unlock()
			log.Printf("[client] 同步到 %d 条隧道", len(cm.Tunnels))
		case "add_tunnel":
			if cm.Tunnel != nil {
				c.mu.Lock()
				c.tunnels[cm.Tunnel.ID] = *cm.Tunnel
				t := *cm.Tunnel
				c.mu.Unlock()
				log.Printf("[client] 新增隧道 %s (%s) -> %s:%d", t.Name, t.Proto, t.LocalHost, t.LocalPort)
				if t.Proto == "udp" {
					go c.openUDPRelay(t)
				}
			}
		case "remove_tunnel":
			c.mu.Lock()
			t, had := c.tunnels[cm.TunnelID]
			delete(c.tunnels, cm.TunnelID)
			if ln, ok := c.p2pListeners[cm.TunnelID]; ok {
				ln.Close()
				delete(c.p2pListeners, cm.TunnelID)
			}
			_ = t
			c.mu.Unlock()
			_ = had
		case "new_conn":
			go c.handleNewConn(cm.ConnID, cm.TunnelID)
		case "p2p_offer":
			go c.handleP2POffer(cm)
		case "relay_accept":
			go c.handleRelayAccept(cm.ConnID)
		case "ping":
			select {
			case ch <- Msg{Type: "pong"}:
			default:
			}
		}
	}
}

// ctrlWriter 统一从 channel 写入控制连接，避免并发写 bufio.Writer
func (c *Client) ctrlWriter(ch chan Msg, w *bufio.Writer) {
	for m := range ch {
		if err := writeMsg(w, m); err != nil {
			return
		}
	}
}

// handleNewConn 收到新连接通知：拨本地服务 + 回连工作端口，双向透传
func (c *Client) handleNewConn(connID, tunnelID string) {
	c.mu.Lock()
	t, ok := c.tunnels[tunnelID]
	workPort := c.workPort
	c.mu.Unlock()
	if !ok {
		return
	}
	local, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", t.LocalHost, t.LocalPort), 5*time.Second)
	if err != nil {
		log.Printf("[client] 拨本地 %s:%d 失败: %v", t.LocalHost, t.LocalPort, err)
		return
	}
	work, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", c.serverHost, workPort), 10*time.Second)
	if err != nil {
		local.Close()
		return
	}
	ww := bufio.NewWriter(work)
	if err := writeMsg(ww, Msg{Type: "work_auth", Token: c.token, ConnID: connID}); err != nil {
		local.Close()
		work.Close()
		return
	}
	go pump(local, work, nil, nil) // 本地回包
	pump(work, local, nil, nil)    // 工作连接 -> 本地
	closer(local, work)
}

// openUDPRelay 为 UDP 隧道建立一条到服务端的长连接中继
func (c *Client) openUDPRelay(t TunnelConfig) {
	for {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", c.serverHost, c.workPort), 10*time.Second)
		if err != nil {
			time.Sleep(3 * time.Second)
			continue
		}
		r := bufio.NewReader(conn)
		ww := bufio.NewWriter(conn)
		if err := writeMsg(ww, Msg{Type: "udp_register", Token: c.token, TunnelID: t.ID}); err != nil {
			conn.Close()
			time.Sleep(3 * time.Second)
			continue
		}
		rc := &udpRelayClient{conn: conn, w: ww}
		c.mu.Lock()
		c.relays[t.ID] = rc
		c.mu.Unlock()
		log.Printf("[client] UDP 中继已建立: %s", t.Name)

		broken := false
		for {
			m, err := readMsg(r)
			if err != nil {
				broken = true
				break
			}
			if m.Type == "udp_data" && m.From != "" {
				go c.forwardUDP(t, m.From, m.Data, rc)
			}
		}
		c.mu.Lock()
		if c.relays[t.ID] == rc {
			delete(c.relays, t.ID)
		}
		c.mu.Unlock()
		conn.Close()
		if broken {
			log.Printf("[client] UDP 中继断开，重连: %s", t.Name)
			time.Sleep(3 * time.Second)
		}
	}
}

// forwardUDP 把服务端转发来的 UDP 数据报发往本地，并把本地回复经同一条中继回传
func (c *Client) forwardUDP(t TunnelConfig, from string, data []byte, rc *udpRelayClient) {
	local, err := net.DialTimeout("udp", fmt.Sprintf("%s:%d", t.LocalHost, t.LocalPort), 5*time.Second)
	if err != nil {
		return
	}
	defer local.Close()
	if _, err := local.Write(data); err != nil {
		return
	}
	local.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 65535)
	n, err := local.Read(buf)
	if err != nil || n == 0 {
		return
	}
	_ = rc.writeMsg(Msg{Type: "udp_data", TunnelID: t.ID, From: from, Data: buf[:n]})
}
