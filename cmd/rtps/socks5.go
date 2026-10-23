package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/Liu670/rtp/internal/core"
)

// SOCKS5 局域网网关
//
// 服务端在 ProxyPort 上提供一个带令牌鉴权的 SOCKS5 代理。把 Clash / 本地代理的
// SOCKS5 地址填成「服务端IP:ProxyPort」、账号密码填 rtp 令牌后，所有流量都会从
// 服务端本机出口，从而可以访问服务端所在局域网内的任意设备（192.168.x.x / 10.x.x.x
// 等），就像和服务端处在同一个局域网。该能力独立于客户端是否连接。
//
// 实现遵循 RFC 1928（SOCKS5）+ RFC 1929（用户名/密码子协商）。支持：
//   - CONNECT：TCP 隧道（HTTP/SSH/RDP/SMB/摄像头管理等绝大多数场景）
//   - UDP ASSOCIATE：UDP 中继（DNS over proxy、QUIC、部分游戏等）

const (
	socksVer5     = 0x05
	socksUserPass = 0x02
	socksNoAccept = 0xff

	socksAuthVer = 0x01
	socksAuthOK  = 0x00

	socksCmdConnect  = 0x01
	socksCmdUDPAssoc = 0x03

	socksAtypIPv4   = 0x01
	socksAtypDomain = 0x03
	socksAtypIPv6   = 0x04

	socksRepOK          = 0x00
	socksRepGenErr      = 0x01
	socksRepNetUnreach  = 0x03
	socksRepHostUnreach = 0x04
	socksRepRefused     = 0x05
	socksRepCmdNotSup   = 0x07
	socksRepAddrNotSup  = 0x08
)

// serveSOCKS5 监听并服务 SOCKS5 连接；ProxyPort<=0 时由调用方保证不会进入本函数。
func (s *Server) serveSOCKS5(addr string) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Printf("[socks5] 监听 %s 失败: %v", addr, err)
		core.LogEvent("admin", "warn", fmt.Sprintf("SOCKS5 网关端口 %s 监听失败: %v", addr, err))
		return
	}
	s.mu.Lock()
	s.socksLn = ln
	s.mu.Unlock()
	log.Printf("[socks5] 局域网网关已开启: %s", addr)
	core.LogEvent("admin", "ok", fmt.Sprintf("SOCKS5 局域网网关已开启: %s", addr))
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		go s.handleSOCKS5(c)
	}
}

func (s *Server) handleSOCKS5(conn net.Conn) {
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))

	// 1) 方法协商：仅声明支持「用户名/密码(0x02)」
	hdr := make([]byte, 2)
	if _, err := io.ReadFull(conn, hdr); err != nil {
		return
	}
	if hdr[0] != socksVer5 {
		return
	}
	n := int(hdr[1])
	if n <= 0 || n > 255 {
		return
	}
	methods := make([]byte, n)
	if _, err := io.ReadFull(conn, methods); err != nil {
		return
	}
	if _, err := conn.Write([]byte{socksVer5, socksUserPass}); err != nil {
		return
	}

	// 2) 用户名/密码子协商（令牌即 rtp token）
	if !s.socksAuth(conn) {
		return
	}

	// 鉴权成功，清除握手阶段的读超时
	conn.SetReadDeadline(time.Time{})

	// 3) 请求头
	req := make([]byte, 4)
	if _, err := io.ReadFull(conn, req); err != nil {
		return
	}
	if req[0] != socksVer5 {
		return
	}
	cmd := req[1]
	atyp := req[3]

	target, err := readSocksAddr(conn, atyp)
	if err != nil {
		socksReply(conn, socksRepAddrNotSup, nil)
		return
	}

	switch cmd {
	case socksCmdConnect:
		s.socksConnect(conn, target)
	case socksCmdUDPAssoc:
		s.socksUDPAssociate(conn)
	default:
		socksReply(conn, socksRepCmdNotSup, nil)
	}
}

// socksAuth 处理 RFC 1929 子协商；用户名或密码任一等于有效 rtp 令牌即通过。
func (s *Server) socksAuth(conn net.Conn) bool {
	buf := make([]byte, 2)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return false
	}
	if buf[0] != socksAuthVer {
		return false
	}
	ulen := int(buf[1])
	if ulen > 255 {
		return false
	}
	uname := make([]byte, ulen)
	if _, err := io.ReadFull(conn, uname); err != nil {
		return false
	}
	plen := make([]byte, 1)
	if _, err := io.ReadFull(conn, plen); err != nil {
		return false
	}
	passwd := make([]byte, int(plen[0]))
	if _, err := io.ReadFull(conn, passwd); err != nil {
		return false
	}
	ok := s.proxyAuthOK(string(uname), string(passwd))
	if ok {
		_, _ = conn.Write([]byte{socksAuthVer, socksAuthOK})
	} else {
		_, _ = conn.Write([]byte{socksAuthVer, 0x01})
		log.Printf("[socks5] 鉴权失败（账号或密码错误） from %s", conn.RemoteAddr())
	}
	return ok
}

// readSocksAddr 读取 SOCKS5 请求中的目标地址（IPv4 / 域名 / IPv6），返回 host:port。
func readSocksAddr(conn net.Conn, atyp byte) (string, error) {
	switch atyp {
	case socksAtypIPv4:
		b := make([]byte, 4+2)
		if _, e := io.ReadFull(conn, b); e != nil {
			return "", e
		}
		ip := net.IP(b[:4])
		port := int(binary.BigEndian.Uint16(b[4:]))
		return net.JoinHostPort(ip.String(), strconv.Itoa(port)), nil
	case socksAtypIPv6:
		b := make([]byte, 16+2)
		if _, e := io.ReadFull(conn, b); e != nil {
			return "", e
		}
		ip := net.IP(b[:16])
		port := int(binary.BigEndian.Uint16(b[16:]))
		return net.JoinHostPort(ip.String(), strconv.Itoa(port)), nil
	case socksAtypDomain:
		l := make([]byte, 1)
		if _, e := io.ReadFull(conn, l); e != nil {
			return "", e
		}
		dl := int(l[0])
		db := make([]byte, dl+2)
		if _, e := io.ReadFull(conn, db); e != nil {
			return "", e
		}
		domain := string(db[:dl])
		port := int(binary.BigEndian.Uint16(db[dl:]))
		return net.JoinHostPort(domain, strconv.Itoa(port)), nil
	default:
		return "", fmt.Errorf("unsupported atyp %d", atyp)
	}
}

// socksConnect 处理 CONNECT：从服务端本机拨号到目标并双向透传（含流量统计）。
func (s *Server) socksConnect(conn net.Conn, target string) {
	remote, err := net.DialTimeout("tcp", target, 15*time.Second)
	if err != nil {
		rep := byte(socksRepHostUnreach)
		if ne, ok := err.(net.Error); ok && ne.Timeout() {
			rep = byte(socksRepHostUnreach)
		}
		socksReply(conn, rep, nil)
		return
	}
	defer remote.Close()
	log.Printf("[socks5] CONNECT %s", target)
	core.LogEvent("admin", "info", "SOCKS5 访问: "+target)

	socksReply(conn, socksRepOK, nil)

	id, upC, downC := s.px.openConn(hostOnly(conn.RemoteAddr().String()), target, "socks5")
	defer s.px.closeConn(id)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); core.Pump(remote, conn, upC, nil) }()
	go func() { defer wg.Done(); core.Pump(conn, remote, downC, nil) }()
	wg.Wait()
}

// socksUDPAssociate 处理 UDP ASSOCIATE：服务端开一个 UDP 监听，把客户端发来的
// SOCKS UDP 数据报转发到目标，并把目标的回包经 SOCKS UDP 头回传给客户端。
func (s *Server) socksUDPAssociate(conn net.Conn) {
	// 返回给客户端的绑定地址：取 TCP 连接本端 IP（客户端连到的那个本地 IP）
	tcpLocal, ok := conn.LocalAddr().(*net.TCPAddr)
	if !ok {
		socksReply(conn, socksRepGenErr, nil)
		return
	}
	udpLn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		socksReply(conn, socksRepGenErr, nil)
		return
	}
	var (
		clientAddr *net.UDPAddr
		relays     = map[string]*net.UDPConn{}
		rmu        sync.Mutex
	)
	defer func() {
		udpLn.Close()
		rmu.Lock()
		for _, rc := range relays {
			rc.Close()
		}
		rmu.Unlock()
	}()

	bndIP := tcpLocal.IP
	if bndIP == nil || bndIP.IsUnspecified() {
		bndIP = net.IPv4zero
	}
	socksReply(conn, socksRepOK, &socksAddr{IP: bndIP, Port: udpLn.LocalAddr().(*net.UDPAddr).Port})

	buf := make([]byte, 65535)

	// TCP 关闭时一并关闭 UDP 监听，结束本次关联
	go func() {
		_, _ = io.Copy(io.Discard, conn)
		udpLn.Close()
	}()

	for {
		n, from, err := udpLn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		if clientAddr == nil {
			clientAddr = from
		}
		// 解析 SOCKS UDP 头：RSV(2) FRAG(1) ATYP(1) DST... DATA
		if n < 4 {
			continue
		}
		atyp := buf[3]
		var target *net.UDPAddr
		var dataOff int
		switch atyp {
		case socksAtypIPv4:
			if n < 4+4+2 {
				continue
			}
			target = &net.UDPAddr{IP: net.IP(append([]byte(nil), buf[4:8]...)), Port: int(binary.BigEndian.Uint16(buf[8:10]))}
			dataOff = 10
		case socksAtypIPv6:
			if n < 4+16+2 {
				continue
			}
			target = &net.UDPAddr{IP: net.IP(append([]byte(nil), buf[4:20]...)), Port: int(binary.BigEndian.Uint16(buf[20:22]))}
			dataOff = 22
		case socksAtypDomain:
			if n < 5 {
				continue
			}
			dl := int(buf[4])
			if n < 5+dl+2 {
				continue
			}
			domain := string(buf[5 : 5+dl])
			port := int(binary.BigEndian.Uint16(buf[5+dl : 5+dl+2]))
			ra, e := net.ResolveUDPAddr("udp", net.JoinHostPort(domain, strconv.Itoa(port)))
			if e != nil {
				continue
			}
			target = ra
			dataOff = 5 + dl + 2
		default:
			continue
		}

		key := target.String()
		rmu.Lock()
		rc, exists := relays[key]
		if !exists {
			rc, err = net.DialUDP("udp", nil, target)
			if err != nil {
				rmu.Unlock()
				continue
			}
			relays[key] = rc
			ca := clientAddr
			go func(rc *net.UDPConn, ca *net.UDPAddr) {
				b2 := make([]byte, 65535)
				for {
					m, e := rc.Read(b2)
					if e != nil {
						return
					}
					if ca == nil {
						continue
					}
					out := append(buildSocksUDPHeader(target), b2[:m]...)
					_, _ = udpLn.WriteToUDP(out, ca)
				}
			}(rc, ca)
		}
		rmu.Unlock()
		_, _ = rc.Write(buf[dataOff:n])
	}
}

type socksAddr struct {
	IP   net.IP
	Port int
}

// socksReply 写 SOCKS5 应答；bind 非 nil 时回显绑定地址，否则用 0.0.0.0:0。
func socksReply(conn net.Conn, rep byte, bind *socksAddr) {
	if bind == nil || bind.IP == nil {
		_, _ = conn.Write([]byte{socksVer5, rep, 0x00, socksAtypIPv4, 0, 0, 0, 0, 0, 0})
		return
	}
	var atyp byte = socksAtypIPv4
	ipb := bind.IP.To4()
	if ipb == nil {
		atyp = socksAtypIPv6
		ipb = bind.IP.To16()
	}
	if ipb == nil {
		ipb = []byte{0, 0, 0, 0}
	}
	port := make([]byte, 2)
	binary.BigEndian.PutUint16(port, uint16(bind.Port))
	_, _ = conn.Write(append([]byte{socksVer5, rep, 0x00, atyp}, append(ipb, port...)...))
}

// buildSocksUDPHeader 构造 SOCKS UDP 数据报头：RSV(0,0) FRAG(0) ATYP DST ADDR PORT。
func buildSocksUDPHeader(a *net.UDPAddr) []byte {
	var atyp byte = socksAtypIPv4
	ipb := a.IP.To4()
	if ipb == nil {
		atyp = socksAtypIPv6
		ipb = a.IP.To16()
	}
	if ipb == nil {
		atyp = socksAtypIPv4
		ipb = []byte{0, 0, 0, 0}
	}
	port := make([]byte, 2)
	binary.BigEndian.PutUint16(port, uint16(a.Port))
	return append([]byte{0, 0, 0, atyp}, append(ipb, port...)...)
}
