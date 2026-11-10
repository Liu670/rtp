package main

// NAT 兼容的 SOCKS5 UDP 中继。
//
// 原实现的问题：每个 UDP ASSOCIATE 单独开一个随机端口 UDP socket，并把服务器
// 内网 IP（如 10.x.x.x）作为 BND.ADDR 返回给客户端。在 NAT / 端口映射场景下
// （共享 IP 服务商只映射固定端口），客户端拿到的地址公网不可达，所有走 UDP 的
// 流量（DNS over proxy、QUIC 等）全部失败 —— 典型表现为手机端 VPN 全局模式下
// 浏览器报「网络故障」，而带域名直传的 TCP CONNECT 流量照常工作。
//
// 改造方案：每个代理监听（普通 SOCKS5 / TLS）共享一个 UDP socket，绑定在与
// TCP 相同的内部端口号上（UDP/TCP 端口空间相互独立），NAT 把外部端口转发到
// 内部端口时 TCP 与 UDP 一起透传；BND.ADDR 返回配置的公网地址
// （base_domain + 对外端口）。
//
// 会话归属遵循 RFC 1968 惯例：客户端的 UDP 源端口与 TCP 控制连接端口不同，
// 服务端把 ASSOCIATE 后收到的第一个 UDP 数据报来源绑定到最早未匹配的会话。

import (
	"encoding/binary"
	"io"
	"log"
	"net"
	"strconv"
	"sync"
)

// udpAssoc 一次 UDP ASSOCIATE 会话：控制 TCP 连接 + 客户端 UDP 地址 + 按目标缓存的 socket
type udpAssoc struct {
	ctrl    net.Conn
	ca      *net.UDPAddr // 客户端 UDP 源地址（收到首个数据报后绑定）
	targets map[string]*net.UDPConn
	tmu     sync.Mutex
}

// closeTargets 关闭该会话的所有目标 socket（下行转发 goroutine 随之退出）
func (a *udpAssoc) closeTargets() {
	a.tmu.Lock()
	for _, rc := range a.targets {
		_ = rc.Close()
	}
	a.targets = map[string]*net.UDPConn{}
	a.tmu.Unlock()
}

// proxyUDPEnv 一个代理监听的共享 UDP 中继环境
type proxyUDPEnv struct {
	pc      *net.UDPConn // 共享 UDP socket（内部端口 = TCP 监听端口）
	pubHost string      // 对外公网地址（配置的 base_domain，空 = 0.0.0.0）
	pubPort int         // 对外公网端口（NAT 映射号）
	mu      sync.Mutex
	pending []*udpAssoc          // 已 ASSOCIATE 但尚未绑定 UDP 来源的会话
	assocs  map[string]*udpAssoc // 客户端 UDP 源地址 -> 会话
}

// startProxyUDPEnv 绑定共享 UDP 端口并启动上行分发循环；失败返回 nil（退化为本地模式）
func (s *Server) startProxyUDPEnv(tcpPort int, pubHost string, pubPort int) *proxyUDPEnv {
	pc, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4zero, Port: tcpPort})
	if err != nil {
		log.Printf("[socks5-udp] 共享 UDP 端口 %d 绑定失败: %v（UDP 中继不可用）", tcpPort, err)
		return nil
	}
	env := &proxyUDPEnv{pc: pc, pubHost: pubHost, pubPort: pubPort, assocs: map[string]*udpAssoc{}}
	go env.uplinkLoop()
	return env
}

// stop 关闭共享 socket 并清理所有会话
func (env *proxyUDPEnv) stop() {
	if env == nil {
		return
	}
	env.mu.Lock()
	assocs := env.assocs
	pending := env.pending
	env.assocs = map[string]*udpAssoc{}
	env.pending = nil
	env.mu.Unlock()
	_ = env.pc.Close()
	for _, a := range assocs {
		a.closeTargets()
	}
	for _, a := range pending {
		a.closeTargets()
	}
}

// register 登记一次 UDP ASSOCIATE（会话先进入 pending，待首个 UDP 数据报绑定来源）
func (env *proxyUDPEnv) register(ctrl net.Conn) *udpAssoc {
	a := &udpAssoc{ctrl: ctrl, targets: map[string]*net.UDPConn{}}
	env.mu.Lock()
	env.pending = append(env.pending, a)
	env.mu.Unlock()
	return a
}

// unregister 会话结束：从 pending 与 assocs 中移除并关闭目标 socket
func (env *proxyUDPEnv) unregister(a *udpAssoc) {
	env.mu.Lock()
	for i, p := range env.pending {
		if p == a {
			env.pending = append(env.pending[:i], env.pending[i+1:]...)
			break
		}
	}
	for k, v := range env.assocs {
		if v == a {
			delete(env.assocs, k)
		}
	}
	env.mu.Unlock()
	a.closeTargets()
}

// targetFor 取（或创建）到指定目标的已连接 UDP socket，并启动回包转发 goroutine
func (env *proxyUDPEnv) targetFor(a *udpAssoc, key string, target *net.UDPAddr) *net.UDPConn {
	a.tmu.Lock()
	defer a.tmu.Unlock()
	if rc, ok := a.targets[key]; ok {
		return rc
	}
	rc, err := net.DialUDP("udp", nil, target)
	if err != nil {
		return nil
	}
	a.targets[key] = rc
	go func() {
		b := make([]byte, 65535)
		for {
			n, e := rc.Read(b)
			if e != nil {
				a.tmu.Lock()
				if a.targets[key] == rc {
					delete(a.targets, key)
				}
				a.tmu.Unlock()
				return
			}
			if a.ca == nil {
				return
			}
			out := append(buildSocksUDPHeader(target), b[:n]...)
			_, _ = env.pc.WriteToUDP(out, a.ca)
		}
	}()
	return rc
}

// uplinkLoop 上行分发：客户端 UDP 数据报 → 解析 SOCKS UDP 头 → 转发到目标
func (env *proxyUDPEnv) uplinkLoop() {
	buf := make([]byte, 65535)
	for {
		n, from, err := env.pc.ReadFromUDP(buf)
		if err != nil {
			return // socket 已关闭
		}
		env.mu.Lock()
		a := env.assocs[from.String()]
		if a == nil && len(env.pending) > 0 {
			// 首个数据报：绑定到最早未匹配的 ASSOCIATE 会话（RFC 1968 惯例）
			a = env.pending[0]
			env.pending = env.pending[1:]
			a.ca = from
			env.assocs[from.String()] = a
			log.Printf("[socks5-udp] 客户端 UDP 已绑定: %s", from)
		}
		env.mu.Unlock()
		if a == nil {
			continue // 未经 ASSOCIATE 登记的来源（公网扫描噪音大，静默丢弃）
		}
		if n < 4 || buf[2] != 0 {
			continue // 长度不足或分片（FRAG>0 不支持）
		}
		atyp := buf[3]
		var target *net.UDPAddr
		var off int
		switch atyp {
		case socksAtypIPv4:
			if n < 4+4+2 {
				continue
			}
			target = &net.UDPAddr{IP: net.IP(append([]byte(nil), buf[4:8]...)), Port: int(binary.BigEndian.Uint16(buf[8:10]))}
			off = 10
		case socksAtypIPv6:
			if n < 4+16+2 {
				continue
			}
			target = &net.UDPAddr{IP: net.IP(append([]byte(nil), buf[4:20]...)), Port: int(binary.BigEndian.Uint16(buf[20:22]))}
			off = 22
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
			off = 5 + dl + 2
		default:
			continue
		}
		rc := env.targetFor(a, target.String(), target)
		if rc == nil {
			log.Printf("[socks5-udp] 目标 socket 创建失败: %s", target)
			continue
		}
		if _, werr := rc.Write(buf[off:n]); werr != nil {
			log.Printf("[socks5-udp] 转发失败 -> %s: %v", target, werr)
		}
	}
}

// socksUDPAssociateShared 处理 UDP ASSOCIATE：登记会话后通告公网地址，
// 上/下行转发由共享 socket 的循环完成；控制连接关闭即结束会话。
func (s *Server) socksUDPAssociateShared(conn net.Conn, env *proxyUDPEnv) {
	// BND.ADDR：返回客户端实际可达的公网地址（NAT 映射号），而非服务器内网 IP
	pub := &socksAddr{IP: net.IPv4zero, Port: env.pubPort}
	if ip := net.ParseIP(env.pubHost); ip != nil {
		if v4 := ip.To4(); v4 != nil {
			pub.IP = v4
		}
	} else if env.pubHost != "" {
		if ips, err := net.LookupIP(env.pubHost); err == nil {
			for _, ip := range ips {
				if v4 := ip.To4(); v4 != nil {
					pub.IP = v4
					break
				}
			}
		}
	}
	log.Printf("[socks5-udp] UDP ASSOCIATE from %s（通告 %s:%d）", conn.RemoteAddr(), pub.IP, pub.Port)
	socksReply(conn, socksRepOK, pub)

	a := env.register(conn)
	defer env.unregister(a)
	// 阻塞读控制连接直到关闭，结束本次关联
	_, _ = io.Copy(io.Discard, conn)
}
