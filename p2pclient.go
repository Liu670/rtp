package main

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"sync"
	"time"
)

// ---------- 客户端 P2P 状态 ----------

func (c *Client) ensureP2P() {
	c.mu.Lock()
	if c.p2pConn != nil {
		c.mu.Unlock()
		return
	}
	// 使用未连接的 UDP 套接字，可同时向服务器(信令)和对端(打洞/数据)收发
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		c.mu.Unlock()
		return
	}
	srv, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", c.serverHost, c.p2pPort))
	if err != nil {
		conn.Close()
		c.mu.Unlock()
		return
	}
	c.p2pConn = conn
	c.p2pServerAddr = srv
	c.p2pLinks = map[string]*p2pLink{}
	c.p2pPeer = map[string]string{}
	c.p2pRelayLocal = map[string]net.Conn{}
	c.p2pListeners = map[string]net.Listener{}
	c.mu.Unlock()
	go c.p2pPingLoop()
	go c.p2pRecvLoop()
}

func (c *Client) p2pPingLoop() {
	tick := time.NewTicker(3 * time.Second)
	defer tick.Stop()
	c.sendP2PPing()
	for range tick.C {
		c.mu.Lock()
		dead := c.p2pConn == nil
		c.mu.Unlock()
		if dead {
			return
		}
		c.sendP2PPing()
	}
}

func (c *Client) sendP2PPing() {
	c.mu.Lock()
	conn := c.p2pConn
	srv := c.p2pServerAddr
	c.mu.Unlock()
	if conn == nil || srv == nil {
		return
	}
	b, _ := json.Marshal(Msg{Type: "p2p_ping", Token: c.token, ClientID: c.name})
	conn.WriteTo(b, srv)
}

func (c *Client) p2pRecvLoop() {
	buf := make([]byte, 4096)
	for {
		c.mu.Lock()
		conn := c.p2pConn
		srv := c.p2pServerAddr
		c.mu.Unlock()
		if conn == nil {
			return
		}
		n, from, err := conn.ReadFrom(buf)
		if err != nil {
			return
		}
		if srv != nil && from.String() == srv.String() {
			continue // 服务器 pong
		}
		// RUDP 帧是裸二进制（非 JSON），交给对应 link 解析
		link := c.linkForAddr(from.String())
		if link == nil {
			continue
		}
		link.onPacket(buf[:n])
	}
}

func (c *Client) linkForAddr(addr string) *p2pLink {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.p2pLinks[addr]
}

func (c *Client) getOrCreateLink(peerAddr string) *p2pLink {
	c.mu.Lock()
	defer c.mu.Unlock()
	if l, ok := c.p2pLinks[peerAddr]; ok {
		return l
	}
	l := newP2PLink(c, peerAddr)
	c.p2pLinks[peerAddr] = l
	return l
}

// ---------- P2P offer / 隧道设置 ----------

func (c *Client) handleP2POffer(m Msg) {
	c.ensureP2P()
	c.mu.Lock()
	t, ok := c.tunnels[m.TunnelID]
	if !ok {
		c.mu.Unlock()
		return
	}
	c.p2pPeer[m.TunnelID] = m.PeerAddr
	if m.Role == "visitor" {
		t.VisitorPort = m.VisitorPort
		c.tunnels[m.TunnelID] = t
	}
	c.mu.Unlock()

	link := c.getOrCreateLink(m.PeerAddr)
	// 双向打洞
	go func() {
		end := time.Now().Add(10 * time.Second)
		for time.Now().Before(end) {
			link.punchOnce()
			time.Sleep(400 * time.Millisecond)
		}
	}()

	if m.Role == "visitor" {
		c.startP2PVisitor(t)
	}
	log.Printf("[p2p] 收到 offer 角色=%s 对端=%s 隧道=%s", m.Role, m.PeerAddr, t.Name)
}

func (c *Client) startP2PVisitor(t TunnelConfig) {
	c.mu.Lock()
	if _, exists := c.p2pListeners[t.ID]; exists {
		c.mu.Unlock()
		return
	}
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", t.VisitorPort))
	if err != nil {
		c.mu.Unlock()
		log.Printf("[p2p] visitor 监听端口 %d 失败: %v", t.VisitorPort, err)
		return
	}
	c.p2pListeners[t.ID] = ln
	c.mu.Unlock()
	log.Printf("[p2p] visitor 监听本地 %d -> 隧道 %s", t.VisitorPort, t.Name)
	go func() {
		for {
			lc, err := ln.Accept()
			if err != nil {
				return
			}
			go c.handleP2PVisitorConn(t, lc)
		}
	}()
}

// 访问端本地连接：优先 P2P 直连，超时回退服务器中转
func (c *Client) handleP2PVisitorConn(t TunnelConfig, lc net.Conn) {
	c.mu.Lock()
	peer := c.p2pPeer[t.ID]
	c.mu.Unlock()
	link := c.linkForAddr(peer)
	if link != nil && link.established {
		link.openStream(t, lc)
		return
	}
	if link != nil && link.tryConnect(t, lc, 6*time.Second) {
		return
	}
	// 回退：请求服务器中转
	connID := "r-" + randID()[:10]
	c.mu.Lock()
	c.p2pRelayLocal[connID] = lc
	c.mu.Unlock()
	select {
	case c.ctrlSend <- Msg{Type: "p2p_relay_open", TunnelID: t.ID, ConnID: connID}:
	default:
	}
}

func (c *Client) handleRelayAccept(connID string) {
	c.mu.Lock()
	lc, ok := c.p2pRelayLocal[connID]
	if ok {
		delete(c.p2pRelayLocal, connID)
	}
	wp := c.workPort
	c.mu.Unlock()
	if !ok {
		return
	}
	work, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", c.serverHost, wp), 10*time.Second)
	if err != nil {
		lc.Close()
		return
	}
	ww := bufio.NewWriter(work)
	if err := writeMsg(ww, Msg{Type: "work_auth", Token: c.token, ConnID: connID}); err != nil {
		lc.Close()
		work.Close()
		return
	}
	go pump(lc, work, nil, nil)
	pump(work, lc, nil, nil)
	closer(lc, work)
}

func (c *Client) tunnelByID(id string) *TunnelConfig {
	c.mu.Lock()
	defer c.mu.Unlock()
	if t, ok := c.tunnels[id]; ok {
		return &t
	}
	return nil
}

// notifyP2PLink 通知服务端 P2P 直连已建立（用于 UI 状态）
func (c *Client) notifyP2PLink(tunnelID string) {
	c.mu.Lock()
	ch := c.ctrlSend
	c.mu.Unlock()
	if ch == nil {
		return
	}
	select {
	case ch <- Msg{Type: "p2p_link", TunnelID: tunnelID, Ok: true}:
	default:
	}
}

// ---------- RUDP：UDP 上的可靠、按流多路字节传输 ----------

const (
	ruMagic   = 0x5254 // 'RT'
	ruSYN     = 1
	ruDAT     = 2
	ruFIN     = 3
	ruWIN     = 16
	ruMSS     = 1300
	ruRTO     = 250 * time.Millisecond
	ruMaxWait = 40
)

type p2pLink struct {
	client *Client
	conn   *net.UDPConn
	peer   string
	mu     sync.Mutex
	nextGSeq uint32
	sendBuf  map[uint32][]byte
	ackdGSeq uint32
	recvNext uint32
	ooo      map[uint32][]byte
	established bool
	streams  map[uint16]*p2pStream
	dead     bool
	retry    int
}

func newP2PLink(client *Client, peer string) *p2pLink {
	l := &p2pLink{
		client:  client,
		conn:    client.p2pConn,
		peer:    peer,
		sendBuf: map[uint32][]byte{},
		ooo:     map[uint32][]byte{},
		streams: map[uint16]*p2pStream{},
	}
	go l.retransmit()
	return l
}

func (l *p2pLink) peerAddr() *net.UDPAddr {
	a, _ := net.ResolveUDPAddr("udp", l.peer)
	return a
}

func (l *p2pLink) punchOnce() {
	a := l.peerAddr()
	if a == nil {
		return
	}
	l.conn.WriteTo([]byte("RT-PUNCH"), a)
}

func ruEncode(typ uint8, stream uint16, gseq, gack uint32, data []byte) []byte {
	b := make([]byte, 15+len(data))
	binary.BigEndian.PutUint16(b[0:2], ruMagic)
	b[2] = typ
	binary.BigEndian.PutUint16(b[3:5], stream)
	binary.BigEndian.PutUint32(b[5:9], gseq)
	binary.BigEndian.PutUint32(b[9:13], gack)
	binary.BigEndian.PutUint16(b[13:15], uint16(len(data)))
	copy(b[15:], data)
	return b
}

func (l *p2pLink) sendRaw(b []byte) {
	a := l.peerAddr()
	if a == nil {
		return
	}
	l.conn.WriteTo(b, a)
}

func (l *p2pLink) sendMsg(typ uint8, stream uint16, data []byte) uint32 {
	l.mu.Lock()
	gseq := l.nextGSeq
	l.nextGSeq++
	b := ruEncode(typ, stream, gseq, l.recvNext, data)
	l.sendBuf[gseq] = b
	l.mu.Unlock()
	l.sendRaw(b)
	return gseq
}

func (l *p2pLink) retransmit() {
	tick := time.NewTicker(ruRTO)
	defer tick.Stop()
	for range tick.C {
		l.mu.Lock()
		if l.dead {
			l.mu.Unlock()
			return
		}
		if len(l.sendBuf) == 0 {
			l.mu.Unlock()
			continue
		}
		l.retry++
		dead := l.retry > ruMaxWait
		for _, b := range l.sendBuf {
			l.sendRaw(b)
		}
		if dead {
			l.dead = true
			l.mu.Unlock()
			l.killStreams()
			return
		}
		l.mu.Unlock()
	}
}

func (l *p2pLink) onPacket(b []byte) {
	if len(b) < 15 {
		return
	}
	if binary.BigEndian.Uint16(b[0:2]) != ruMagic {
		return
	}
	typ := b[2]
	stream := binary.BigEndian.Uint16(b[3:5])
	gseq := binary.BigEndian.Uint32(b[5:9])
	gack := binary.BigEndian.Uint32(b[9:13])
	dlen := int(binary.BigEndian.Uint16(b[13:15]))
	var data []byte
	if dlen > 0 && len(b) >= 15+dlen {
		data = b[15 : 15+dlen]
	}
	l.mu.Lock()
	l.established = true
	for k := range l.sendBuf {
		if k < gack {
			delete(l.sendBuf, k)
		}
	}
	if len(l.sendBuf) == 0 {
		l.retry = 0
	}
	var toDeliver [][]byte
	if gseq == l.recvNext {
		l.recvNext++
		toDeliver = append(toDeliver, data)
		for {
			nb, ok := l.ooo[l.recvNext]
			if !ok {
				break
			}
			delete(l.ooo, l.recvNext)
			toDeliver = append(toDeliver, nb)
			l.recvNext++
		}
	} else if gseq > l.recvNext {
		l.ooo[gseq] = data
	}
	l.mu.Unlock()
	l.sendAck()
	for _, d := range toDeliver {
		l.deliver(typ, stream, d)
	}
}

func (l *p2pLink) sendAck() {
	l.mu.Lock()
	g := l.recvNext
	dead := l.dead
	l.mu.Unlock()
	if dead {
		return
	}
	l.sendRaw(ruEncode(ruDAT, 0, 0, g, nil))
}

func (l *p2pLink) deliver(typ uint8, stream uint16, data []byte) {
	if len(data) < 4 {
		return
	}
	streamSeq := binary.BigEndian.Uint32(data[0:4])
	payload := data[4:]
	l.mu.Lock()
	st, ok := l.streams[stream]
	if !ok {
		if typ == ruSYN {
			st = &p2pStream{id: stream, expectedSeq: 0, buf: map[uint32][]byte{}, link: l}
			l.streams[stream] = st
			l.mu.Unlock()
			st.onSyn(payload)
			return
		}
		l.mu.Unlock()
		return
	}
	l.mu.Unlock()
	st.push(streamSeq, payload, typ == ruFIN)
}

// tryConnect 访问端尝试 P2P 握手；成功返回 true 并把 lc 接入流
func (l *p2pLink) tryConnect(t TunnelConfig, lc net.Conn, timeout time.Duration) bool {
	st := l.newStream()
	ch := make(chan bool, 1)
	st.estCh = ch
	l.registerStream(st)
	synData := make([]byte, 4+1+len(t.ID))
	binary.BigEndian.PutUint32(synData[0:4], 0)
	synData[4] = byte(len(t.ID))
	copy(synData[5:], t.ID)
	l.sendMsg(ruSYN, st.id, synData)
	select {
	case <-ch:
		st.attachLocal(lc)
		go st.localPump(lc)
		st.link.client.notifyP2PLink(t.ID)
		return true
	case <-time.After(timeout):
		return false
	}
}

func (l *p2pLink) openStream(t TunnelConfig, lc net.Conn) {
	st := l.newStream()
	l.registerStream(st)
	st.attachLocal(lc)
	synData := make([]byte, 4+1+len(t.ID))
	binary.BigEndian.PutUint32(synData[0:4], 0)
	synData[4] = byte(len(t.ID))
	copy(synData[5:], t.ID)
	l.sendMsg(ruSYN, st.id, synData)
	go st.localPump(lc)
	st.link.client.notifyP2PLink(t.ID)
}

func (l *p2pLink) newStream() *p2pStream {
	return &p2pStream{id: uint16(time.Now().UnixNano() % 65536), expectedSeq: 0, buf: map[uint32][]byte{}, link: l}
}

func (l *p2pLink) registerStream(st *p2pStream) {
	l.mu.Lock()
	l.streams[st.id] = st
	l.mu.Unlock()
}

func (l *p2pLink) killStreams() {
	l.mu.Lock()
	sts := make([]*p2pStream, 0, len(l.streams))
	for _, s := range l.streams {
		sts = append(sts, s)
	}
	l.mu.Unlock()
	for _, s := range sts {
		s.closeLocal()
	}
}

// ---------- P2P 流 ----------

type p2pStream struct {
	id          uint16
	expectedSeq uint32
	buf         map[uint32][]byte
	mu          sync.Mutex
	link        *p2pLink
	local       net.Conn
	estCh       chan bool
	closed      bool
}

func (st *p2pStream) onSyn(data []byte) {
	if len(data) < 1 {
		return
	}
	tlen := int(data[0])
	if len(data) < 1+tlen {
		return
	}
	tunnelID := string(data[1 : 1+tlen])
	payload := data[1+tlen:]
	tc := st.link.client.tunnelByID(tunnelID)
	if tc == nil {
		return
	}
	st.link.client.notifyP2PLink(tunnelID)
	remote, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", tc.LocalHost, tc.LocalPort), 5*time.Second)
	if err != nil {
		return
	}
	st.attachLocal(remote)
	log.Printf("[p2p] 目标端建立流到 %s:%d", tc.LocalHost, tc.LocalPort)
	go st.remotePump(remote)
	if len(payload) > 0 {
		st.push(0, payload, false)
	}
}

func (st *p2pStream) attachLocal(lc net.Conn) {
	st.mu.Lock()
	st.local = lc
	// 把已缓冲的有序数据刷到本地
	for {
		d, ok := st.buf[st.expectedSeq]
		if !ok {
			break
		}
		delete(st.buf, st.expectedSeq)
		st.expectedSeq++
		if len(d) > 0 {
			lc.Write(d)
		}
	}
	st.mu.Unlock()
}

func (st *p2pStream) closeLocal() {
	st.mu.Lock()
	if st.local != nil {
		st.local.Close()
	}
	st.closed = true
	st.mu.Unlock()
}

// push 按 streamSeq 顺序写入本地连接
func (st *p2pStream) push(streamSeq uint32, payload []byte, fin bool) {
	st.mu.Lock()
	if st.closed {
		st.mu.Unlock()
		return
	}
	st.buf[streamSeq] = payload
	for {
		d, ok := st.buf[st.expectedSeq]
		if !ok {
			break
		}
		delete(st.buf, st.expectedSeq)
		st.expectedSeq++
		if st.local != nil && len(d) > 0 {
			st.local.Write(d)
		}
	}
	if fin {
		st.closed = true
		if st.local != nil {
			st.local.Close()
		}
	}
	if st.estCh != nil {
		select {
		case st.estCh <- true:
		default:
		}
		st.estCh = nil
	}
	st.mu.Unlock()
}

// localPump 本地连接 -> 对端（访问端）
func (st *p2pStream) localPump(lc net.Conn) {
	buf := make([]byte, ruMSS)
	seq := uint32(0)
	for {
		n, err := lc.Read(buf)
		if n > 0 {
			out := make([]byte, 4+n)
			binary.BigEndian.PutUint32(out[0:4], seq)
			copy(out[4:], buf[:n])
			seq++
			st.link.sendMsg(ruDAT, st.id, out)
		}
		if err != nil {
			st.link.sendMsg(ruFIN, st.id, ruFinPayload(seq))
			return
		}
	}
}

// remotePump 内网服务 -> 对端（目标端）
func (st *p2pStream) remotePump(remote net.Conn) {
	buf := make([]byte, ruMSS)
	seq := uint32(0)
	for {
		n, err := remote.Read(buf)
		if n > 0 {
			out := make([]byte, 4+n)
			binary.BigEndian.PutUint32(out[0:4], seq)
			copy(out[4:], buf[:n])
			seq++
			st.link.sendMsg(ruDAT, st.id, out)
		}
		if err != nil {
			st.link.sendMsg(ruFIN, st.id, ruFinPayload(seq))
			return
		}
	}
}

func ruFinPayload(streamSeq uint32) []byte {
	out := make([]byte, 4)
	binary.BigEndian.PutUint32(out[0:4], streamSeq)
	return out
}
