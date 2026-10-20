package main

import (
	"github.com/Liu670/rtp/internal/core"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"time"
)

// ---------- P2P 候选地址与状态 ----------

// p2pTunnelsFor 返回涉及某客户端（作为 target 或 visitor）的 P2P 隧道配置
func (s *Server) p2pTunnelsFor(clientID string) []TunnelConfig {
	s.mu.Lock()
	var out []TunnelConfig
	for _, t := range s.tunnels {
		if t.Proto == "p2p" && (t.ClientID == clientID || t.VisitorClientID == clientID) {
			out = append(out, t.TunnelConfig)
		}
	}
	s.mu.Unlock()
	return out
}

func (s *Server) setCandidate(clientID string, addr net.Addr) {
	s.p2pMu.Lock()
	s.p2pCandidates[clientID] = addr
	s.p2pMu.Unlock()
	for _, tc := range s.p2pTunnelsFor(clientID) {
		s.tryP2POffer(tc)
	}
}

func (s *Server) getCandidate(clientID string) (net.Addr, bool) {
	s.p2pMu.Lock()
	defer s.p2pMu.Unlock()
	a, ok := s.p2pCandidates[clientID]
	return a, ok
}

func (s *Server) setP2PState(tunnelID, st string) {
	s.p2pMu.Lock()
	s.p2pStates[tunnelID] = st
	s.p2pMu.Unlock()
}

func (s *Server) getP2PState(tunnelID string) string {
	s.p2pMu.Lock()
	defer s.p2pMu.Unlock()
	return s.p2pStates[tunnelID]
}

func (s *Server) controlOf(id string) *controlConn {
	s.controlsMu.Lock()
	defer s.controlsMu.Unlock()
	return s.controls[id]
}

func (s *Server) clientByToken(tok string) *ClientState {
	s.controlsMu.Lock()
	defer s.controlsMu.Unlock()
	if tok == s.cfg.AdminToken {
		return s.clients["default"]
	}
	for _, c := range s.clients {
		if c.Token == tok {
			return c
		}
	}
	return nil
}

// ---------- P2P 信令 UDP 监听 ----------

func (s *Server) startP2PListener(addr string) {
	pc, err := net.ListenPacket("udp", addr)
	if err != nil {
		log.Printf("[p2p] 信令端口 %s 监听失败: %v", addr, err)
		return
	}
	log.Printf("[p2p] 信令监听 %s", addr)
	go func() {
		buf := make([]byte, 1500)
		for {
			n, from, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			var m Msg
			if err := json.Unmarshal(buf[:n], &m); err != nil {
				continue
			}
			if m.Type == "p2p_ping" {
				cs := s.clientByToken(m.Token)
				if cs == nil {
					continue
				}
				s.setCandidate(cs.ID, from)
				_ = writeUDP(pc, from, Msg{Type: "p2p_pong"})
			}
		}
	}()
}

func writeUDP(pc net.PacketConn, addr net.Addr, m Msg) error {
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	_, err = pc.WriteTo(b, addr)
	return err
}

// ---------- P2P offer 下发 ----------

func (s *Server) tryP2POffer(tc TunnelConfig) {
	if tc.Proto != "p2p" {
		return
	}
	// 已在下发过/直连中则跳过，避免每次 ping 重复推送
	if st := s.getP2PState(tc.ID); st == "punching" || st == "direct" {
		return
	}
	ca, okA := s.getCandidate(tc.ClientID)        // target 候选
	cb, okB := s.getCandidate(tc.VisitorClientID) // visitor 候选
	if !okA || !okB {
		s.setP2PState(tc.ID, "waiting")
		return
	}
	if !s.clientOnline(tc.ClientID) || !s.clientOnline(tc.VisitorClientID) {
		s.setP2PState(tc.ID, "waiting")
		return
	}
	s.setP2PState(tc.ID, "punching")
	session := core.RandID()
	if cc := s.controlOf(tc.ClientID); cc != nil {
		_ = cc.write(Msg{Type: "p2p_offer", TunnelID: tc.ID, Role: "target", PeerAddr: cb.String(), Session: session})
	}
	if cc := s.controlOf(tc.VisitorClientID); cc != nil {
		_ = cc.write(Msg{Type: "p2p_offer", TunnelID: tc.ID, Role: "visitor", PeerAddr: ca.String(), VisitorPort: tc.VisitorPort, Session: session})
	}
	core.LogEvent("info", fmt.Sprintf("P2P 穿透启动: %s (target=%s visitor=%s)", tc.Name, tc.ClientID, tc.VisitorClientID))
}

// 客户端上线后，对其相关的 p2p 隧道尝试下发 offer
func (s *Server) onClientOnline(clientID string) {
	for _, tc := range s.p2pTunnelsFor(clientID) {
		s.tryP2POffer(tc)
	}
}

// ---------- P2P relay 回退（服务器中转） ----------

func (s *Server) handleP2PRelayOpen(tunnelID, visitorConnID string) {
	s.mu.Lock()
	t, ok := s.tunnels[tunnelID]
	s.mu.Unlock()
	if !ok {
		return
	}
	csT := s.clientOf(t.ClientID)
	csV := s.clientOf(t.VisitorClientID)
	if csT == nil || csT.cc == nil || csV == nil || csV.cc == nil {
		return
	}
	targetConnID := "p2p-" + core.RandID()[:10]
	chA := make(chan net.Conn, 1)
	chB := make(chan net.Conn, 1)
	s.pendingMu.Lock()
	s.pending[targetConnID] = chA
	s.pending[visitorConnID] = chB
	s.pendingMu.Unlock()
	// target 端建立到内网服务的连接
	_ = csT.cc.write(Msg{Type: "new_conn", ConnID: targetConnID, TunnelID: t.ID})
	// visitor 端建立到自身本地监听端口的连接
	_ = csV.cc.write(Msg{Type: "relay_accept", ConnID: visitorConnID})
	s.setP2PState(tunnelID, "relay")
	core.LogEvent("warn", fmt.Sprintf("P2P 回退到服务器中转: %s", t.Name))

	go func() {
		var a, b net.Conn
		select {
		case a = <-chA:
		case <-time.After(20 * time.Second):
		}
		select {
		case b = <-chB:
		case <-time.After(20 * time.Second):
		}
		s.pendingMu.Lock()
		delete(s.pending, targetConnID)
		delete(s.pending, visitorConnID)
		s.pendingMu.Unlock()
		if a != nil && b != nil {
			go core.Pump(b, a, nil, nil)
			core.Pump(a, b, nil, nil)
			core.CloseBoth(a, b)
		} else {
			if a != nil {
				a.Close()
			}
			if b != nil {
				b.Close()
			}
		}
	}()
}
